package summary

import (
	"strings"

	"github.com/kenshin579/auto-trading-journal/internal/etfclass"
	"github.com/kenshin579/auto-trading-journal/internal/model"
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

// aggregateIndexWeight 는 거래를 지수/나머지 버킷별로 집계한다.
//
// 보유 원금 = max(0, 매수수량-매도수량) × (총매수금액/총매수수량).
// 버킷은 종목 단위로 한 번 정하므로 같은 종목의 거래는 모두 같은 칸에 들어간다.
// 비중 분모에는 미분류도 포함된다(지수+나머지+미분류 = 100%).
func aggregateIndexWeight(trades []model.Trade) []indexWeightRow {
	type key struct{ code, name, account, currency string }
	stocks := map[key]*stockAgg{}
	for _, t := range trades {
		if t.TradeType != "매수" && t.TradeType != "매도" {
			continue
		}
		k := key{t.StockCode, t.StockName, t.Account, t.Currency}
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
		return nil
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

	for _, a := range stocks {
		group, bucket := bucketOf(a.sector, a.industry)
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
	return rows
}
