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

// countrySectorRow 은 한 국가 안 한 섹터의 집계(원화 기준).
type countrySectorRow struct {
	Sector string
	Buy    float64 // 매수금액 합
	Sell   float64 // 매도금액 합
	Net    float64 // 순매수 = Buy - Sell
	Weight float64 // 비중 = Buy / 국가 총매수 (국가 안에서 100%)
}

// countrySectorGroup 은 한 국가의 섹터 집계 묶음.
type countrySectorGroup struct {
	Country  string // "국내"/"미국"/"일본"/통화코드
	TotalBuy float64
	Rows     []countrySectorRow // 매수금액 내림차순
}

// sectorKey 는 집계 라벨을 정한다. ETF 는 한 덩어리로 뭉치지 않도록 산업(OpenAI category)으로
// "ETF·{category}" 세분화한다(예 "ETF·반도체"/"ETF·미국주식"). 산업이 빈 미분류 ETF 는 "ETF".
// 일반 종목은 섹터(KIS 업종 / FMP 영문) 그대로.
func sectorKey(sector, industry string) string {
	if sector == "ETF" && industry != "" {
		return "ETF·" + industry
	}
	return sector
}

// countryLabel 은 통화 코드를 국가 라벨로 바꾼다. 미정의 통화는 코드 그대로.
func countryLabel(currency string) string {
	switch currency {
	case "KRW":
		return "국내"
	case "USD":
		return "미국"
	case "JPY":
		return "일본"
	default:
		return currency
	}
}

// countryOrder 는 국가 표시 순서(작을수록 먼저). 정의되지 않은 통화는 뒤로(사전식).
func countryOrder(currency string) int {
	switch currency {
	case "KRW":
		return 0
	case "USD":
		return 1
	case "JPY":
		return 2
	default:
		return 3
	}
}

// countrySectorGroups 는 거래를 국가(통화)별·섹터별로 집계한다.
//   - 섹터가 빈 거래는 제외, 섹터 행이 하나도 없는 국가는 제외.
//   - 국가 순서: 국내, 미국, 일본, 그 외(통화 사전식).
//   - 각 국가 내 섹터는 매수금액 내림차순(동률 시 섹터명 사전식).
//   - 금액은 모두 원화(AmountKRW). 비중 = 섹터 매수금액 / 국가 총매수금액.
func countrySectorGroups(trades []model.Trade) []countrySectorGroup {
	type agg struct {
		buy, sell float64
	}
	// currency -> sectorKey -> agg
	byCur := map[string]map[string]*agg{}
	for _, t := range trades {
		if t.Sector == "" {
			continue
		}
		key := sectorKey(t.Sector, t.Industry)
		sectors, ok := byCur[t.Currency]
		if !ok {
			sectors = map[string]*agg{}
			byCur[t.Currency] = sectors
		}
		a := sectors[key]
		if a == nil {
			a = &agg{}
			sectors[key] = a
		}
		switch t.TradeType {
		case "매수":
			a.buy += t.AmountKRW
		case "매도":
			a.sell += t.AmountKRW
		}
	}

	currencies := make([]string, 0, len(byCur))
	for c := range byCur {
		currencies = append(currencies, c)
	}
	sort.Slice(currencies, func(i, j int) bool {
		oi, oj := countryOrder(currencies[i]), countryOrder(currencies[j])
		if oi != oj {
			return oi < oj
		}
		return currencies[i] < currencies[j]
	})

	groups := make([]countrySectorGroup, 0, len(currencies))
	for _, cur := range currencies {
		sectors := byCur[cur]
		var totalBuy float64
		rows := make([]countrySectorRow, 0, len(sectors))
		for sec, a := range sectors {
			totalBuy += a.buy
			rows = append(rows, countrySectorRow{Sector: sec, Buy: a.buy, Sell: a.sell, Net: a.buy - a.sell})
		}
		for i := range rows {
			if totalBuy != 0 {
				rows[i].Weight = rows[i].Buy / totalBuy
			}
		}
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].Buy != rows[j].Buy {
				return rows[i].Buy > rows[j].Buy
			}
			return rows[i].Sector < rows[j].Sector
		})
		groups = append(groups, countrySectorGroup{Country: countryLabel(cur), TotalBuy: totalBuy, Rows: rows})
	}
	return groups
}

// countrySectorPie 는 국가별 섹터 비중 pie 차트의 헬퍼 데이터(W:X) 범위.
type countrySectorPie struct {
	title string
	rng   rowRange
}

