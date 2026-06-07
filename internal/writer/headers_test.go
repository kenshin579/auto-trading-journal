package writer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHeaders(t *testing.T) {
	assert.Len(t, DomesticHeaders, 10)
	assert.Equal(t, "종목코드", DomesticHeaders[2])
	assert.Equal(t, "종목명", DomesticHeaders[3])
	assert.Len(t, ForeignHeaders, 15)
	assert.Len(t, OldDomesticHeadersV1, 9)
}

func TestForeignHeaderPositions(t *testing.T) {
	// 해외: 통화=2, 종목코드=3, 종목명=4 (0-based)
	assert.Equal(t, "통화", ForeignHeaders[2])
	assert.Equal(t, "종목코드", ForeignHeaders[3])
	assert.Equal(t, "종목명", ForeignHeaders[4])
}

func TestCurrencyPatterns(t *testing.T) {
	assert.Equal(t, "$#,##0.00", CurrencyPatterns["USD"])
	assert.Equal(t, "¥#,##0", CurrencyPatterns["JPY"])
	assert.Equal(t, "#,##0.00", CurrencyPatternDefault)
	assert.Equal(t, []int{7, 8, 11, 12, 13}, ForeignCurrencyCols)
}
