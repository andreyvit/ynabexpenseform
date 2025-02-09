package main

import "fmt"

type Amount int64 // in milliunits

func (a Amount) String() string {
	return fmt.Sprintf("$%.2f", float64(a)/1000)
}

func (a Amount) RoundedUpToDeciCents() Amount {
	return (a + 99) / 100 * 100
}

type Monetary struct {
	Amount   Amount
	Currency *Currency
}

func (m Monetary) String() string {
	return FormatAmount(m.Amount, m.Currency, false)
}
