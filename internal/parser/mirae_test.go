package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMiraeDomestic_Parse(t *testing.T) {
	p := MiraeDomestic{}
	assert.True(t, p.CanParse([]string{"일자", "종목명", "기간 중 매수"}))
	trades, err := p.Parse("../../testdata/mirae_domestic.csv", "미래에셋증권_국내계좌")
	require.NoError(t, err)
	require.Len(t, trades, 2)
	assert.Equal(t, "매수", trades[0].TradeType)
	assert.Equal(t, 700000.0, trades[0].Amount)
	assert.Equal(t, "매도", trades[1].TradeType)
	assert.Equal(t, 1500.0, trades[1].Fee)
	assert.Equal(t, 50000.0, trades[1].Profit)
	assert.InDelta(t, 7.14, trades[1].ProfitRate, 0.001)
}
