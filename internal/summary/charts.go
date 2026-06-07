package summary

import (
	"context"
	"log/slog"

	gsheets "google.golang.org/api/sheets/v4"
)

// chartSource 는 (sheetId, 행/열 범위) 로 차트 데이터 소스(GridRange) 를 만든다.
// 0 값인 StartRowIndex/StartColumnIndex 도 전송되도록 ForceSendFields 를 설정한다.
// (Python _build_*_chart_spec 의 내부 source_range 헬퍼)
func chartSource(sheetID int64, dataStart, dataEnd, col int) *gsheets.ChartData {
	gr := &gsheets.GridRange{
		SheetId:          sheetID,
		StartRowIndex:    int64(dataStart),
		EndRowIndex:      int64(dataEnd),
		StartColumnIndex: int64(col),
		EndColumnIndex:   int64(col + 1),
	}
	force := []string{"EndRowIndex", "EndColumnIndex"}
	if dataStart == 0 {
		force = append(force, "StartRowIndex")
	}
	if col == 0 {
		force = append(force, "StartColumnIndex")
	}
	gr.ForceSendFields = force

	return &gsheets.ChartData{
		SourceRange: &gsheets.ChartSourceRange{
			Sources: []*gsheets.GridRange{gr},
		},
	}
}

// chartPosition 은 차트 오버레이 위치(anchorCell + 크기) 를 만든다.
func chartPosition(sheetID int64, anchorRow, anchorCol, width, height int) *gsheets.EmbeddedObjectPosition {
	anchor := &gsheets.GridCoordinate{
		SheetId:     sheetID,
		RowIndex:    int64(anchorRow),
		ColumnIndex: int64(anchorCol),
	}
	force := []string{}
	if anchorRow == 0 {
		force = append(force, "RowIndex")
	}
	if anchorCol == 0 {
		force = append(force, "ColumnIndex")
	}
	if len(force) > 0 {
		anchor.ForceSendFields = force
	}
	return &gsheets.EmbeddedObjectPosition{
		OverlayPosition: &gsheets.OverlayPosition{
			AnchorCell:   anchor,
			WidthPixels:  int64(width),
			HeightPixels: int64(height),
		},
	}
}

// buildBasicChartSpec 는 BasicChart(COLUMN/LINE/BAR 공용) 스펙을 만든다.
// (Python _build_basic_chart_spec, py:989-1051)
func buildBasicChartSpec(
	sheetID int64, title, chartType string,
	domainCol int, seriesCols []int,
	dataStart, dataEnd int,
	anchorRow, anchorCol, width, height int,
) *gsheets.EmbeddedChart {
	series := make([]*gsheets.BasicChartSeries, 0, len(seriesCols))
	for _, col := range seriesCols {
		series = append(series, &gsheets.BasicChartSeries{
			Series:     chartSource(sheetID, dataStart, dataEnd, col),
			TargetAxis: "LEFT_AXIS",
		})
	}

	return &gsheets.EmbeddedChart{
		Spec: &gsheets.ChartSpec{
			Title: title,
			BasicChart: &gsheets.BasicChartSpec{
				ChartType:      chartType,
				LegendPosition: "BOTTOM_LEGEND",
				HeaderCount:    1,
				Domains: []*gsheets.BasicChartDomain{
					{Domain: chartSource(sheetID, dataStart, dataEnd, domainCol)},
				},
				Series:          series,
				ForceSendFields: []string{"HeaderCount"},
			},
		},
		Position: chartPosition(sheetID, anchorRow, anchorCol, width, height),
	}
}

// buildPieChartSpec 는 PieChart 스펙을 만든다.
// (Python _build_pie_chart_spec, py:1052-1107)
func buildPieChartSpec(
	sheetID int64, title string,
	labelCol, valueCol int,
	dataStart, dataEnd int,
	anchorRow, anchorCol, width, height int,
) *gsheets.EmbeddedChart {
	return &gsheets.EmbeddedChart{
		Spec: &gsheets.ChartSpec{
			Title: title,
			PieChart: &gsheets.PieChartSpec{
				LegendPosition: "RIGHT_LEGEND",
				Domain:         chartSource(sheetID, dataStart, dataEnd, labelCol),
				Series:         chartSource(sheetID, dataStart, dataEnd, valueCol),
			},
		},
		Position: chartPosition(sheetID, anchorRow, anchorCol, width, height),
	}
}

