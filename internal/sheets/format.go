package sheets

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"time"

	"google.golang.org/api/googleapi"
	gsheets "google.golang.org/api/sheets/v4"
)

// ColumnFormat 은 컬럼별 숫자 포맷 정보. (Python dict {'col','pattern','type'})
// Col 은 1-based 컬럼 번호, Type 이 비면 "NUMBER" 로 처리한다.
type ColumnFormat struct {
	Col     int
	Pattern string
	Type    string
}

// ColorRange 는 배경색 적용 범위 정보. (Python dict
// {'start_row','end_row','start_col','end_col','color'})
// 행/열은 1-based, Color 는 RGBA(0~1) 색상.
type ColorRange struct {
	StartRow int
	EndRow   int
	StartCol int
	EndCol   int
	Color    *gsheets.Color
}

// executeWithRetry 는 API 요청을 실행하고 429/5xx 에러 시 지수 백오프로 재시도한다.
// (Python _execute_with_retry, max_retries 기본 3)
//
// Python 정책(modules/google_sheets_client.py:73-87):
//   - 총 시도 = max_retries + 1 (즉 최초 1회 + 재시도 3회).
//   - 재시도 가능 조건: HttpError status == 429 이고 attempt < max_retries.
//   - 대기 = min(2**attempt + random(0,1), 64) 초.
//
// Go 포팅에서는 429 외에 일시적 5xx(500/502/503/504)도 재시도 대상으로 포함한다
// (배치 batchUpdate 의 안정성을 위한 보수적 확장). 그 외 에러는 즉시 반환.
func executeWithRetry(ctx context.Context, fn func() error) error {
	const maxRetries = 3
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		lastErr = fn()
		if lastErr == nil {
			return nil
		}
		if attempt < maxRetries && isRetryable(lastErr) {
			wait := math.Min(math.Pow(2, float64(attempt))+rand.Float64(), 64)
			timer := time.NewTimer(time.Duration(wait * float64(time.Second)))
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
			continue
		}
		return lastErr
	}
	return lastErr
}

// isRetryable 은 에러가 레이트리밋(429) 또는 일시적 서버 오류(5xx)인지 판정한다.
func isRetryable(err error) bool {
	var gErr *googleapi.Error
	if errors.As(err, &gErr) {
		switch gErr.Code {
		case 429, 500, 502, 503, 504:
			return true
		}
	}
	return false
}

// gridRange 는 0-based 인덱스로 GridRange 를 만들고, 0 값 인덱스를 JSON 으로 강제
// 전송하도록 ForceSendFields 를 설정한다. 인자가 -1 이면 해당 인덱스를 생략한다.
func gridRange(sheetID int64, startRow, endRow, startCol, endCol int) *gsheets.GridRange {
	gr := &gsheets.GridRange{SheetId: sheetID}
	var force []string
	if startRow >= 0 {
		gr.StartRowIndex = int64(startRow)
		force = append(force, "StartRowIndex")
	}
	if endRow >= 0 {
		gr.EndRowIndex = int64(endRow)
		if endRow == 0 {
			force = append(force, "EndRowIndex")
		}
	}
	if startCol >= 0 {
		gr.StartColumnIndex = int64(startCol)
		force = append(force, "StartColumnIndex")
	}
	if endCol >= 0 {
		gr.EndColumnIndex = int64(endCol)
		if endCol == 0 {
			force = append(force, "EndColumnIndex")
		}
	}
	gr.ForceSendFields = force
	return gr
}

// ── 배치 요청 빌더 (API 호출 없음) ──────────────────────────────────

// buildNumberFormatRequests 는 컬럼별 숫자 포맷 repeatCell 요청을 생성한다.
// (Python build_number_format_requests) sheetID 는 0-based sheetId,
// startRow/endRow 는 1-based(endRow inclusive).
func buildNumberFormatRequests(sheetID int64, columnFormats []ColumnFormat, startRow, endRow int) []*gsheets.Request {
	reqs := make([]*gsheets.Request, 0, len(columnFormats))
	for _, f := range columnFormats {
		typ := f.Type
		if typ == "" {
			typ = "NUMBER"
		}
		reqs = append(reqs, &gsheets.Request{
			RepeatCell: &gsheets.RepeatCellRequest{
				Range: gridRange(sheetID, startRow-1, endRow, f.Col-1, f.Col),
				Cell: &gsheets.CellData{
					UserEnteredFormat: &gsheets.CellFormat{
						NumberFormat: &gsheets.NumberFormat{Type: typ, Pattern: f.Pattern},
					},
				},
				Fields: "userEnteredFormat.numberFormat",
			},
		})
	}
	return reqs
}

