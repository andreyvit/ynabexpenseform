package main

import (
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/andreyvit/jsonfix"
)

//go:embed config.json
var configJSON []byte

type AppConfig struct {
	YNABToken         string           `json:"ynabToken"`
	BudgetName        string           `json:"budget"`
	PageTitle         string           `json:"page_title"`
	Categories        []string         `json:"categories"`
	Accounts          []string         `json:"accounts"`
	HideBalance       []string         `json:"hide_balance"`
	Currencies        []CurrencyConfig `json:"currencies"`
	BudgetCurrency    string           `json:"budget_currency"`
	DefaultCurrency   string           `json:"default_currency"`
	SecondaryCurrency string           `json:"secondary_currency"`
}

type CurrencyConfig struct {
	Code   string  `json:"code"`
	Rate   float64 `json:"rate"`
	Format string  `json:"format"`
}

var appCfg AppConfig

func main() {
	var addr = flag.String("listen", ":3000", "HTTP listen address")
	flag.Parse()

	err := json.Unmarshal(jsonfix.Bytes(configJSON), &appCfg)
	if err != nil {
		log.Fatalf("Failed to parse config.json: %v", err)
	}

	app, err := New(&appCfg)
	if err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("GET /{$}", wrap(app.handleIndex))
	http.HandleFunc("POST /enter", wrap(app.handleEnterExpense))
	http.HandleFunc("POST /refresh", wrap(app.handleRefresh))

	fmt.Printf("Listening on %s...\n", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}