// writeCountrySectorWeight 는 "나라별 섹터 비중" 섹션을 작성한다.
// 국가별 블록(국가 헤더 + 컬럼 헤더 + 섹터 행)을 A:E 에 쓰고, pie 차트용
// (섹터, 매수금액) 헬퍼 데이터를 W:X 에 별도로 작성한다.
func (g *Generator) writeCountrySectorWeight(ctx context.Context, trades []model.Trade, startRow int) (int, error) {
	groups := countrySectorGroups(trades)

	rows := [][]any{{"[나라별 섹터 비중]", "", "", "", ""}}
	var pieHelper [][]any          // W:X 에 쓸 (섹터, 매수금액)
	var countryHeaderOffsets []int // 국가 헤더 행 0-based 오프셋(색상용)
	g.countrySectorPies = nil

	// 차트 소스 데이터(W:X)는 맨 위 1행이 설명 헤더라 데이터는 startRow+1 부터 시작.
	helperDataTop := startRow + 1
	for _, grp := range groups {
		countryHeaderOffsets = append(countryHeaderOffsets, len(rows))
		rows = append(rows, []any{"▸ " + grp.Country, "", "", "", ""})
		rows = append(rows, []any{"섹터", "매수금액", "매도금액", "순매수", "비중(%)"})
		helperStart := helperDataTop + len(pieHelper)
		for _, r := range grp.Rows {
			rows = append(rows, []any{"  " + r.Sector, r.Buy, r.Sell, r.Net, r.Weight})
			pieHelper = append(pieHelper, []any{r.Sector, r.Buy})
		}
		if len(grp.Rows) > 0 {
			g.countrySectorPies = append(g.countrySectorPies, countrySectorPie{
				title: grp.Country + " 섹터 비중",
				rng:   rowRange{start: helperStart, end: helperDataTop + len(pieHelper) - 1, ok: true},
			})
		}
	}

	endRow := startRow + len(rows) - 1
	rng := fmt.Sprintf("%s!A%d:E%d", DashboardSheet, startRow, endRow)
	if err := g.client.UpdateCells(ctx, rng, rows); err != nil {
		return 0, err
	}
	if len(pieHelper) > 0 {
		// 맨 위 설명 헤더 + (섹터, 매수금액). 매수금액(X열)은 통화 포맷.
		helperRows := append([][]any{{"[차트데이터] 나라별 섹터 비중", "매수금액"}}, pieHelper...)
		hEnd := startRow + len(helperRows) - 1
		hRng := fmt.Sprintf("%s!W%d:X%d", DashboardSheet, startRow, hEnd)
		if err := g.client.UpdateCells(ctx, hRng, helperRows); err != nil {
			return 0, err
		}
		g.pendingRequests = append(g.pendingRequests, sheets.BuildNumberFormatRequests(
			g.dashboardSheetID, []sheets.ColumnFormat{{Col: 24, Pattern: "₩#,##0"}}, helperDataTop, hEnd)...)
	}

	g.collectCountrySectorFormats(startRow, endRow, countryHeaderOffsets)
	slog.Info("대시보드 나라별 섹터 비중 작성", "rows", len(rows), "countries", len(groups))
	return startRow + len(rows), nil
}

// collectCountrySectorFormats 는 나라별 섹터 비중 섹션의 숫자 포맷/헤더 색상을 수집한다.
// 매수/매도/순매수(B·C·D)=원화, 비중(E)=백분율. 텍스트 셀은 숫자 포맷 영향 없음.
func (g *Generator) collectCountrySectorFormats(startRow, endRow int, countryHeaderOffsets []int) {
	sid := g.dashboardSheetID
	build := sheets.BuildNumberFormatRequests
	krw := []sheets.ColumnFormat{
		{Col: 2, Pattern: "₩#,##0"}, {Col: 3, Pattern: "₩#,##0"}, {Col: 4, Pattern: "₩#,##0"},
	}
	pct := []sheets.ColumnFormat{{Col: 5, Pattern: "0.00%", Type: "PERCENT"}}
	g.pendingRequests = append(g.pendingRequests, build(sid, krw, startRow, endRow)...)
	g.pendingRequests = append(g.pendingRequests, build(sid, pct, startRow, endRow)...)

	headerColor := &gsheets.Color{Red: 0.24, Green: 0.52, Blue: 0.78, ForceSendFields: []string{"Red", "Green", "Blue"}}
	countryColor := &gsheets.Color{Red: 0.85, Green: 0.92, Blue: 0.98, ForceSendFields: []string{"Red", "Green", "Blue"}}
	colorRanges := []sheets.ColorRange{
		{StartRow: startRow, EndRow: startRow, StartCol: 1, EndCol: 5, Color: headerColor},
	}
	for _, off := range countryHeaderOffsets {
		r := startRow + off
		colorRanges = append(colorRanges, sheets.ColorRange{StartRow: r, EndRow: r, StartCol: 1, EndCol: 5, Color: countryColor})
	}
	g.pendingRequests = append(g.pendingRequests, sheets.BuildColorRequests(sid, colorRanges)...)
}
