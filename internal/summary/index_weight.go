package summary

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	gsheets "google.golang.org/api/sheets/v4"

	"github.com/kenshin579/auto-trading-journal/internal/etfclass"
	"github.com/kenshin579/auto-trading-journal/internal/model"
	"github.com/kenshin579/auto-trading-journal/internal/sheets"
)

// 그룹(상위 묶음). 미분류는 지수/나머지 어디에도 속하지 않는 독립 그룹이다.
const (
	groupIndex   = "지수"
	groupOther   = "나머지"
	groupUnknown = "미분류"
)

// 버킷(표시 줄 라벨).
const (
	bucketSP500      = "S&P500"
	bucketNasdaq     = "나스닥"
	bucketKorea      = "한국(코스피·코스닥)"
	bucketOtherIndex = "기타 지역·전세계"
	bucketStock      = "개별종목"
	bucketTheme      = "테마·섹터 ETF"
	bucketDividend   = "배당·전략 ETF"
	bucketBondGold   = "채권·금·현금성 ETF"
	bucketUnknown    = "미분류" // 집계 키로만 쓰인다 — 표에는 미분류 그룹 행 하나로만 나온다
)

// bucketAssignment 는 ETF 카테고리가 표에서 어느 그룹·버킷 줄로 가는지.
type bucketAssignment struct{ group, bucket string }

// etfBuckets 는 ETF 카테고리(etfclass.Categories) → (그룹, 버킷) 매핑.
// etfclass 에 카테고리를 추가하면 여기에도 넣어야 한다(TestETFBuckets_CoversTaxonomy 가 강제).
var etfBuckets = map[string]bucketAssignment{
	"S&P500":   {groupIndex, bucketSP500},
	"나스닥":      {groupIndex, bucketNasdaq},
	"한국주식":     {groupIndex, bucketKorea},
	"미국주식(기타)": {groupIndex, bucketOtherIndex},
	"중국주식":     {groupIndex, bucketOtherIndex},
	"일본주식":     {groupIndex, bucketOtherIndex},
	"인도주식":     {groupIndex, bucketOtherIndex},
	"베트남주식":    {groupIndex, bucketOtherIndex},
	"글로벌주식":    {groupIndex, bucketOtherIndex},
	"반도체":      {groupOther, bucketTheme},
	"2차전지":     {groupOther, bucketTheme},
	"바이오·헬스케어": {groupOther, bucketTheme},
	"AI·로봇":    {groupOther, bucketTheme},
	"신재생에너지":   {groupOther, bucketTheme},
	"원자력":      {groupOther, bucketTheme},
	"방위·우주항공":  {groupOther, bucketTheme},
	"자동차":      {groupOther, bucketTheme},
	"금융":       {groupOther, bucketTheme},
	"건설":       {groupOther, bucketTheme},
	"필수소비재":    {groupOther, bucketTheme},
	"IT·인터넷":   {groupOther, bucketTheme},
	"리츠·부동산":   {groupOther, bucketTheme},
	"기타테마":     {groupOther, bucketTheme},
	"배당":       {groupOther, bucketDividend},
	"팩터·스타일":   {groupOther, bucketDividend},
	"채권":       {groupOther, bucketBondGold},
	"원자재":      {groupOther, bucketBondGold},
	"통화·단기금리":  {groupOther, bucketBondGold},
}

// bucketOf 는 거래의 (섹터, 산업)으로 표시 그룹/버킷을 정한다.
//   - 섹터가 비면 미분류(FMP 미커버/미지원 통화/키 없음).
//   - 섹터가 ETF 가 아니면 개별종목.
//   - ETF 인데 산업이 taxonomy 밖이면(분류기 없을 때의 KIS 지수명·FMP 산업 폴백 등) 미분류 —
//     임의로 테마에 넣지 않는다. 지수인지 아닌지 모르는 것을 아는 척하면 배분 판단이 틀어진다.
//
// 입력은 시트를 왕복해 온 값이라(ReadAllTrades) 캐시와 달리 사용자가 셀을 손으로 고쳐
// 공백이 붙을 수 있다 — 맵 조회 전에 TrimSpace 한다.
func bucketOf(sector, industry string) (group, bucket string) {
	sector = strings.TrimSpace(sector)
	industry = strings.TrimSpace(industry)
	if sector == "" {
		return groupUnknown, bucketUnknown
	}
	if sector != etfclass.SectorETF {
		return groupOther, bucketStock
	}
	if gb, ok := etfBuckets[industry]; ok {
		return gb.group, gb.bucket
	}
	return groupUnknown, bucketUnknown
}

