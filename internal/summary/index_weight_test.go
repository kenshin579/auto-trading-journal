package summary

import (
	"testing"

	"github.com/kenshin579/auto-trading-journal/internal/etfclass"
	"github.com/kenshin579/auto-trading-journal/internal/model"
	"github.com/stretchr/testify/assert"
)

// 이 파일의 테스트는 etfclass.SectorETF 상수가 아니라 리터럴 "ETF" 로 단언한다 —
// 상수를 참조하면 상수가 바뀌는 사고(캐시·시트에 이미 저장된 값과 어긋남)를 못 잡는다.
func TestBucketOf_Index(t *testing.T) {
	cases := map[string]string{
		"S&P500":   bucketSP500,
		"나스닥":      bucketNasdaq,
		"한국주식":     bucketKorea,
		"미국주식(기타)": bucketOtherIndex,
		"중국주식":     bucketOtherIndex,
		"일본주식":     bucketOtherIndex,
		"인도주식":     bucketOtherIndex,
		"베트남주식":    bucketOtherIndex,
		"글로벌주식":    bucketOtherIndex,
	}
	for industry, wantBucket := range cases {
		g, b := bucketOf("ETF", industry)
		assert.Equal(t, groupIndex, g, industry)
		assert.Equal(t, wantBucket, b, industry)
	}
}

func TestBucketOf_Other(t *testing.T) {
	theme := []string{"반도체", "2차전지", "바이오·헬스케어", "AI·로봇", "신재생에너지", "원자력",
		"방위·우주항공", "자동차", "금융", "건설", "필수소비재", "IT·인터넷", "리츠·부동산", "기타테마"}
	for _, industry := range theme {
		g, b := bucketOf("ETF", industry)
		assert.Equal(t, groupOther, g, industry)
		assert.Equal(t, bucketTheme, b, industry)
	}

	// 배당·전략 계열: 배당(고배당·배당성장·커버드콜)과 팩터·스타일(성장/가치/퀄리티)
	for _, industry := range []string{"배당", "팩터·스타일"} {
		g, b := bucketOf("ETF", industry)
		assert.Equal(t, groupOther, g, industry)
		assert.Equal(t, bucketDividend, b, industry)
	}

	for _, industry := range []string{"채권", "원자재", "통화·단기금리"} {
		g, b := bucketOf("ETF", industry)
		assert.Equal(t, groupOther, g, industry)
		assert.Equal(t, bucketBondGold, b, industry)
	}
}

// 일반 종목은 섹터가 무엇이든 개별종목.
func TestBucketOf_Stock(t *testing.T) {
	g, b := bucketOf("전기·전자", "반도체 제조업")
	assert.Equal(t, groupOther, g)
	assert.Equal(t, bucketStock, b)

	g, b = bucketOf("Financial Services", "Asset Management")
	assert.Equal(t, groupOther, g, "BDC·자산운용사도 개별종목")
	assert.Equal(t, bucketStock, b)
}

// 섹터가 비었거나 taxonomy 밖 ETF 산업은 미분류 — 개별종목으로 밀어넣지 않는다.
func TestBucketOf_Unknown(t *testing.T) {
	g, b := bucketOf("", "")
	assert.Equal(t, groupUnknown, g)
	assert.Equal(t, bucketUnknown, b)

	g, b = bucketOf("ETF", "S&P 500 Future Index TR") // 분류기 없을 때의 KIS 지수명 폴백
	assert.Equal(t, groupUnknown, g)
	assert.Equal(t, bucketUnknown, b)

	g, b = bucketOf("ETF", "Asset Management - Income") // 분류기 없을 때의 FMP 산업 폴백
	assert.Equal(t, groupUnknown, g)
	assert.Equal(t, bucketUnknown, b)

	g, b = bucketOf("ETF", "")
	assert.Equal(t, groupUnknown, g)
	assert.Equal(t, bucketUnknown, b)
}