// buildTextFormatRequests 는 지정 컬럼을 TEXT("@") 포맷으로 만드는 repeatCell 요청을
// 생성한다. (Python build_text_format_requests) col 은 1-based; col<=0 이면 nil 반환.
func buildTextFormatRequests(sheetID int64, col int, startRow, endRow int) []*gsheets.Request {
	if col <= 0 {
		return nil
	}
	return []*gsheets.Request{{
		RepeatCell: &gsheets.RepeatCellRequest{
			Range: gridRange(sheetID, startRow-1, endRow, col-1, col),
			Cell: &gsheets.CellData{
				UserEnteredFormat: &gsheets.CellFormat{
					NumberFormat: &gsheets.NumberFormat{Type: "TEXT", Pattern: "@"},
				},
			},
			Fields: "userEnteredFormat.numberFormat",
		},
	}}
}

// buildColorRequests 는 배경색 repeatCell 요청을 생성한다. (Python build_color_requests)
func buildColorRequests(sheetID int64, colorRanges []ColorRange) []*gsheets.Request {
	reqs := make([]*gsheets.Request, 0, len(colorRanges))
	for _, cr := range colorRanges {
		reqs = append(reqs, &gsheets.Request{
			RepeatCell: &gsheets.RepeatCellRequest{
				Range: gridRange(sheetID, cr.StartRow-1, cr.EndRow, cr.StartCol-1, cr.EndCol),
				Cell: &gsheets.CellData{
					UserEnteredFormat: &gsheets.CellFormat{BackgroundColor: cr.Color},
				},
				Fields: "userEnteredFormat.backgroundColor",
			},
		})
	}
	return reqs
}

// ── batchUpdate 실행 헬퍼 ──────────────────────────────────

// batchUpdate 는 requests 를 단일 batchUpdate 로 실행하며 재시도를 적용한다.
func (c *Client) batchUpdate(ctx context.Context, requests []*gsheets.Request) error {
	req := &gsheets.BatchUpdateSpreadsheetRequest{Requests: requests}
	return executeWithRetry(ctx, func() error {
		_, err := c.service.Spreadsheets.BatchUpdate(c.spreadsheetID, req).Context(ctx).Do()
		return err
	})
}

// ExecuteBatchRequests 는 여러 요청을 한 번의 batchUpdate 로 실행한다.
// (Python execute_batch_requests) #57 최적화: 호출자가 다수 요청을 모아 1회 호출.
func (c *Client) ExecuteBatchRequests(ctx context.Context, requests []*gsheets.Request) error {
	if len(requests) == 0 {
		return nil
	}
	if err := c.batchUpdate(ctx, requests); err != nil {
		return fmt.Errorf("배치 요청 실행 실패: %w", err)
	}
	return nil
}

// ── 색상/포맷 API 메서드 ──────────────────────────────────

// ApplyColorToRange 는 특정 범위에 배경색을 적용한다. (Python apply_color_to_range)
// 행/열은 1-based(end inclusive).
func (c *Client) ApplyColorToRange(ctx context.Context, sheetName string, startRow, endRow, startCol, endCol int, color *gsheets.Color) error {
	sheetID, ok, err := c.GetSheetID(ctx, sheetName)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("시트 '%s'를 찾을 수 없습니다", sheetName)
	}
	reqs := buildColorRequests(sheetID, []ColorRange{{
		StartRow: startRow, EndRow: endRow, StartCol: startCol, EndCol: endCol, Color: color,
	}})
	if err := c.batchUpdate(ctx, reqs); err != nil {
		return fmt.Errorf("색상 적용 실패: %w", err)
	}
	return nil
}

// ApplyNumberFormat 은 지정 컬럼들에 동일한 숫자 포맷(NUMBER)을 적용한다.
// (Python apply_number_format) columns 는 1-based 컬럼 번호 리스트.
func (c *Client) ApplyNumberFormat(ctx context.Context, sheetName string, startRow, endRow int, columns []int, formatPattern string) error {
	sheetID, ok, err := c.GetSheetID(ctx, sheetName)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("시트 '%s'를 찾을 수 없습니다", sheetName)
	}
	formats := make([]ColumnFormat, 0, len(columns))
	for _, col := range columns {
		formats = append(formats, ColumnFormat{Col: col, Pattern: formatPattern, Type: "NUMBER"})
	}
	if err := c.batchUpdate(ctx, buildNumberFormatRequests(sheetID, formats, startRow, endRow)); err != nil {
		return fmt.Errorf("숫자 포맷 적용 실패: %w", err)
	}
	return nil
}

