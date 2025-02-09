package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIndex_simple(t *testing.T) {
	appCfg = AppConfig{
		PageTitle: "Test Expenses",
		Currencies: []CurrencyConfig{
			{Code: "USD", Rate: 1.0, Format: "$%0.2f"},
			{Code: "GEL", Rate: 2.6, Format: "₾%0.2f"},
		},
		BudgetCurrency:  "USD",
		DefaultCurrency: "GEL",
	}
	app, err := New(&appCfg)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/?mock=simple", nil)
	w := httptest.NewRecorder()

	err = app.handleIndex(w, req)
	if err != nil {
		t.Fatal(err)
	}
	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200, got %d", resp.StatusCode)
	}
	bodyBytes := w.Body.String()
	if !strings.Contains(bodyBytes, "Test Expenses") {
		t.Errorf("Expected page title in output")
	}
	if !strings.Contains(bodyBytes, "Cash") {
		t.Errorf("Expected mock account name 'Cash' in output")
	}
	if !strings.Contains(bodyBytes, "Groceries") {
		t.Errorf("Expected mock category 'Groceries' in output")
	}
	if !strings.Contains(bodyBytes, "Milk") {
		t.Errorf("Expected mock transaction 'Milk' in output")
	}
}
