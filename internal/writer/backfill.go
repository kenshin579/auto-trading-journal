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
//
// resolve 가 빈 값을 돌려주면 시트의 기존 값(existing)을 유지한다 — 키 미설정이나 일시 실패로
// 조회가 비었을 때 이미 채워둔 열을 지우지 않기 위해서다(사용자가 손으로 채운 값 포함).
// 섹터/산업은 각각 판단해 한쪽만 실패한 경우 나머지를 되돌리지 않는다. 둘 다 비면 빈 값.
// existing 은 시트 조회 결과라 keys 보다 짧거나 행이 짧을 수 있다(인덱스 방어).
//
// 두 번째 반환값은 기존 값을 유지한(=조회가 빈) 키 목록이다. 유지는 안전하지만 조용해서,
// 옛 스키마 값이 그대로 남아 대시보드 분류가 틀리는 상황을 호출부가 경고로 알린다.
func backfillValues(keys, aux []string, existing [][]string, resolve func(key, aux string) (sector, industry string)) ([][]interface{}, []string) {
	out := make([][]interface{}, len(keys))
	var kept []string
	for i, key := range keys {
		a := ""
		if i < len(aux) {
			a = aux[i]
		}
		s, ind := resolve(key, a)
		if i < len(existing) {
			old := existing[i]
			usedOld := false
			if s == "" && len(old) > 0 && old[0] != "" {
				s, usedOld = old[0], true
			}
			if ind == "" && len(old) > 1 && old[1] != "" {
				ind, usedOld = old[1], true
			}
			if usedOld {
				kept = append(kept, key)
			}
		}
		out[i] = []interface{}{s, ind}
	}
	return out, kept
}

// existingPairs 는 시트 조회 결과를 행별 [섹터, 산업] 문자열 쌍으로 정규화한다
// (빈 행·짧은 행·nil 셀은 "").
func existingPairs(vals [][]interface{}) [][]string {
	out := make([][]string, len(vals))
	for i, row := range vals {
		pair := []string{"", ""}
		for j := 0; j < 2 && j < len(row); j++ {
			if row[j] != nil {
				pair[j] = fmt.Sprintf("%v", row[j])
			}
		}
		out[i] = pair
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

// backfillLogTopN 은 "기존 값 유지" 경고에 실을 상위 종목 수(전체를 찍으면 행 수만큼 길어진다).
const backfillLogTopN = 5

// BackfillSectors 는 기존 매매일지 시트의 섹터/산업 열을 일괄 채운다(시트당 한 번의 batch).
//   - 국내 시트: 종목코드(C)+종목명(D) → domesticResolve → E·F 열.
//   - 해외 시트: 통화(C)+종목코드/티커(D) → foreignResolve(ticker, currency) → F·G 열.
//
// 조회 결과가 비면 시트의 기존 값을 유지한다(빈 값으로 덮지 않는다) — 키 미설정·일시 실패로
// 시트가 지워지면 사용자가 손으로 채운 값까지 잃기 때문이다. 그래서 쓰기 전에 해당 열을 읽는다.
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
		// 쓸 범위를 그대로 한 번 읽어 기존 값을 확보한다(시트당 읽기 1회 추가 — 시트 수가
		// 3~5개라 분당 60회 읽기 쿼터에 여유가 있다). 조회가 비었을 때 이미 채워둔 열을
		// 지우지 않기 위해서다. 읽기가 실패하면 기존 값을 모르는 채로 덮게 되므로 스킵한다.
		existingVals, err := w.client.GetValues(ctx, writeRange)
		if err != nil {
			slog.Error("기존 섹터/산업 조회 실패, 스킵", "sheet", sheetName, "err", err)
			continue
		}
		values, kept := backfillValues(keys, aux, existingPairs(existingVals), resolve)
		if len(kept) > 0 {
			slog.Warn("조회 결과가 비어 시트의 기존 섹터/산업을 유지함 — API 키 미설정이나 일시 실패일 수 있다. "+
				"옛 스키마 값이 남으면 대시보드 지수 분류가 틀어지므로, 키를 확인하고 백필을 다시 돌릴 것",
				"sheet", sheetName, "행수", len(kept), "상위", kept[:min(len(kept), backfillLogTopN)])
		}
		if err := w.client.BatchUpdateValues(ctx, map[string][][]interface{}{writeRange: values}); err != nil {
			return fmt.Errorf("섹터 백필(%s): %w", sheetName, err)
		}
		slog.Info("섹터/산업 백필 완료", "sheet", sheetName, "rows", len(keys))
	}
	return nil
}
