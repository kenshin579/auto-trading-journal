package model

import (
	"strconv"
	"strings"
)

type Trade struct {
	Date         string
	TradeType    string // 매수 / 매도
	StockName    string
	StockCode    string
	Quantity     float64
	Price        float64
	Amount       float64
	Currency     string // KRW / USD / JPY
	ExchangeRate float64
	AmountKRW    float64
	Fee          float64
	Tax          float64
	Profit       float64
	ProfitKRW    float64
	ProfitRate   float64 // 퍼센트(14.68)
	Account      string
	// Sector: 국내는 KIS 업종 한글명, 해외는 FMP 영문 섹터. ETF·펀드(국내·해외 공통)는 "ETF" 고정.
	// 미지원 통화·미커버 종목은 "".
	Sector string
	// Industry: 국내는 KIS 표준산업분류, 해외는 FMP 영문 산업. ETF·펀드(국내·해외 공통)는
	// internal/etfclass taxonomy 카테고리. 미지원 통화·미커버 종목은 "".
	Industry string
}

// DupKey: (date, trade_type, stock_name, quantity_str, price_str)
type DupKey [5]string

func (t Trade) IsForeign() bool  { return strings.Contains(t.Account, "해외") }
func (t Trade) IsDomestic() bool { return !t.IsForeign() }

// rate: Python `self.profit_rate / 100 if self.profit_rate else 0` 와 동일.
// 반올림하지 않아 Python의 float64 결과(예 0.14679999999999999)와 정확히 일치한다.
func rate(p float64) float64 {
	if p == 0 {
		return 0
	}
	return p / 100
}

// ToDomesticRow: 국내 12컬럼 (종목명 뒤 섹터/산업)
func (t Trade) ToDomesticRow() []any {
	return []any{t.Date, t.TradeType, t.StockCode, t.StockName, t.Sector, t.Industry,
		t.Quantity, t.Price, t.Amount, t.Fee, t.Profit, rate(t.ProfitRate)}
}

// ToForeignRow: 해외 17컬럼 (종목명 뒤 섹터/산업 — FMP 로 채워지고, ETF·펀드는 국내와 같은
// 표기로 통일된다. 미지원 통화·미커버 종목만 공란)
func (t Trade) ToForeignRow() []any {
	return []any{t.Date, t.TradeType, t.Currency, t.StockCode, t.StockName, t.Sector, t.Industry,
		t.Quantity, t.Price, t.Amount, t.ExchangeRate, t.AmountKRW,
		t.Fee, t.Tax, t.Profit, t.ProfitKRW, rate(t.ProfitRate)}
}

func (t Trade) ToSheetRow() []any {
	if t.IsForeign() {
		return t.ToForeignRow()
	}
	return t.ToDomesticRow()
}

// numStr: 정수면 소수점 제거 (Python _num_str)
func numStr(v float64) string {
	if v == float64(int64(v)) {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'g', -1, 64)
}

func (t Trade) DuplicateKey() DupKey {
	return DupKey{t.Date, t.TradeType, t.StockName, numStr(t.Quantity), numStr(t.Price)}
}
