package writer

import (
	"context"
	"fmt"
	"log/slog"
)

// colRun 은 신 헤더 기준 빈 열 삽입 구간(start: 0-based 시작 인덱스, count: 개수).
type colRun struct {
	start, count int
}

// migrationInserts 는 oldHeader 를 newHeader 로 변환하기 위해 삽입할 빈 열 구간들을
// 계산한다. old 의 모든 컬럼이 new 에 같은 순서로 존재해야 하며(중간에 신규 컬럼만 추가됨),
// 그렇지 않으면 (nil, false) 를 반환한다.
//
// 반환된 run 들은 new 인덱스 기준이며 **왼쪽→오른쪽 순서로** 삽입해야 한다(각 삽입 후
// 시트의 해당 위치까지가 new 포맷과 일치하므로 다음 run 의 new 인덱스가 그대로 유효하다).
func migrationInserts(oldHeader, newHeader []string) ([]colRun, bool) {
	isNew := make([]bool, len(newHeader))
	oi := 0
	for ni, col := range newHeader {
		if oi < len(oldHeader) && oldHeader[oi] == col {
			oi++ // 기존 컬럼
		} else {
			isNew[ni] = true // 신규 삽입 컬럼
		}
	}
	if oi != len(oldHeader) {
		return nil, false // old 컬럼이 new 에 순서대로 모두 매칭되지 않음
	}

	var runs []colRun
	for ni := 0; ni < len(newHeader); ni++ {
		if !isNew[ni] {
			continue
		}
		start := ni
		count := 0
		for ni < len(newHeader) && isNew[ni] {
			count++
			ni++
		}
		ni-- // for 루프의 ni++ 보정
		runs = append(runs, colRun{start: start, count: count})
	}
	return runs, true
}

// ensureNewFormat 은 기존 시트가 구 포맷이면 신 포맷으로 자동 마이그레이션한다.
// 신 포맷/빈 시트/매매일지 아님 → no-op. 시트 헤더만 1회 조회한다.
func (w *Writer) ensureNewFormat(ctx context.Context, sheetName string) error {
	headerVals, err := w.client.GetValues(ctx, sheetName+"!A1:Q1")
	if err != nil {
		return err
	}
	headerRow := extractHeaderRow(headerVals)
	switch {
	case headersEqual(headerRow, OldDomesticHeadersV2),
		headersEqual(headerRow, OldDomesticHeadersV1):
		return w.migrateSheet(ctx, sheetName, headerRow, DomesticHeaders)
	case headersEqual(headerRow, OldForeignHeadersV1):
		return w.migrateSheet(ctx, sheetName, headerRow, ForeignHeaders)
	}
	return nil
}

// migrateSheet 은 구 포맷 시트를 신 포맷으로 자동 변환한다.
// 종목명 뒤 섹터/산업(국내 V1 은 종목코드까지) 빈 열을 삽입해 기존 데이터를 보존한 채
// 컬럼을 정렬하고, 헤더를 신 포맷으로 교체한다.
func (w *Writer) migrateSheet(ctx context.Context, sheetName string, oldHeader, newHeader []string) error {
	runs, ok := migrationInserts(oldHeader, newHeader)
	if !ok {
		return fmt.Errorf("writer: 시트 %s 헤더를 신 포맷으로 매핑 불가", sheetName)
	}
	// 왼쪽→오른쪽 순서로 빈 열 삽입(기존 데이터는 오른쪽으로 밀림).
	for _, r := range runs {
		if err := w.client.InsertColumns(ctx, sheetName, r.start, r.count); err != nil {
			return fmt.Errorf("writer: 시트 %s 열 삽입(%d,%d): %w", sheetName, r.start, r.count, err)
		}
	}
	if err := w.client.UpdateCells(ctx, sheetName+"!A1", [][]interface{}{toAnyRow(newHeader)}); err != nil {
		return fmt.Errorf("writer: 시트 %s 헤더 갱신: %w", sheetName, err)
	}
	w.invalidateCache()
	slog.Info("시트 자동 마이그레이션 완료(섹터/산업 열 삽입)", "sheet", sheetName,
		"old_cols", len(oldHeader), "new_cols", len(newHeader))
	return nil
}
