package summary

import (
	"testing"

	"github.com/kenshin579/auto-trading-journal/internal/etfclass"
	"github.com/stretchr/testify/assert"
)

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
}
