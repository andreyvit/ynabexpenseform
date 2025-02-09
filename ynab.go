package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/andreyvit/mvp/httpcall"
)

func LoadYNABData(ctx context.Context, cfg *AppConfig) (*YNABData, error) {
	budgetID, err := findBudgetID(ctx, cfg)
	if err != nil {
		return nil, err
	}

	accounts, err := loadAccounts(ctx, cfg, budgetID)
	if err != nil {
		return nil, err
	}

	categories, err := loadCategories(ctx, cfg, budgetID)
	if err != nil {
		return nil, err
	}

	transactions, err := loadAllTransactions(ctx, cfg, budgetID, accounts, categories)
	if err != nil {
		return nil, err
	}

	return &YNABData{
		BudgetID:     budgetID,
		Accounts:     accounts,
		Categories:   categories,
		Transactions: transactions,
	}, nil
}

// Create a transaction in YNAB
func CreateYNABTransaction(ctx context.Context, cfg *AppConfig, data *YNABData, tx *YNABTransaction) error {
	// Similarly, you'd call the POST transactions endpoint:
	req := &httpcall.Request{
		Context: ctx,
		CallID:  "CreateTransaction",
		Method:  http.MethodPost,
		Path:    fmt.Sprintf("budgets/%s/transactions", data.BudgetID),
		Input: map[string]interface{}{
			"transaction": map[string]interface{}{
				"date":        tx.Date,
				"amount":      -tx.Amount,
				"account_id":  tx.Account.ID,
				"category_id": tx.Category.ID,
				"memo":        tx.Comment,
				"cleared":     "cleared",
				"approved":    true,
			},
		},
	}
	configureCall(req, cfg)

	return req.Do()
}

var MockData = map[string]func() *YNABData{
	"simple": func() *YNABData {
		c1 := &YNABCategory{ID: "C1", Name: "Groceries"}
		c2 := &YNABCategory{ID: "C2", Name: "Dining Out"}
		a1 := &YNABAccount{ID: "A1", Name: "Cash", Balance: 345600} // $345.60
		a2 := &YNABAccount{ID: "A2", Name: "Bank", Balance: -10000} // -$100.00
		return &YNABData{
			Accounts: []*YNABAccount{
				a1,
				a2,
			},
			Categories: []*YNABCategory{
				c1,
				c2,
			},
			Transactions: []*YNABTransaction{
				{Date: "2025-01-14", Category: c2, Account: a2, Comment: "", Amount: 12_990},
				{Date: "2025-01-15", Category: c1, Account: a1, Comment: "Milk", Amount: 3_450},
			},
		}
	},
}

