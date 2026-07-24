package summary

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"time"

	"github.com/kenshin579/auto-trading-journal/internal/model"
	"github.com/kenshin579/auto-trading-journal/internal/sheets"
)

// ── 섹션 4: 투자 지표 ──────────────────────────────────────

// accountStockCount 는 계좌별 종목수 집계 결과.
// held = 잔량이 남은(매수수량 > 매도수량) 종목 수, total = 한 번이라도 거래한 종목 수.
type accountStockCount struct {
	account     string
	held, total int
}

// aggregateAccountStockCount 는 계좌별로 보유/거래 종목수를 세고 계좌명 사전식으로 정렬한다.
// 종목 식별 키는 (종목코드, 종목명, 통화) — 해외처럼 코드가 비어도 이름으로 구분된다.
func aggregateAccountStockCount(trades []model.Trade) []accountStockCount {
	type stockKey struct{ code, name, currency string }
	// 계좌 → 종목 → 순수량(매수 - 매도).
	netQty := map[string]map[stockKey]float64{}
	for _, t := range trades {
		if t.TradeType != "매수" && t.TradeType != "매도" {
			continue
		}
		byStock := netQty[t.Account]
		if byStock == nil {
			byStock = map[stockKey]float64{}
			netQty[t.Account] = byStock
		}
		k := stockKey{t.StockCode, t.StockName, t.Currency}
		if t.TradeType == "매수" {
			byStock[k] += t.Quantity
		} else {
			byStock[k] -= t.Quantity
		}
	}

	accounts := make([]string, 0, len(netQty))
	for a := range netQty {
		accounts = append(accounts, a)
	}
	sort.Strings(accounts)

	result := make([]accountStockCount, 0, len(accounts))
	for _, a := range accounts {
		c := accountStockCount{account: a, total: len(netQty[a])}
		for _, qty := range netQty[a] {
			if qty > 1e-9 { // 부동소수 오차 방지
				c.held++
			}
		}
		result = append(result, c)
	}
	return result
}

