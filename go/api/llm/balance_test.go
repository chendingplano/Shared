package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDeepSeekBalanceClientReturnsAllCurrencies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user/balance" || r.Header.Get("Authorization") != "Bearer key" {
			t.Fatal("unexpected balance request")
		}
		_, _ = w.Write([]byte(`{"balance_infos":[{"currency":"CNY","total_balance":"12.34"},{"currency":"USD","total_balance":"5.67"}]}`))
	}))
	defer server.Close()

	result, err := NewDeepSeekBalanceClient(server.Client()).Fetch(context.Background(), server.URL, "key", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Balances) != 2 || result.Balances[0].CurrencyCode != "CNY" || result.Balances[1].CurrencyCode != "USD" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestDeepSeekBalanceClientCapturesEveryOutcome(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		body      string
		wantError bool
	}{
		{"success", 200, `{"balance_infos":[{"currency":"USD","total_balance":"5.67"}]}`, false},
		{"revoked_key", 401, `{"error":{"message":"Unauthorized"}}`, true},
		{"invalid_json", 200, `{`, true},
		{"missing_balances", 200, `{}`, true},
		{"invalid_amount", 200, `{"balance_infos":[{"currency":"USD","total_balance":"invalid"}]}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			sink := &testUsageCaptureSink{}
			client := NewDeepSeekBalanceClient(server.Client())
			client.ModelName = "deepseek-flash-processor"
			_, err := client.Fetch(context.Background(), server.URL, "key", &RequestCapture{UserID: "user-1", AccountID: "account-1", Sink: sink})
			if (err != nil) != tc.wantError {
				t.Fatalf("Fetch error = %v, wantError %v", err, tc.wantError)
			}
			records := sink.Records()
			if len(records) != 1 {
				t.Fatalf("captured %d events, want 1", len(records))
			}
			record := records[0]
			if record.AccountID != "account-1" || record.UserID != "user-1" || record.Provider != ProviderID("deepseek") || record.BaseURL != server.URL {
				t.Fatalf("unexpected attribution: %+v", record)
			}
			if record.TotalTokens != 0 || record.ModelName != "deepseek-flash-processor" || record.CallReason != "provider_balance" || record.PromptName != "provider_balance" || record.CallLoc == "" {
				t.Fatalf("unexpected balance operation: %+v", record)
			}
			if record.RequestStartedAt.IsZero() || record.RequestFinishedAt.Before(record.RequestStartedAt) || string(record.OutputBody) != tc.body {
				t.Fatalf("missing timing or raw response: %+v", record)
			}
			if tc.wantError && (record.ErrorMessage != err.Error() || record.RawResponse != tc.body) {
				t.Fatalf("missing error details: %+v", record)
			}
			if !tc.wantError && record.ErrorMessage != "" {
				t.Fatalf("unexpected capture error: %s", record.ErrorMessage)
			}
		})
	}
}

func TestDeepSeekBalanceClientUsesDefaultCaptureSink(t *testing.T) {
	sink := &testUsageCaptureSink{}
	withDefaultUsageCaptureSink(sink, func() {
		_, err := NewDeepSeekBalanceClient(nil).Fetch(context.Background(), "://invalid", "key", nil)
		if err == nil {
			t.Fatal("expected invalid URL error")
		}
	})
	if records := sink.Records(); len(records) != 1 || !strings.Contains(records[0].ErrorMessage, "missing protocol") {
		t.Fatalf("expected request construction failure in default sink, got %+v", records)
	}
}
