package writer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHeaders(t *testing.T) {
	assert.Len(t, DomesticHeaders, 12)
	assert.Equal(t, "종목코드", DomesticHeaders[2])
	assert.Equal(t, "종목명", DomesticHeaders[3])
	assert.Len(t, ForeignHeaders, 17)
	assert.Len(t, OldDomesticHeadersV1, 9)
}

func TestHeaders_WithSectorIndustry(t *testing.T) {
	assert.Len(t, DomesticHeaders, 12)
	assert.Equal(t, "종목명", DomesticHeaders[3])
	assert.Equal(t, "섹터", DomesticHeaders[4])
	assert.Equal(t, "산업", DomesticHeaders[5])
	assert.Equal(t, "수량", DomesticHeaders[6])
	assert.Len(t, ForeignHeaders, 17)
	assert.Equal(t, "섹터", ForeignHeaders[5])
	assert.Equal(t, "산업", ForeignHeaders[6])
	assert.Len(t, OldDomesticHeadersV2, 10)
	assert.Len(t, OldForeignHeadersV1, 15)
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
	assert.Equal(t, []int{9, 10, 13, 14, 15}, ForeignCurrencyCols)
}