// indexWeightRow 는 표의 한 줄. bucket 이 빈 줄은 그룹 소계 행이다.
type indexWeightRow struct {
	group   string
	bucket  string
	buy     float64 // 누적 매수금액(원)
	held    float64 // 보유 원금(원) = 잔여수량 × 평균매수단가
	buyPct  float64
	heldPct float64
}

// indexWeightLayout 은 표시 순서(고정). 거래가 없는 버킷도 0 으로 표시해 표 모양을 유지한다.
var indexWeightLayout = []bucketAssignment{
	{groupIndex, ""},
	{groupIndex, bucketSP500},
	{groupIndex, bucketNasdaq},
	{groupIndex, bucketKorea},
	{groupIndex, bucketOtherIndex},
	{groupOther, ""},
	{groupOther, bucketStock},
	{groupOther, bucketTheme},
	{groupOther, bucketDividend},
	{groupOther, bucketBondGold},
	{groupUnknown, ""},
}

// stockAgg 는 종목 단위(코드·이름·계좌·통화) 집계. 보유 원금 계산용.
type stockAgg struct {
	buyQty, buyAmount, sellQty float64
	sector, industry           string
}

// stockAmount 는 진단 로그용 (종목명, 금액) 쌍.
type stockAmount struct {
	name   string
	amount float64
}

// indexWeightDiag 는 표에 나오지 않지만 사용자가 알아야 하는 신호.
type indexWeightDiag struct {
	unclassified []stockAmount // 미분류로 빠진 종목(누적매수 내림차순)
	oversold     []stockAmount // 매도수량 > 매수수량 — 기간 밖 매수분 누락 의심(누적매수 내림차순)
}

// diagTopN 은 로그에 실을 상위 N 종목명. 전체를 찍으면 로그가 종목 수만큼 길어진다.
const diagTopN = 5

// logIndexWeightDiag 는 표에 안 나오는 신호를 경고로 남긴다.
func logIndexWeightDiag(d indexWeightDiag) {
	if len(d.unclassified) > 0 {
		slog.Warn("지수 분류 미분류 종목 — OpenAI/FMP 키 또는 taxonomy 밖 카테고리 확인 필요",
			"종목수", len(d.unclassified), "상위", diagNames(d.unclassified))
	}
	if len(d.oversold) > 0 {
		slog.Warn("매도수량 > 매수수량 — 기간 밖 매수분 누락으로 보임. 해당 종목의 보유원금이 0 으로 잡혀 "+
			"실제보다 적게 표시된다. 더 긴 기간의 거래내역 CSV 를 input/ 에 넣고 재실행할 것",
			"종목수", len(d.oversold), "상위", diagNames(d.oversold))
	}
}

// diagNames 는 로그에 실을 상위 N 종목의 "이름(₩금액)" 목록.
// 이름만 찍으면 "미분류 500만 원 중 첫 종목이 90% 인지 10% 인지"를 알 수 없다.
func diagNames(list []stockAmount) []string {
	n := len(list)
	if n > diagTopN {
		n = diagTopN
	}
	names := make([]string, 0, n)
	for _, s := range list[:n] {
		names = append(names, fmt.Sprintf("%s(₩%s)", s.name, formatThousands(s.amount)))
	}
	return names
}

