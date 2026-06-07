package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHankookDomestic_Parse(t *testing.T) {
	p := HankookDomestic{}
	assert.True(t, p.CanParse([]string{"매매일자", "종목코드", "매입단가"}))
	trades, err := p.Parse("../../testdata/hankook_domestic.csv", "한국투자증권_국내계좌")
	require.NoError(t, err)
	require.NotEmpty(t, trades)
	for _, tr := range trades {
		assert.NotEmpty(t, tr.StockCode)
		assert.Equal(t, "KRW", tr.Currency)
	}
}
