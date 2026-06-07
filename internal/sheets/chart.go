package sheets

import (
	"context"
	"fmt"

	gsheets "google.golang.org/api/sheets/v4"
)

// GetCharts 는 지정 시트에 포함된 임베디드 차트 목록을 반환한다. (Python get_charts)
// 시트를 찾지 못하면 빈 슬라이스를 반환한다.
func (c *Client) GetCharts(ctx context.Context, sheetName string) ([]*gsheets.EmbeddedChart, error) {
	ss, err := c.GetSpreadsheetMetadata(ctx)
	if err != nil {
		return nil, err
	}
	for _, s := range ss.Sheets {
		if s.Properties == nil {
			continue
		}
		if s.Properties.Title == sheetName {
			return s.Charts, nil
		}
	}
	return nil, nil
}

// DeleteAllCharts 는 지정 시트의 모든 차트를 삭제한다. (Python delete_all_charts)
// 차트가 없으면 아무 것도 하지 않는다.
func (c *Client) DeleteAllCharts(ctx context.Context, sheetName string) error {
	charts, err := c.GetCharts(ctx, sheetName)
	if err != nil {
		return err
	}
	if len(charts) == 0 {
		return nil
	}
	reqs := make([]*gsheets.Request, 0, len(charts))
	for _, ch := range charts {
		reqs = append(reqs, &gsheets.Request{
			DeleteEmbeddedObject: &gsheets.DeleteEmbeddedObjectRequest{
				ObjectId: ch.ChartId,
			},
		})
	}
	if err := c.batchUpdate(ctx, reqs); err != nil {
		return fmt.Errorf("차트 삭제 실패 (%s): %w", sheetName, err)
	}
	return nil
}

// AddCharts 는 여러 차트를 한 번의 batchUpdate 로 추가한다. (Python add_charts)
// 호출자는 완성된 *gsheets.EmbeddedChart 스펙(위치 포함)을 전달한다.
func (c *Client) AddCharts(ctx context.Context, chartSpecs []*gsheets.EmbeddedChart) error {
	if len(chartSpecs) == 0 {
		return nil
	}
	reqs := make([]*gsheets.Request, 0, len(chartSpecs))
	for _, spec := range chartSpecs {
		reqs = append(reqs, &gsheets.Request{
			AddChart: &gsheets.AddChartRequest{Chart: spec},
		})
	}
	if err := c.batchUpdate(ctx, reqs); err != nil {
		return fmt.Errorf("차트 추가 실패: %w", err)
	}
	return nil
}
