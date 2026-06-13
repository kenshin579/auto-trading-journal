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

// backfillValues 는 종목코드 슬라이스를 (섹터,산업) 2D 값으로 변환한다(E:F 열 일괄 기록용).
func backfillValues(codes []string, resolve func(code string) (sector, industry string)) [][]interface{} {
	out := make([][]interface{}, len(codes))
	for i, code := range codes {
		s, ind := resolve(code)
		out[i] = []interface{}{s, ind}
	}
	return out
}

// firstColStrings 는 한 열 조회 결과([][]interface{})의 각 행 첫 셀을 문자열로 추출한다(빈 행은 "").
func firstColStrings(vals [][]interface{}) []string {
	out := make([]string, len(vals))
	for i, row := range vals {
		if len(row) > 0 && row[0] != nil {
			out[i] = fmt.Sprintf("%v", row[0])
		}
	}
	return out
}

// BackfillSectors 는 국내 매매일지 시트의 기존 행에 섹터(E)/산업(F) 을 일괄 채운다.
// 종목코드(C) 로 resolve 한 결과를 시트당 한 번의 batch 로 기록한다.
// 신 포맷 국내 시트만 대상(해외는 설계상 공란, 구포맷/비매매일지는 스킵).
func (w *Writer) BackfillSectors(ctx context.Context, resolve func(code string) (sector, industry string)) error {
	names, err := w.getSheets(ctx)
	if err != nil {
		return err
	}
	for _, sheetName := range names {
		headerVals, err := w.client.GetValues(ctx, sheetName+"!A1:Q1")
		if err != nil {
			slog.Error("헤더 조회 실패, 스킵", "sheet", sheetName, "err", err)
			continue
		}
		if !headersEqual(extractHeaderRow(headerVals), DomesticHeaders) {
			continue // 국내 신 포맷만
		}
		codeVals, err := w.client.GetValues(ctx, sheetName+"!C2:C")
		if err != nil {
			slog.Error("종목코드 조회 실패, 스킵", "sheet", sheetName, "err", err)
			continue
		}
		codes := firstColStrings(codeVals)
		if len(codes) == 0 {
			continue
		}
		for i := range codes {
			codes[i] = padStockCode(codes[i]) // 앞 0 유실 코드 복원
		}
		values := backfillValues(codes, resolve)
		rng := fmt.Sprintf("%s!E2:F%d", sheetName, len(codes)+1)
		if err := w.client.BatchUpdateValues(ctx, map[string][][]interface{}{rng: values}); err != nil {
			return fmt.Errorf("섹터 백필(%s): %w", sheetName, err)
		}
		slog.Info("섹터/산업 백필 완료", "sheet", sheetName, "rows", len(codes))
	}
	return nil
}
