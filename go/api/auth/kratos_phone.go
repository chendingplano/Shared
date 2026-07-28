////////////////////////////////////////////////////////////
//
// Description:
// Phone/SMS login for Kratos (Phase 1: Chinese mobile numbers).
// See ChenWeb/openspec/changes/add-phone-login-china/design.md for the
// architecture this implements (native Kratos "code" method + custom SMS
// courier channel, not a bespoke side-channel).
//
////////////////////////////////////////////////////////////

package auth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/chendingplano/shared/go/api/ApiUtils"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/chendingplano/shared/go/api/sysdatastores"
	"github.com/labstack/echo/v4"
	ory "github.com/ory/client-go"
)

// kratosMessageIDAccountNotFound is Kratos's stable message ID for
// schema.NewNoCodeAuthnCredentials() (text/id.go: ErrorValidationNoCodeUser =
// 4000035, text "This account does not exist or has not setup sign in with
// code.") - this is what the code method's login strategy actually returns
// for an unrecognized identifier (confirmed live against a running Kratos
// instance 2026-07-28; schema.NewAccountNotFoundError()/4000037 is a
// different, unrelated error path not reached by this flow). Kratos
// guarantees message IDs are stable across versions, so this is the correct
// way to distinguish "no account, fall back to registration" from "code
// sent, awaiting verification" - both cases return a non-2xx response from
// the Go SDK, since a login/registration flow isn't "complete" (in Kratos's
// sense) until the final code-verification step.
const kratosMessageIDAccountNotFound = 4000035

// toE164CN converts an 11-digit CN mobile number (already validated against
// cnMobileNumberPattern by the caller) to E.164 format. Kratos's own
// identifier normalization (Kratos/src/kratos/x/normalize.go) calls
// phonenumbers.Parse(value, "") with no default region, so it rejects bare
// national-format numbers with "invalid country code" - Kratos must always
// receive E.164, even though users type (and the frontend displays) the
// bare 11-digit form.
func toE164CN(bareNumber string) string {
	return "+86" + bareNumber
}

type phoneSendCodeRequest struct {
	Phone string `json:"phone"`
}

type phoneVerifyRequest struct {
	Phone    string `json:"phone"`
	Code     string `json:"code"`
	FlowID   string `json:"flow_id"`
	FlowType string `json:"flow_type"` // "login" or "registration"
}

