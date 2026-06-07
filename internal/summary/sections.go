package summary

import (
	"context"
	"fmt"
	"log/slog"
	"sort"

	"github.com/kenshin579/auto-trading-journal/internal/model"
	"github.com/kenshin579/auto-trading-journal/internal/sheets"
)

// ── 섹션 1: 포트폴리오 요약 ──────────────────────────────────

// writePortfolioSummary 는 포트폴리오 요약(2행)을 작성한다.
// (Python _write_portfolio_summary, py:127-159)
func (g *Generator) writePortfolioSummary(ctx context.Context, trades []model.Trade, startRow int) (int, error) {
	var totalBuy, totalSell, totalProfit float64
	var sellCount, profitable int
	for _, t := range trades {
		switch t.TradeType {
		case "매수":
			totalBuy += t.AmountKRW
		case "매도":
			totalSell += t.AmountKRW
			totalProfit += t.ProfitKRW
			sellCount++
			if t.ProfitKRW > 0 {
				profitable++
			}
		}
	}

	var totalReturn float64
	if totalSell != 0 {
		totalReturn = totalProfit / totalSell
	}
	totalCount := len(trades)
	var winRate float64
	if sellCount != 0 {
		winRate = float64(profitable) / float64(sellCount)
	}

	headers := []any{
		"지표", "총 매수금액(원)", "총 매도금액(원)", "총 실현손익(원)",
		"총 수익률(%)", "총 거래건수", "승률(%)",
	}
	values := []any{
		"값", totalBuy, totalSell, totalProfit,
		totalReturn, totalCount, winRate,
	}

	ranges := map[string][][]interface{}{
		fmt.Sprintf("%s!A%d:G%d", DashboardSheet, startRow, startRow):     {headers},
		fmt.Sprintf("%s!A%d:G%d", DashboardSheet, startRow+1, startRow+1): {values},
	}
	if err := g.client.BatchUpdateValues(ctx, ranges); err != nil {
		return 0, err
	}

	return startRow + 2, nil
}

// ── 섹션 2: 월별 성과 ──────────────────────────────────────

// monthlyRow 는 (연월, 계좌) 기준 집계 결과.
type monthlyRow struct {
	month, account                          string
	buyCount, sellCount                     int
	buyAmount, sellAmount, profit, profitRt float64
}

// aggregateMonthly 는 (연월, 계좌) 기준으로 집계하고 (연월, 계좌) 사전식으로 정렬한다.
// (Python _write_monthly_summary 의 집계/정렬, py:167-193)
func aggregateMonthly(trades []model.Trade) []monthlyRow {
	type key struct{ month, account string }
	type agg struct {
		buyCount, sellCount           int
		buyAmount, sellAmount, profit float64
	}
	groups := make(map[key]*agg)
	for _, t := range trades {
		month := t.Date
		if len(month) >= 7 {
			month = t.Date[:7]
		}
		k := key{month, t.Account}
		a := groups[k]
		if a == nil {
			a = &agg{}
			groups[k] = a
		}
		switch t.TradeType {
		case "매수":
			a.buyCount++
			a.buyAmount += t.AmountKRW
		case "매도":
			a.sellCount++
			a.sellAmount += t.AmountKRW
			a.profit += t.ProfitKRW
		}
	}

	keys := make([]key, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].month != keys[j].month {
			return keys[i].month < keys[j].month
		}
		return keys[i].account < keys[j].account
	})

	rows := make([]monthlyRow, 0, len(keys))
	for _, k := range keys {
		a := groups[k]
		var rt float64
		if a.sellAmount != 0 {
			rt = a.profit / a.sellAmount
		}
		rows = append(rows, monthlyRow{
			month: k.month, account: k.account,
			buyCount: a.buyCount, buyAmount: a.buyAmount,
			sellCount: a.sellCount, sellAmount: a.sellAmount,
			profit: a.profit, profitRt: rt,
		})
	}
	return rows
}

// writeMonthlySummary 는 월별 성과 섹션을 작성한다.
// (Python _write_monthly_summary, py:160-208)
func (g *Generator) writeMonthlySummary(ctx context.Context, trades []model.Trade, startRow int) (int, error) {
	headers := []any{
		"연월", "계좌", "매수건수", "매수금액(원)",
		"매도건수", "매도금액(원)", "실현손익(원)", "수익률(%)",
	}

	rows := aggregateMonthly(trades)

	if len(rows) == 0 {
		rng := fmt.Sprintf("%s!A%d:H%d", DashboardSheet, startRow, startRow)
		if err := g.client.BatchUpdateValues(ctx, map[string][][]interface{}{rng: {headers}}); err != nil {
			return 0, err
		}
		return startRow + 1, nil
	}

	allRows := make([][]interface{}, 0, len(rows)+1)
	allRows = append(allRows, headers)
	for _, r := range rows {
		allRows = append(allRows, []any{
			r.month, r.account,
			r.buyCount, r.buyAmount,
			r.sellCount, r.sellAmount,
			r.profit, r.profitRt,
		})
	}
	endRow := startRow + len(rows)
	rng := fmt.Sprintf("%s!A%d:H%d", DashboardSheet, startRow, endRow)
	if err := g.client.BatchUpdateValues(ctx, map[string][][]interface{}{rng: allRows}); err != nil {
		return 0, err
	}
	slog.Info("대시보드 월별 성과 작성", "rows", len(rows))
	return endRow + 1, nil
}

