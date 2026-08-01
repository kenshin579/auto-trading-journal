package etfclass

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidate(t *testing.T) {
	assert.Equal(t, "한국주식", Validate("한국주식"))
	assert.Equal(t, "금융", Validate("금융"))
	assert.Equal(t, "방위·우주항공", Validate("방위·우주항공"))
	assert.Equal(t, "반도체", Validate("  반도체  "), "공백 트림")
	assert.Equal(t, FallbackCategory, Validate("아무말"))
	assert.Equal(t, FallbackCategory, Validate(""))
}

// apiKey 가 비면 분류기를 만들지 않는다(호출부가 폴백을 쓰도록).
func TestNew_EmptyKeyReturnsNil(t *testing.T) {
	assert.Nil(t, New("", "gpt-4o-mini"))
	assert.NotNil(t, New("sk-test", ""))
}

// 미국 시장대표는 추종 지수 단위로 쪼개진다. 옛 카테고리 "미국주식" 은 taxonomy 에서 빠졌다.
func TestValidate_USIndexSubdivided(t *testing.T) {
	assert.Equal(t, "S&P500", Validate("S&P500"))
	assert.Equal(t, "나스닥", Validate("나스닥"))
	assert.Equal(t, "미국주식(기타)", Validate("미국주식(기타)"))
	assert.Equal(t, FallbackCategory, Validate("미국주식"), "옛 카테고리는 taxonomy 밖")
}

// 규칙 본문에 지수 오분류 방지 규칙이 들어있는지 확인한다.
// classifyRules 를 보는 이유: systemPrompt 는 Categories 나열을 포함해서
// 카테고리 이름을 assert 하면 규칙을 통째로 지워도 통과한다(동어반복).
func TestClassifyRules_ContainsIndexGuards(t *testing.T) {
	for _, want := range []string{"커버드콜", "레버리지", "인버스", "팩터", "실물·선물", "배당성장"} {
		assert.Contains(t, classifyRules, want)
	}
}

// 입력이 영문 펀드명(FMP CompanyName)인 경우가 많으므로 영문 큐도 있어야 한다.
func TestClassifyRules_ContainsEnglishCues(t *testing.T) {
	for _, want := range []string{"Covered Call", "Equity Premium Income", "Dividend", "Growth", "Value", "Momentum", "Inverse", "Equal Weight"} {
		assert.Contains(t, classifyRules, want)
	}
}

// 배당 펀드가 추종 지수 이름(다우존스) 때문에 시장대표로 새지 않도록 하는 규칙.
// SCHD("Schwab U.S. Dividend Equity ETF")·국내 미국배당다우존스 계열이 이 규칙에 걸린다.
func TestClassifyRules_DividendRulePrecedesIndexCue(t *testing.T) {
	assert.Contains(t, classifyRules, "배당이 핵심인 펀드")
	assert.Contains(t, classifyRules, "다우존스가 들어가도")
	dividendAt := strings.Index(classifyRules, "배당이 핵심인 펀드")
	indexAt := strings.Index(classifyRules, "다우존스산업평균")
	assert.Less(t, dividendAt, indexAt, "배당 예외가 지수 규칙보다 앞에 와야 한다")
}

// 모든 카테고리가 정규화를 왕복해도 자기 자신이어야 한다.
// (공백 든 카테고리를 나중에 추가하면 정규화가 그 카테고리 자신을 깨뜨린다.)
func TestValidate_RoundTripsAllCategories(t *testing.T) {
	for _, c := range Categories {
		assert.Equal(t, c, Validate(c), c)
	}
}

// 카테고리 표기 흔들림(전각 괄호·공백)은 정규화해서 받는다.
// 정규화 실패 시 기타테마로 떨어지면 지수 비중이 과소평가되므로 중요하다.
func TestValidate_NormalizesPunctuation(t *testing.T) {
	assert.Equal(t, "미국주식(기타)", Validate("미국주식（기타）"), "전각 괄호")
	assert.Equal(t, "미국주식(기타)", Validate("미국주식 (기타)"), "내부 공백")
	assert.Equal(t, "미국주식(기타)", Validate("미국주식　(기타)"), "전각 공백")
	assert.Equal(t, "바이오·헬스케어", Validate(" 바이오·헬스케어 "))
	assert.Equal(t, FallbackCategory, Validate("미국주식"), "정규화해도 없는 카테고리")
}

// 팩터·스타일 펀드(SCHG 등)를 담을 카테고리.
func TestValidate_FactorStyleCategory(t *testing.T) {
	assert.Equal(t, "팩터·스타일", Validate("팩터·스타일"))
}
