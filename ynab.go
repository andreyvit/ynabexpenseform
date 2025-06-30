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

	// Generate transfer pseudo-categories
	transferCategories := GenerateTransferCategories(accounts)

	// Combine real categories and transfer categories
	allCategories := make([]*YNABCategory, 0, len(categories)+len(transferCategories))
	allCategories = append(allCategories, categories...)
	allCategories = append(allCategories, transferCategories...)

	transactions, err := loadAllTransactions(ctx, cfg, budgetID, accounts, categories)
	if err != nil {
		return nil, err
	}

	return &YNABData{
		BudgetID:      budgetID,
		Accounts:      accounts,
		Categories:    categories,
		Transactions:  transactions,
		AllCategories: allCategories,
	}, nil
}

// Create a transaction in YNAB
func CreateYNABTransaction(ctx context.Context, cfg *AppConfig, data *YNABData, tx *YNABTransaction) error {
	// Create the transaction input map
	txMap := map[string]interface{}{
		"date":       tx.Date,
		"amount":     -tx.Amount, // Negative for outflow
		"account_id": tx.Account.ID,
		"memo":       tx.Comment,
		"cleared":    "cleared",
		"approved":   true,
	}

	// Handle transfer vs regular transaction
	if tx.Category != nil && tx.Category.IsTransferCategory() {
		// Get the target account ID from the transfer category
		targetAccountID := tx.Category.TransferTargetID()

		// Find the target account to get its transfer payee ID
		var targetAccount *YNABAccount
		for _, acc := range data.Accounts {
			if acc.ID == targetAccountID {
				targetAccount = acc
				break
			}
		}

		if targetAccount == nil {
			return fmt.Errorf("target account %s not found", targetAccountID)
		}

		// For transfers, use the target account's transfer_payee_id
		// and don't set a category
		txMap["payee_id"] = targetAccount.TransferPayeeID
		txMap["category_id"] = nil

		// Save transfer account reference for UI display
		tx.TransferAccount = targetAccount
		tx.IsTransfer = true
	} else {
		// Regular expense transaction
		txMap["category_id"] = tx.Category.ID
	}

	// Create the API request
	req := &httpcall.Request{
		Context: ctx,
		CallID:  "CreateTransaction",
		Method:  http.MethodPost,
		Path:    fmt.Sprintf("budgets/%s/transactions", data.BudgetID),
		Input: map[string]interface{}{
			"transaction": txMap,
		},
	}
	configureCall(req, cfg)

	return req.Do()
}

