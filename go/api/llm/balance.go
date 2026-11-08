package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/loggerutil"
)

type Balance struct {
	Amount       float64
	CurrencyCode string
}
type BalanceResult struct {
	Balances   []Balance
	RawPayload []byte
}
type DeepSeekBalanceClient struct {
	HTTPClient *http.Client
	// ModelName attributes balance usage events to a project-selected model.
	// It does not change the provider's account-level balance request.
	ModelName string
}

func NewDeepSeekBalanceClient(client *http.Client) *DeepSeekBalanceClient {
	return &DeepSeekBalanceClient{HTTPClient: client}
}

// Fetch records each balance read through the same usage sink as model calls,
// including failed reads. Balance reads consume no model tokens.
func (c *DeepSeekBalanceClient) Fetch(ctx context.Context, baseURL, apiKey string, capture *RequestCapture) (result BalanceResult, err error) {
	started := time.Now()
	var raw []byte
	defer func() {
		in := UsageCaptureInput{
			Provider: ProviderID("deepseek"), BaseURL: baseURL, APIKey: apiKey,
			ModelName:  strings.TrimSpace(c.ModelName),
			PromptName: "provider_balance", CallReason: "provider_balance", CallLoc: "LLM_BALANCE_001",
			RequestStartedAt: started, RequestFinishedAt: time.Now(),
			OutputBody: raw, ErrorMessage: errorString(err),
		}
		if capture != nil {
			in.AccountID, in.ProfileID = capture.AccountID, capture.ProfileID
			in.RunID = capture.RunID
			in.InputBodyRef, in.OutputBodyRef = capture.InputBodyRef, capture.OutputBodyRef
		}
		if err != nil {
			in.RawResponse = string(raw)
		}
		captureUsageRecord(ctx, Request{Capture: capture}, in, loggerutil.CreateDefaultLogger("20261027-367"))
	}()
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.deepseek.com"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/user/balance", nil)
	if err != nil {
		return BalanceResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return BalanceResult{}, err
	}
	defer resp.Body.Close()
	raw, err = io.ReadAll(resp.Body)
	if err != nil {
		return BalanceResult{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return BalanceResult{}, fmt.Errorf("deepseek balance request failed: status %d", resp.StatusCode)
	}
	var decoded struct {
		BalanceInfos []struct {
			Currency     string `json:"currency"`
			TotalBalance string `json:"total_balance"`
		} `json:"balance_infos"`
	}
	if err = json.Unmarshal(raw, &decoded); err != nil {
		return BalanceResult{}, err
	}
	if len(decoded.BalanceInfos) == 0 {
		return BalanceResult{}, errors.New("deepseek balance response missing balance_infos")
	}
	result = BalanceResult{RawPayload: raw}
	for _, info := range decoded.BalanceInfos {
		amount, err := strconv.ParseFloat(strings.TrimSpace(info.TotalBalance), 64)
		if err != nil {
			return BalanceResult{}, err
		}
		result.Balances = append(result.Balances, Balance{Amount: amount, CurrencyCode: info.Currency})
	}
	return result, nil
}
