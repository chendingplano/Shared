// Package auth: SMS courier relay for Kratos's phone/code login method.
//
// Kratos's courier can call an arbitrary HTTP "generic" channel to deliver an
// SMS (see courier.channels in kratos.yml), but the request body it builds is
// a static Jsonnet template - it cannot compute Aliyun Dysmsapi's HMAC-SHA1
// request signature. This relay receives the {to, code} payload from that
// Jsonnet template and performs the actual signed Aliyun SendSms call, so
// Aliyun credentials never need to live inside Kratos's own config/templates.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/labstack/echo/v4"
)

// cnMobileNumberPattern matches the bare 11-digit CN mobile number as users
// type it (used to validate incoming user input in kratos_phone.go).
var cnMobileNumberPattern = regexp.MustCompile(`^1[3-9]\d{9}$`)

// cnMobileE164Pattern matches the E.164-formatted number Kratos actually
// sends as the courier recipient. Kratos's identifier normalization
// (Kratos/src/kratos/x/normalize.go, nyaruka/phonenumbers with no default
// region) requires full E.164 input, so kratos_phone.go prepends "+86"
// before ever handing a phone number to Kratos - by the time it reaches this
// relay via the courier, it's always in this form, not the bare form above.
var cnMobileE164Pattern = regexp.MustCompile(`^\+861[3-9]\d{9}$`)

// smsCourierRelayRequest is the payload our own Jsonnet request-config
// template (Kratos/kratos/templates/courier/sms/request.config.jsonnet)
// produces from Kratos's courier ctx.
type smsCourierRelayRequest struct {
	To   string `json:"to"`
	Code string `json:"code"`
}

// HandleSMSCourierRelay is called by Kratos's "sms" courier channel, not by
// an end user - there is no session/cookie auth available. Access is
// restricted with a shared-secret header instead.
func HandleSMSCourierRelay(c echo.Context) error {
	logger := loggerutil.CreateDefaultLogger("SHD_SMS_072801")

	sharedSecret := strings.TrimSpace(os.Getenv("SMS_RELAY_SHARED_SECRET"))
	if sharedSecret == "" {
		logger.Error("SMS_RELAY_SHARED_SECRET is not configured; refusing all relay requests")
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "sms relay is not configured",
		})
	}
	presented := c.Request().Header.Get("X-Internal-Relay-Secret")
	if subtle.ConstantTimeCompare([]byte(presented), []byte(sharedSecret)) != 1 {
		logger.Warn("rejected sms relay request with invalid shared secret")
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}

	var req smsCourierRelayRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		logger.Error("failed to decode sms courier relay payload", "error", err)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid payload"})
	}

	phone := strings.TrimSpace(req.To)
	if !cnMobileE164Pattern.MatchString(phone) {
		logger.Warn("rejected sms relay request for non-CN-mobile-shaped recipient", "to", phone)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "recipient is not a valid CN mobile number"})
	}
	// Aliyun's domestic SendSms action expects the bare national number
	// (e.g. "13800003333"), not the E.164 form Kratos hands us.
	bareNumber := strings.TrimPrefix(phone, "+86")
	if strings.TrimSpace(req.Code) == "" {
		logger.Error("sms relay payload missing code", "to", phone)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "missing code"})
	}

	allowed, _, retryAfter := CheckSMSSendRateLimit(phone)
	if !allowed {
		logger.Warn("sms send rate limit exceeded", "to", phone, "retry_after", retryAfter.String())
		return c.JSON(http.StatusTooManyRequests, map[string]any{
			"error":       "too many codes requested for this phone number, try again later",
			"retry_after": retryAfter.Seconds(),
		})
	}

	if err := sendAliyunSMS(logger, bareNumber, req.Code); err != nil {
		logger.Error("aliyun sms send failed", "to", phone, "error", err)
		return c.JSON(http.StatusBadGateway, map[string]string{"error": "failed to send sms"})
	}

	logger.Info("sms code dispatched", "to", phone)
	return c.NoContent(http.StatusOK)
}

// aliyunSMSConfig is read fresh on every send (not cached at process start)
// so credential rotation via env var update doesn't require special handling
// beyond a normal process restart, matching how the rest of this package
// reads its OAuth/Kratos config from os.Getenv per-request.
type aliyunSMSConfig struct {
	AccessKeyID     string
	AccessKeySecret string
	SignName        string
	TemplateCode    string
}

