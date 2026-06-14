package writer

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

// padStockCode 는 숫자로만 된 6자리 미만 종목코드를 앞 0 패딩해 6자리로 복원한다.
// (시트에 숫자로 저장돼 앞 0 이 유실된 코드 — 예 "55550"→"055550" — 를 KIS 조회용으로 복원.)
// 영문 포함/이미 6자리/빈값은 그대로 둔다.
func padStockCode(code string) string {
	if code == "" || len(code) >= 6 {
		return code
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			return code
		}
	}
	return strings.Repeat("0", 6-len(code)) + code
}

// backfillValues 는 (키, 보조) 슬라이스를 (섹터,산업) 2D 값으로 변환한다(섹터/산업 2열 일괄 기록용).
// 국내: 키=종목코드, 보조=종목명(9999 ETF OpenAI 입력). 해외: 키=티커, 보조=통화(거래소 접미사).
func backfillValues(keys, aux []string, resolve func(key, aux string) (sector, industry string)) [][]interface{} {
	out := make([][]interface{}, len(keys))
	for i, key := range keys {
		a := ""
		if i < len(aux) {
			a = aux[i]
		}
		s, ind := resolve(key, a)
		out[i] = []interface{}{s, ind}
	}
	return out
}

// colStrings 는 조회 결과([][]interface{})의 각 행 idx 번째 셀을 문자열로 추출한다(없으면 "").
func colStrings(vals [][]interface{}, idx int) []string {
	out := make([]string, len(vals))
	for i, row := range vals {
		if len(row) > idx && row[idx] != nil {
			out[i] = fmt.Sprintf("%v", row[idx])
		}
	}
	return out
}

// BackfillSectors 는 기존 매매일지 시트의 섹터/산업 열을 일괄 채운다(시트당 한 번의 batch).
//   - 국내 시트: 종목코드(C)+종목명(D) → domesticResolve → E·F 열.
//   - 해외 시트: 통화(C)+종목코드/티커(D) → foreignResolve(ticker, currency) → F·G 열.
//
// 구포맷/비매매일지 시트는 스킵.
func (w *Writer) BackfillSectors(
	ctx context.Context,
	domesticResolve func(code, name string) (sector, industry string),
	foreignResolve func(ticker, currency string) (sector, industry string),
) error {
	sheetNames, err := w.getSheets(ctx)
	if err != nil {
		return err
	}
	for _, sheetName := range sheetNames {
		headerVals, err := w.client.GetValues(ctx, sheetName+"!A1:Q1")
		if err != nil {
			slog.Error("헤더 조회 실패, 스킵", "sheet", sheetName, "err", err)
			continue
		}
		header := extractHeaderRow(headerVals)
		rowVals, err := w.client.GetValues(ctx, sheetName+"!C2:D")
		if err != nil {
			slog.Error("종목 행 조회 실패, 스킵", "sheet", sheetName, "err", err)
			continue
		}

		var keys, aux []string // keys=resolve 첫 인자, aux=둘째 인자
		var resolve func(key, aux string) (string, string)
		var writeRange string
		switch {
		case headersEqual(header, DomesticHeaders):
			keys = colStrings(rowVals, 0) // C: 종목코드
			aux = colStrings(rowVals, 1)  // D: 종목명
			for i := range keys {
				keys[i] = padStockCode(keys[i]) // 앞 0 유실 코드 복원(국내만)
			}
			resolve = domesticResolve
			writeRange = fmt.Sprintf("%s!E2:F%d", sheetName, len(keys)+1)
		case headersEqual(header, ForeignHeaders):
			aux = colStrings(rowVals, 0)  // C: 통화
			keys = colStrings(rowVals, 1) // D: 종목코드(티커)
			resolve = foreignResolve
			writeRange = fmt.Sprintf("%s!F2:G%d", sheetName, len(keys)+1)
		default:
			continue // 구포맷/비매매일지 스킵
		}
		if len(keys) == 0 {
			continue
		}
		values := backfillValues(keys, aux, resolve)
		if err := w.client.BatchUpdateValues(ctx, map[string][][]interface{}{writeRange: values}); err != nil {
			return fmt.Errorf("섹터 백필(%s): %w", sheetName, err)
		}
		slog.Info("섹터/산업 백필 완료", "sheet", sheetName, "rows", len(keys))
	}
	return nil
}
