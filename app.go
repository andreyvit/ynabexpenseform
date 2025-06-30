package main

import "fmt"

type App struct {
	Currencies        []*Currency
	CurrenciesByCode  map[string]*Currency
	DefaultCurrency   *Currency
	BudgetCurrency    *Currency
	SecondaryCurrency *Currency
	HideBalance       []string
}

func New(cfg *AppConfig) (*App, error) {
	currencies := make([]*Currency, 0)
	for _, c := range cfg.Currencies {
		currencies = append(currencies, &Currency{
			Code:   c.Code,
			Rate:   c.Rate,
			Format: c.Format,
		})
	}

	currenciesByCode := make(map[string]*Currency, len(cfg.Currencies))
	for _, c := range currencies {
		currenciesByCode[c.Code] = c
	}

	defaultCurrency := currenciesByCode[cfg.DefaultCurrency]
	if defaultCurrency == nil {
		return nil, fmt.Errorf("default currency %q not found", cfg.DefaultCurrency)
	}

	budgetCurrency := currenciesByCode[cfg.BudgetCurrency]
	if budgetCurrency == nil {
		return nil, fmt.Errorf("budget currency %q not found", cfg.BudgetCurrency)
	}

	var secondaryCurrency *Currency
	if cfg.SecondaryCurrency != "" {
		secondaryCurrency = currenciesByCode[cfg.SecondaryCurrency]
		if secondaryCurrency == nil {
			return nil, fmt.Errorf("secondary currency %q not found", cfg.SecondaryCurrency)
		}
	}

	orderedCurrencies := make([]*Currency, 0, len(currencies))
	orderedCurrencies = append(orderedCurrencies, defaultCurrency)
	for _, c := range currencies {
		if c == defaultCurrency {
			continue
		}
		orderedCurrencies = append(orderedCurrencies, c)
	}

	return &App{
		Currencies:        orderedCurrencies,
		CurrenciesByCode:  currenciesByCode,
		DefaultCurrency:   defaultCurrency,
		BudgetCurrency:    budgetCurrency,
		SecondaryCurrency: secondaryCurrency,
		HideBalance:       cfg.HideBalance,
	}, nil
}

func (app *App) ConvertAmount(amount Amount, from, to *Currency) Amount {
	if from == to {
		return amount
	} else if from == app.BudgetCurrency {
		return Amount(float64(amount)*to.Rate + 0.5)
	} else if to == app.BudgetCurrency {
		return Amount(float64(amount)/from.Rate + 0.5)
	} else {
		interim := app.ConvertAmount(amount, from, app.BudgetCurrency)
		return app.ConvertAmount(interim, app.BudgetCurrency, to)
	}
}

func (app *App) Convert(amount Amount, from, to *Currency) Monetary {
	return Monetary{
		Amount:   app.ConvertAmount(amount, from, to),
		Currency: to,
	}
}
