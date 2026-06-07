package writer

import (
	"context"
	"log/slog"
	"strconv"
	"strings"

	"github.com/kenshin579/auto-trading-journal/internal/model"
	"golang.org/x/text/unicode/norm"
	gsheets "google.golang.org/api/sheets/v4"
)

// ReadAllTrades 는 모든 매매일지 시트에서 Trade 리스트를 읽어 반환한다.
// 헤더 행(1행)을 검증하여 매매일지 시트만 식별한다. (Python read_all_trades)
//
//   - DomesticHeaders 일치 → 국내
//   - ForeignHeaders 일치 → 해외
//   - OldDomesticHeadersV1 일치 → 경고 로그 + 스킵
//   - 그 외 → 스킵(매매일지 시트 아님)
//
// NFD/NFC 유니코드 중복 시트는 하나만 읽는다.
func (w *Writer) ReadAllTrades(ctx context.Context) ([]model.Trade, error) {
	names, err := w.client.ListSheets(ctx)
	if err != nil {
		return nil, err
	}

	allTrades := make([]model.Trade, 0)
	seen := make(map[string]bool)

	for _, sheetName := range names {
		// NFD/NFC 유니코드 중복 시트 방지.
		normalized := norm.NFC.String(sheetName)
		if seen[normalized] {
			slog.Info("시트 스킵(유니코드 중복)", "sheet", sheetName, "normalized", normalized)
			continue
		}
		seen[normalized] = true

		headerVals, err := w.client.GetValues(ctx, sheetName+"!A1:O1")
		if err != nil {
			slog.Error("헤더 조회 실패", "sheet", sheetName, "err", err)
			continue
		}
		headerRow := extractHeaderRow(headerVals)
		if len(headerRow) == 0 {
			continue
		}

		var isForeign bool
		switch {
		case headersEqual(headerRow, DomesticHeaders):
			isForeign = false
		case headersEqual(headerRow, ForeignHeaders):
			isForeign = true
		case headersEqual(headerRow, OldDomesticHeadersV1):
			slog.Warn("시트가 옛 9컬럼 포맷입니다. 종목코드 컬럼 추가를 위해 시트를 삭제 후 재실행하거나 "+
				"D열에 '종목코드' 컬럼을 수동 삽입하세요. (이번 실행에서는 스킵)", "sheet", sheetName)
			continue
		default:
			slog.Debug("시트 스킵(매매일지 헤더 불일치)", "sheet", sheetName)
			continue
		}

		trades := w.readTradesFromSheet(ctx, sheetName, isForeign, normalized)
		allTrades = append(allTrades, trades...)
		kind := "국내"
		if isForeign {
			kind = "해외"
		}
		slog.Info("시트 읽기 완료", "sheet", normalized, "count", len(trades), "kind", kind)
	}

	slog.Info("전체 매매일지 읽기 완료", "total", len(allTrades))
	return allTrades, nil
}

// readTradesFromSheet 는 개별 매매일지 시트에서 Trade 리스트를 반환한다.
// (Python _read_trades_from_sheet) account 는 정규화된 시트 이름.
func (w *Writer) readTradesFromSheet(ctx context.Context, sheetName string, isForeign bool, account string) []model.Trade {
	grid, err := w.client.GetRawGridData(ctx, sheetName, "A2:O10000")
	if err != nil {
		slog.Error("시트 데이터 읽기 실패", "sheet", sheetName, "err", err)
		return nil
	}
	if grid == nil {
		return nil
	}

	minCols := 10
	if isForeign {
		minCols = 15
	}

	trades := make([]model.Trade, 0)
	for _, row := range grid.RowData {
		values := row.Values
		if len(values) < minCols {
			continue
		}
		dateVal := values[0].FormattedValue
		if dateVal == "" {
			continue
		}
		// 그리드 셀을 plain row 로 변환: col0=날짜(formattedValue), 나머지=effectiveValue.
		plain := gridRowToPlain(values, dateVal)
		tr := rowToTrade(plain, isForeign, account)
		trades = append(trades, tr)
	}
	return trades
}

// gridRowToPlain 은 그리드 셀 리스트를 []interface{} 로 변환한다.
// col0 은 dateVal(formattedValue), 나머지는 effectiveValue(string/float64/nil).
func gridRowToPlain(values []*gsheets.CellData, dateVal string) []interface{} {
	plain := make([]interface{}, len(values))
	for i, cell := range values {
		if i == 0 {
			plain[i] = dateVal
			continue
		}
		plain[i] = cellEffective(cell)
	}
	return plain
}

