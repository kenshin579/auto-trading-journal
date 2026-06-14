package summary

import (
	"testing"

	"github.com/kenshin579/auto-trading-journal/internal/model"
	"github.com/stretchr/testify/assert"
)

func TestCountryLabel(t *testing.T) {
	assert.Equal(t, "국내", countryLabel("KRW"))
	assert.Equal(t, "미국", countryLabel("USD"))
	assert.Equal(t, "일본", countryLabel("JPY"))
	assert.Equal(t, "HKD", countryLabel("HKD")) // 미정의 통화는 코드 그대로
}

func TestCountrySectorGroups(t *testing.T) {
	trades := []model.Trade{
		// 국내
		{Currency: "KRW", Sector: "전기·전자", TradeType: "매수", AmountKRW: 1_000_000},
		{Currency: "KRW", Sector: "전기·전자", TradeType: "매수", AmountKRW: 500_000},
		{Currency: "KRW", Sector: "전기·전자", TradeType: "매도", AmountKRW: 300_000},
		{Currency: "KRW", Sector: "금융", TradeType: "매수", AmountKRW: 500_000},
		{Currency: "KRW", Sector: "", TradeType: "매수", AmountKRW: 999_999}, // 섹터 빈 값 → 제외
		// 미국
		{Currency: "USD", Sector: "Technology", TradeType: "매수", AmountKRW: 2_000_000},
		{Currency: "USD", Sector: "Financial Services", TradeType: "매수", AmountKRW: 1_000_000},
		// 일본
		{Currency: "JPY", Sector: "Consumer Cyclical", TradeType: "매수", AmountKRW: 500_000},
	}
	groups := countrySectorGroups(trades)

	// 국가 순서: 국내, 미국, 일본
	if assert.Len(t, groups, 3) {
		assert.Equal(t, "국내", groups[0].Country)
		assert.Equal(t, "미국", groups[1].Country)
		assert.Equal(t, "일본", groups[2].Country)
	}

	// 국내: 전기·전자(매수 150만, 매도 30만, 순 120만, 비중 150/200=0.75) > 금융(매수 50만, 비중 0.25)
	kr := groups[0]
	assert.Equal(t, 2_000_000.0, kr.TotalBuy)
	assert.Equal(t, "전기·전자", kr.Rows[0].Sector)
	assert.Equal(t, 1_500_000.0, kr.Rows[0].Buy)
	assert.Equal(t, 300_000.0, kr.Rows[0].Sell)
	assert.Equal(t, 1_200_000.0, kr.Rows[0].Net)
	assert.InDelta(t, 0.75, kr.Rows[0].Weight, 1e-9)
	assert.Equal(t, "금융", kr.Rows[1].Sector)
	assert.InDelta(t, 0.25, kr.Rows[1].Weight, 1e-9)

	// 미국: Technology(2/3) > Financial Services(1/3)
	us := groups[1]
	assert.Equal(t, "Technology", us.Rows[0].Sector)
	assert.InDelta(t, 2.0/3.0, us.Rows[0].Weight, 1e-9)
	assert.Equal(t, "Financial Services", us.Rows[1].Sector)

	// 일본: 단일 섹터 비중 1.0
	jp := groups[2]
	assert.Len(t, jp.Rows, 1)
	assert.InDelta(t, 1.0, jp.Rows[0].Weight, 1e-9)
}

// 국내 ETF 는 섹터 대신 산업(OpenAI category)으로 "ETF·{category}" 세분화한다.
// 산업이 빈 미분류 ETF 는 "ETF" 로 남는다. 일반 종목 섹터는 그대로.
func TestCountrySectorGroups_ETFSubdividedByIndustry(t *testing.T) {
	trades := []model.Trade{
		{Currency: "KRW", Sector: "ETF", Industry: "반도체", TradeType: "매수", AmountKRW: 3_000_000},
		{Currency: "KRW", Sector: "ETF", Industry: "미국주식", TradeType: "매수", AmountKRW: 1_000_000},
		{Currency: "KRW", Sector: "ETF", Industry: "", TradeType: "매수", AmountKRW: 500_000}, // 미분류 → "ETF"
		{Currency: "KRW", Sector: "금융", TradeType: "매수", AmountKRW: 500_000},
	}
	groups := countrySectorGroups(trades)
	if !assert.Len(t, groups, 1) {
		return
	}
	byLabel := map[string]float64{}
	for _, r := range groups[0].Rows {
		byLabel[r.Sector] = r.Buy
	}
	assert.Equal(t, 3_000_000.0, byLabel["ETF·반도체"])
	assert.Equal(t, 1_000_000.0, byLabel["ETF·미국주식"])
	assert.Equal(t, 500_000.0, byLabel["ETF"]) // 미분류 ETF
	assert.Equal(t, 500_000.0, byLabel["금융"])  // 일반 종목 섹터 그대로
}

// 섹터가 모두 빈 국가는 제외된다.
func TestCountrySectorGroups_SkipsEmptyCountry(t *testing.T) {
	trades := []model.Trade{
		{Currency: "USD", Sector: "", TradeType: "매수", AmountKRW: 100},
	}
	assert.Empty(t, countrySectorGroups(trades))
}
