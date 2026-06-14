package summary

import (
	"context"
	"fmt"
	"log/slog"
	"sort"

	gsheets "google.golang.org/api/sheets/v4"

	"github.com/kenshin579/auto-trading-journal/internal/model"
	"github.com/kenshin579/auto-trading-journal/internal/sheets"
)

// collectHeaderColors 는 대시보드 헤더 행 배경색 요청을 pendingRequests 에 수집한다.
// (Python _collect_header_colors, py:688-707)
func (g *Generator) collectHeaderColors(monthlyStart, trendStart, stockStart int) {
	headerColor := &gsheets.Color{
		Red:             0.24,
		Green:           0.52,
		Blue:            0.78,
		ForceSendFields: []string{"Red", "Green", "Blue"},
	}

	type hr struct {
		row, endCol int
	}
	headerRows := []hr{
		{row: 1, endCol: 7},            // 포트폴리오 요약 (A~G)
		{row: monthlyStart, endCol: 8}, // 월별 성과 (A~H)
		{row: trendStart, endCol: 11},  // 월별 성과 추이 (A~K)
		{row: stockStart, endCol: 11},  // 종목별 현황 (A~K)
	}

	colorRanges := make([]sheets.ColorRange, 0, len(headerRows))
	for _, h := range headerRows {
		colorRanges = append(colorRanges, sheets.ColorRange{
			StartRow: h.row, EndRow: h.row,
			StartCol: 1, EndCol: h.endCol,
			Color: headerColor,
		})
	}
	g.pendingRequests = append(g.pendingRequests,
		sheets.BuildColorRequests(g.dashboardSheetID, colorRanges)...)
}

// collectDashboardFormats 는 대시보드 숫자 포맷 요청을 pendingRequests 에 수집한다.
// (Python _collect_dashboard_formats, py:708-777)
func (g *Generator) collectDashboardFormats(monthlyStart, metricsStart, insightsStart,
	trendStart, trendEnd, stockStart, totalRows int) {
	_ = insightsStart // 섹션 4/5 는 collectMetricsFormats 에서 행별 처리.
	sid := g.dashboardSheetID
	build := sheets.BuildNumberFormatRequests

	// 섹션 1: 포트폴리오 요약 (행 2).
	portfolioFormats := []sheets.ColumnFormat{
		{Col: 2, Pattern: "₩#,##0"},                 // B: 총 매수금액
		{Col: 3, Pattern: "₩#,##0"},                 // C: 총 매도금액
		{Col: 4, Pattern: "₩#,##0"},                 // D: 총 실현손익
		{Col: 5, Pattern: "0.00%", Type: "PERCENT"}, // E: 총 수익률
		{Col: 6, Pattern: "#,##0"},                  // F: 총 거래건수
		{Col: 7, Pattern: "0.00%", Type: "PERCENT"}, // G: 승률
	}
	g.pendingRequests = append(g.pendingRequests, build(sid, portfolioFormats, 2, 2)...)

	// 섹션 2: 월별 성과 (monthly_start+1 ~ metrics_start-2).
	monthlyDataEnd := metricsStart - 2
	if monthlyDataEnd > monthlyStart {
		monthlyFormats := []sheets.ColumnFormat{
			{Col: 3, Pattern: "#,##0"},                  // C: 매수건수
			{Col: 4, Pattern: "₩#,##0"},                 // D: 매수금액
			{Col: 5, Pattern: "#,##0"},                  // E: 매도건수
			{Col: 6, Pattern: "₩#,##0"},                 // F: 매도금액
			{Col: 7, Pattern: "₩#,##0"},                 // G: 실현손익
			{Col: 8, Pattern: "0.00%", Type: "PERCENT"}, // H: 수익률
		}
		g.pendingRequests = append(g.pendingRequests,
			build(sid, monthlyFormats, monthlyStart+1, monthlyDataEnd)...)
	}

	// 섹션 6: 월별 성과 추이 (trend_start+1 ~ trend_end-1).
	// (나라별 섹터 섹션이 추이와 종목별 사이에 끼므로 stockStart 로 추정하지 않고 trendEnd 사용.)
	trendDataEnd := trendEnd - 1
	if trendDataEnd > trendStart {
		trendFormats := []sheets.ColumnFormat{
			{Col: 2, Pattern: "#,##0"},                  // B: 매도건수
			{Col: 3, Pattern: "₩#,##0"},                 // C: 실현손익
			{Col: 4, Pattern: "0.00%", Type: "PERCENT"}, // D: 수익률
			{Col: 5, Pattern: "0.00%", Type: "PERCENT"}, // E: 승률
			{Col: 6, Pattern: "0.00%", Type: "PERCENT"}, // F: 평균수익률
			{Col: 7, Pattern: "0.00%", Type: "PERCENT"}, // G: 평균손실률
			{Col: 8, Pattern: "0.00"},                   // H: 손익비
			{Col: 9, Pattern: "0.00"},                   // I: Profit Factor
			{Col: 10, Pattern: "₩#,##0"},                // J: 기대값
			{Col: 11, Pattern: "0.0%", Type: "PERCENT"}, // K: 전월대비
		}
		g.pendingRequests = append(g.pendingRequests,
			build(sid, trendFormats, trendStart+1, trendDataEnd)...)
	}

	// 섹션 3: 종목별 현황 (stock_start+1 ~ total_rows).
	stockDataEnd := totalRows
	if stockDataEnd > stockStart {
		stockFormats := []sheets.ColumnFormat{
			{Col: 5, Pattern: "#,##0"},                   // E: 총매수수량
			{Col: 6, Pattern: "₩#,##0"},                  // F: 총매수금액
			{Col: 7, Pattern: "#,##0"},                   // G: 총매도수량
			{Col: 8, Pattern: "₩#,##0"},                  // H: 총매도금액
			{Col: 9, Pattern: "₩#,##0"},                  // I: 실현손익
			{Col: 10, Pattern: "0.00%", Type: "PERCENT"}, // J: 수익률
			{Col: 11, Pattern: "0.00%", Type: "PERCENT"}, // K: 투자비중
		}
		g.pendingRequests = append(g.pendingRequests,
			build(sid, stockFormats, stockStart+1, stockDataEnd)...)
	}
}