// writeInvestmentMetrics 는 투자 지표 섹션을 작성한다.
// (Python _write_investment_metrics, py:269-377)
func (g *Generator) writeInvestmentMetrics(ctx context.Context, trades []model.Trade, startRow int) (int, error) {
	var buyTrades, sellTrades []model.Trade
	var totalBuy float64
	for _, t := range trades {
		switch t.TradeType {
		case "매수":
			buyTrades = append(buyTrades, t)
			totalBuy += t.AmountKRW
		case "매도":
			sellTrades = append(sellTrades, t)
		}
	}

	rows := [][]any{{"[투자 지표]", ""}}
	var pctRows, krwRows []int // % / 원화 포맷 행 (0-based offset)

	// 계좌별 투자비중 (파이 차트용 데이터를 N~O열에 별도 작성).
	rows = append(rows, []any{"계좌별 투자비중", ""})
	accountBuy := map[string]float64{}
	for _, t := range buyTrades {
		accountBuy[t.Account] += t.AmountKRW
	}
	accounts := sortedKeys(accountBuy)
	var pieData [][]any
	for _, account := range accounts {
		amount := accountBuy[account]
		pctRows = append(pctRows, len(rows))
		var ratio float64
		if totalBuy != 0 {
			ratio = amount / totalBuy
		}
		rows = append(rows, []any{"  " + account, ratio})
		pieData = append(pieData, []any{account, ratio})
	}

	if len(pieData) > 0 {
		pieEndRow := startRow + len(pieData) - 1
		rng := fmt.Sprintf("%s!N%d:O%d", DashboardSheet, startRow, pieEndRow)
		if err := g.client.UpdateCells(ctx, rng, pieData); err != nil {
			return 0, err
		}
		g.pieDataRange = rowRange{start: startRow, end: pieEndRow, ok: true}
	} else {
		g.pieDataRange = rowRange{}
	}

	// 계좌별 종목수 (보유 / 전체). 제목 행의 B·C 열이 헤더 역할을 한다.
	if counts := aggregateAccountStockCount(trades); len(counts) > 0 {
		rows = append(rows, []any{"계좌별 종목수", "보유", "전체"})
		var heldSum, totalSum int
		for _, c := range counts {
			rows = append(rows, []any{"  " + c.account, c.held, c.total})
			heldSum += c.held
			totalSum += c.total
		}
		// 합계는 계좌별 값의 단순 합 — 같은 종목을 두 계좌에서 거래하면 2로 센다.
		// (표의 세로 합과 어긋나지 않도록 의도한 것. 라벨로 그 의미를 명시한다.)
		rows = append(rows, []any{"  합계(중복 포함)", heldSum, totalSum})
	}

	// 통화별 투자비중.
	rows = append(rows, []any{"통화별 투자비중", ""})
	currencyBuy := map[string]float64{}
	for _, t := range buyTrades {
		currencyBuy[t.Currency] += t.AmountKRW
	}
	for _, currency := range sortedKeys(currencyBuy) {
		pctRows = append(pctRows, len(rows))
		var ratio float64
		if totalBuy != 0 {
			ratio = currencyBuy[currency] / totalBuy
		}
		rows = append(rows, []any{"  " + currency, ratio})
	}

	// 섹터별 투자비중 (섹터 분류기가 있을 때만). (Python py:312-324)
	if g.sc != nil {
		sectorMap, err := g.getSectorMap(ctx, buyTrades)
		if err != nil {
			return 0, err
		}
		if len(sectorMap) > 0 {
			rows = append(rows, []any{"섹터별 투자비중", ""})
			sectorBuy := map[string]float64{}
			for _, t := range buyTrades {
				sector := sectorMap[t.StockName]
				if sector == "" {
					sector = "기타"
				}
				sectorBuy[sector] += t.AmountKRW
			}
			// 금액 내림차순 정렬 (동률은 섹터명 사전식).
			type sb struct {
				sector string
				amount float64
			}
			list := make([]sb, 0, len(sectorBuy))
			for s, a := range sectorBuy {
				list = append(list, sb{s, a})
			}
			sort.Slice(list, func(i, j int) bool {
				if list[i].amount != list[j].amount {
					return list[i].amount > list[j].amount
				}
				return list[i].sector < list[j].sector
			})
			for _, e := range list {
				pctRows = append(pctRows, len(rows))
				var ratio float64
				if totalBuy != 0 {
					ratio = e.amount / totalBuy
				}
				rows = append(rows, []any{"  " + e.sector, ratio})
			}
		}
	}

	// 상위 5종목 집중도.
	stockBuy := map[string]float64{}
	for _, t := range buyTrades {
		stockBuy[t.StockName] += t.AmountKRW
	}
	buyVals := make([]float64, 0, len(stockBuy))
	for _, v := range stockBuy {
		buyVals = append(buyVals, v)
	}
	sort.Sort(sort.Reverse(sort.Float64Slice(buyVals)))
	var top5sum float64
	for i := 0; i < len(buyVals) && i < 5; i++ {
		top5sum += buyVals[i]
	}
	var top5Ratio float64
	if totalBuy != 0 {
		top5Ratio = top5sum / totalBuy
	}
	pctRows = append(pctRows, len(rows))
	rows = append(rows, []any{"상위 5종목 집중도", top5Ratio})

	// 평균 수익률 / 평균 손실률.
	var profitRateSum, lossRateSum float64
	var profitRateCount, lossRateCount int
	for _, t := range sellTrades {
		if t.ProfitRate > 0 {
			profitRateSum += t.ProfitRate
			profitRateCount++
		} else if t.ProfitRate < 0 {
			lossRateSum += t.ProfitRate
			lossRateCount++
		}
	}
	var avgProfit, avgLoss float64
	if profitRateCount > 0 {
		avgProfit = profitRateSum / float64(profitRateCount) / 100
	}
	if lossRateCount > 0 {
		avgLoss = lossRateSum / float64(lossRateCount) / 100
	}
	pctRows = append(pctRows, len(rows))
	rows = append(rows, []any{"평균 수익률", avgProfit})
	pctRows = append(pctRows, len(rows))
	rows = append(rows, []any{"평균 손실률", avgLoss})

	// 손익비.
	var plRatio float64
	if avgLoss != 0 {
		plRatio = math.Abs(avgProfit / avgLoss)
	}
	rows = append(rows, []any{"손익비", round2(plRatio)})

	// 수익 Top 10 / 손실 Top 10 (종목별 합산).
	if len(sellTrades) > 0 {
		stockProfit := map[string]float64{}
		for _, t := range sellTrades {
			stockProfit[t.StockName] += t.ProfitKRW
		}
		type sp struct {
			name   string
			profit float64
		}
		sorted := make([]sp, 0, len(stockProfit))
		for n, p := range stockProfit {
			sorted = append(sorted, sp{n, p})
		}
		// 이익 내림차순 (동률은 종목명 사전식).
		sort.Slice(sorted, func(i, j int) bool {
			if sorted[i].profit != sorted[j].profit {
				return sorted[i].profit > sorted[j].profit
			}
			return sorted[i].name < sorted[j].name
		})

		rows = append(rows, []any{"수익 Top 10", ""})
		for i := 0; i < len(sorted) && i < 10; i++ {
			krwRows = append(krwRows, len(rows))
			rows = append(rows, []any{"  " + sorted[i].name, sorted[i].profit})
		}

		// 손실 Top 10: 마지막 10개를 역순(가장 큰 손실부터)으로, profit<0 만. (Python py:362-365)
		rows = append(rows, []any{"손실 Top 10", ""})
		last10 := sorted
		if len(sorted) > 10 {
			last10 = sorted[len(sorted)-10:]
		}
		for i := len(last10) - 1; i >= 0; i-- {
			if last10[i].profit < 0 {
				krwRows = append(krwRows, len(rows))
				rows = append(rows, []any{"  " + last10[i].name, last10[i].profit})
			}
		}
	}

	// 데이터 작성.
	endRow := startRow + len(rows) - 1
	// 계좌별 종목수 블록만 C열을 쓴다(나머지 행은 2열 ragged).
	// 2열 행의 C열이 비워지는 것은 EnsureDashboardSheet 의 A1:Z 선(先)클리어에 의존한다
	// — 클리어를 없애거나 쓰기 순서를 바꾸면 이전 실행의 C열 값이 남을 수 있다.
	rng := fmt.Sprintf("%s!A%d:C%d", DashboardSheet, startRow, endRow)
	if err := g.client.UpdateCells(ctx, rng, rows); err != nil {
		return 0, err
	}

	// 행별 포맷 적용.
	g.collectMetricsFormats(startRow, pctRows, krwRows, len(rows))

	slog.Info("대시보드 투자 지표 작성", "rows", len(rows))
	return startRow + len(rows), nil
}

