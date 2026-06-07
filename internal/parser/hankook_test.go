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
	require.Len(t, trades, 2)
	for _, tr := range trades {
		assert.NotEmpty(t, tr.StockCode)
		assert.Equal(t, "KRW", tr.Currency)
	}
	// trades[0] = 매수 (buyQty>0 AND buyAmount>0)
	assert.Equal(t, "매수", trades[0].TradeType)
	assert.Equal(t, 450000.0, trades[0].Amount)
	assert.Equal(t, 0.0, trades[0].Fee)
	// trades[1] = 매도: Fee=commission+tax(480+150), Tax=0, Profit=실현손익
	assert.Equal(t, "매도", trades[1].TradeType)
	assert.Equal(t, 480000.0, trades[1].Amount)
	assert.Equal(t, 630.0, trades[1].Fee) // 480(수수료)+150(제세금)
	assert.Equal(t, 0.0, trades[1].Tax)
	assert.Equal(t, 30000.0, trades[1].Profit)
	assert.InDelta(t, 6.67, trades[1].ProfitRate, 0.001)
}