// taxonomy 전체가 매핑 표에 있어야 한다(카테고리 추가 시 매핑 누락 방지).
func TestETFBuckets_CoversTaxonomy(t *testing.T) {
	for _, c := range etfclass.Categories {
		_, ok := etfBuckets[c]
		assert.True(t, ok, "매핑 누락: %s", c)
	}

	// 역방향 — taxonomy 에서 사라진 카테고리가 맵에 유령으로 남지 않도록.
	for k := range etfBuckets {
		assert.Contains(t, etfclass.Categories, k, "taxonomy 에 없는 키: %s", k)
	}
}

// 시트에서 읽은 값이라 사용자가 손으로 고쳐 공백이 붙을 수 있다.
func TestBucketOf_TrimsInput(t *testing.T) {
	g, b := bucketOf(" ETF ", " S&P500 ")
	assert.Equal(t, groupIndex, g)
	assert.Equal(t, bucketSP500, b)

	g, b = bucketOf("   ", "   ")
	assert.Equal(t, groupUnknown, g, "공백만 있는 입력은 trim 후 빈 값 처리")
	assert.Equal(t, bucketUnknown, b)
}

// 표는 그룹 소계 + 버킷 줄이 고정 순서로 나온다. 금액 0 인 버킷도 표시된다.
func TestAggregateIndexWeight_LayoutAndTotals(t *testing.T) {
	trades := []model.Trade{
		// S&P500 ETF: 100주 매수 100만, 50주 매도 → 보유 50주 × 평균 1만 = 50만
		{StockName: "SPYM", StockCode: "SPYM", Account: "해외", Currency: "USD",
			Sector: "ETF", Industry: "S&P500", TradeType: "매수", Quantity: 100, AmountKRW: 1_000_000},
		{StockName: "SPYM", StockCode: "SPYM", Account: "해외", Currency: "USD",
			Sector: "ETF", Industry: "S&P500", TradeType: "매도", Quantity: 50, AmountKRW: 600_000},
		// 개별종목: 전량 보유
		{StockName: "삼성전자", StockCode: "005930", Account: "국내", Currency: "KRW",
			Sector: "전기·전자", Industry: "반도체 제조업", TradeType: "매수", Quantity: 10, AmountKRW: 1_000_000},
	}
	rows := aggregateIndexWeight(trades)

	byLabel := map[string]indexWeightRow{}
	for _, r := range rows {
		key := r.bucket
		if key == "" {
			key = "▸" + r.group
		}
		byLabel[key] = r
	}

	assert.Equal(t, 1_000_000.0, byLabel[bucketSP500].buy)
	assert.Equal(t, 500_000.0, byLabel[bucketSP500].held, "잔여 50주 × 평균단가 1만")
	assert.Equal(t, 1_000_000.0, byLabel[bucketStock].buy)
	assert.Equal(t, 1_000_000.0, byLabel[bucketStock].held)

	// 그룹 소계
	assert.Equal(t, 1_000_000.0, byLabel["▸"+groupIndex].buy)
	assert.Equal(t, 500_000.0, byLabel["▸"+groupIndex].held)
	assert.Equal(t, 1_000_000.0, byLabel["▸"+groupOther].buy)

	// 비중: 누적은 50/50, 보유는 1/3 : 2/3
	assert.InDelta(t, 0.5, byLabel["▸"+groupIndex].buyPct, 1e-9)
	assert.InDelta(t, 1.0/3.0, byLabel["▸"+groupIndex].heldPct, 1e-9)
	assert.InDelta(t, 2.0/3.0, byLabel["▸"+groupOther].heldPct, 1e-9)

	// 거래 없는 버킷도 0 으로 표시된다
	assert.Contains(t, byLabel, bucketNasdaq)
	assert.Equal(t, 0.0, byLabel[bucketNasdaq].buy)

	// 미분류가 0 이면 표시하지 않는다
	assert.NotContains(t, byLabel, "▸"+groupUnknown)
}

