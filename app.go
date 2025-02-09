package main

import "fmt"

type App struct {
	Currencies       []*Currency
	CurrenciesByCode map[string]*Currency
	DefaultCurrency  *Currency
	BudgetCurrency   *Currency
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

	orderedCurrencies := make([]*Currency, 0, len(currencies))
	orderedCurrencies = append(orderedCurrencies, defaultCurrency)
	for _, c := range currencies {
		if c == defaultCurrency {
			continue
		}
		orderedCurrencies = append(orderedCurrencies, c)
	}

	return &App{
		Currencies:       orderedCurrencies,
		CurrenciesByCode: currenciesByCode,
		DefaultCurrency:  defaultCurrency,
		BudgetCurrency:   budgetCurrency,
	}, nil
}
