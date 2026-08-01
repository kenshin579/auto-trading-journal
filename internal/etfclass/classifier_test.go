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
func TestSystemPrompt_ContainsIndexGuards(t *testing.T) {
	for _, want := range []string{"커버드콜", "레버리지", "인버스", "팩터", "S&P500", "나스닥", "미국주식(기타)"} {
		assert.Contains(t, systemPrompt, want)
	}
}