// aggregateIndexWeight 는 거래를 지수/나머지 버킷별로 집계한다.
//
// 보유 원금 = max(0, 매수수량-매도수량) × (총매수금액/총매수수량).
// 버킷은 종목 단위로 한 번 정하므로 같은 종목의 거래는 모두 같은 칸에 들어간다.
// 비중 분모에는 미분류도 포함된다(지수+나머지+미분류 = 100%).
//
// 두 번째 반환값은 표에 나오지 않는 진단 신호(미분류·과매도 종목)다.
func aggregateIndexWeight(trades []model.Trade) ([]indexWeightRow, indexWeightDiag) {
	var diag indexWeightDiag
	stocks := map[stockKey]*stockAgg{}
	for _, t := range trades {
		if t.TradeType != "매수" && t.TradeType != "매도" {
			continue
		}
		k := stockKeyOf(t)
		a := stocks[k]
		if a == nil {
			a = &stockAgg{}
			stocks[k] = a
		}
		// 섹터/산업은 처음 만나는 비어 있지 않은 값을 쓴다.
		// 공백-only 는 빈 값으로 본다 — bucketOf 의 판정과 같은 기준이어야
		// 손편집된 시트에서 진짜 값이 공백에 선점되지 않는다.
		if a.sector == "" && strings.TrimSpace(t.Sector) != "" {
			a.sector, a.industry = t.Sector, t.Industry
		}
		if t.TradeType == "매수" {
			a.buyQty += t.Quantity
			a.buyAmount += t.AmountKRW
		} else {
			a.sellQty += t.Quantity
		}
	}
	if len(stocks) == 0 {
		return nil, diag
	}

	type sums struct{ buy, held float64 }
	byBucket := map[string]*sums{} // "group|bucket"
	byGroup := map[string]*sums{}
	var totalBuy, totalHeld float64

	add := func(m map[string]*sums, k string, buy, held float64) {
		s := m[k]
		if s == nil {
			s = &sums{}
			m[k] = s
		}
		s.buy += buy
		s.held += held
	}

	for k, a := range stocks {
		group, bucket := bucketOf(a.sector, a.industry)
		if group == groupUnknown {
			diag.unclassified = append(diag.unclassified, stockAmount{k.name, a.buyAmount})
		}
		if a.sellQty > a.buyQty {
			diag.oversold = append(diag.oversold, stockAmount{k.name, a.buyAmount})
		}
		held := 0.0
		if a.buyQty > 0 {
			remain := a.buyQty - a.sellQty
			if remain > 0 {
				held = remain * (a.buyAmount / a.buyQty)
			}
		}
		add(byBucket, group+"|"+bucket, a.buyAmount, held)
		add(byGroup, group, a.buyAmount, held)
		totalBuy += a.buyAmount
		totalHeld += held
	}

	// stocks 맵 순회는 비결정적이라 정렬이 없으면 로그 순서가 실행마다 흔들린다.
	sortByAmountDesc := func(list []stockAmount) {
		sort.Slice(list, func(i, j int) bool {
			if list[i].amount != list[j].amount {
				return list[i].amount > list[j].amount
			}
			return list[i].name < list[j].name
		})
	}
	sortByAmountDesc(diag.unclassified)
	sortByAmountDesc(diag.oversold)

	pct := func(v, total float64) float64 {
		if total == 0 {
			return 0
		}
		return v / total
	}

	rows := make([]indexWeightRow, 0, len(indexWeightLayout))
	for _, l := range indexWeightLayout {
		var s *sums
		if l.bucket == "" {
			s = byGroup[l.group]
		} else {
			s = byBucket[l.group+"|"+l.bucket]
		}
		if s == nil {
			s = &sums{}
		}
		// 미분류는 금액이 없으면 표시하지 않는다(정상 상태에서 노이즈 방지).
		if l.group == groupUnknown && s.buy == 0 && s.held == 0 {
			continue
		}
		rows = append(rows, indexWeightRow{
			group: l.group, bucket: l.bucket,
			buy: s.buy, held: s.held,
			buyPct: pct(s.buy, totalBuy), heldPct: pct(s.held, totalHeld),
		})
	}
	return rows, diag
}

// indexWeightValues 는 시트에 쓸 A:E 셀 값과 그룹 행 오프셋(0-based)을 만든다.
// diag 는 미분류 행 라벨에 종목수를 싣는 데만 쓴다 — 로그는 휘발성이라 시트에서
// 미분류를 본 시점엔 이미 사라졌을 수 있어, 시트만으로 규모를 알 수 있어야 한다.
func indexWeightValues(rows []indexWeightRow, diag indexWeightDiag) ([][]any, []int) {
	values := [][]any{
		{"[지수 vs 나머지 투자]", "", "", "", ""},
		{"구분", "누적매수금액", "비중(%)", "보유원금", "비중(%)"},
	}
	var groupOffsets []int
	for _, r := range rows {
		label := "  " + r.bucket
		if r.bucket == "" {
			label = "▸ " + r.group
			if r.group == groupUnknown && len(diag.unclassified) > 0 {
				label = fmt.Sprintf("▸ %s (%d종목)", r.group, len(diag.unclassified))
			}
			groupOffsets = append(groupOffsets, len(values))
		}
		values = append(values, []any{label, r.buy, r.buyPct, r.held, r.heldPct})
	}
	return values, groupOffsets
}

// indexWeightPieHelper 는 파이 차트용 (그룹, 보유원금) 데이터를 만든다(첫 행은 헤더).
func indexWeightPieHelper(rows []indexWeightRow) [][]any {
	helper := [][]any{{"[차트데이터] 지수 vs 나머지", "보유원금"}}
	for _, r := range rows {
		if r.bucket == "" {
			helper = append(helper, []any{r.group, r.held})
		}
	}
	return helper
}