// rowToTrade 는 시트 행 데이터를 Trade 객체로 변환한다(ToDomesticRow/ToForeignRow 의 역변환).
// (Python _row_to_trade)
//
// 저장된 수익률은 소수(0.0714)이므로 *100 하여 ProfitRate(7.14)로 복원한다.
func rowToTrade(row []interface{}, isForeign bool, account string) model.Trade {
	date := getStr(row, 0)
	if isForeign {
		// 해외: A~O (15컬럼) — 일자,구분,통화,종목코드,종목명,수량,단가,금액(외화),
		//                       환율,금액(원화),수수료,세금,손익(외화),손익(원화),수익률
		return model.Trade{
			Date:         date,
			TradeType:    getStr(row, 1),
			Currency:     getStr(row, 2),
			StockCode:    getCode(row, 3),
			StockName:    getStr(row, 4),
			Quantity:     getNum(row, 5),
			Price:        getNum(row, 6),
			Amount:       getNum(row, 7),
			ExchangeRate: getNum(row, 8),
			AmountKRW:    getNum(row, 9),
			Fee:          getNum(row, 10),
			Tax:          getNum(row, 11),
			Profit:       getNum(row, 12),
			ProfitKRW:    getNum(row, 13),
			ProfitRate:   getNum(row, 14) * 100,
			Account:      account,
		}
	}
	// 국내: A~J (10컬럼) — 일자,구분,종목코드,종목명,수량,단가,금액,수수료,손익,수익률
	amount := getNum(row, 6)
	profit := getNum(row, 8)
	return model.Trade{
		Date:         date,
		TradeType:    getStr(row, 1),
		StockCode:    getCode(row, 2),
		StockName:    getStr(row, 3),
		Quantity:     getNum(row, 4),
		Price:        getNum(row, 5),
		Amount:       amount,
		Currency:     "KRW",
		ExchangeRate: 1.0,
		AmountKRW:    amount,
		Fee:          getNum(row, 7),
		Tax:          0.0,
		Profit:       profit,
		ProfitKRW:    profit,
		ProfitRate:   getNum(row, 9) * 100,
		Account:      account,
	}
}

// ── 행 값 추출 헬퍼 (plain row: string/float64/nil) ──────────────

// getNum 은 row[i] 에서 숫자를 추출한다(없거나 변환 불가면 0). (Python _get_num)
// 그리드 effectiveValue 는 float64 이지만, 문자열로 들어온 경우(테스트/USER_ENTERED)도
// 천단위 쉼표를 제거해 파싱한다.
func getNum(row []interface{}, i int) float64 {
	if i < 0 || i >= len(row) {
		return 0
	}
	switch x := row[i].(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case string:
		if f, err := strconv.ParseFloat(strings.ReplaceAll(x, ",", ""), 64); err == nil {
			return f
		}
		return 0
	default:
		return 0
	}
}

// getStr 은 row[i] 에서 문자열을 추출한다(문자열이 아니면 ""). (Python _get_str)
func getStr(row []interface{}, i int) string {
	if i < 0 || i >= len(row) {
		return ""
	}
	if s, ok := row[i].(string); ok {
		return s
	}
	return ""
}

// getCode 는 종목코드를 추출한다: 문자열 우선, 숫자로 저장된 경우도 보강.
// (Python _get_code) 앞 0 이 있던 코드는 숫자 저장 시점에 잘렸으므로 복구 불가.
func getCode(row []interface{}, i int) string {
	if i < 0 || i >= len(row) {
		return ""
	}
	switch x := row[i].(type) {
	case string:
		return x
	case float64:
		return normalizeNum(x)
	case int:
		return normalizeNum(float64(x))
	default:
		return ""
	}
}

// extractHeaderRow 는 GetValues 결과(1행)를 문자열 리스트로 추출한다.
// (Python _extract_header_row)
func extractHeaderRow(values [][]interface{}) []string {
	if len(values) == 0 {
		return nil
	}
	first := values[0]
	out := make([]string, len(first))
	for i, cell := range first {
		if s, ok := cell.(string); ok {
			out[i] = s
		}
	}
	return out
}

// headersEqual 은 두 헤더 슬라이스가 동일한지 비교한다.
func headersEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
