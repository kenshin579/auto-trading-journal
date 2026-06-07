package writer

import (
	"context"
	"log/slog"

	"github.com/kenshin579/auto-trading-journal/internal/sheets"
)

// Writer 는 Google Sheets 시트 생성/삽입/포맷을 담당한다. (Python SheetWriter)
type Writer struct {
	client     *sheets.Client
	sheetCache []string // 시트 목록 캐시. nil 이면 미초기화.
}

// New 는 Writer 를 생성한다. (Python SheetWriter.__init__)
func New(c *sheets.Client) *Writer {
	return &Writer{client: c}
}

// getSheets 는 시트 목록을 캐시와 함께 반환한다. (Python _get_sheets)
func (w *Writer) getSheets(ctx context.Context) ([]string, error) {
	if w.sheetCache == nil {
		names, err := w.client.ListSheets(ctx)
		if err != nil {
			return nil, err
		}
		w.sheetCache = names
	}
	return w.sheetCache, nil
}

// invalidateCache 는 시트 목록 캐시를 무효화한다. (Python _invalidate_cache)
func (w *Writer) invalidateCache() {
	w.sheetCache = nil
}

// EnsureSheetExists 는 시트가 없으면 생성하고 헤더를 삽입하며 freeze + filter 를 적용한다.
// 새로 생성했으면 true 를 반환한다. (Python ensure_sheet_exists)
func (w *Writer) EnsureSheetExists(ctx context.Context, sheetName string, isForeign bool) (bool, error) {
	names, err := w.getSheets(ctx)
	if err != nil {
		return false, err
	}
	for _, n := range names {
		if n == sheetName {
			if err := w.ApplySheetFormatting(ctx, sheetName, isForeign); err != nil {
				return false, err
			}
			return false, nil
		}
	}

	headers := DomesticHeaders
	if isForeign {
		headers = ForeignHeaders
	}
	if err := w.client.CreateSheet(ctx, sheetName); err != nil {
		return false, err
	}
	if err := w.client.UpdateCells(ctx, sheetName+"!A1", [][]interface{}{toAnyRow(headers)}); err != nil {
		return false, err
	}
	w.invalidateCache()
	slog.Info("시트 생성 및 헤더 삽입 완료", "sheet", sheetName)

	if err := w.ApplySheetFormatting(ctx, sheetName, isForeign); err != nil {
		return false, err
	}
	return true, nil
}

// ApplySheetFormatting 은 시트에 freeze + filter + 배경색 초기화(+종목코드 TEXT 포맷)를
// 1회 batchUpdate 로 적용한다. (Python apply_sheet_formatting)
func (w *Writer) ApplySheetFormatting(ctx context.Context, sheetName string, isForeign bool) error {
	headers := DomesticHeaders
	if isForeign {
		headers = ForeignHeaders
	}
	numCols := len(headers)
	// 종목코드는 숫자로 보여도 텍스트(정렬 통일·앞0 보존)로 다룬다.
	codeCol := indexOf(headers, "종목코드") + 1
	// Python clear_background_colors 기본값(end_row=1000, end_col=26)을 명시 전달.
	return w.client.ApplySheetFormattingBatch(
		ctx, sheetName,
		1,       // freezeRowCount
		1,       // filterStartRow
		1,       // filterStartCol
		numCols, // filterEndCol
		1000,    // clearBgEndRow
		26,      // clearBgEndCol
		codeCol, // textFormatCol
	)
}

// toAnyRow 는 []string 을 []interface{} 행으로 변환한다.
func toAnyRow(ss []string) []interface{} {
	row := make([]interface{}, len(ss))
	for i, s := range ss {
		row[i] = s
	}
	return row
}