var MockData = map[string]func() *YNABData{
	"simple": func() *YNABData {
		// Regular categories
		c1 := &YNABCategory{ID: "C1", Name: "Groceries"}
		c2 := &YNABCategory{ID: "C2", Name: "Dining Out"}

		// Accounts - keep "Cash" for test compatibility
		a1 := &YNABAccount{ID: "A1", Name: "Cash", Balance: 345600, TransferPayeeID: "TP-A1"}              // $345.60
		a2 := &YNABAccount{ID: "A2", Name: "Held By Assistant", Balance: 125000, TransferPayeeID: "TP-A2"} // $125.00
		a3 := &YNABAccount{ID: "A3", Name: "Alisa Business", Balance: 750000, TransferPayeeID: "TP-A3"}    // $750.00

		accounts := []*YNABAccount{a1, a2, a3}
		categories := []*YNABCategory{c1, c2}
		transferCategories := GenerateTransferCategories(accounts)
		allCategories := append(categories, transferCategories...)

		// Get references to transfer categories
		var transferToA1, transferToA2, transferToA3 *YNABCategory
		for _, tc := range transferCategories {
			if tc.TransferToID == a1.ID {
				transferToA1 = tc
			} else if tc.TransferToID == a2.ID {
				transferToA2 = tc
			} else if tc.TransferToID == a3.ID {
				transferToA3 = tc
			}
		}

		return &YNABData{
			Accounts:      accounts,
			Categories:    categories,
			AllCategories: allCategories,
			Transactions: []*YNABTransaction{
				// Regular transactions - keep "Milk" for test compatibility
				{Date: "2025-01-14", Category: c2, Account: a1, Comment: "Lunch meeting", Amount: 12_990},
				{Date: "2025-01-15", Category: c1, Account: a1, Comment: "Milk", Amount: 3_450},

				// Transfer transactions - using negative amounts to represent outflows
				{Date: "2025-01-16", Category: transferToA2, Account: a1, Comment: "Moving funds", Amount: -50_000, IsTransfer: true, TransferAccount: a2},
				{Date: "2025-01-17", Category: transferToA3, Account: a2, Comment: "", Amount: -75_000, IsTransfer: true, TransferAccount: a3},
				{Date: "2025-01-18", Category: transferToA1, Account: a3, Comment: "Reimbursement", Amount: -35_000, IsTransfer: true, TransferAccount: a1},
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

	// Load payees to get transfer_payee_ids
	payeesMap, err := loadPayeesForAccounts(ctx, cfg, budgetID)
	if err != nil {
		return nil, err
	}

	// Associate transfer payees with accounts
	for _, a := range resp.Data.Accounts {
		if payeeID, ok := payeesMap[a.ID]; ok {
			a.TransferPayeeID = payeeID
		}
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

// loadPayeesForAccounts loads all payees and returns a map of account ID to transfer payee ID
func loadPayeesForAccounts(ctx context.Context, cfg *AppConfig, budgetID string) (map[string]string, error) {
	var resp struct {
		Data struct {
			Payees []struct {
				ID                string `json:"id"`
				Name              string `json:"name"`
				TransferAccountID string `json:"transfer_account_id"`
			} `json:"payees"`
		} `json:"data"`
	}

	req := &httpcall.Request{
		Context:   ctx,
		CallID:    "ListPayees",
		Method:    http.MethodGet,
		Path:      fmt.Sprintf("budgets/%s/payees", budgetID),
		OutputPtr: &resp,
	}
	configureCall(req, cfg)
	if err := req.Do(); err != nil {
		return nil, err
	}

	// Map of account ID to transfer payee ID
	accountToPayeeID := make(map[string]string)

	// Process each payee
	for _, p := range resp.Data.Payees {
		if p.TransferAccountID != "" {
			// This is a transfer payee
			accountToPayeeID[p.TransferAccountID] = p.ID
		}
	}

	return accountToPayeeID, nil
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
				ID                    string `json:"id"`
				AccountID             string `json:"account_id"`
				CategoryID            string `json:"category_id"`
				Date                  string `json:"date"`
				Memo                  string `json:"memo"`
				Amount                Amount `json:"amount"` // milliunits in YNAB
				TransferAccountID     string `json:"transfer_account_id"`
				PayeeID               string `json:"payee_id"`
				PayeeName             string `json:"payee_name"`
				TransferTransactionID string `json:"transfer_transaction_id"`
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

	accountsByID := make(map[string]*YNABAccount)
	for _, a := range accounts {
		accountsByID[a.ID] = a
	}

	categoriesByID := make(map[string]*YNABCategory)
	for _, c := range categories {
		categoriesByID[c.ID] = c
	}

	result := make([]*YNABTransaction, 0, len(resp.Data.Transactions))
	log.Printf("loaded %d transactions", len(resp.Data.Transactions))

	// We'll filter transfers to only show negative amounts (outflows)

	for _, t := range resp.Data.Transactions {
		account := accountsByID[t.AccountID]
		if account == nil {
			log.Printf("account %q not found", t.AccountID)
			continue
		}

		// Check if this is a transfer by looking at transfer_account_id
		isTransfer := t.TransferAccountID != ""
		var transferAccount *YNABAccount
		var category *YNABCategory

		if isTransfer {
			// For transfers, find the target account
			transferAccount = accountsByID[t.TransferAccountID]

			// Only include transfers between accounts we're tracking
			if transferAccount == nil {
				log.Printf("transfer to account %q not in our list, skipping", t.TransferAccountID)
				continue
			}

			// For transfers, only show the outflow (negative amount)
			// This eliminates duplicate display of transfers
			if t.Amount > 0 {
				log.Printf("skipping positive transfer amount %d", t.Amount)
				continue
			}

			// Create a pseudo-category for the transfer
			category = &YNABCategory{
				ID:           "transfer-to-" + t.TransferAccountID,
				Name:         "Transfer to " + transferAccount.Name,
				IsTransfer:   true,
				TransferToID: t.TransferAccountID,
			}
		} else {
			// For regular transactions, get the category
			category = categoriesByID[t.CategoryID]
			if category == nil {
				log.Printf("category %q not found", t.CategoryID)
				continue
			}
		}

		tx := &YNABTransaction{
			ID:              t.ID,
			Date:            t.Date,
			Category:        category,
			Account:         account,
			TransferAccount: transferAccount,
			Comment:         t.Memo,
			Amount:          t.Amount,
			AmountUSD:       t.Amount,
			IsTransfer:      isTransfer,
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