// ApplyNumberFormatToColumns 는 컬럼별로 서로 다른 숫자 포맷을 1회 batchUpdate 로
// 적용한다. (Python apply_number_format_to_columns)
func (c *Client) ApplyNumberFormatToColumns(ctx context.Context, sheetName string, columnFormats []ColumnFormat, startRow, endRow int) error {
	sheetID, ok, err := c.GetSheetID(ctx, sheetName)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("시트 '%s'를 찾을 수 없습니다", sheetName)
	}
	reqs := buildNumberFormatRequests(sheetID, columnFormats, startRow, endRow)
	if len(reqs) == 0 {
		return nil
	}
	if err := c.batchUpdate(ctx, reqs); err != nil {
		return fmt.Errorf("컬럼별 숫자 포맷 적용 실패 (%s): %w", sheetName, err)
	}
	return nil
}

// BatchApplyColors 는 여러 범위에 한 번에 배경색을 적용한다. (Python batch_apply_colors)
func (c *Client) BatchApplyColors(ctx context.Context, sheetName string, colorRanges []ColorRange) error {
	sheetID, ok, err := c.GetSheetID(ctx, sheetName)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("시트 '%s'를 찾을 수 없습니다", sheetName)
	}
	reqs := buildColorRequests(sheetID, colorRanges)
	if len(reqs) == 0 {
		return nil
	}
	if err := c.batchUpdate(ctx, reqs); err != nil {
		return fmt.Errorf("배치 색상 적용 실패: %w", err)
	}
	return nil
}

// ── 시트 관리 ──────────────────────────────────

// CreateSheet 는 새 시트(탭)를 추가하고 ID 캐시를 무효화한다. (Python create_sheet)
func (c *Client) CreateSheet(ctx context.Context, title string) error {
	reqs := []*gsheets.Request{{
		AddSheet: &gsheets.AddSheetRequest{
			Properties: &gsheets.SheetProperties{Title: title},
		},
	}}
	if err := c.batchUpdate(ctx, reqs); err != nil {
		return fmt.Errorf("시트 '%s' 생성 실패: %w", title, err)
	}
	c.InvalidateSheetIDCache()
	return nil
}

// DeleteSheet 는 시트(탭)를 삭제하고 ID 캐시를 무효화한다. (Python delete_sheet)
func (c *Client) DeleteSheet(ctx context.Context, sheetName string) error {
	sheetID, ok, err := c.GetSheetID(ctx, sheetName)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("시트 '%s'를 찾을 수 없습니다", sheetName)
	}
	reqs := []*gsheets.Request{{
		DeleteSheet: &gsheets.DeleteSheetRequest{SheetId: sheetID},
	}}
	if err := c.batchUpdate(ctx, reqs); err != nil {
		return fmt.Errorf("시트 '%s' 삭제 실패: %w", sheetName, err)
	}
	c.InvalidateSheetIDCache()
	return nil
}

// ClearBackgroundColors 는 시트 전체의 배경색을 제거한다. (Python clear_background_colors)
// endRow 기본 1000, endCol 기본 26(Z).
func (c *Client) ClearBackgroundColors(ctx context.Context, sheetName string, endRow, endCol int) error {
	sheetID, ok, err := c.GetSheetID(ctx, sheetName)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("시트 '%s'를 찾을 수 없습니다", sheetName)
	}
	reqs := []*gsheets.Request{{
		RepeatCell: &gsheets.RepeatCellRequest{
			Range:  gridRange(sheetID, 0, endRow, 0, endCol),
			Cell:   &gsheets.CellData{UserEnteredFormat: &gsheets.CellFormat{}},
			Fields: "userEnteredFormat.backgroundColor",
		},
	}}
	if err := c.batchUpdate(ctx, reqs); err != nil {
		return fmt.Errorf("시트 '%s' 배경색 초기화 실패: %w", sheetName, err)
	}
	return nil
}

// ClearNumberFormats 는 시트 전체의 숫자 포맷을 초기화한다. (Python clear_number_formats)
func (c *Client) ClearNumberFormats(ctx context.Context, sheetName string, endRow, endCol int) error {
	sheetID, ok, err := c.GetSheetID(ctx, sheetName)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("시트 '%s'를 찾을 수 없습니다", sheetName)
	}
	reqs := []*gsheets.Request{{
		RepeatCell: &gsheets.RepeatCellRequest{
			Range:  gridRange(sheetID, 0, endRow, 0, endCol),
			Cell:   &gsheets.CellData{UserEnteredFormat: &gsheets.CellFormat{}},
			Fields: "userEnteredFormat.numberFormat",
		},
	}}
	if err := c.batchUpdate(ctx, reqs); err != nil {
		return fmt.Errorf("시트 '%s' 숫자 포맷 초기화 실패: %w", sheetName, err)
	}
	return nil
}