// writeIndexWeight 는 "지수 vs 나머지 투자" 섹션을 작성한다.
// 표는 A:E, 파이 차트용 헬퍼 데이터는 Y:Z 에 쓴다
// (N:O=계좌별 파이, W:X=나라별 섹터 파이가 이미 쓰고 있고, 초기화 범위가 A1:Z 라 AA 이후는 안 지워진다).
func (g *Generator) writeIndexWeight(ctx context.Context, trades []model.Trade, startRow int) (int, error) {
	rows, diag := aggregateIndexWeight(trades)
	values, groupOffsets := indexWeightValues(rows, diag)

	// 쓰기보다 먼저 비운다 — 중간에 실패하고 돌아가면 이전 실행의 범위가 남아
	// 엉뚱한 행을 가리키는 차트가 만들어질 수 있다.
	g.indexWeightPie = rowRange{}

	endRow := startRow + len(values) - 1
	rng := fmt.Sprintf("%s!A%d:E%d", DashboardSheet, startRow, endRow)
	if err := g.client.UpdateCells(ctx, rng, values); err != nil {
		return 0, err
	}

	// 그룹 소계 보유원금의 합. 전량 매도한 포트폴리오는 행이 있어도 값이 전부 0 이라
	// 파이가 빈 원으로 그려진다 — 그때는 차트를 만들지 않는다.
	var heldTotal float64
	for _, r := range rows {
		if r.bucket == "" {
			heldTotal += r.held
		}
	}

	if helper := indexWeightPieHelper(rows); len(helper) > 1 && heldTotal > 0 {
		hEnd := startRow + len(helper) - 1
		// 열을 옮기면 charts.go 의 indexWeightLabelCol/ValueCol 도 함께 바꿀 것.
		hRng := fmt.Sprintf("%s!Y%d:Z%d", DashboardSheet, startRow, hEnd)
		if err := g.client.UpdateCells(ctx, hRng, helper); err != nil {
			return 0, err
		}
		g.indexWeightPie = rowRange{start: startRow + 1, end: hEnd, ok: true}
		// Z열(26) 보유원금 통화 포맷 — 차트 축의 과학적 표기 방지.
		g.pendingRequests = append(g.pendingRequests, sheets.BuildNumberFormatRequests(
			g.dashboardSheetID, []sheets.ColumnFormat{{Col: 26, Pattern: "₩#,##0"}}, startRow+1, hEnd)...)
	}

	g.collectIndexWeightFormats(startRow, endRow, groupOffsets)
	logIndexWeightDiag(diag)
	slog.Info("대시보드 지수 vs 나머지 작성", "rows", len(rows))
	return startRow + len(values), nil
}

// collectIndexWeightFormats 는 표의 숫자 포맷(B·D=원화, C·E=백분율)과 헤더/그룹 배경색을 수집한다.
func (g *Generator) collectIndexWeightFormats(startRow, endRow int, groupOffsets []int) {
	sid := g.dashboardSheetID
	build := sheets.BuildNumberFormatRequests
	krw := []sheets.ColumnFormat{{Col: 2, Pattern: "₩#,##0"}, {Col: 4, Pattern: "₩#,##0"}}
	pct := []sheets.ColumnFormat{
		{Col: 3, Pattern: "0.00%", Type: "PERCENT"},
		{Col: 5, Pattern: "0.00%", Type: "PERCENT"},
	}
	// 데이터는 startRow+2 부터(startRow=제목행, startRow+1=컬럼 헤더).
	// 거래가 없으면 데이터 행 자체가 없어 포맷을 걸 곳이 없다.
	if endRow >= startRow+2 {
		g.pendingRequests = append(g.pendingRequests, build(sid, krw, startRow+2, endRow)...)
		g.pendingRequests = append(g.pendingRequests, build(sid, pct, startRow+2, endRow)...)
	}

	headerColor := &gsheets.Color{Red: 0.24, Green: 0.52, Blue: 0.78, ForceSendFields: []string{"Red", "Green", "Blue"}}
	groupColor := &gsheets.Color{Red: 0.85, Green: 0.92, Blue: 0.98, ForceSendFields: []string{"Red", "Green", "Blue"}}
	colorRanges := []sheets.ColorRange{
		{StartRow: startRow, EndRow: startRow, StartCol: 1, EndCol: 5, Color: headerColor},
	}
	for _, off := range groupOffsets {
		r := startRow + off
		colorRanges = append(colorRanges, sheets.ColorRange{StartRow: r, EndRow: r, StartCol: 1, EndCol: 5, Color: groupColor})
	}
	g.pendingRequests = append(g.pendingRequests, sheets.BuildColorRequests(sid, colorRanges)...)
}
