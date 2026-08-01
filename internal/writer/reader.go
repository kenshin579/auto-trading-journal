package writer

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/kenshin579/auto-trading-journal/internal/model"
	"golang.org/x/text/unicode/norm"
	gsheets "google.golang.org/api/sheets/v4"
)

// tradeGridRange 는 매매일지 시트를 헤더 포함 1회에 읽는 범위다.
// 국내 12컬럼(A~L), 해외 17컬럼(A~Q) 공통.
const tradeGridRange = "A1:Q10000"

// ReadAllTrades 는 모든 매매일지 시트에서 Trade 리스트를 읽어 반환한다.
// 헤더 행(1행)을 검증하여 매매일지 시트만 식별한다. (Python read_all_trades)
//
//   - DomesticHeaders 일치 → 국내
//   - ForeignHeaders 일치 → 해외
//   - 구 포맷 헤더 → 자동 마이그레이션 후 재조회
//   - 그 외 → 스킵(매매일지 시트 아님)
//
// 읽기 쿼터(분당 60회)를 아끼기 위해 **시트당 그리드 조회 1회**만 사용한다
// (헤더는 조회 결과의 1행에서 얻는다).
//
// 조회 실패는 스킵하지 않고 **에러로 전파**한다. 일부 시트만 실패한 채 진행하면
// 호출측(대시보드)이 부분 데이터로 시트를 통째로 재작성해 기존 내용을 잃는다.
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

		grid, err := w.client.GetRawGridData(ctx, sheetName, tradeGridRange)
		if err != nil {
			return nil, fmt.Errorf("시트 '%s' 읽기 실패: %w", sheetName, err)
		}
		headerRow := headerFromGrid(grid)
		if len(headerRow) == 0 {
			continue
		}

		var isForeign bool
		var newHeader []string
		switch {
		case headersEqual(headerRow, DomesticHeaders):
			isForeign = false
		case headersEqual(headerRow, ForeignHeaders):
			isForeign = true
		case headersEqual(headerRow, OldDomesticHeadersV2),
			headersEqual(headerRow, OldDomesticHeadersV1):
			isForeign, newHeader = false, DomesticHeaders
		case headersEqual(headerRow, OldForeignHeadersV1):
			isForeign, newHeader = true, ForeignHeaders
		default:
			slog.Debug("시트 스킵(매매일지 헤더 불일치)", "sheet", sheetName)
			continue
		}

		// 구 포맷이면 마이그레이션 후 재조회(열 삽입으로 앞서 읽은 그리드가 무효화된다).
		if newHeader != nil {
			if err := w.migrateSheet(ctx, sheetName, headerRow, newHeader); err != nil {
				return nil, fmt.Errorf("시트 '%s' 자동 마이그레이션 실패: %w", sheetName, err)
			}
			if grid, err = w.client.GetRawGridData(ctx, sheetName, tradeGridRange); err != nil {
				return nil, fmt.Errorf("시트 '%s' 마이그레이션 후 읽기 실패: %w", sheetName, err)
			}
		}

		trades := tradesFromGrid(grid, isForeign, normalized)
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

// tradesFromGrid 는 헤더 포함 그리드(1행=헤더)에서 데이터 행만 Trade 로 변환한다.
// (Python _read_trades_from_sheet) account 는 정규화된 시트 이름.
func tradesFromGrid(grid *gsheets.GridData, isForeign bool, account string) []model.Trade {
	if grid == nil || len(grid.RowData) <= 1 {
		return nil
	}

	minCols := 12
	if isForeign {
		minCols = 17
	}

	trades := make([]model.Trade, 0, len(grid.RowData)-1)
	for _, row := range grid.RowData[1:] { // 1행(헤더) 제외
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

// headerFromGrid 는 그리드 1행을 헤더 문자열 리스트로 추출한다.
// A1:Q 범위로 읽으면 실제 컬럼 수보다 뒤쪽 빈 셀이 딸려올 수 있으므로 후행 공백은 제거한다
// (headersEqual 은 길이까지 비교한다).
func headerFromGrid(grid *gsheets.GridData) []string {
	if grid == nil || len(grid.RowData) == 0 {
		return nil
	}
	values := grid.RowData[0].Values
	out := make([]string, len(values))
	for i, cell := range values {
		if s, ok := cellEffective(cell).(string); ok {
			out[i] = s
		}
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
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
		// 해외 17컬럼: 0일자 1구분 2통화 3종목코드 4종목명 5섹터 6산업 7수량 8단가
		//              9금액(외화) 10환율 11금액(원화) 12수수료 13세금 14손익(외화) 15손익(원화) 16수익률
		return model.Trade{
			Date:         date,
			TradeType:    getStr(row, 1),
			Currency:     getStr(row, 2),
			StockCode:    getCode(row, 3),
			StockName:    getStr(row, 4),
			Sector:       getStr(row, 5),
			Industry:     getStr(row, 6),
			Quantity:     getNum(row, 7),
			Price:        getNum(row, 8),
			Amount:       getNum(row, 9),
			ExchangeRate: getNum(row, 10),
			AmountKRW:    getNum(row, 11),
			Fee:          getNum(row, 12),
			Tax:          getNum(row, 13),
			Profit:       getNum(row, 14),
			ProfitKRW:    getNum(row, 15),
			ProfitRate:   getNum(row, 16) * 100,
			Account:      account,
		}
	}
	// 국내 12컬럼: 0일자 1구분 2종목코드 3종목명 4섹터 5산업 6수량 7단가 8금액 9수수료 10손익 11수익률
	amount := getNum(row, 8)
	profit := getNum(row, 10)
	return model.Trade{
		Date:         date,
		TradeType:    getStr(row, 1),
		StockCode:    getCode(row, 2),
		StockName:    getStr(row, 3),
		Sector:       getStr(row, 4),
		Industry:     getStr(row, 5),
		Quantity:     getNum(row, 6),
		Price:        getNum(row, 7),
		Amount:       amount,
		Currency:     "KRW",
		ExchangeRate: 1.0,
		AmountKRW:    amount,
		Fee:          getNum(row, 9),
		Tax:          0.0,
		Profit:       profit,
		ProfitKRW:    profit,
		ProfitRate:   getNum(row, 11) * 100,
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
