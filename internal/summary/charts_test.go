package summary

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildBasicChartSpec_Column(t *testing.T) {
	const sheetID int64 = 42
	ec := buildBasicChartSpec(
		sheetID, "월별 실현손익 추이", "COLUMN",
		0, []int{2}, 5, 10,
		0, chartColStart, 600, 370,
	)

	require.NotNil(t, ec.Spec)
	require.NotNil(t, ec.Spec.BasicChart)
	assert.Nil(t, ec.Spec.PieChart)
	assert.Equal(t, "COLUMN", ec.Spec.BasicChart.ChartType)
	assert.Equal(t, "BOTTOM_LEGEND", ec.Spec.BasicChart.LegendPosition)
	assert.Equal(t, int64(1), ec.Spec.BasicChart.HeaderCount)
	assert.Equal(t, "월별 실현손익 추이", ec.Spec.Title)

	// 도메인(X축): col 0 → StartColumnIndex 가 0 이므로 ForceSendFields 필요.
	require.Len(t, ec.Spec.BasicChart.Domains, 1)
	dsrc := ec.Spec.BasicChart.Domains[0].Domain.SourceRange.Sources[0]
	assert.Equal(t, sheetID, dsrc.SheetId)
	assert.Equal(t, int64(5), dsrc.StartRowIndex)
	assert.Equal(t, int64(10), dsrc.EndRowIndex)
	assert.Equal(t, int64(0), dsrc.StartColumnIndex)
	assert.Equal(t, int64(1), dsrc.EndColumnIndex)
	assert.Contains(t, dsrc.ForceSendFields, "StartColumnIndex")
	assert.NotContains(t, dsrc.ForceSendFields, "StartRowIndex") // dataStart=5 != 0

	// 시리즈(Y축): col 2 → StartColumnIndex 가 2 이므로 Force 불필요.
	require.Len(t, ec.Spec.BasicChart.Series, 1)
	ssrc := ec.Spec.BasicChart.Series[0].Series.SourceRange.Sources[0]
	assert.Equal(t, int64(2), ssrc.StartColumnIndex)
	assert.Equal(t, int64(3), ssrc.EndColumnIndex)
	assert.Equal(t, "LEFT_AXIS", ec.Spec.BasicChart.Series[0].TargetAxis)
	assert.NotContains(t, ssrc.ForceSendFields, "StartColumnIndex")

	// 위치: anchor 열 = chartColStart, anchor 행 = 0 → ForceSendFields 필요.
	anchor := ec.Position.OverlayPosition.AnchorCell
	assert.Equal(t, int64(chartColStart), anchor.ColumnIndex)
	assert.Equal(t, int64(0), anchor.RowIndex)
	assert.Contains(t, anchor.ForceSendFields, "RowIndex")
	assert.Equal(t, int64(600), ec.Position.OverlayPosition.WidthPixels)
	assert.Equal(t, int64(370), ec.Position.OverlayPosition.HeightPixels)
}

func TestBuildBasicChartSpec_LineMultiSeries(t *testing.T) {
	ec := buildBasicChartSpec(
		7, "월별 승률 & 수익률 추이", "LINE",
		0, []int{3, 4}, 0, 6,
		chartRowSpacing, chartColStart, 600, 370,
	)
	assert.Equal(t, "LINE", ec.Spec.BasicChart.ChartType)
	require.Len(t, ec.Spec.BasicChart.Series, 2)

	// dataStart=0 → 도메인 SourceRange 의 StartRowIndex 가 Force 되어야 함.
	dsrc := ec.Spec.BasicChart.Domains[0].Domain.SourceRange.Sources[0]
	assert.Contains(t, dsrc.ForceSendFields, "StartRowIndex")
	assert.Contains(t, dsrc.ForceSendFields, "StartColumnIndex")

	// anchor 행 = chartRowSpacing(!=0) → RowIndex Force 불필요.
	anchor := ec.Position.OverlayPosition.AnchorCell
	assert.Equal(t, int64(chartRowSpacing), anchor.RowIndex)
	assert.NotContains(t, anchor.ForceSendFields, "RowIndex")
}

func TestBuildPieChartSpec(t *testing.T) {
	const sheetID int64 = 99
	ec := buildPieChartSpec(
		sheetID, "계좌별 투자비중",
		chartColStart, chartColStart+1,
		3, 8,
		chartRowSpacing*2, chartColStart, 450, 370,
	)

	require.NotNil(t, ec.Spec.PieChart)
	assert.Nil(t, ec.Spec.BasicChart)
	assert.Equal(t, "RIGHT_LEGEND", ec.Spec.PieChart.LegendPosition)
	assert.Equal(t, "계좌별 투자비중", ec.Spec.Title)

	dsrc := ec.Spec.PieChart.Domain.SourceRange.Sources[0]
	assert.Equal(t, sheetID, dsrc.SheetId)
	assert.Equal(t, int64(3), dsrc.StartRowIndex)
	assert.Equal(t, int64(8), dsrc.EndRowIndex)
	assert.Equal(t, int64(chartColStart), dsrc.StartColumnIndex)

	vsrc := ec.Spec.PieChart.Series.SourceRange.Sources[0]
	assert.Equal(t, int64(chartColStart+1), vsrc.StartColumnIndex)
	assert.Equal(t, int64(chartColStart+2), vsrc.EndColumnIndex)

	// chartColStart(13) != 0 → 컬럼 Force 불필요, anchor 열도 13.
	assert.NotContains(t, dsrc.ForceSendFields, "StartColumnIndex")
	assert.Equal(t, int64(chartColStart), ec.Position.OverlayPosition.AnchorCell.ColumnIndex)
	assert.Equal(t, int64(450), ec.Position.OverlayPosition.WidthPixels)
}

func TestChartSource_EndFieldsAlwaysForced(t *testing.T) {
	// End 인덱스는 0 이 될 일이 거의 없지만 항상 전송되도록 보장한다.
	gr := chartSource(1, 0, 0, 5).SourceRange.Sources[0]
	assert.Contains(t, gr.ForceSendFields, "EndRowIndex")
	assert.Contains(t, gr.ForceSendFields, "EndColumnIndex")
	assert.Contains(t, gr.ForceSendFields, "StartRowIndex") // dataStart=0
}