// ── 섹션 5: 매매 인사이트 ──────────────────────────────────

// writeTradingInsights 는 매매 인사이트 섹션을 작성한다.
// (Python _write_trading_insights, py:379-502)
func (g *Generator) writeTradingInsights(ctx context.Context, trades []model.Trade, startRow int) (int, error) {
	var sellTrades []model.Trade
	for _, t := range trades {
		if t.TradeType == "매도" {
			sellTrades = append(sellTrades, t)
		}
	}

	rows := [][]any{{"[매매 인사이트]", ""}}
	var pctRows, krwRows []int

	if len(sellTrades) == 0 {
		rows = append(rows, []any{"매도 거래 없음", ""})
		endRow := startRow + len(rows) - 1
		rng := fmt.Sprintf("%s!A%d:B%d", DashboardSheet, startRow, endRow)
		if err := g.client.UpdateCells(ctx, rng, rows); err != nil {
			return 0, err
		}
		return startRow + len(rows), nil
	}

	// --- 공통 계산 ---
	var profitable, losing []model.Trade
	for _, t := range sellTrades {
		if t.ProfitKRW > 0 {
			profitable = append(profitable, t)
		} else {
			losing = append(losing, t)
		}
	}
	winRate := float64(len(profitable)) / float64(len(sellTrades))
	lossRate := 1 - winRate

	var avgProfitAmount, avgLossAmount float64
	if len(profitable) > 0 {
		var s float64
		for _, t := range profitable {
			s += t.ProfitKRW
		}
		avgProfitAmount = s / float64(len(profitable))
	}
	if len(losing) > 0 {
		var s float64
		for _, t := range losing {
			s += t.ProfitKRW
		}
		avgLossAmount = s / float64(len(losing))
	}

	// --- 5-1. 매매 기대값 (Expectancy) ---
	rows = append(rows, []any{"매매 기대값 (Expectancy)", ""})
	expectancy := (avgProfitAmount * winRate) - (math.Abs(avgLossAmount) * lossRate)
	krwRows = append(krwRows, len(rows))
	rows = append(rows, []any{"  1건당 기대 수익", expectancy})

	// --- 5-2. Profit Factor ---
	var totalGrossProfit, totalGrossLoss float64
	for _, t := range profitable {
		totalGrossProfit += t.ProfitKRW
	}
	for _, t := range losing {
		totalGrossLoss += t.ProfitKRW
	}
	totalGrossLoss = math.Abs(totalGrossLoss)
	var profitFactor float64
	if totalGrossLoss != 0 {
		profitFactor = totalGrossProfit / totalGrossLoss
	}
	rows = append(rows, []any{"Profit Factor", round2(profitFactor)})

	// --- 5-3. 연속 승/패 기록 ---
	rows = append(rows, []any{"연속 승/패 기록", ""})
	sortedSells := sortSellsByDate(sellTrades)
	maxWins, maxLosses, curWins, curLosses := calcStreaks(sortedSells)
	rows = append(rows, []any{"  최대 연승", maxWins})
	rows = append(rows, []any{"  최대 연패", maxLosses})
	if curWins > 0 {
		rows = append(rows, []any{"  현재 연승", curWins})
	} else {
		rows = append(rows, []any{"  현재 연패", curLosses})
	}

	// --- 5-4. 요일별 성과 ---
	rows = append(rows, []any{"요일별 성과", ""})
	dayNames := []string{"월요일", "화요일", "수요일", "목요일", "금요일", "토요일", "일요일"}
	dayStats := calcDayOfWeekStats(sortedSells)
	for weekday := 0; weekday < 7; weekday++ {
		stats, ok := dayStats[weekday]
		if ok {
			rows = append(rows, []any{
				"  " + dayNames[weekday],
				fmt.Sprintf("%d건 / ₩%s / 승률 %.1f%%",
					stats.count, formatThousands(stats.profitSum), stats.winRate),
			})
		}
	}

	// --- 5-5. 월별 수익 추세 (MoM 변화) ---
	rows = append(rows, []any{"월별 수익 추세", ""})
	monthlyProfits := calcMonthlyProfits(sortedSells)
	havePrev := false
	var prevProfit float64
	for _, mp := range monthlyProfits {
		var label string
		if havePrev && prevProfit != 0 {
			change := mp.profit - prevProfit
			changePct := change / math.Abs(prevProfit) * 100
			label = fmt.Sprintf("  %s (전월대비 %+.1f%%)", mp.month, changePct)
		} else {
			label = "  " + mp.month
		}
		krwRows = append(krwRows, len(rows))
		rows = append(rows, []any{label, mp.profit})
		prevProfit = mp.profit
		havePrev = true
	}

	// --- 5-6. 손절/익절 규율 분석 ---
	rows = append(rows, []any{"손절/익절 규율", ""})
	krwRows = append(krwRows, len(rows))
	rows = append(rows, []any{"  평균 수익 금액", avgProfitAmount})
	krwRows = append(krwRows, len(rows))
	rows = append(rows, []any{"  평균 손실 금액", avgLossAmount})
	var profitMultiple float64
	if avgLossAmount != 0 {
		profitMultiple = avgProfitAmount / math.Abs(avgLossAmount)
	}
	rows = append(rows, []any{"  평균 수익 배수", round2(profitMultiple)})

	// --- 5-7. 거래 빈도 분석 ---
	rows = append(rows, []any{"거래 빈도 분석", ""})
	freq := calcTradeFrequency(sortedSells)
	rows = append(rows, []any{"  월 평균 거래건수", round1(freq.avgMonthly)})
	rows = append(rows, []any{
		fmt.Sprintf("  거래 가장 많은 달 (%s)", freq.maxMonth), freq.maxCount,
	})
	rows = append(rows, []any{
		fmt.Sprintf("  거래 가장 적은 달 (%s)", freq.minMonth), freq.minCount,
	})

	// --- 5-8. 수익 집중도 분석 ---
	rows = append(rows, []any{"수익 집중도", ""})
	stockProfit := map[string]float64{}
	for _, t := range sellTrades {
		stockProfit[t.StockName] += t.ProfitKRW
	}
	var positiveVals []float64
	var totalPositive float64
	for _, v := range stockProfit {
		if v > 0 {
			positiveVals = append(positiveVals, v)
			totalPositive += v
		}
	}
	sort.Sort(sort.Reverse(sort.Float64Slice(positiveVals)))
	var top3, top5 float64
	for i := 0; i < len(positiveVals); i++ {
		if i < 3 {
			top3 += positiveVals[i]
		}
		if i < 5 {
			top5 += positiveVals[i]
		}
	}
	var top3Ratio, top5Ratio float64
	if totalPositive != 0 {
		top3Ratio = top3 / totalPositive
		top5Ratio = top5 / totalPositive
	}
	pctRows = append(pctRows, len(rows))
	rows = append(rows, []any{"  상위 3종목 수익 비중", top3Ratio})
	pctRows = append(pctRows, len(rows))
	rows = append(rows, []any{"  상위 5종목 수익 비중", top5Ratio})

	// 데이터 작성.
	endRow := startRow + len(rows) - 1
	rng := fmt.Sprintf("%s!A%d:B%d", DashboardSheet, startRow, endRow)
	if err := g.client.UpdateCells(ctx, rng, rows); err != nil {
		return 0, err
	}

	// 행별 포맷 적용.
	g.collectMetricsFormats(startRow, pctRows, krwRows, len(rows))

	slog.Info("대시보드 매매 인사이트 작성", "rows", len(rows))
	return startRow + len(rows), nil
}