// HandlePhoneSendCodeKratos handles POST /auth/phone/send-code: validates a
// CN mobile number, then starts a Kratos login flow (existing identity) or,
// if none exists, a registration flow (new identity) - either way, the
// code-method submission causes Kratos's courier to dispatch an SMS via the
// "sms" channel (see Kratos/kratos/kratos.yml), relayed through
// HandleSMSCourierRelay to Aliyun.
func HandlePhoneSendCodeKratos(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "SHD_PHN_072802")
	defer rc.Close()
	logger := rc.GetLogger()

	clientIP := c.RealIP()
	allowed, _, retryAfter := CheckLoginRateLimit(clientIP)
	if !allowed {
		logger.Warn("rate limit exceeded for phone send-code", "ip", clientIP, "retry_after", retryAfter.String())
		return c.JSON(http.StatusTooManyRequests, KratosErrorResponse{
			Status:  "error",
			Message: "Too many attempts. Please try again later.",
			LOC:     "SHD_PHN_072803",
		})
	}

	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return c.JSON(http.StatusBadRequest, KratosErrorResponse{Status: "error", Message: "Failed to read request body", LOC: "SHD_PHN_072804"})
	}
	var req phoneSendCodeRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return c.JSON(http.StatusBadRequest, KratosErrorResponse{Status: "error", Message: "invalid request body", LOC: "SHD_PHN_072805"})
	}
	if !cnMobileNumberPattern.MatchString(req.Phone) {
		return c.JSON(http.StatusBadRequest, KratosErrorResponse{Status: "error", Message: "invalid CN mobile number", LOC: "SHD_PHN_072806"})
	}
	e164Phone := toE164CN(req.Phone)

	if kratosClient == nil {
		InitKratosClient()
	}
	ctx := context.Background()

	// Step 1: try login (existing identity) first.
	loginFlow, _, err := kratosClient.client.FrontendAPI.CreateNativeLoginFlow(ctx).Execute()
	if err != nil {
		logger.Error("failed to create native login flow", "error", err)
		return c.JSON(http.StatusInternalServerError, KratosErrorResponse{Status: "error", Message: "Failed to initialize login", LOC: "SHD_PHN_072807"})
	}

	_, resp, err := kratosClient.client.FrontendAPI.UpdateLoginFlow(ctx).
		Flow(loginFlow.Id).
		UpdateLoginFlowBody(ory.UpdateLoginFlowWithCodeMethodAsUpdateLoginFlowBody(&ory.UpdateLoginFlowWithCodeMethod{
			Method:     "code",
			Identifier: &e164Phone,
		})).
		Execute()

	if err == nil {
		// Extremely unlikely on the "send code" step (would mean the flow
		// was already considered complete), but handle it defensively.
		logger.Warn("login flow completed on send-code submission (unexpected)", "phone", req.Phone)
		return c.JSON(http.StatusOK, map[string]any{"status": "ok", "flow_id": loginFlow.Id, "flow_type": "login"})
	}

	respBody := readAndCloseBody(resp)
	if !kratosBodyHasMessageID(respBody, kratosMessageIDAccountNotFound) {
		// Any other non-2xx here means Kratos accepted the identifier and
		// is asking for the code next - i.e. the SMS was queued for send.
		logger.Info("phone login code requested", "phone", req.Phone, "flow_id", loginFlow.Id)
		return c.JSON(http.StatusOK, map[string]any{"status": "ok", "flow_id": loginFlow.Id, "flow_type": "login"})
	}

	// Step 2: no existing identity - fall back to registration.
	logger.Info("no identity for phone, falling back to registration", "phone", req.Phone)
	regFlow, _, err := kratosClient.client.FrontendAPI.CreateNativeRegistrationFlow(ctx).Execute()
	if err != nil {
		logger.Error("failed to create native registration flow", "error", err)
		return c.JSON(http.StatusInternalServerError, KratosErrorResponse{Status: "error", Message: "Failed to initialize registration", LOC: "SHD_PHN_072808"})
	}

	_, regResp, err := kratosClient.client.FrontendAPI.UpdateRegistrationFlow(ctx).
		Flow(regFlow.Id).
		UpdateRegistrationFlowBody(ory.UpdateRegistrationFlowWithCodeMethodAsUpdateRegistrationFlowBody(&ory.UpdateRegistrationFlowWithCodeMethod{
			Method: "code",
			Traits: map[string]any{"phone": e164Phone},
		})).
		Execute()

	if err == nil {
		logger.Warn("registration flow completed on send-code submission (unexpected)", "phone", req.Phone)
		return c.JSON(http.StatusOK, map[string]any{"status": "ok", "flow_id": regFlow.Id, "flow_type": "registration"})
	}

	regRespBody := readAndCloseBody(regResp)
	logger.Info("phone registration code requested", "phone", req.Phone, "flow_id", regFlow.Id, "resp_snippet", truncateForLog(regRespBody))
	return c.JSON(http.StatusOK, map[string]any{"status": "ok", "flow_id": regFlow.Id, "flow_type": "registration"})
}

