package writer

import (
	"testing"

	"github.com/kenshin579/auto-trading-journal/internal/model"
	"github.com/stretchr/testify/assert"
)

func TestColLetter(t *testing.T) {
	assert.Equal(t, "A", colLetter(1))
	assert.Equal(t, "Z", colLetter(26))
	assert.Equal(t, "AA", colLetter(27))
	assert.Equal(t, "O", colLetter(15))
	assert.Equal(t, "J", colLetter(10))
}

func TestGroupConsecutiveRows(t *testing.T) {
	assert.Equal(t, [][2]int{{2, 4}, {6, 6}}, groupConsecutiveRows([]int{2, 3, 4, 6}))
	assert.Nil(t, groupConsecutiveRows(nil))
	assert.Equal(t, [][2]int{{5, 5}}, groupConsecutiveRows([]int{5}))
	// 정렬되지 않은 입력도 처리.
	assert.Equal(t, [][2]int{{2, 4}, {7, 8}}, groupConsecutiveRows([]int{4, 2, 3, 8, 7}))
}

func TestNormalizeNum(t *testing.T) {
	// model.numStr 와 동일해야 중복 비교가 일치한다.
	assert.Equal(t, "10", normalizeNum(10))
	assert.Equal(t, "10.5", normalizeNum(10.5))
	assert.Equal(t, "70000", normalizeNum(70000))
	assert.Equal(t, "0", normalizeNum(0))
}

func TestNormalizeCellValue(t *testing.T) {
	assert.Equal(t, "", normalizeCellValue(nil))
	assert.Equal(t, "10", normalizeCellValue(float64(10)))
	assert.Equal(t, "10.5", normalizeCellValue(float64(10.5)))
	assert.Equal(t, "28230", normalizeCellValue("28,230"))
	assert.Equal(t, "75000", normalizeCellValue("75000"))
}

// TestKeyAgreement 는 그리드 셀에서 추출한 정규화 키가 model.Trade.DuplicateKey() 와
// 동일한지 확인한다(중복 필터 동작의 핵심).
func TestKeyAgreement(t *testing.T) {
	tr := model.Trade{
		Date: "2026-02-13", TradeType: "매도", StockName: "삼성전자",
		Quantity: 10, Price: 75000,
	}
	dk := tr.DuplicateKey()

	// 그리드에서 수량/단가가 숫자로 저장된 경우.
	keyFromNum := model.DupKey{
		"2026-02-13", "매도", "삼성전자",
		normalizeCellValue(float64(10)), normalizeCellValue(float64(75000)),
	}
	assert.Equal(t, dk, keyFromNum)

	// 그리드에서 단가가 "75,000" 문자열로 저장된 경우도 일치해야 한다.
	keyFromStr := model.DupKey{
		"2026-02-13", "매도", "삼성전자",
		normalizeCellValue("10"), normalizeCellValue("75,000"),
	}
	assert.Equal(t, dk, keyFromStr)
}