// ── 섹션 6: 월별 성과 추이 ──────────────────────────────────

// writeMonthlyTrend 는 월별 성과 추이 섹션을 작성한다.
// (Python _write_monthly_trend, py:504-544)
func (g *Generator) writeMonthlyTrend(ctx context.Context, trades []model.Trade, startRow int) (int, error) {
	headers := []any{
		"연월", "매도건수", "실현손익(원)", "수익률(%)",
		"승률(%)", "평균수익률(%)", "평균손실률(%)",
		"손익비", "Profit Factor", "기대값(원)", "전월대비(%)",
	}

	var sellTrades []model.Trade
	for _, t := range trades {
		if t.TradeType == "매도" {
			sellTrades = append(sellTrades, t)
		}
	}
	sortedSells := sortSellsByDate(sellTrades)
	trendData := calcMonthlyTrend(sortedSells)

	rows := make([][]any, 0, len(trendData))
	for _, d := range trendData {
		var mom any = ""
		if d.momChangeOK {
			mom = d.momChange
		}
		rows = append(rows, []any{
			d.month, d.sellCount, d.profitKRW, d.returnRate,
			d.winRate, d.avgProfitRate, d.avgLossRate,
			d.plRatio, d.profitFactor, d.expectancy, mom,
		})
	}

	if len(rows) > 0 {
		allRows := make([][]any, 0, len(rows)+1)
		allRows = append(allRows, headers)
		allRows = append(allRows, rows...)
		endRow := startRow + len(rows)
		rng := fmt.Sprintf("%s!A%d:K%d", DashboardSheet, startRow, endRow)
		if err := g.client.UpdateCells(ctx, rng, allRows); err != nil {
			return 0, err
		}
		slog.Info("대시보드 월별 성과 추이 작성", "rows", len(rows))
		return endRow + 1, nil
	}

	rng := fmt.Sprintf("%s!A%d:K%d", DashboardSheet, startRow, startRow)
	if err := g.client.UpdateCells(ctx, rng, [][]any{headers}); err != nil {
		return 0, err
	}
	return startRow + 1, nil
}