// ── 섹션 3: 종목별 현황 ──────────────────────────────────────

// stockRow 는 (종목명, 종목코드, 계좌, 통화) 기준 집계 결과.
type stockRow struct {
	name, code, account, currency          string
	buyQty, buyAmount, sellQty, sellAmount float64
	profit, profitRt, weight               float64
}

// aggregateStock 는 (종목명, 종목코드, 계좌, 통화) 기준으로 집계하고 사전식 정렬한다.
// 투자비중은 매수금액 / 전체 매수금액. (Python _write_stock_summary 의 집계/정렬, py:218-246)
func aggregateStock(trades []model.Trade) []stockRow {
	type key struct{ name, code, account, currency string }
	type agg struct {
		buyQty, buyAmount, sellQty, sellAmount, profit float64
	}
	groups := make(map[key]*agg)
	for _, t := range trades {
		k := key{t.StockName, t.StockCode, t.Account, t.Currency}
		a := groups[k]
		if a == nil {
			a = &agg{}
			groups[k] = a
		}
		switch t.TradeType {
		case "매수":
			a.buyQty += t.Quantity
			a.buyAmount += t.AmountKRW
		case "매도":
			a.sellQty += t.Quantity
			a.sellAmount += t.AmountKRW
			a.profit += t.ProfitKRW
		}
	}

	var totalBuyAmount float64
	for _, a := range groups {
		totalBuyAmount += a.buyAmount
	}

	keys := make([]key, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].name != keys[j].name {
			return keys[i].name < keys[j].name
		}
		if keys[i].code != keys[j].code {
			return keys[i].code < keys[j].code
		}
		if keys[i].account != keys[j].account {
			return keys[i].account < keys[j].account
		}
		return keys[i].currency < keys[j].currency
	})

	rows := make([]stockRow, 0, len(keys))
	for _, k := range keys {
		a := groups[k]
		var rt, weight float64
		if a.sellAmount != 0 {
			rt = a.profit / a.sellAmount
		}
		if totalBuyAmount != 0 {
			weight = a.buyAmount / totalBuyAmount
		}
		rows = append(rows, stockRow{
			name: k.name, code: k.code, account: k.account, currency: k.currency,
			buyQty: a.buyQty, buyAmount: a.buyAmount,
			sellQty: a.sellQty, sellAmount: a.sellAmount,
			profit: a.profit, profitRt: rt, weight: weight,
		})
	}
	return rows
}

// writeStockSummary 는 종목별 현황 섹션을 작성한다.
// (Python _write_stock_summary, py:210-267)
func (g *Generator) writeStockSummary(ctx context.Context, trades []model.Trade, startRow int) (int, error) {
	headers := []any{
		"종목코드", "종목명", "계좌", "통화",
		"총매수수량", "총매수금액(원)", "총매도수량", "총매도금액(원)",
		"실현손익(원)", "수익률(%)", "투자비중(%)",
	}

	rows := aggregateStock(trades)

	if len(rows) == 0 {
		rng := fmt.Sprintf("%s!A%d:K%d", DashboardSheet, startRow, startRow)
		if err := g.client.BatchUpdateValues(ctx, map[string][][]interface{}{rng: {headers}}); err != nil {
			return 0, err
		}
		return startRow + 1, nil
	}

	allRows := make([][]interface{}, 0, len(rows)+1)
	allRows = append(allRows, headers)
	for _, r := range rows {
		allRows = append(allRows, []any{
			r.code, r.name, r.account, r.currency,
			r.buyQty, r.buyAmount,
			r.sellQty, r.sellAmount,
			r.profit, r.profitRt, r.weight,
		})
	}
	endRow := startRow + len(rows)

	// 종목코드 컬럼(A=1)을 쓰기 전에 TEXT 로 → USER_ENTERED 숫자 변환 방지(정렬·앞0 보존).
	// (Python apply_number_format_to_columns col=1 '@' TEXT, py:253-257)
	if err := g.client.ApplyNumberFormatToColumns(
		ctx, DashboardSheet,
		[]sheets.ColumnFormat{{Col: 1, Pattern: "@", Type: "TEXT"}},
		startRow, endRow,
	); err != nil {
		return 0, err
	}

	rng := fmt.Sprintf("%s!A%d:K%d", DashboardSheet, startRow, endRow)
	if err := g.client.BatchUpdateValues(ctx, map[string][][]interface{}{rng: allRows}); err != nil {
		return 0, err
	}
	slog.Info("대시보드 종목별 현황 작성", "rows", len(rows))
	return endRow + 1, nil
}
