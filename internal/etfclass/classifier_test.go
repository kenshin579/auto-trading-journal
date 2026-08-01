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