// ── 순수 계산 헬퍼 ──────────────────────────────────────────

// monthlyTrendRow 는 한 달의 월별 추이 지표. (Python _calc_monthly_trend 의 dict)
type monthlyTrendRow struct {
	month         string
	sellCount     int
	profitKRW     float64
	returnRate    float64
	winRate       float64
	avgProfitRate float64
	avgLossRate   float64
	plRatio       float64
	profitFactor  float64
	expectancy    float64
	momChange     float64
	momChangeOK   bool
}

// calcMonthlyTrend 은 정렬된 매도 거래로부터 월별 성과 추이를 계산한다.
// (Python _calc_monthly_trend, py:547-613)
func calcMonthlyTrend(sortedSells []model.Trade) []monthlyTrendRow {
	monthGroups := map[string][]model.Trade{}
	for _, t := range sortedSells {
		monthGroups[monthOf(t.Date)] = append(monthGroups[monthOf(t.Date)], t)
	}

	months := make([]string, 0, len(monthGroups))
	for m := range monthGroups {
		months = append(months, m)
	}
	sort.Strings(months)

	results := make([]monthlyTrendRow, 0, len(months))
	havePrev := false
	var prevProfit float64

	for _, month := range months {
		sells := monthGroups[month]
		var profitable, losing []model.Trade
		for _, t := range sells {
			if t.ProfitKRW > 0 {
				profitable = append(profitable, t)
			} else {
				losing = append(losing, t)
			}
		}

		sellCount := len(sells)
		var totalSellAmount, profitKRW float64
		for _, t := range sells {
			totalSellAmount += t.AmountKRW
			profitKRW += t.ProfitKRW
		}

		var returnRate float64
		if totalSellAmount != 0 {
			returnRate = profitKRW / totalSellAmount
		}
		winRate := float64(len(profitable)) / float64(sellCount)

		var avgProfitRate, avgLossRate float64
		if len(profitable) > 0 {
			var s float64
			for _, t := range profitable {
				s += t.ProfitRate
			}
			avgProfitRate = s / float64(len(profitable)) / 100
		}
		if len(losing) > 0 {
			var s float64
			for _, t := range losing {
				s += t.ProfitRate
			}
			avgLossRate = s / float64(len(losing)) / 100
		}

		var plRatio float64
		if avgLossRate != 0 {
			plRatio = math.Abs(avgProfitRate / avgLossRate)
		}

		var grossProfit, grossLoss float64
		for _, t := range profitable {
			grossProfit += t.ProfitKRW
		}
		for _, t := range losing {
			grossLoss += t.ProfitKRW
		}
		grossLoss = math.Abs(grossLoss)
		var profitFactor float64
		if grossLoss != 0 {
			profitFactor = grossProfit / grossLoss
		}

		var avgProfitAmount, avgLossAmount float64
		if len(profitable) > 0 {
			avgProfitAmount = grossProfit / float64(len(profitable))
		}
		if len(losing) > 0 {
			avgLossAmount = grossLoss / float64(len(losing))
		}
		expectancy := (avgProfitAmount * winRate) - (avgLossAmount * (1 - winRate))

		row := monthlyTrendRow{
			month:         month,
			sellCount:     sellCount,
			profitKRW:     profitKRW,
			returnRate:    returnRate,
			winRate:       winRate,
			avgProfitRate: avgProfitRate,
			avgLossRate:   avgLossRate,
			plRatio:       round2(plRatio),
			profitFactor:  round2(profitFactor),
			expectancy:    expectancy,
		}
		if havePrev && prevProfit != 0 {
			row.momChange = (profitKRW - prevProfit) / math.Abs(prevProfit)
			row.momChangeOK = true
		}
		prevProfit = profitKRW
		havePrev = true

		results = append(results, row)
	}

	return results
}

