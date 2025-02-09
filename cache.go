package main

import (
	"context"
	"log"
	"sync"
	"time"
)

var (
	cachedData    *YNABData
	cachedDataTS  time.Time
	cachedDataMut sync.Mutex
)

const cacheDuration = 5 * time.Minute

func loadYNABDataWithCaching(ctx context.Context, mock string, fresh bool) (*YNABData, error) {
	cachedDataMut.Lock()
	defer cachedDataMut.Unlock()

	if cachedData != nil && !fresh && time.Since(cachedDataTS) < cacheDuration {
		return cachedData, nil
	}

	start := time.Now()
	var data *YNABData
	if mock, ok := MockData[mock]; ok {
		data = mock()
	} else {
		var err error
		data, err = LoadYNABData(ctx, &appCfg)
		if err != nil {
			return nil, err
		}
		log.Printf("Loaded YNAB data in %v ms", time.Since(start).Milliseconds())
	}

	cachedData = data
	cachedDataTS = start
	return data, nil
}

func clearCache() {
	cachedDataMut.Lock()
	defer cachedDataMut.Unlock()
	cachedData = nil
	cachedDataTS = time.Time{}
}

func appendTransactionToCachedData(tx *YNABTransaction) {
	cachedDataMut.Lock()
	defer cachedDataMut.Unlock()
	if cachedData == nil {
		return
	}
	cachedData.Transactions = append(cachedData.Transactions, tx)
	if account := cachedData.AccountByID(tx.Account.ID); account != nil {
		account.Balance += tx.Amount
	}
}
