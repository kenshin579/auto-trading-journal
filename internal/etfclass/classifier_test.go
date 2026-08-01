package etfclass

import (
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

// 프롬프트에 지수 오분류 방지 규칙이 들어있는지 확인한다.
// (LLM 응답 자체는 테스트하지 않는다 — 규칙 누락만 잡는다.)
// 카테고리 이름은 Categories 목록이 프롬프트에 그대로 들어가 항상 통과하므로 넣지 않는다.
func TestSystemPrompt_ContainsIndexGuards(t *testing.T) {
	for _, want := range []string{"커버드콜", "레버리지", "인버스", "팩터", "실물·선물"} {
		assert.Contains(t, systemPrompt, want)
	}
}

// 입력이 영문 펀드명(FMP CompanyName)인 경우가 많으므로 영문 큐도 있어야 한다.
// 예: JEPQ 의 입력은 "JPMorgan Nasdaq Equity Premium Income ETF" — 한글 큐로는 안 잡힌다.
func TestSystemPrompt_ContainsEnglishCues(t *testing.T) {
	for _, want := range []string{"Covered Call", "Equity Premium Income", "Growth", "Value", "Momentum", "Inverse", "Equal Weight"} {
		assert.Contains(t, systemPrompt, want)
	}
}

// 카테고리 표기 흔들림(전각 괄호·공백)은 정규화해서 받는다.
// 정규화 실패 시 기타테마로 떨어지면 지수 비중이 과소평가되므로 중요하다.
func TestValidate_NormalizesPunctuation(t *testing.T) {
	assert.Equal(t, "미국주식(기타)", Validate("미국주식（기타）"), "전각 괄호")
	assert.Equal(t, "미국주식(기타)", Validate("미국주식 (기타)"), "내부 공백")
	assert.Equal(t, "바이오·헬스케어", Validate(" 바이오·헬스케어 "))
	assert.Equal(t, FallbackCategory, Validate("미국주식"), "정규화해도 없는 카테고리")
}

// 팩터·스타일 펀드(SCHG 등)를 담을 카테고리.
func TestValidate_FactorStyleCategory(t *testing.T) {
	assert.Equal(t, "팩터·스타일", Validate("팩터·스타일"))
}