// 전량 매도한 종목은 보유 원금 0, 누적 매수금액은 남는다.
func TestAggregateIndexWeight_FullySoldHasZeroHeld(t *testing.T) {
	trades := []model.Trade{
		{StockName: "QQQM", StockCode: "QQQM", Account: "해외", Currency: "USD",
			Sector: "ETF", Industry: "나스닥", TradeType: "매수", Quantity: 10, AmountKRW: 1_000_000},
		{StockName: "QQQM", StockCode: "QQQM", Account: "해외", Currency: "USD",
			Sector: "ETF", Industry: "나스닥", TradeType: "매도", Quantity: 10, AmountKRW: 1_200_000},
	}
	rows := aggregateIndexWeight(trades)
	for _, r := range rows {
		if r.bucket == bucketNasdaq {
			assert.Equal(t, 1_000_000.0, r.buy)
			assert.Equal(t, 0.0, r.held)
		}
	}
}

// 매도 수량이 매수를 넘어도 보유 원금은 음수가 되지 않는다.
func TestAggregateIndexWeight_OversoldClampedToZero(t *testing.T) {
	trades := []model.Trade{
		{StockName: "TIGER 코스피", StockCode: "277630", Account: "국내", Currency: "KRW",
			Sector: "ETF", Industry: "한국주식", TradeType: "매수", Quantity: 10, AmountKRW: 100_000},
		{StockName: "TIGER 코스피", StockCode: "277630", Account: "국내", Currency: "KRW",
			Sector: "ETF", Industry: "한국주식", TradeType: "매도", Quantity: 30, AmountKRW: 330_000},
	}
	rows := aggregateIndexWeight(trades)
	for _, r := range rows {
		assert.GreaterOrEqual(t, r.held, 0.0, r.bucket)
	}
}

// 섹터가 빈 거래는 미분류 줄로 모이고, 분모에도 포함된다.
func TestAggregateIndexWeight_UnknownShownWhenNonZero(t *testing.T) {
	trades := []model.Trade{
		{StockName: "AAA", StockCode: "AAA", Account: "해외", Currency: "EUR",
			Sector: "", Industry: "", TradeType: "매수", Quantity: 1, AmountKRW: 1_000_000},
		{StockName: "SPYM", StockCode: "SPYM", Account: "해외", Currency: "USD",
			Sector: "ETF", Industry: "S&P500", TradeType: "매수", Quantity: 1, AmountKRW: 1_000_000},
	}
	rows := aggregateIndexWeight(trades)
	var unknown *indexWeightRow
	for i := range rows {
		if rows[i].group == groupUnknown && rows[i].bucket == "" {
			unknown = &rows[i]
		}
	}
	if assert.NotNil(t, unknown, "미분류 그룹 행이 있어야 한다") {
		assert.Equal(t, 1_000_000.0, unknown.buy)
		assert.InDelta(t, 0.5, unknown.buyPct, 1e-9, "분모에 미분류 포함")
	}
}

// 같은 종목의 행 중 하나가 공백-only 섹터여도 진짜 값이 선점되지 않는다.
// (사용자가 시트 셀을 손으로 건드린 경우. 공백이 먼저 잡히면 그 종목이 미분류로 빠진다.)
func TestAggregateIndexWeight_BlankSectorDoesNotPreempt(t *testing.T) {
	trades := []model.Trade{
		{StockName: "SPYM", StockCode: "SPYM", Account: "해외", Currency: "USD",
			Sector: "   ", Industry: "", TradeType: "매수", Quantity: 1, AmountKRW: 500_000},
		{StockName: "SPYM", StockCode: "SPYM", Account: "해외", Currency: "USD",
			Sector: "ETF", Industry: "S&P500", TradeType: "매수", Quantity: 1, AmountKRW: 500_000},
	}
	rows := aggregateIndexWeight(trades)
	for _, r := range rows {
		if r.bucket == bucketSP500 {
			assert.Equal(t, 1_000_000.0, r.buy, "두 행 모두 S&P500 로 집계")
		}
		if r.group == groupUnknown {
			assert.Fail(t, "미분류 행이 생기면 안 된다")
		}
	}
}

// 거래가 없으면 빈 슬라이스.
func TestAggregateIndexWeight_Empty(t *testing.T) {
	assert.Empty(t, aggregateIndexWeight(nil))
}
