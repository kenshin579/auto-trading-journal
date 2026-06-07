package writer

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	"github.com/kenshin579/auto-trading-journal/internal/model"
	"github.com/kenshin579/auto-trading-journal/internal/sheets"
	gsheets "google.golang.org/api/sheets/v4"
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

// ── 순수 헬퍼 ──────────────────────────────────────────────

// normalizeNum 은 숫자를 duplicate_key()의 _num_str() 형식과 일치하도록 정규화한다.
// 정수면 소수점 제거(2.0 → "2"). (Python _normalize_num 의 숫자 경로 = model.numStr)
func normalizeNum(v float64) string {
	if v == float64(int64(v)) {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'g', -1, 64)
}

// normalizeCellValue 는 그리드 셀 값(숫자 또는 문자열)을 정규화한다. (Python _normalize_num)
// - nil → ""
// - 숫자(float64): normalizeNum 와 동일
// - 문자열: 천단위 쉼표 제거 ("28,230" → "28230")
func normalizeCellValue(v interface{}) string {
	switch x := v.(type) {
	case nil:
		return ""
	case float64:
		return normalizeNum(x)
	case string:
		return strings.ReplaceAll(x, ",", "")
	default:
		return strings.ReplaceAll(fmt.Sprintf("%v", x), ",", "")
	}
}

// groupConsecutiveRows 는 행 번호 리스트를 연속 구간 [start,end] 으로 묶는다.
// (Python _group_consecutive_rows)
func groupConsecutiveRows(rows []int) [][2]int {
	if len(rows) == 0 {
		return nil
	}
	sorted := make([]int, len(rows))
	copy(sorted, rows)
	sort.Ints(sorted)
	var ranges [][2]int
	start, end := sorted[0], sorted[0]
	for _, r := range sorted[1:] {
		if r == end+1 {
			end = r
		} else {
			ranges = append(ranges, [2]int{start, end})
			start, end = r, r
		}
	}
	ranges = append(ranges, [2]int{start, end})
	return ranges
}

// colLetter 는 컬럼 번호를 문자로 변환한다 (1=A, 26=Z, 27=AA). (Python _col_letter)
func colLetter(colNum int) string {
	result := ""
	for colNum > 0 {
		colNum--
		result = string(rune('A'+colNum%26)) + result
		colNum /= 26
	}
	return result
}

// ── 그리드 셀 추출 헬퍼 ──────────────────────────────────────

// cellEffective 는 셀의 effectiveValue 에서 stringValue 또는 numberValue 를
// interface{} 로 반환한다(없으면 nil). (Python _get_cell_value)
func cellEffective(cell *gsheets.CellData) interface{} {
	if cell == nil || cell.EffectiveValue == nil {
		return nil
	}
	ev := cell.EffectiveValue
	if ev.StringValue != nil {
		return *ev.StringValue
	}
	if ev.NumberValue != nil {
		return *ev.NumberValue
	}
	return nil
}

// cellHasValue 는 셀의 effectiveValue 에 number/string/bool 중 하나가 있는지 판정한다.
func cellHasValue(cell *gsheets.CellData) bool {
	if cell == nil || cell.EffectiveValue == nil {
		return false
	}
	ev := cell.EffectiveValue
	return ev.NumberValue != nil || ev.StringValue != nil || ev.BoolValue != nil
}

// ── 중복 필터 ──────────────────────────────────────────────

// GetExistingKeys 는 기존 데이터에서 중복 체크용 키 셋을 반환한다.
// 키: (일자, 구분, 종목명, 수량, 단가). (Python get_existing_keys)
//
// 날짜는 formattedValue(시리얼 넘버 방지), 숫자는 effectiveValue(원본 정밀도)를 사용해
// model.Trade.DuplicateKey() 와 비교 가능하도록 정규화한다.
func (w *Writer) GetExistingKeys(ctx context.Context, sheetName string, isForeign bool) (map[model.DupKey]bool, error) {
	keys := make(map[model.DupKey]bool)
	grid, err := w.client.GetRawGridData(ctx, sheetName, "A2:O10000")
	if err != nil {
		// Python 은 예외를 잡아 빈 셋을 반환. 동등하게 soft 처리.
		slog.Error("기존 키 로드 실패", "sheet", sheetName, "err", err)
		return keys, nil
	}
	if grid == nil {
		return keys, nil
	}

	for _, row := range grid.RowData {
		values := row.Values
		if len(values) < 6 {
			continue
		}
		// 날짜(col 0): formattedValue 사용.
		dateVal := values[0].FormattedValue
		if dateVal == "" {
			continue
		}
		tradeType := stringFromCell(cellEffective(values[1]))

		var key model.DupKey
		if isForeign && len(values) >= 7 {
			// 해외: 종목명=4, 수량=5, 단가=6
			stockName := stringFromCell(cellEffective(values[4]))
			key = model.DupKey{
				dateVal, tradeType, stockName,
				normalizeCellValue(cellEffective(values[5])),
				normalizeCellValue(cellEffective(values[6])),
			}
		} else {
			// 국내(10컬럼): 종목명=3, 수량=4, 단가=5
			stockName := stringFromCell(cellEffective(values[3]))
			key = model.DupKey{
				dateVal, tradeType, stockName,
				normalizeCellValue(cellEffective(values[4])),
				normalizeCellValue(cellEffective(values[5])),
			}
		}
		keys[key] = true
	}

	slog.Info("기존 키 로드", "sheet", sheetName, "count", len(keys))
	return keys, nil
}