// calcStreaks 는 정렬된 매도 거래의 연속 승/패 기록을 계산한다.
// 반환: (최대 연승, 최대 연패, 현재 연승, 현재 연패). (Python _calc_streaks, py:616-635)
func calcStreaks(sortedSells []model.Trade) (maxWins, maxLosses, curWins, curLosses int) {
	for _, t := range sortedSells {
		if t.ProfitKRW > 0 {
			curWins++
			curLosses = 0
			if curWins > maxWins {
				maxWins = curWins
			}
		} else {
			curLosses++
			curWins = 0
			if curLosses > maxLosses {
				maxLosses = curLosses
			}
		}
	}
	return
}

// dayStat 은 요일별 (거래건수, 실현손익합, 승률%).
type dayStat struct {
	count     int
	profitSum float64
	winRate   float64
}

// calcDayOfWeekStats 는 요일별 (거래건수, 실현손익합, 승률%) 을 계산한다.
// weekday 키: 월=0 ... 일=6 (Python datetime.weekday()).
// (Python _calc_day_of_week_stats, py:638-654)
func calcDayOfWeekStats(sortedSells []model.Trade) map[int]dayStat {
	dayGroups := map[int][]model.Trade{}
	for _, t := range sortedSells {
		wd := weekdayMondayZero(t.Date)
		if wd < 0 {
			continue
		}
		dayGroups[wd] = append(dayGroups[wd], t)
	}

	result := map[int]dayStat{}
	for wd, group := range dayGroups {
		count := len(group)
		var profitSum float64
		wins := 0
		for _, t := range group {
			profitSum += t.ProfitKRW
			if t.ProfitKRW > 0 {
				wins++
			}
		}
		var wr float64
		if count != 0 {
			wr = float64(wins) / float64(count) * 100
		}
		result[wd] = dayStat{count: count, profitSum: profitSum, winRate: wr}
	}
	return result
}

