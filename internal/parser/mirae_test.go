package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMiraeForeign_Parse(t *testing.T) {
	p := MiraeForeign{}
	assert.True(t, p.CanParse([]string{"매매일", "통화", "종목번호"}))
	trades, err := p.Parse("../../testdata/mirae_foreign.csv", "미래에셋증권_해외계좌")
	require.NoError(t, err)
	require.Len(t, trades, 2)
	// trades[0] = 매수 (row1)
	assert.Equal(t, "매수", trades[0].TradeType)
	assert.Equal(t, "USD", trades[0].Currency)
	assert.Equal(t, "AAPL", trades[0].StockCode)
	assert.Equal(t, 1360.0, trades[0].ExchangeRate)
	assert.Equal(t, 5.0, trades[0].Quantity)
	assert.Equal(t, 877.5, trades[0].Amount)
	assert.Equal(t, 1193400.0, trades[0].AmountKRW)
	// trades[1] = 매도 (row2): 핵심 컬럼 매핑 회귀 방지
	assert.Equal(t, "매도", trades[1].TradeType)
	assert.Equal(t, 910.0, trades[1].Amount)        // col13 매도금액
	assert.Equal(t, 1241650.0, trades[1].AmountKRW) // col14 원화매도금액
	assert.Equal(t, 910.0, trades[1].Fee)           // col15 수수료
	assert.Equal(t, 150.0, trades[1].Tax)           // col16 세금
	assert.Equal(t, 32.5, trades[1].Profit)         // col19 매매손익
	assert.Equal(t, 46700.0, trades[1].ProfitKRW)   // col22 총평가손익 (NOT col20/21)
	assert.InDelta(t, 3.71, trades[1].ProfitRate, 0.001)
}

func TestMiraeDomestic_ParseCP949(t *testing.T) {
	// 증권사 CSV는 CP949 인코딩이 흔하다. 디코딩 없이는 한글 헤더 매칭/파싱이 깨진다.
	p := MiraeDomestic{}
	// DetectParser도 CP949 헤더를 인식해야 한다.
	det, err := DetectParser("../../testdata/mirae_domestic_cp949.csv")
	require.NoError(t, err)
	assert.Equal(t, "MiraeDomesticParser", det.Name())

	trades, err := p.Parse("../../testdata/mirae_domestic_cp949.csv", "미래에셋증권_국내계좌")
	require.NoError(t, err)
	require.Len(t, trades, 2)
	assert.Equal(t, "삼성전자", trades[0].StockName) // 한글 정상 디코딩
	assert.Equal(t, "매수", trades[0].TradeType)
	assert.Equal(t, 700000.0, trades[0].Amount)
}

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
