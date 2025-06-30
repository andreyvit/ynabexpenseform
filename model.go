package main

type YNABData struct {
	BudgetID     string
	Accounts     []*YNABAccount
	Categories   []*YNABCategory
	Transactions []*YNABTransaction
	// Combined list of real categories and transfer pseudo-categories
	AllCategories []*YNABCategory
}

func (data *YNABData) CategoryByID(id string) *YNABCategory {
	// Check real categories first
	for _, c := range data.Categories {
		if c.ID == id {
			return c
		}
	}

	// Then check transfer pseudo-categories
	for _, c := range data.AllCategories {
		if c.ID == id {
			return c
		}
	}

	return nil
}

func (data *YNABData) AccountByID(id string) *YNABAccount {
	for _, a := range data.Accounts {
		if a.ID == id {
			return a
		}
	}
	return nil
}

type YNABAccount struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Balance         Amount `json:"balance"`
	TransferPayeeID string `json:"transfer_payee_id"`
}

type YNABAccountViewModel struct {
	*YNABAccount
	SecondaryBalance *Monetary
}

type YNABCategory struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	IsTransfer   bool   `json:"-"`
	TransferToID string `json:"-"`
}

// IsTransferCategory returns true if this is a transfer pseudo-category
func (c *YNABCategory) IsTransferCategory() bool {
	return c.IsTransfer
}

// TransferTargetID returns the target account ID for a transfer
func (c *YNABCategory) TransferTargetID() string {
	return c.TransferToID
}

type YNABTransaction struct {
	ID              string
	Date            string
	Category        *YNABCategory
	Account         *YNABAccount
	TransferAccount *YNABAccount
	Comment         string
	Amount          Amount
	AmountUSD       Amount
	IsTransfer      bool
}

// GenerateTransferCategories creates pseudo-categories for transfers between accounts
func GenerateTransferCategories(accounts []*YNABAccount) []*YNABCategory {
	categories := make([]*YNABCategory, 0, len(accounts))

	for _, account := range accounts {
		transferCat := &YNABCategory{
			ID:           "transfer-to-" + account.ID,
			Name:         "Transfer to " + account.Name,
			IsTransfer:   true,
			TransferToID: account.ID,
		}
		categories = append(categories, transferCat)
	}

	return categories
}

type Currency struct {
	Code   string
	Rate   float64
	Format string
}