// monthProfit 은 (연월, 실현손익합).
type monthProfit struct {
	month  string
	profit float64
}

// calcMonthlyProfits 는 월별 실현손익을 집계해 연월 사전식 정렬로 반환한다.
// (Python _calc_monthly_profits, py:657-663)
func calcMonthlyProfits(sortedSells []model.Trade) []monthProfit {
	monthly := map[string]float64{}
	for _, t := range sortedSells {
		monthly[monthOf(t.Date)] += t.ProfitKRW
	}
	months := make([]string, 0, len(monthly))
	for m := range monthly {
		months = append(months, m)
	}
	sort.Strings(months)
	out := make([]monthProfit, 0, len(months))
	for _, m := range months {
		out = append(out, monthProfit{month: m, profit: monthly[m]})
	}
	return out
}

// tradeFrequency 는 월별 거래빈도 통계. (Python _calc_trade_frequency 의 dict)
type tradeFrequency struct {
	avgMonthly float64
	maxMonth   string
	maxCount   int
	minMonth   string
	minCount   int
}

// calcTradeFrequency 는 월별 거래빈도 통계를 계산한다.
// (Python _calc_trade_frequency, py:666-686)
// max/min 동률 시 Python max()/min(key=...) 은 최초 등장(여기서는 연월 사전식 최소) 항목 선택.
func calcTradeFrequency(sortedSells []model.Trade) tradeFrequency {
	monthlyCounts := map[string]int{}
	for _, t := range sortedSells {
		monthlyCounts[monthOf(t.Date)]++
	}
	if len(monthlyCounts) == 0 {
		return tradeFrequency{avgMonthly: 0, maxMonth: "-", maxCount: 0, minMonth: "-", minCount: 0}
	}

	months := make([]string, 0, len(monthlyCounts))
	total := 0
	for m, c := range monthlyCounts {
		months = append(months, m)
		total += c
	}
	sort.Strings(months)
	avg := float64(total) / float64(len(monthlyCounts))

	maxMonth, minMonth := months[0], months[0]
	maxCount, minCount := monthlyCounts[months[0]], monthlyCounts[months[0]]
	for _, m := range months[1:] {
		c := monthlyCounts[m]
		if c > maxCount {
			maxCount = c
			maxMonth = m
		}
		if c < minCount {
			minCount = c
			minMonth = m
		}
	}
	return tradeFrequency{
		avgMonthly: avg,
		maxMonth:   maxMonth, maxCount: maxCount,
		minMonth: minMonth, minCount: minCount,
	}
}

// getSectorMap 은 매수 종목 리스트로부터 섹터 매핑을 조회한다.
// 분류 실패 시 로그 후 빈 맵을 반환한다(Python _get_sector_map, py:1108-1118).
func (g *Generator) getSectorMap(ctx context.Context, buyTrades []model.Trade) (map[string]string, error) {
	// (name, code, currency) 중복 제거.
	seen := map[[3]string]bool{}
	var input []SectorStock
	for _, t := range buyTrades {
		k := [3]string{t.StockName, t.StockCode, t.Currency}
		if seen[k] {
			continue
		}
		seen[k] = true
		input = append(input, SectorStock{Name: t.StockName, Code: t.StockCode, Currency: t.Currency})
	}
	result, err := g.sc.Classify(ctx, input)
	if err != nil {
		slog.Error("섹터 분류 실패, 건너뜀", "error", err)
		return map[string]string{}, nil
	}
	return result, nil
}