// stringFromCell 은 effectiveValue interface{} 를 문자열로 변환한다(nil → "").
// (Python str(_get_cell_value(...) or ""))
func stringFromCell(v interface{}) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return normalizeNum(x)
	default:
		return fmt.Sprintf("%v", x)
	}
}

// FindLastRow 는 시트의 마지막 데이터 행 + 1(다음 삽입 위치)을 반환한다.
// (Python find_last_row)
func (w *Writer) FindLastRow(ctx context.Context, sheetName string) (int, error) {
	rowCount := 10000
	meta, err := w.client.GetSpreadsheetMetadata(ctx)
	if err == nil && meta != nil {
		for _, s := range meta.Sheets {
			if s.Properties != nil && s.Properties.Title == sheetName {
				if s.Properties.GridProperties != nil && s.Properties.GridProperties.RowCount > 0 {
					rowCount = int(s.Properties.GridProperties.RowCount)
				}
				break
			}
		}
	}

	grid, err := w.client.GetRawGridData(ctx, sheetName, fmt.Sprintf("A1:A%d", rowCount))
	if err != nil {
		slog.Error("마지막 행 탐색 실패", "sheet", sheetName, "err", err)
		return 2, nil
	}
	if grid == nil {
		return 2, nil
	}

	lastRow := 1
	emptyCount := 0
	for i, row := range grid.RowData {
		isEmpty := true
		for _, cell := range row.Values {
			if cellHasValue(cell) {
				isEmpty = false
				break
			}
		}
		if isEmpty {
			emptyCount++
			if emptyCount >= 100 {
				break
			}
		} else {
			lastRow = i + 1
			emptyCount = 0
		}
	}
	return lastRow + 1, nil
}

// ── 삽입 ──────────────────────────────────────────────────

// InsertTrades 는 거래 데이터를 시트에 삽입하고 숫자 포맷을 적용한다.
// 삽입된 거래 수를 반환한다. (Python insert_trades)
func (w *Writer) InsertTrades(ctx context.Context, sheetName string, trades []model.Trade, isForeign bool) (int, error) {
	if len(trades) == 0 {
		return 0, nil
	}

	startRow, err := w.FindLastRow(ctx, sheetName)
	if err != nil {
		return 0, err
	}
	numCols := len(DomesticHeaders)
	if isForeign {
		numCols = len(ForeignHeaders)
	}

	// 데이터 행 준비 (컬럼 수 맞춤).
	rowsData := make([][]interface{}, 0, len(trades))
	for _, trade := range trades {
		var row []interface{}
		if isForeign {
			row = trade.ToForeignRow()
		} else {
			row = trade.ToDomesticRow()
		}
		if len(row) > numCols {
			row = row[:numCols]
		} else if len(row) < numCols {
			for len(row) < numCols {
				row = append(row, "")
			}
		}
		rowsData = append(rowsData, row)
	}

	endRow := startRow + len(trades) - 1
	rangeStr := fmt.Sprintf("%s!A%d:%s%d", sheetName, startRow, colLetter(numCols), endRow)

	if err := w.client.BatchUpdateValues(ctx, map[string][][]interface{}{rangeStr: rowsData}); err != nil {
		slog.Error("시트 데이터 삽입 실패", "sheet", sheetName, "err", err)
		return 0, fmt.Errorf("시트 '%s' 데이터 삽입 실패: %w", sheetName, err)
	}

	slog.Info("시트 삽입 완료", "sheet", sheetName, "count", len(trades),
		"start_row", startRow, "end_row", endRow)

	if err := w.applyNumberFormats(ctx, sheetName, startRow, endRow, isForeign, trades); err != nil {
		return 0, err
	}

	return len(trades), nil
}

// applyNumberFormats 는 컬럼별 숫자 포맷을 적용한다. (Python _apply_number_formats)
func (w *Writer) applyNumberFormats(ctx context.Context, sheetName string, startRow, endRow int, isForeign bool, trades []model.Trade) error {
	if !isForeign {
		return w.client.ApplyNumberFormatToColumns(ctx, sheetName, DomesticFormats, startRow, endRow)
	}

	// 해외: 통화 무관 컬럼 일괄 적용.
	if err := w.client.ApplyNumberFormatToColumns(ctx, sheetName, ForeignFormatsCommon, startRow, endRow); err != nil {
		return err
	}

	// 해외: 통화별 외화 컬럼 행 단위 적용.
	if len(trades) == 0 {
		return nil
	}
	currencyRows := make(map[string][]int)
	order := make([]string, 0)
	for i, trade := range trades {
		if _, ok := currencyRows[trade.Currency]; !ok {
			order = append(order, trade.Currency)
		}
		currencyRows[trade.Currency] = append(currencyRows[trade.Currency], startRow+i)
	}

	for _, currency := range order {
		pattern, ok := CurrencyPatterns[currency]
		if !ok {
			pattern = CurrencyPatternDefault
		}
		formats := make([]sheets.ColumnFormat, 0, len(ForeignCurrencyCols))
		for _, col := range ForeignCurrencyCols {
			formats = append(formats, sheets.ColumnFormat{Col: col, Pattern: pattern})
		}
		// 연속 행 구간을 묶어 API 호출 최소화.
		for _, rg := range groupConsecutiveRows(currencyRows[currency]) {
			if err := w.client.ApplyNumberFormatToColumns(ctx, sheetName, formats, rg[0], rg[1]); err != nil {
				return err
			}
		}
	}
	return nil
}
