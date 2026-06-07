package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseFloat(t *testing.T) {
	assert.Equal(t, 0.0, parseFloat(""))
	assert.Equal(t, 1234.5, parseFloat(" 1,234.5 "))
	assert.Equal(t, 70000.0, parseFloat("\"70,000\"")) // 쌍따옴표+콤마
}

func TestConvertDate(t *testing.T) {
	assert.Equal(t, "2026-02-13", convertDate(" 2026/02/13 "))
	assert.Equal(t, "2026-02-13", convertDate("\"2026/02/13\""))
}