// collectMetricsFormats 는 투자 지표/매매 인사이트 섹션의 행별 포맷 요청을 수집한다.
// (Python _collect_metrics_formats, py:779-802)
func (g *Generator) collectMetricsFormats(startRow int, pctOffsets, krwOffsets []int, totalRows int) {
	sid := g.dashboardSheetID
	build := sheets.BuildNumberFormatRequests

	// 1단계: 섹션 전체 B열을 기본 NUMBER 포맷으로 초기화.
	defaultFmt := []sheets.ColumnFormat{{Col: 2, Pattern: "#,##0.##"}}
	g.pendingRequests = append(g.pendingRequests,
		build(sid, defaultFmt, startRow, startRow+totalRows-1)...)

	// 2단계: pct/krw 개별 포맷 적용.
	pctFmt := []sheets.ColumnFormat{{Col: 2, Pattern: "0.00%", Type: "PERCENT"}}
	krwFmt := []sheets.ColumnFormat{{Col: 2, Pattern: "₩#,##0"}}

	for _, pair := range []struct {
		offsets []int
		fmt     []sheets.ColumnFormat
	}{
		{pctOffsets, pctFmt},
		{krwOffsets, krwFmt},
	} {
		if len(pair.offsets) == 0 {
			continue
		}
		absRows := make([]int, 0, len(pair.offsets))
		for _, o := range pair.offsets {
			absRows = append(absRows, startRow+o)
		}
		sort.Ints(absRows)
		for _, gr := range groupConsecutiveRows(absRows) {
			g.pendingRequests = append(g.pendingRequests, build(sid, pair.fmt, gr[0], gr[1])...)
		}
	}
}

// groupConsecutiveRows 는 정렬된 행 번호를 연속 구간 [start,end] 으로 묶는다.
// (Python _group_consecutive_rows, py:804-818)
func groupConsecutiveRows(rows []int) [][2]int {
	if len(rows) == 0 {
		return nil
	}
	var groups [][2]int
	start, end := rows[0], rows[0]
	for _, r := range rows[1:] {
		if r == end+1 {
			end = r
		} else {
			groups = append(groups, [2]int{start, end})
			start, end = r, r
		}
	}
	groups = append(groups, [2]int{start, end})
	return groups
}

// ── 소형 유틸 ──────────────────────────────────────────────

// monthOf 는 "YYYY-MM-DD" 에서 "YYYY-MM" 을 반환한다(7자 미만이면 원본).
func monthOf(date string) string {
	if len(date) >= 7 {
		return date[:7]
	}
	return date
}

// weekdayMondayZero 는 "YYYY-MM-DD" 의 요일을 월=0..일=6 으로 반환한다(파싱 실패 시 -1).
// Python datetime.weekday() 와 동일한 매핑.
func weekdayMondayZero(date string) int {
	tm, err := time.Parse("2006-01-02", date)
	if err != nil {
		return -1
	}
	// time.Weekday: 일=0..토=6. 월=0..일=6 으로 변환.
	return (int(tm.Weekday()) + 6) % 7
}

// sortSellsByDate 는 매도 거래를 date 오름차순으로 안정 정렬한 새 슬라이스를 반환한다.
// (Python sorted(sell_trades, key=lambda t: t.date))
func sortSellsByDate(sells []model.Trade) []model.Trade {
	out := make([]model.Trade, len(sells))
	copy(out, sells)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out
}

// sortedKeys 는 map[string]float64 의 키를 사전식 정렬해 반환한다.
func sortedKeys(m map[string]float64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// round2 는 소수점 둘째 자리 반올림(Python round(x, 2)).
func round2(x float64) float64 { return math.Round(x*100) / 100 }

// round1 은 소수점 첫째 자리 반올림(Python round(x, 1)).
func round1(x float64) float64 { return math.Round(x*10) / 10 }

// formatThousands 는 ₩ 라벨용 천단위 콤마 정수 포맷("%,.0f").
func formatThousands(x float64) string {
	neg := x < 0
	n := int64(math.Round(math.Abs(x)))
	s := fmt.Sprintf("%d", n)
	// 천단위 콤마 삽입.
	var b []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			b = append(b, ',')
		}
		b = append(b, c)
	}
	if neg {
		return "-" + string(b)
	}
	return string(b)
}