// FreezeRows 는 시트 상단 N행을 고정한다. (Python freeze_rows) rowCount 기본 1.
func (c *Client) FreezeRows(ctx context.Context, sheetName string, rowCount int) error {
	sheetID, ok, err := c.GetSheetID(ctx, sheetName)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("시트 '%s'를 찾을 수 없습니다", sheetName)
	}
	reqs := []*gsheets.Request{{
		UpdateSheetProperties: &gsheets.UpdateSheetPropertiesRequest{
			Properties: &gsheets.SheetProperties{
				SheetId: sheetID,
				GridProperties: &gsheets.GridProperties{
					FrozenRowCount:  int64(rowCount),
					ForceSendFields: []string{"FrozenRowCount"},
				},
			},
			Fields: "gridProperties.frozenRowCount",
		},
	}}
	if err := c.batchUpdate(ctx, reqs); err != nil {
		return fmt.Errorf("행 고정 실패 (%s): %w", sheetName, err)
	}
	return nil
}

// SetAutoFilter 는 시트에 자동 필터를 설정한다(기존 필터 제거 후 재설정).
// (Python set_auto_filter) 행/열은 1-based(endCol inclusive).
func (c *Client) SetAutoFilter(ctx context.Context, sheetName string, startRow, startCol, endCol int) error {
	sheetID, ok, err := c.GetSheetID(ctx, sheetName)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("시트 '%s'를 찾을 수 없습니다", sheetName)
	}
	// 기존 필터 제거 시도(없으면 무시).
	clearReq := []*gsheets.Request{{
		ClearBasicFilter: &gsheets.ClearBasicFilterRequest{SheetId: sheetID},
	}}
	_ = c.batchUpdate(ctx, clearReq)

	reqs := []*gsheets.Request{{
		SetBasicFilter: &gsheets.SetBasicFilterRequest{
			Filter: &gsheets.BasicFilter{
				Range: gridRange(sheetID, startRow-1, -1, startCol-1, endCol),
			},
		},
	}}
	if err := c.batchUpdate(ctx, reqs); err != nil {
		return fmt.Errorf("자동 필터 설정 실패 (%s): %w", sheetName, err)
	}
	return nil
}

// ApplySheetFormattingBatch 는 행고정 + 필터 + 배경색초기화(+옵션 TEXT 포맷)를
// 1회 batchUpdate 로 적용한다. (Python apply_sheet_formatting_batch)
// textFormatCol<=0 이면 TEXT 포맷을 적용하지 않는다.
func (c *Client) ApplySheetFormattingBatch(
	ctx context.Context, sheetName string,
	freezeRowCount, filterStartRow, filterStartCol, filterEndCol int,
	clearBgEndRow, clearBgEndCol int,
	textFormatCol int,
) error {
	sheetID, ok, err := c.GetSheetID(ctx, sheetName)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("시트 '%s'를 찾을 수 없습니다", sheetName)
	}

	requests := []*gsheets.Request{
		{
			UpdateSheetProperties: &gsheets.UpdateSheetPropertiesRequest{
				Properties: &gsheets.SheetProperties{
					SheetId: sheetID,
					GridProperties: &gsheets.GridProperties{
						FrozenRowCount:  int64(freezeRowCount),
						ForceSendFields: []string{"FrozenRowCount"},
					},
				},
				Fields: "gridProperties.frozenRowCount",
			},
		},
		{ClearBasicFilter: &gsheets.ClearBasicFilterRequest{SheetId: sheetID}},
		{
			SetBasicFilter: &gsheets.SetBasicFilterRequest{
				Filter: &gsheets.BasicFilter{
					Range: gridRange(sheetID, filterStartRow-1, -1, filterStartCol-1, filterEndCol),
				},
			},
		},
		{
			RepeatCell: &gsheets.RepeatCellRequest{
				Range:  gridRange(sheetID, 0, clearBgEndRow, 0, clearBgEndCol),
				Cell:   &gsheets.CellData{UserEnteredFormat: &gsheets.CellFormat{}},
				Fields: "userEnteredFormat.backgroundColor",
			},
		},
	}

	// 종목코드 등 텍스트로 다뤄야 하는 컬럼을 TEXT 포맷으로 (헤더 제외, 2행~).
	requests = append(requests, buildTextFormatRequests(sheetID, textFormatCol, 2, clearBgEndRow)...)

	if err := c.batchUpdate(ctx, requests); err != nil {
		return fmt.Errorf("시트 포맷팅 일괄 적용 실패 (%s): %w", sheetName, err)
	}
	return nil
}