// createCharts 는 대시보드 차트를 생성한다.
// trendStart: 섹션 5(월별 성과 추이) 헤더 행 (1-based)
// trendEnd:   섹션 5 마지막 데이터 행 (1-based)
// (Python _create_charts, py:872-987)
func (g *Generator) createCharts(ctx context.Context, trendStart, trendEnd int) error {
	sheetID, ok, err := g.client.GetSheetID(ctx, DashboardSheet)
	if err != nil {
		return err
	}
	if !ok {
		slog.Error("대시보드 시트 ID를 찾을 수 없어 차트 생성 건너뜀")
		return nil
	}

	// 섹션 5 데이터가 없으면 (헤더만 있으면) 차트 생성 스킵.
	if trendEnd <= trendStart {
		slog.Info("월별 성과 추이 데이터 없음, 차트 생성 건너뜀")
		return nil
	}

	// 0-based 인덱스 변환 (헤더 포함).
	dataStart := trendStart - 1
	dataEnd := trendEnd

	// 중복 방지를 위해 기존 차트 삭제.
	if err := g.client.DeleteAllCharts(ctx, DashboardSheet); err != nil {
		return err
	}

	specs := make([]*gsheets.EmbeddedChart, 0, 6)

	// 차트 1: 월별 실현손익 추이 (Column).
	specs = append(specs, buildBasicChartSpec(
		sheetID, "월별 실현손익 추이", "COLUMN",
		0, []int{2}, dataStart, dataEnd,
		0, chartColStart, 600, 370,
	))

	// 차트 2: 월별 승률·수익률 추이 (Line).
	specs = append(specs, buildBasicChartSpec(
		sheetID, "월별 승률 & 수익률 추이", "LINE",
		0, []int{3, 4}, dataStart, dataEnd,
		chartRowSpacing, chartColStart, 600, 370,
	))

	// 차트 3: 계좌별 투자비중 (Pie).
	if g.pieDataRange.ok {
		specs = append(specs, buildPieChartSpec(
			sheetID, "계좌별 투자비중",
			chartColStart, chartColStart+1,
			g.pieDataRange.start-1, g.pieDataRange.end,
			chartRowSpacing*2, chartColStart, 450, 370,
		))
	}

	// 차트 4: 손익비·Profit Factor 추이 (Line).
	specs = append(specs, buildBasicChartSpec(
		sheetID, "손익비 & Profit Factor 추이", "LINE",
		0, []int{7, 8}, dataStart, dataEnd,
		chartRowSpacing*2, chartColSecondary, 450, 370,
	))

	// 차트 5, 6: 월별 매수/매도 건수·금액 추이.
	if g.tradeCountDataRange.ok {
		tcStart := g.tradeCountDataRange.start
		tcEnd := g.tradeCountDataRange.end

		// 차트 5: 건수 추이 (Column, 그룹). Q열(16) 연월, R/S열(17/18) 건수.
		specs = append(specs, buildBasicChartSpec(
			sheetID, "월별 매수/매도 건수 추이", "COLUMN",
			16, []int{17, 18}, tcStart-1, tcEnd,
			chartRowSpacing*3, chartColStart, 600, 370,
		))

		// 차트 6: 금액 추이 (Column, 그룹). Q열(16) 연월, T/U열(19/20) 금액.
		specs = append(specs, buildBasicChartSpec(
			sheetID, "월별 매수/매도 금액 추이", "COLUMN",
			16, []int{19, 20}, tcStart-1, tcEnd,
			chartRowSpacing*3, chartColSecondary, 600, 370,
		))
	}

	if len(specs) > 0 {
		if err := g.client.AddCharts(ctx, specs); err != nil {
			return err
		}
		slog.Info("대시보드 차트 생성 완료", "count", len(specs))
	}
	return nil
}
