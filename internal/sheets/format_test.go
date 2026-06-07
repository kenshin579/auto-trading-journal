package sheets

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gsheets "google.golang.org/api/sheets/v4"
)

// contains 는 슬라이스에 값이 있는지 확인하는 헬퍼.
func contains(ss []string, v string) bool {
	for _, s := range ss {
		if s == v {
			return true
		}
	}
	return false
}

func TestBuildTextFormatRequests(t *testing.T) {
	reqs := buildTextFormatRequests(123, 3, 2, 5) // sheetID, col(1-based), startRow, endRow
	require.Len(t, reqs, 1)
	rc := reqs[0].RepeatCell
	require.NotNil(t, rc)
	assert.Equal(t, int64(123), rc.Range.SheetId)
	assert.Equal(t, int64(1), rc.Range.StartRowIndex)    // startRow-1
	assert.Equal(t, int64(5), rc.Range.EndRowIndex)      // endRow
	assert.Equal(t, int64(2), rc.Range.StartColumnIndex) // col-1
	assert.Equal(t, int64(3), rc.Range.EndColumnIndex)   // col
	assert.Equal(t, "@", rc.Cell.UserEnteredFormat.NumberFormat.Pattern)
	assert.Equal(t, "TEXT", rc.Cell.UserEnteredFormat.NumberFormat.Type)
	assert.Equal(t, "userEnteredFormat.numberFormat", rc.Fields)
}

func TestBuildTextFormatRequestsZeroBasedForceSend(t *testing.T) {
	// col=1 → StartColumnIndex=0 이어야 하고 ForceSendFields 에 포함되어야 한다.
	reqs := buildTextFormatRequests(7, 1, 1, 10)
	require.Len(t, reqs, 1)
	gr := reqs[0].RepeatCell.Range
	assert.Equal(t, int64(0), gr.StartColumnIndex)
	assert.Equal(t, int64(0), gr.StartRowIndex) // startRow-1 = 0
	assert.True(t, contains(gr.ForceSendFields, "StartColumnIndex"))
	assert.True(t, contains(gr.ForceSendFields, "StartRowIndex"))
}

func TestBuildTextFormatRequestsInvalidCol(t *testing.T) {
	assert.Nil(t, buildTextFormatRequests(1, 0, 1, 10))
	assert.Nil(t, buildTextFormatRequests(1, -3, 1, 10))
	assert.Empty(t, buildTextFormatRequests(1, 0, 1, 10))
}

func TestBuildNumberFormatRequests(t *testing.T) {
	formats := []ColumnFormat{
		{Col: 1, Pattern: "#,##0"},                          // Type 비어 있음 → NUMBER
		{Col: 4, Pattern: "0.00%", Type: "PERCENT"},
	}
	reqs := buildNumberFormatRequests(99, formats, 2, 100)
	require.Len(t, reqs, 2)

	r0 := reqs[0].RepeatCell
	assert.Equal(t, int64(99), r0.Range.SheetId)
	assert.Equal(t, int64(1), r0.Range.StartRowIndex)    // startRow-1
	assert.Equal(t, int64(100), r0.Range.EndRowIndex)    // endRow
	assert.Equal(t, int64(0), r0.Range.StartColumnIndex) // col-1 = 0
	assert.Equal(t, int64(1), r0.Range.EndColumnIndex)
	assert.True(t, contains(r0.Range.ForceSendFields, "StartColumnIndex"))
	assert.Equal(t, "NUMBER", r0.Cell.UserEnteredFormat.NumberFormat.Type)
	assert.Equal(t, "#,##0", r0.Cell.UserEnteredFormat.NumberFormat.Pattern)
	assert.Equal(t, "userEnteredFormat.numberFormat", r0.Fields)

	r1 := reqs[1].RepeatCell
	assert.Equal(t, int64(3), r1.Range.StartColumnIndex) // col-1
	assert.Equal(t, int64(4), r1.Range.EndColumnIndex)
	assert.Equal(t, "PERCENT", r1.Cell.UserEnteredFormat.NumberFormat.Type)
}

func TestBuildNumberFormatRequestsEmpty(t *testing.T) {
	assert.Empty(t, buildNumberFormatRequests(1, nil, 1, 10))
}

func TestBuildColorRequests(t *testing.T) {
	color := &gsheets.Color{Red: 1, Green: 0.5, Blue: 0}
	ranges := []ColorRange{
		{StartRow: 1, EndRow: 1, StartCol: 1, EndCol: 9, Color: color},
		{StartRow: 5, EndRow: 8, StartCol: 2, EndCol: 4, Color: color},
	}
	reqs := buildColorRequests(42, ranges)
	require.Len(t, reqs, 2)

	r0 := reqs[0].RepeatCell
	assert.Equal(t, int64(42), r0.Range.SheetId)
	assert.Equal(t, int64(0), r0.Range.StartRowIndex)    // startRow-1 = 0
	assert.Equal(t, int64(1), r0.Range.EndRowIndex)      // endRow
	assert.Equal(t, int64(0), r0.Range.StartColumnIndex) // startCol-1 = 0
	assert.Equal(t, int64(9), r0.Range.EndColumnIndex)
	// 0 값 인덱스가 ForceSendFields 에 포함되어 JSON 으로 전송되어야 한다.
	assert.True(t, contains(r0.Range.ForceSendFields, "StartRowIndex"))
	assert.True(t, contains(r0.Range.ForceSendFields, "StartColumnIndex"))
	assert.Equal(t, color, r0.Cell.UserEnteredFormat.BackgroundColor)
	assert.Equal(t, "userEnteredFormat.backgroundColor", r0.Fields)

	r1 := reqs[1].RepeatCell
	assert.Equal(t, int64(4), r1.Range.StartRowIndex) // 5-1
	assert.Equal(t, int64(8), r1.Range.EndRowIndex)
	assert.Equal(t, int64(1), r1.Range.StartColumnIndex) // 2-1
	assert.Equal(t, int64(4), r1.Range.EndColumnIndex)
}

func TestBuildColorRequestsEmpty(t *testing.T) {
	assert.Empty(t, buildColorRequests(1, nil))
}

func TestGridRangeOmitsNegative(t *testing.T) {
	// SetAutoFilter 처럼 endRow 를 생략(-1)하는 경우 EndRowIndex 가 설정/강제전송되지 않아야 한다.
	gr := gridRange(10, 0, -1, 0, 9)
	assert.Equal(t, int64(0), gr.EndRowIndex)
	assert.False(t, contains(gr.ForceSendFields, "EndRowIndex"))
	assert.True(t, contains(gr.ForceSendFields, "StartRowIndex"))
	assert.True(t, contains(gr.ForceSendFields, "StartColumnIndex"))
}
