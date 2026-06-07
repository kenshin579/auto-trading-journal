package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func sampleDomestic() Trade {
	return Trade{Date: "2026-02-13", TradeType: "매도", StockName: "삼성전자", StockCode: "005930",
		Quantity: 10, Price: 70000, Amount: 700000, Currency: "KRW", ExchangeRate: 1,
		AmountKRW: 700000, Fee: 1500, Profit: 50000, ProfitRate: 14.68, Account: "미래에셋증권_국내계좌"}
}

func TestToDomesticRow(t *testing.T) {
	r := sampleDomestic().ToDomesticRow()
	// 일자, 구분, 종목코드, 종목명, 수량, 단가, 금액, 수수료, 손익, 수익률(소수)
	assert.Equal(t, []any{"2026-02-13", "매도", "005930", "삼성전자",
		10.0, 70000.0, 700000.0, 1500.0, 50000.0, 0.1468}, r)
}

func TestDuplicateKey_NumberNormalization(t *testing.T) {
	tr := sampleDomestic()
	tr.Quantity = 10
	tr.Price = 70000
	assert.Equal(t, DupKey{"2026-02-13", "매도", "삼성전자", "10", "70000"}, tr.DuplicateKey())
}
