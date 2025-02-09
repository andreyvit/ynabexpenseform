package main

type YNABData struct {
	BudgetID     string
	Accounts     []*YNABAccount
	Categories   []*YNABCategory
	Transactions []*YNABTransaction
}

func (data *YNABData) CategoryByID(id string) *YNABCategory {
	for _, c := range data.Categories {
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
	ID      string `json:"id"`
	Name    string `json:"name"`
	Balance Amount `json:"balance"`
}

type YNABCategory struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type YNABTransaction struct {
	ID        string
	Date      string
	Category  *YNABCategory
	Account   *YNABAccount
	Comment   string
	Amount    Amount
	AmountUSD Amount
}

type Currency struct {
	Code   string
	Rate   float64
	Format string
}
