// Package sheets 는 Google Sheets API v4 래퍼를 제공한다.
// Python modules/google_sheets_client.py 의 Go 포팅.
package sheets

import (
	"context"
	"fmt"

	"google.golang.org/api/option"
	gsheets "google.golang.org/api/sheets/v4"
)

// scope 는 스프레드시트 읽기/쓰기 권한.
const scope = "https://www.googleapis.com/auth/spreadsheets"

// Client 는 Google Sheets API v4 클라이언트 래퍼.
type Client struct {
	service       *gsheets.Service
	spreadsheetID string

	// sheetIDCache 는 시트 이름 → sheetId 캐시. nil 이면 미초기화.
	sheetIDCache map[string]int64
}

// New 는 서비스 계정 키로 인증된 Client 를 생성한다. (Python __init__/_connect)
func New(ctx context.Context, spreadsheetID, serviceAccountPath string) (*Client, error) {
	svc, err := gsheets.NewService(ctx,
		option.WithCredentialsFile(serviceAccountPath),
		option.WithScopes(scope),
	)
	if err != nil {
		return nil, fmt.Errorf("Google Sheets API 연결 실패: %w", err)
	}
	return &Client{
		service:       svc,
		spreadsheetID: spreadsheetID,
		sheetIDCache:  make(map[string]int64),
	}, nil
}

// GetSpreadsheetMetadata 는 스프레드시트 전체 메타데이터를 반환한다. (Python get_spreadsheet_metadata)
func (c *Client) GetSpreadsheetMetadata(ctx context.Context) (*gsheets.Spreadsheet, error) {
	ss, err := c.service.Spreadsheets.Get(c.spreadsheetID).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("스프레드시트 메타데이터 조회 실패: %w", err)
	}
	return ss, nil
}

// ListSheets 는 스프레드시트의 모든 시트 이름을 반환한다. (Python list_sheets)
func (c *Client) ListSheets(ctx context.Context) ([]string, error) {
	ss, err := c.GetSpreadsheetMetadata(ctx)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(ss.Sheets))
	for _, s := range ss.Sheets {
		if s.Properties == nil {
			continue
		}
		names = append(names, s.Properties.Title)
	}
	return names, nil
}

// GetSheetID 는 시트 이름으로 sheetId 를 반환한다(내부 캐시). (Python get_sheet_id)
// 반환값: (sheetId, 발견 여부, error).
func (c *Client) GetSheetID(ctx context.Context, name string) (int64, bool, error) {
	if c.sheetIDCache == nil {
		c.sheetIDCache = make(map[string]int64)
	}
	if id, ok := c.sheetIDCache[name]; ok {
		return id, true, nil
	}
	ss, err := c.GetSpreadsheetMetadata(ctx)
	if err != nil {
		return 0, false, err
	}
	for _, s := range ss.Sheets {
		if s.Properties == nil {
			continue
		}
		c.sheetIDCache[s.Properties.Title] = s.Properties.SheetId
	}
	id, ok := c.sheetIDCache[name]
	return id, ok, nil
}

// InvalidateSheetIDCache 는 시트 ID 캐시를 초기화한다(시트 생성/삭제 후 호출). (Python invalidate_sheet_id_cache)
func (c *Client) InvalidateSheetIDCache() {
	c.sheetIDCache = make(map[string]int64)
}

// GetRawGridData 는 effectiveValue + formattedValue 를 함께 조회한다. (Python get_raw_grid_data)
// sheetName 과 rangeA1(예: "A2:O10000")을 받아 내부에서 "sheetName!rangeA1" 로 조합한다.
func (c *Client) GetRawGridData(ctx context.Context, sheetName, rangeA1 string) (*gsheets.GridData, error) {
	rangeName := fmt.Sprintf("%s!%s", sheetName, rangeA1)
	resp, err := c.service.Spreadsheets.Get(c.spreadsheetID).
		Ranges(rangeName).
		Fields("sheets.data.rowData.values(effectiveValue,formattedValue)").
		Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("시트 GridData 조회 실패: %w", err)
	}
	for _, s := range resp.Sheets {
		for _, d := range s.Data {
			return d, nil
		}
	}
	return &gsheets.GridData{}, nil
}

// GetValues 는 지정한 A1 범위(예: "SheetName!A1:Z")의 값을 반환한다. (Python get_sheet_data)
func (c *Client) GetValues(ctx context.Context, rangeA1 string) ([][]interface{}, error) {
	resp, err := c.service.Spreadsheets.Values.Get(c.spreadsheetID, rangeA1).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("시트 데이터 조회 실패: %w", err)
	}
	return resp.Values, nil
}

// UpdateCells 는 지정한 A1 범위에 값을 기록한다(USER_ENTERED). (Python update_cells)
func (c *Client) UpdateCells(ctx context.Context, rangeA1 string, data [][]interface{}) error {
	vr := &gsheets.ValueRange{Values: data}
	_, err := c.service.Spreadsheets.Values.Update(c.spreadsheetID, rangeA1, vr).
		ValueInputOption("USER_ENTERED").
		Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("셀 업데이트 실패: %w", err)
	}
	return nil
}

// BatchUpdateValues 는 여러 A1 범위의 값을 한 번에 기록한다(USER_ENTERED). (Python batch_update_cells)
func (c *Client) BatchUpdateValues(ctx context.Context, ranges map[string][][]interface{}) error {
	data := make([]*gsheets.ValueRange, 0, len(ranges))
	for rangeA1, values := range ranges {
		data = append(data, &gsheets.ValueRange{Range: rangeA1, Values: values})
	}
	req := &gsheets.BatchUpdateValuesRequest{
		ValueInputOption: "USER_ENTERED",
		Data:             data,
	}
	_, err := c.service.Spreadsheets.Values.BatchUpdate(c.spreadsheetID, req).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("배치 업데이트 실패: %w", err)
	}
	return nil
}

// ClearValues 는 지정한 A1 범위(예: "SheetName!A2:Z")의 값을 비운다. (Python clear_sheet)
func (c *Client) ClearValues(ctx context.Context, rangeA1 string) error {
	_, err := c.service.Spreadsheets.Values.Clear(c.spreadsheetID, rangeA1, &gsheets.ClearValuesRequest{}).
		Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("시트 데이터 삭제 실패: %w", err)
	}
	return nil
}