func findBudgetID(ctx context.Context, cfg *AppConfig) (string, error) {
	var resp struct {
		Data struct {
			Budgets []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"budgets"`
		} `json:"data"`
	}
	req := &httpcall.Request{
		Context:   ctx,
		CallID:    "ListBudgets",
		Method:    http.MethodGet,
		Path:      "budgets",
		OutputPtr: &resp,
	}
	configureCall(req, cfg)
	if err := req.Do(); err != nil {
		return "", err
	}
	for _, b := range resp.Data.Budgets {
		if b.Name == cfg.BudgetName {
			return b.ID, nil
		}
	}
	return "", fmt.Errorf("budget named %q not found in YNAB", cfg.BudgetName)
}

func loadAccounts(ctx context.Context, cfg *AppConfig, budgetID string) ([]*YNABAccount, error) {
	var resp struct {
		Data struct {
			Accounts []*YNABAccount `json:"accounts"`
		} `json:"data"`
	}
	req := &httpcall.Request{
		Context:   ctx,
		CallID:    "ListAccounts",
		Method:    http.MethodGet,
		Path:      fmt.Sprintf("budgets/%s/accounts", budgetID),
		OutputPtr: &resp,
	}
	configureCall(req, cfg)
	if err := req.Do(); err != nil {
		return nil, err
	}

	accountsByName := make(map[string]*YNABAccount)
	for _, a := range resp.Data.Accounts {
		accountsByName[a.Name] = a
	}

	var result []*YNABAccount
	for _, name := range cfg.Accounts {
		if a, ok := accountsByName[name]; ok {
			result = append(result, a)
		} else {
			return nil, fmt.Errorf("account named %q not found", name)
		}
	}
	return result, nil
}

func loadCategories(ctx context.Context, cfg *AppConfig, budgetID string) ([]*YNABCategory, error) {
	var resp struct {
		Data struct {
			CategoryGroups []struct {
				Categories []*YNABCategory `json:"categories"`
			} `json:"category_groups"`
		} `json:"data"`
	}

	req := &httpcall.Request{
		Context:   ctx,
		CallID:    "ListCategories",
		Method:    http.MethodGet,
		Path:      fmt.Sprintf("budgets/%s/categories", budgetID),
		OutputPtr: &resp,
	}
	configureCall(req, cfg)
	if err := req.Do(); err != nil {
		return nil, err
	}

	categoriesByName := make(map[string]*YNABCategory)
	for _, cg := range resp.Data.CategoryGroups {
		for _, c := range cg.Categories {
			categoriesByName[c.Name] = c
		}
	}

	var result []*YNABCategory
	for _, name := range cfg.Categories {
		if c, ok := categoriesByName[name]; ok {
			result = append(result, c)
		} else {
			return nil, fmt.Errorf("category named %q not found", name)
		}
	}
	return result, nil
}

func loadAllTransactions(
	ctx context.Context,
	cfg *AppConfig,
	budgetID string,
	accounts []*YNABAccount,
	categories []*YNABCategory,
) ([]*YNABTransaction, error) {
	var resp struct {
		Data struct {
			Transactions []struct {
				ID         string `json:"id"`
				AccountID  string `json:"account_id"`
				CategoryID string `json:"category_id"`
				Date       string `json:"date"`
				Memo       string `json:"memo"`
				Amount     Amount `json:"amount"` // milliunits in YNAB
			} `json:"transactions"`
		} `json:"data"`
	}
	req := &httpcall.Request{
		Context:   ctx,
		CallID:    "ListTransactions",
		Method:    http.MethodGet,
		Path:      fmt.Sprintf("budgets/%s/transactions", budgetID),
		OutputPtr: &resp,
	}
	configureCall(req, cfg)
	if err := req.Do(); err != nil {
		return nil, err
	}

	accoountsByID := make(map[string]*YNABAccount)
	for _, a := range accounts {
		accoountsByID[a.ID] = a
	}

	categoriesByID := make(map[string]*YNABCategory)
	for _, c := range categories {
		categoriesByID[c.ID] = c
	}

	result := make([]*YNABTransaction, 0, len(resp.Data.Transactions))

	log.Printf("loaded %d transactions", len(resp.Data.Transactions))

	for _, t := range resp.Data.Transactions {
		account := accoountsByID[t.AccountID]
		if account == nil {
			log.Printf("account %q not found", t.AccountID)
			continue
		}
		category := categoriesByID[t.CategoryID]
		if category == nil {
			log.Printf("category %q not found", t.CategoryID)
			continue
		}

		tx := &YNABTransaction{
			ID:        t.ID,
			Date:      t.Date,
			Category:  category,
			Account:   account,
			Comment:   t.Memo,
			Amount:    t.Amount,
			AmountUSD: t.Amount,
		}
		result = append(result, tx)
	}
	return result, nil
}

func configureCall(req *httpcall.Request, cfg *AppConfig) {
	req.BaseURL = "https://api.youneedabudget.com/v1/"
	req.Headers = map[string][]string{
		"Authorization": {"Bearer " + cfg.YNABToken},
	}
	req.OnStarted(func(r *httpcall.Request) {
		log.Printf("> %s: %s\n", r.CallID, r.Curl())
	})
}