func loadAliyunSMSConfig() (aliyunSMSConfig, error) {
	cfg := aliyunSMSConfig{
		AccessKeyID:     strings.TrimSpace(os.Getenv("ALIYUN_SMS_ACCESS_KEY_ID")),
		AccessKeySecret: strings.TrimSpace(os.Getenv("ALIYUN_SMS_ACCESS_KEY_SECRET")),
		SignName:        strings.TrimSpace(os.Getenv("ALIYUN_SMS_SIGN_NAME")),
		TemplateCode:    strings.TrimSpace(os.Getenv("ALIYUN_SMS_TEMPLATE_CODE")),
	}
	if cfg.AccessKeyID == "" || cfg.AccessKeySecret == "" || cfg.SignName == "" || cfg.TemplateCode == "" {
		return cfg, fmt.Errorf("missing one or more of ALIYUN_SMS_ACCESS_KEY_ID, ALIYUN_SMS_ACCESS_KEY_SECRET, ALIYUN_SMS_SIGN_NAME, ALIYUN_SMS_TEMPLATE_CODE")
	}
	return cfg, nil
}

// sendAliyunSMS calls Aliyun Dysmsapi's SendSms RPC action directly (no SDK
// dependency), mirroring the bzton reference's AliyunSmsUtils.java mechanics
// in Go, with credentials sourced only from environment variables.
func sendAliyunSMS(logger ApiTypes.JimoLogger, phone string, code string) error {
	cfg, err := loadAliyunSMSConfig()
	if err != nil {
		logger.Error("aliyun sms config missing", "error", err)
		return err
	}

	templateParam, err := json.Marshal(map[string]string{"code": code})
	if err != nil {
		return fmt.Errorf("failed to marshal template param: %w", err)
	}

	params := map[string]string{
		"AccessKeyId":      cfg.AccessKeyID,
		"Action":           "SendSms",
		"Version":          "2017-05-25",
		"Format":           "JSON",
		"SignatureMethod":  "HMAC-SHA1",
		"SignatureVersion": "1.0",
		"SignatureNonce":   aliyunNonce(),
		"Timestamp":        time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"RegionId":         "cn-hangzhou",
		"PhoneNumbers":     phone,
		"SignName":         cfg.SignName,
		"TemplateCode":     cfg.TemplateCode,
		"TemplateParam":    string(templateParam),
	}
	params["Signature"] = aliyunSign("POST", params, cfg.AccessKeySecret)

	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}

	req, err := http.NewRequest(http.MethodPost, "https://dysmsapi.aliyuncs.com/", strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("failed to build aliyun request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("aliyun request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return fmt.Errorf("failed to read aliyun response: %w", err)
	}

	var result struct {
		Code    string `json:"Code"`
		Message string `json:"Message"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("failed to parse aliyun response: %w (body: %s)", err, string(body))
	}
	if result.Code != "OK" {
		return fmt.Errorf("aliyun rejected sms send: %s - %s", result.Code, result.Message)
	}
	return nil
}

func aliyunNonce() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return base64.RawURLEncoding.EncodeToString(buf)
}

// aliyunSign implements Aliyun's RPC request-signing algorithm: sort params,
// build a canonicalized query string, prepend "<method>&%2F&", HMAC-SHA1
// with key "<AccessKeySecret>&", base64-encode.
func aliyunSign(method string, params map[string]string, accessKeySecret string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var canonical strings.Builder
	for i, k := range keys {
		if i > 0 {
			canonical.WriteByte('&')
		}
		canonical.WriteString(aliyunPercentEncode(k))
		canonical.WriteByte('=')
		canonical.WriteString(aliyunPercentEncode(params[k]))
	}

	stringToSign := method + "&" + aliyunPercentEncode("/") + "&" + aliyunPercentEncode(canonical.String())

	mac := hmac.New(sha1.New, []byte(accessKeySecret+"&"))
	mac.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// aliyunPercentEncode matches Aliyun's required RFC3986 encoding, which
// differs from Go's url.QueryEscape (which encodes space as "+", not "%20",
// and does not encode "*" or leave "~" unescaped).
func aliyunPercentEncode(s string) string {
	encoded := url.QueryEscape(s)
	encoded = strings.ReplaceAll(encoded, "+", "%20")
	encoded = strings.ReplaceAll(encoded, "*", "%2A")
	encoded = strings.ReplaceAll(encoded, "%7E", "~")
	return encoded
}
