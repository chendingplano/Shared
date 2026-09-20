package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
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
