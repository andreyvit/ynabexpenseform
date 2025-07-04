package main

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

const maxVisibleTxCount = 100

//go:embed views/*.html
var viewsFS embed.FS

var tmpl = template.New("")

func FormatAmount(amount Amount, currency *Currency, brief bool) string {
	var s string
	if brief && amount%1000 == 0 {
		s = fmt.Sprintf("%.0f", float64(amount)/1000.0)
	} else {
		s = fmt.Sprintf("%.2f", float64(amount)/1000.0)
	}
	return strings.ReplaceAll(currency.Format, "9.99", s)
}

func init() {
	tmpl.Funcs(template.FuncMap{
		"isodate": func(t time.Time) string {
			return t.Format("2006-01-02")
		},
		"fmtamount": func(currency *Currency, amount Amount) string {
			return FormatAmount(amount, currency, false)
		},
		"replace": func(s, old, new string) string {
			return strings.Replace(s, old, new, -1)
		},
	})
	_, err := tmpl.ParseFS(viewsFS,
		"views/layout.html",
		"views/index.html",
		"views/_form.html",
		"views/_balances.html",
		"views/_history.html",
	)
	if err != nil {
		log.Fatalf("** template error: %v", err)
	}
}

func wrap(h func(w http.ResponseWriter, r *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			log.Printf("WARNING: %s %s failed: %v", r.Method, r.URL.Path, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

func (app *App) handleIndex(w http.ResponseWriter, r *http.Request) error {
	mock := r.FormValue("mock")

	data, err := loadYNABDataWithCaching(r.Context(), mock, false)
	if err != nil {
		return err
	}

	transactions := slices.Clone(data.Transactions)
	slices.Reverse(transactions)
	if len(transactions) > maxVisibleTxCount {
		transactions = transactions[:maxVisibleTxCount]
	}

	// Create list of ALL accounts for the form dropdown
	formAccounts := make([]*YNABAccountViewModel, 0, len(data.Accounts))
	for _, a := range data.Accounts {
		vm := &YNABAccountViewModel{
			YNABAccount: a,
		}
		formAccounts = append(formAccounts, vm)
	}

	// Create list of visible accounts for the balances section
	balanceAccounts := make([]*YNABAccountViewModel, 0, len(data.Accounts))
	for _, a := range data.Accounts {
		// Skip accounts with hidden balances
		if slices.Contains(app.HideBalance, a.Name) {
			continue
		}

		vm := &YNABAccountViewModel{
			YNABAccount: a,
		}
		if app.SecondaryCurrency != nil {
			m := app.Convert(a.Balance, app.BudgetCurrency, app.SecondaryCurrency)
			vm.SecondaryBalance = &m
		}
		balanceAccounts = append(balanceAccounts, vm)
	}

	// Build data for the template
	output := struct {
		Accounts        []*YNABAccountViewModel
		BalanceAccounts []*YNABAccountViewModel
		Categories      []*YNABCategory
		Transactions    []*YNABTransaction
		Currencies      []*Currency
		DefaultCurrency *Currency
		BudgetCurrency  *Currency
		DefaultDate     time.Time
		Mock            string
	}{
		Accounts:        formAccounts,       // All accounts for the form dropdown
		BalanceAccounts: balanceAccounts,    // Only visible accounts for the balances section
		Categories:      data.AllCategories, // Use AllCategories to include transfer options
		Transactions:    transactions,
		Currencies:      app.Currencies,
		DefaultCurrency: app.DefaultCurrency,
		BudgetCurrency:  app.BudgetCurrency,
		DefaultDate:     time.Now(),
		Mock:            mock,
	}

	var buf1 strings.Builder
	err = tmpl.ExecuteTemplate(&buf1, "index.html", output)
	if err != nil {
		return err
	}

	templateData := struct {
		Title   string
		Content template.HTML
	}{
		Title:   appCfg.PageTitle,
		Content: template.HTML(buf1.String()),
	}

	var buf2 bytes.Buffer
	err = tmpl.ExecuteTemplate(&buf2, "layout.html", templateData)
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, err = w.Write(buf2.Bytes())
	return err
}

func (app *App) handleRefresh(w http.ResponseWriter, r *http.Request) error {
	clearCache()
	http.Redirect(w, r, "/", http.StatusSeeOther)
	return nil
}

func (app *App) handleEnterExpense(w http.ResponseWriter, r *http.Request) error {
	err := r.ParseForm()
	if err != nil {
		return err
	}

	form := r.Form
	mock := r.FormValue("mock")

	data, err := loadYNABDataWithCaching(r.Context(), mock, false)
	if err != nil {
		return err
	}

	// Extract fields
	dateStr := form.Get("date")
	catID := form.Get("category")
	accID := form.Get("account")
	comment := strings.TrimSpace(form.Get("comment"))
	amountStr := form.Get("amount")
	currencyCode := form.Get("currency")

	if dateStr == "" {
		// default to today
		dateStr = time.Now().Format("2006-01-02")
	}

	amountVal, err := strconv.ParseFloat(strings.TrimSpace(amountStr), 64)
	if err != nil {
		return err
	}
	amount := Amount(amountVal * 1000)

	if currencyCode != app.BudgetCurrency.Code {
		currency := app.CurrenciesByCode[currencyCode]
		if currency == nil {
			return fmt.Errorf("currency %q not found", currencyCode)
		}

		amountComment := FormatAmount(amount, currency, true)
		if comment == "" {
			comment = amountComment
		} else {
			comment = fmt.Sprintf("%s %s", amountComment, comment)
		}

		amount = app.ConvertAmount(amount, currency, app.BudgetCurrency).RoundedUpToDeciCents()
	}

	account := data.AccountByID(accID)
	if account == nil {
		return fmt.Errorf("account %q not found", accID)
	}

	category := data.CategoryByID(catID)
	if category == nil {
		return fmt.Errorf("category %q not found", catID)
	}

	// Create transaction object
	tx := YNABTransaction{
		Date:     dateStr,
		Category: category,
		Account:  account,
		Comment:  comment,
		Amount:   amount,
	}

	// Handle transfer-specific fields
	if category.IsTransferCategory() {
		tx.IsTransfer = true

		// Find the target account for the transfer
		targetID := category.TransferTargetID()
		for _, a := range data.Accounts {
			if a.ID == targetID {
				tx.TransferAccount = a
				break
			}
		}

		// Clean up transfer comments to prevent duplication
		// If no comment provided for a transfer, leave it empty
		// YNAB will automatically display it as a transfer
		if comment == "" {
			tx.Comment = ""
		}
	}

	if mock == "" {
		err = CreateYNABTransaction(context.Background(), &appCfg, data, &tx)
		if err != nil {
			return err
		}
	}

	// Add transaction to the cache, including transfer info if applicable
	appendTransactionToCachedData(&tx)

	http.Redirect(w, r, "/?mock="+url.QueryEscape(mock), http.StatusSeeOther)
	return nil
}
