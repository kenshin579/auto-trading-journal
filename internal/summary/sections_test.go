package summary

import (
	"testing"

	"github.com/kenshin579/auto-trading-journal/internal/model"
)

func TestAggregateMonthly(t *testing.T) {
	trades := []model.Trade{
		{Date: "2026-02-13", TradeType: "매수", AmountKRW: 100, Account: "A"},
		{Date: "2026-02-20", TradeType: "매도", AmountKRW: 150, ProfitKRW: 50, Account: "A"},
		{Date: "2026-01-05", TradeType: "매수", AmountKRW: 200, Account: "B"},
		{Date: "2026-02-01", TradeType: "매수", AmountKRW: 300, Account: "A"},
	}
	rows := aggregateMonthly(trades)

	// 키: (2026-01,B), (2026-02,A) — 사전식 정렬.
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].month != "2026-01" || rows[0].account != "B" {
		t.Errorf("row0 key = (%s,%s), want (2026-01,B)", rows[0].month, rows[0].account)
	}
	if rows[1].month != "2026-02" || rows[1].account != "A" {
		t.Errorf("row1 key = (%s,%s), want (2026-02,A)", rows[1].month, rows[1].account)
	}

	// (2026-02, A): 매수 100+300=400 (2건), 매도 150 (1건), 손익 50.
	r := rows[1]
	if r.buyCount != 2 || r.buyAmount != 400 {
		t.Errorf("buy = (%d,%v), want (2,400)", r.buyCount, r.buyAmount)
	}
	if r.sellCount != 1 || r.sellAmount != 150 {
		t.Errorf("sell = (%d,%v), want (1,150)", r.sellCount, r.sellAmount)
	}
	if r.profit != 50 {
		t.Errorf("profit = %v, want 50", r.profit)
	}
	// 수익률 = 50/150.
	want := 50.0 / 150.0
	if r.profitRt != want {
		t.Errorf("profitRt = %v, want %v", r.profitRt, want)
	}
}

func TestAggregateMonthlyEmpty(t *testing.T) {
	if rows := aggregateMonthly(nil); len(rows) != 0 {
		t.Errorf("expected 0 rows, got %d", len(rows))
	}
}

func TestAggregateMonthlyZeroSellAmount(t *testing.T) {
	// 매도금액 0 → 수익률 0 (0 나눗셈 방지).
	trades := []model.Trade{
		{Date: "2026-03-01", TradeType: "매수", AmountKRW: 100, Account: "A"},
	}
	rows := aggregateMonthly(trades)
	if len(rows) != 1 || rows[0].profitRt != 0 {
		t.Errorf("expected single row profitRt=0, got %+v", rows)
	}
}

func TestAggregateStock(t *testing.T) {
	trades := []model.Trade{
		{StockName: "삼성전자", StockCode: "005930", Account: "A", Currency: "KRW",
			TradeType: "매수", Quantity: 10, AmountKRW: 700000},
		{StockName: "삼성전자", StockCode: "005930", Account: "A", Currency: "KRW",
			TradeType: "매도", Quantity: 5, AmountKRW: 400000, ProfitKRW: 50000},
		{StockName: "Apple", StockCode: "AAPL", Account: "B", Currency: "USD",
			TradeType: "매수", Quantity: 2, AmountKRW: 300000},
	}
	rows := aggregateStock(trades)

	// 정렬: 종목명 사전식 → "Apple"(영문 대문자) < "삼성전자"(한글).
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].name != "Apple" {
		t.Errorf("row0 name = %s, want Apple", rows[0].name)
	}
	if rows[1].name != "삼성전자" {
		t.Errorf("row1 name = %s, want 삼성전자", rows[1].name)
	}

	// 삼성전자: 매수수량 10/금액 700000, 매도수량 5/금액 400000, 손익 50000.
	s := rows[1]
	if s.buyQty != 10 || s.buyAmount != 700000 {
		t.Errorf("buy = (%v,%v), want (10,700000)", s.buyQty, s.buyAmount)
	}
	if s.sellQty != 5 || s.sellAmount != 400000 || s.profit != 50000 {
		t.Errorf("sell = (%v,%v,%v), want (5,400000,50000)", s.sellQty, s.sellAmount, s.profit)
	}
	if s.profitRt != 50000.0/400000.0 {
		t.Errorf("profitRt = %v", s.profitRt)
	}

	// 투자비중: 전체 매수금액 = 700000 + 300000 = 1000000.
	if s.weight != 700000.0/1000000.0 {
		t.Errorf("weight = %v, want 0.7", s.weight)
	}
	if rows[0].weight != 300000.0/1000000.0 {
		t.Errorf("Apple weight = %v, want 0.3", rows[0].weight)
	}
}

func TestAggregateStockSortTieBreak(t *testing.T) {
	// 같은 종목명·코드, 계좌 다름 → 계좌 사전식.
	trades := []model.Trade{
		{StockName: "X", StockCode: "1", Account: "B", Currency: "KRW", TradeType: "매수", AmountKRW: 10},
		{StockName: "X", StockCode: "1", Account: "A", Currency: "KRW", TradeType: "매수", AmountKRW: 20},
	}
	rows := aggregateStock(trades)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].account != "A" || rows[1].account != "B" {
		t.Errorf("account order = (%s,%s), want (A,B)", rows[0].account, rows[1].account)
	}
}