// flushPendingRequests 는 수집된 포맷/색상 요청을 1회 batchUpdate 로 전송 후 비운다.
// (#57 배칭 보존) (Python _flush_pending_requests, py:820-825)
func (g *Generator) flushPendingRequests(ctx context.Context) error {
	if len(g.pendingRequests) == 0 {
		return nil
	}
	if err := g.client.ExecuteBatchRequests(ctx, g.pendingRequests); err != nil {
		return err
	}
	slog.Info("대시보드 포맷/색상 일괄 적용 완료", "requests", len(g.pendingRequests))
	g.pendingRequests = nil
	return nil
}

// writeTradeCountData 는 월별 매수/매도 건수·금액 차트용 데이터를 Q~U열에 작성한다.
// (Python _write_trade_count_data, py:827-870)
func (g *Generator) writeTradeCountData(ctx context.Context, trades []model.Trade) error {
	type stat struct {
		buyCount, sellCount   int
		buyAmount, sellAmount float64
	}
	monthStats := map[string]*stat{}
	for _, t := range trades {
		month := monthOf(t.Date)
		s := monthStats[month]
		if s == nil {
			s = &stat{}
			monthStats[month] = s
		}
		switch t.TradeType {
		case "매수":
			s.buyCount++
			s.buyAmount += t.AmountKRW
		case "매도":
			s.sellCount++
			s.sellAmount += t.AmountKRW
		}
	}

	if len(monthStats) == 0 {
		g.tradeCountDataRange = rowRange{}
		return nil
	}

	months := make([]string, 0, len(monthStats))
	for m := range monthStats {
		months = append(months, m)
	}
	sort.Strings(months)

	rows := [][]any{{"연월", "매수건수", "매도건수", "매수금액(원)", "매도금액(원)"}}
	for _, m := range months {
		s := monthStats[m]
		rows = append(rows, []any{m, s.buyCount, s.sellCount, s.buyAmount, s.sellAmount})
	}

	startRow := 1
	endRow := startRow + len(rows) - 1
	rng := fmt.Sprintf("%s!Q%d:U%d", DashboardSheet, startRow, endRow)
	if err := g.client.UpdateCells(ctx, rng, rows); err != nil {
		return err
	}
	g.tradeCountDataRange = rowRange{start: startRow, end: endRow, ok: true}

	// 금액 컬럼(T, U)에 숫자 포맷 적용 (차트 Y축 과학적 표기법 방지).
	amountFormats := []sheets.ColumnFormat{
		{Col: 20, Pattern: "₩#,##0"}, // T열: 매수금액
		{Col: 21, Pattern: "₩#,##0"}, // U열: 매도금액
	}
	g.pendingRequests = append(g.pendingRequests,
		sheets.BuildNumberFormatRequests(g.dashboardSheetID, amountFormats, startRow+1, endRow)...)

	slog.Info("월별 매수/매도 차트 데이터 작성", "months", len(rows)-1)
	return nil
}
