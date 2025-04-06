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

	accounts := make([]*YNABAccountViewModel, 0, len(data.Accounts))
	for _, a := range data.Accounts {
		vm := &YNABAccountViewModel{
			YNABAccount: a,
		}
		if app.SecondaryCurrency != nil {
			m := app.Convert(a.Balance, app.BudgetCurrency, app.SecondaryCurrency)
			vm.SecondaryBalance = &m
		}
		accounts = append(accounts, vm)
	}

	// Build data for the template
	output := struct {
		Accounts        []*YNABAccountViewModel
		Categories      []*YNABCategory
		Transactions    []*YNABTransaction
		Currencies      []*Currency
		DefaultCurrency *Currency
		BudgetCurrency  *Currency
		DefaultDate     time.Time
		Mock            string
	}{
		Accounts:        accounts,
		Categories:      data.Categories,
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

	tx := YNABTransaction{
		Date:     dateStr,
		Category: category,
		Account:  account,
		Comment:  comment,
		Amount:   amount,
	}

	if mock == "" {
		err = CreateYNABTransaction(context.Background(), &appCfg, data, &tx)
		if err != nil {
			return err
		}
	}

	appendTransactionToCachedData(&tx)

	http.Redirect(w, r, "/?mock="+url.QueryEscape(mock), http.StatusSeeOther)
	return nil
}