// HandlePhoneVerifyCodeKratos handles POST /auth/phone/verify: submits the
// user-entered code to the flow started by HandlePhoneSendCodeKratos, and on
// success sets the session_token cookie exactly like email login does.
func HandlePhoneVerifyCodeKratos(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "SHD_PHN_072809")
	defer rc.Close()
	logger := rc.GetLogger()

	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return c.JSON(http.StatusBadRequest, KratosErrorResponse{Status: "error", Message: "Failed to read request body", LOC: "SHD_PHN_072810"})
	}
	var req phoneVerifyRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return c.JSON(http.StatusBadRequest, KratosErrorResponse{Status: "error", Message: "invalid request body", LOC: "SHD_PHN_072811"})
	}
	if !cnMobileNumberPattern.MatchString(req.Phone) || req.Code == "" || req.FlowID == "" {
		return c.JSON(http.StatusBadRequest, KratosErrorResponse{Status: "error", Message: "missing or invalid phone/code/flow_id", LOC: "SHD_PHN_072812"})
	}
	e164Phone := toE164CN(req.Phone)

	if kratosClient == nil {
		InitKratosClient()
	}
	ctx := context.Background()

	var session *ory.Session
	var sessionToken *string
	var identity *ory.Identity

	switch req.FlowType {
	case "registration":
		result, resp, err := kratosClient.client.FrontendAPI.UpdateRegistrationFlow(ctx).
			Flow(req.FlowID).
			UpdateRegistrationFlowBody(ory.UpdateRegistrationFlowWithCodeMethodAsUpdateRegistrationFlowBody(&ory.UpdateRegistrationFlowWithCodeMethod{
				Method: "code",
				Code:   &req.Code,
				Traits: map[string]any{"phone": e164Phone},
			})).
			Execute()
		if err != nil {
			errorMessage := "Invalid or expired code"
			if respBody := readAndCloseBody(resp); len(respBody) > 0 {
				errorMessage = parseKratosUIError(respBody, errorMessage)
			}
			logger.Warn("phone registration verify failed", "phone", req.Phone, "error", err)
			return c.JSON(http.StatusUnauthorized, KratosErrorResponse{Status: "error", Message: errorMessage, LOC: "SHD_PHN_072813"})
		}
		session = result.Session
		sessionToken = result.SessionToken
		id := result.Identity
		identity = &id
	case "login":
		result, resp, err := kratosClient.client.FrontendAPI.UpdateLoginFlow(ctx).
			Flow(req.FlowID).
			UpdateLoginFlowBody(ory.UpdateLoginFlowWithCodeMethodAsUpdateLoginFlowBody(&ory.UpdateLoginFlowWithCodeMethod{
				Method:     "code",
				Code:       &req.Code,
				Identifier: &e164Phone,
			})).
			Execute()
		if err != nil {
			errorMessage := "Invalid or expired code"
			if respBody := readAndCloseBody(resp); len(respBody) > 0 {
				errorMessage = parseKratosUIError(respBody, errorMessage)
			}
			logger.Warn("phone login verify failed", "phone", req.Phone, "error", err)
			return c.JSON(http.StatusUnauthorized, KratosErrorResponse{Status: "error", Message: errorMessage, LOC: "SHD_PHN_072814"})
		}
		session = &result.Session
		sessionToken = result.SessionToken
		identity = session.Identity
	default:
		return c.JSON(http.StatusBadRequest, KratosErrorResponse{Status: "error", Message: "flow_type must be 'login' or 'registration'", LOC: "SHD_PHN_072815"})
	}

	if session == nil || sessionToken == nil {
		logger.Error("phone verify succeeded but no session/token returned", "phone", req.Phone, "flow_type", req.FlowType)
		return c.JSON(http.StatusInternalServerError, KratosErrorResponse{Status: "error", Message: "Verification succeeded but session creation failed", LOC: "SHD_PHN_072816"})
	}

	setSessionTokenCookie(c, *sessionToken)

	identityID := ""
	if identity != nil {
		identityID = identity.Id
	}
	customLayout := "2006-01-02 15:04:05"
	expiredTimeStr := time.Now().Add(cookie_timeout_hours * time.Hour).Format(customLayout)
	sysdatastores.AddSessionLog(sysdatastores.SessionLogDef{
		LoginMethod:  "kratos_phone_" + req.FlowType,
		SessionID:    session.Id,
		AuthToken:    ApiUtils.MaskToken(*sessionToken),
		Status:       "active",
		UserName:     req.Phone,
		UserNameType: "phone",
		UserRegID:    identityID,
		CallerLoc:    "SHD_PHN_072817",
		ExpiresAt:    &expiredTimeStr,
	})

	logger.Info("phone login success", "phone", req.Phone, "flow_type", req.FlowType, "identity_id", identityID)

	redirectURL := GetRedirectURL(rc, req.Phone, false, false)
	return c.JSON(http.StatusOK, map[string]any{
		"status":       "ok",
		"redirect_url": redirectURL,
	})
}

// readAndCloseBody reads and closes an *http.Response body, returning nil
// bytes if resp is nil (which the ory SDK returns for some transport-level
// errors, not just non-2xx HTTP responses).
func readAndCloseBody(resp *http.Response) []byte {
	if resp == nil || resp.Body == nil {
		return nil
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return b
}

// kratosBodyHasMessageID walks a Kratos flow JSON response looking for the
// given message ID anywhere in its (nested) "messages" arrays - Kratos
// attaches ErrorValidationAccountNotFound to the identifier node, not the
// top-level flow, so a full walk is needed rather than checking one fixed path.
func kratosBodyHasMessageID(body []byte, id int) bool {
	var parsed any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return false
	}
	return walkForMessageID(parsed, float64(id))
}

func walkForMessageID(node any, id float64) bool {
	switch v := node.(type) {
	case map[string]any:
		if msgID, ok := v["id"].(float64); ok && msgID == id {
			return true
		}
		for _, child := range v {
			if walkForMessageID(child, id) {
				return true
			}
		}
	case []any:
		for _, child := range v {
			if walkForMessageID(child, id) {
				return true
			}
		}
	}
	return false
}

func truncateForLog(b []byte) string {
	const max = 500
	if len(b) > max {
		return string(b[:max]) + "...(truncated)"
	}
	return string(b)
}
