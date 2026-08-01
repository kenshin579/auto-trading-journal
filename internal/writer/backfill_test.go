package writer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/kenshin579/auto-trading-journal/internal/sheets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gsheets "google.golang.org/api/sheets/v4"
)

func TestBackfillValues(t *testing.T) {
	resolve := func(code, name string) (string, string) {
		switch code {
		case "214150":
			return "의료·정밀기기", "의료용 기기 제조업"
		case "487240":
			return "ETF", "미국주식" // 종목명 기반 ETF 분류
		default:
			return "", ""
		}
	}
	got, kept := backfillValues(
		[]string{"214150", "", "487240"},
		[]string{"클래시스", "", "KODEX 미국AI전력핵심설비"},
		nil, // 기존 값 없음(빈 시트)
		resolve)
	want := [][]interface{}{
		{"의료·정밀기기", "의료용 기기 제조업"},
		{"", ""},
		{"ETF", "미국주식"},
	}
	assert.Equal(t, want, got)
	assert.Empty(t, kept, "기존 값이 없으면 유지한 행도 없다")
}

// 해외용: 키=티커, 보조=통화 → FMP resolve.
func TestBackfillValues_Foreign(t *testing.T) {
	resolve := func(ticker, currency string) (string, string) {
		if ticker == "AAPL" && currency == "USD" {
			return "Technology", "Consumer Electronics"
		}
		return "", "" // 미커버/미지원 통화
	}
	got, _ := backfillValues(
		[]string{"AAPL", "1321"},
		[]string{"USD", "JPY"},
		nil,
		resolve)
	want := [][]interface{}{
		{"Technology", "Consumer Electronics"},
		{"", ""},
	}
	assert.Equal(t, want, got)
}

// resolve 가 빈 값을 돌려주면 시트의 기존 값을 유지한다 — 키 미설정이나 일시 실패로 조회가
// 비었을 때 이미 채워둔 열(사용자가 손으로 채운 값 포함)을 지우지 않기 위해서다.
func TestBackfillValues_KeepsExistingWhenResolveBlank(t *testing.T) {
	resolve := func(code, name string) (string, string) { return "", "" } // 키 미설정
	existing := [][]string{
		{"전기·전자", "반도체 제조업"},
		{"ETF", "S&P500"},
		{"", ""}, // 원래 비어 있던 행
	}
	got, kept := backfillValues(
		[]string{"005930", "379780", "999999"},
		[]string{"삼성전자", "RISE 미국S&P500", "유령"},
		existing, resolve)

	assert.Equal(t, [][]interface{}{
		{"전기·전자", "반도체 제조업"},
		{"ETF", "S&P500"},
		{"", ""}, // 둘 다 비면 그대로 빈 값
	}, got)
	assert.Equal(t, []string{"005930", "379780"}, kept, "기존 값을 유지한 행은 보고된다")
}

// resolve 가 값을 돌려주면 기존 값을 덮어쓴다(갱신이 백필의 목적).
func TestBackfillValues_OverwritesWhenResolveHasValue(t *testing.T) {
	resolve := func(code, name string) (string, string) { return "ETF", "S&P500" }
	existing := [][]string{{"Financial Services", "Asset Management - Global"}}

	got, kept := backfillValues([]string{"SPYM"}, []string{"USD"}, existing, resolve)

	assert.Equal(t, [][]interface{}{{"ETF", "S&P500"}}, got, "새 조회 결과가 우선")
	assert.Empty(t, kept)
}

// 섹터/산업은 각각 판단한다 — 한쪽만 비는 경우(ETF 산업 분류 실패 등)에
// 나머지 한쪽까지 통째로 되돌리지 않는다.
func TestBackfillValues_MergesPerColumn(t *testing.T) {
	resolve := func(code, name string) (string, string) { return "ETF", "" }
	existing := [][]string{{"ETF", "미국주식"}}

	got, kept := backfillValues([]string{"379780"}, []string{"RISE 미국S&P500"}, existing, resolve)

	assert.Equal(t, [][]interface{}{{"ETF", "미국주식"}}, got, "빈 산업만 기존 값으로 채운다")
	assert.Equal(t, []string{"379780"}, kept)
}

// existing 은 시트 조회 결과라 keys 보다 짧거나(마지막 빈 행 생략) 행이 짧을 수 있다.
func TestBackfillValues_ShortExistingRows(t *testing.T) {
	resolve := func(code, name string) (string, string) { return "", "" }
	existing := [][]string{
		{"전기·전자"}, // 산업 셀이 없는 행
		// 둘째 행은 아예 없음
	}
	got, _ := backfillValues([]string{"005930", "000660"}, []string{"삼성전자", "SK하이닉스"}, existing, resolve)

	assert.Equal(t, [][]interface{}{
		{"전기·전자", ""},
		{"", ""},
	}, got, "짧은 행·부족한 행에서 패닉 없이 빈 값")
}

// existingPairs 는 시트 조회 결과를 행별 [섹터, 산업] 쌍으로 정규화한다.
func TestExistingPairs(t *testing.T) {
	vals := [][]interface{}{
		{"전기·전자", "반도체 제조업"},
		{},           // 빈 행
		{"ETF"},      // 산업 셀 없음
		{nil, "산업만"}, // 섹터 셀이 nil
	}
	assert.Equal(t, [][]string{
		{"전기·전자", "반도체 제조업"},
		{"", ""},
		{"ETF", ""},
		{"", "산업만"},
	}, existingPairs(vals))
}

func TestPadStockCode(t *testing.T) {
	assert.Equal(t, "055550", padStockCode("55550"))  // 앞 0 유실 복원
	assert.Equal(t, "005935", padStockCode("5935"))   // 삼성전자우
	assert.Equal(t, "005930", padStockCode("005930")) // 이미 6자리 → 그대로
	assert.Equal(t, "0088N0", padStockCode("0088N0")) // 영문 포함 6자리 → 그대로
	assert.Equal(t, "", padStockCode(""))             // 빈값 → 그대로
	assert.Equal(t, "487240", padStockCode("487240")) // 6자리 숫자 → 그대로
}

func TestColStrings(t *testing.T) {
	vals := [][]interface{}{
		{"005930", "삼성전자"},
		{}, // 빈 행
		{"487240", "KODEX 미국AI전력핵심설비"},
	}
	assert.Equal(t, []string{"005930", "", "487240"}, colStrings(vals, 0))         // 코드(C)
	assert.Equal(t, []string{"삼성전자", "", "KODEX 미국AI전력핵심설비"}, colStrings(vals, 1)) // 종목명(D)
}

// --- BackfillSectors 통합 테스트 ---------------------------------------------
// 새로 생긴 "쓸 범위를 먼저 읽는다" 경로는 순수 함수로 검증되지 않는다.
// 읽기 범위가 쓰기 범위와 어긋나면 행이 밀려 모든 종목의 섹터가 틀어지고,
// 읽기 실패 시 스킵하지 않으면 시트를 빈 값으로 덮는다.

// fakeBackfillSheets 는 Values.Get / Values.BatchUpdate / 시트목록만 흉내낸다.
type fakeBackfillSheets struct {
	mu sync.Mutex
	// valuesByRange: 요청 범위 → 응답 값. 없으면 빈 값.
	valuesByRange map[string][][]interface{}
	// failRange: 이 범위 조회는 400 을 돌려준다.
	failRange string
	// sheetNames: 비면 기본 시트 1개("미래에셋증권_국내계좌")만 있는 것으로 본다.
	sheetNames []string

	readRanges []string
	writes     []*gsheets.ValueRange
}

func (f *fakeBackfillSheets) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")

	if indexOfSubstr(r.URL.Path, "/values:batchUpdate") >= 0 {
		var req gsheets.BatchUpdateValuesRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.writes = append(f.writes, req.Data...)
		_, _ = w.Write([]byte(`{}`))
		return
	}
	if idx := indexOfSubstr(r.URL.Path, "/values/"); idx >= 0 {
		rng := r.URL.Path[idx+len("/values/"):]
		f.readRanges = append(f.readRanges, rng)
		if rng == f.failRange {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"code":400,"message":"boom"}}`))
			return
		}
		body, _ := json.Marshal(map[string]any{"values": f.valuesByRange[rng]})
		_, _ = w.Write(body)
		return
	}
	// 시트 목록
	names := f.sheetNames
	if len(names) == 0 {
		names = []string{"미래에셋증권_국내계좌"}
	}
	ss := &gsheets.Spreadsheet{}
	for i, n := range names {
		ss.Sheets = append(ss.Sheets, &gsheets.Sheet{
			Properties: &gsheets.SheetProperties{Title: n, SheetId: int64(i)},
		})
	}
	b, _ := json.Marshal(ss)
	_, _ = w.Write(b)
}

// domesticHeaderRow 는 DomesticHeaders 를 API 응답 형태로 만든다.
func domesticHeaderRow() [][]interface{} {
	row := make([]interface{}, len(DomesticHeaders))
	for i, h := range DomesticHeaders {
		row[i] = h
	}
	return [][]interface{}{row}
}

func newBackfillWriter(t *testing.T, f *fakeBackfillSheets) *Writer {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	c, err := sheets.NewWithEndpoint(context.Background(), "test-sheet", srv.URL)
	require.NoError(t, err)
	return New(c)
}

// 조회가 비어도 시트의 기존 섹터/산업은 지워지지 않는다(키 미설정 시나리오).
func TestBackfillSectors_BlankResolveKeepsSheetValues(t *testing.T) {
	f := &fakeBackfillSheets{valuesByRange: map[string][][]interface{}{
		"미래에셋증권_국내계좌!A1:Q1": domesticHeaderRow(),
		"미래에셋증권_국내계좌!C2:D":  {{"005930", "삼성전자"}, {"379780", "RISE 미국S&P500"}},
		"미래에셋증권_국내계좌!E2:F3": {{"전기·전자", "반도체 제조업"}, {"ETF", "S&P500"}},
	}}
	w := newBackfillWriter(t, f)

	blank := func(a, b string) (string, string) { return "", "" } // 키 미설정
	require.NoError(t, w.BackfillSectors(context.Background(), blank, blank))

	require.Len(t, f.writes, 1)
	assert.Equal(t, "미래에셋증권_국내계좌!E2:F3", f.writes[0].Range, "읽은 범위와 쓴 범위가 같아야 행이 안 밀린다")
	assert.Equal(t, [][]interface{}{
		{"전기·전자", "반도체 제조업"},
		{"ETF", "S&P500"},
	}, f.writes[0].Values, "빈 조회 결과로 덮지 않는다")
	assert.Contains(t, f.readRanges, "미래에셋증권_국내계좌!E2:F3", "쓰기 전에 같은 범위를 읽는다")
}

// 조회 결과가 있으면 기존 값을 갱신한다(백필 본래 목적).
func TestBackfillSectors_ResolvedValuesOverwrite(t *testing.T) {
	f := &fakeBackfillSheets{valuesByRange: map[string][][]interface{}{
		"미래에셋증권_국내계좌!A1:Q1": domesticHeaderRow(),
		"미래에셋증권_국내계좌!C2:D":  {{"379780", "RISE 미국S&P500"}},
		"미래에셋증권_국내계좌!E2:F2": {{"ETF", "미국주식"}}, // 구 taxonomy 값
	}}
	w := newBackfillWriter(t, f)

	resolve := func(code, name string) (string, string) { return "ETF", "S&P500" }
	require.NoError(t, w.BackfillSectors(context.Background(), resolve, resolve))

	require.Len(t, f.writes, 1)
	assert.Equal(t, [][]interface{}{{"ETF", "S&P500"}}, f.writes[0].Values)
}

// 기존 값 읽기가 실패하면 그 시트는 쓰지 않고 스킵한다 — 기존 값을 모르는 채 쓰면
// 빈 조회 결과가 시트를 덮어버린다.
func TestBackfillSectors_ExistingReadFailureSkipsSheet(t *testing.T) {
	f := &fakeBackfillSheets{
		valuesByRange: map[string][][]interface{}{
			"미래에셋증권_국내계좌!A1:Q1": domesticHeaderRow(),
			"미래에셋증권_국내계좌!C2:D":  {{"005930", "삼성전자"}},
		},
		failRange: "미래에셋증권_국내계좌!E2:F2",
	}
	w := newBackfillWriter(t, f)

	blank := func(a, b string) (string, string) { return "", "" }
	require.NoError(t, w.BackfillSectors(context.Background(), blank, blank), "스킵이지 에러가 아니다")
	assert.Empty(t, f.writes, "읽기 실패한 시트는 쓰지 않는다")
}

// 요약 레벨 판정: 정상만 Info, 스킵·유지 행·에러는 Error 로 간다.
func TestBackfillSummary_Incomplete(t *testing.T) {
	assert.False(t, backfillSummary{updated: 3}.incomplete(nil), "전부 갱신 = 정상")
	assert.True(t, backfillSummary{updated: 2, skipped: 1}.incomplete(nil), "스킵 시트")
	assert.True(t, backfillSummary{updated: 3, keptRows: 4}.incomplete(nil), "기존 값 유지 행")
	assert.True(t, backfillSummary{updated: 3}.incomplete(assert.AnError), "중간에 에러로 멈춤")
}

// 요약이 갱신/스킵/유지를 실제로 센다 — 한 시트는 갱신(유지 2행), 한 시트는 기존 값
// 조회 실패로 스킵, 한 시트는 구포맷(정상 스킵이라 세지 않는다).
func TestBackfillSectors_SummaryCounts(t *testing.T) {
	f := &fakeBackfillSheets{
		sheetNames: []string{"미래에셋증권_국내계좌", "한국투자증권_국내계좌", "구포맷시트"},
		valuesByRange: map[string][][]interface{}{
			"미래에셋증권_국내계좌!A1:Q1": domesticHeaderRow(),
			"미래에셋증권_국내계좌!C2:D":  {{"005930", "삼성전자"}, {"379780", "RISE 미국S&P500"}},
			"미래에셋증권_국내계좌!E2:F3": {{"전기·전자", "반도체 제조업"}, {"ETF", "S&P500"}},

			"한국투자증권_국내계좌!A1:Q1": domesticHeaderRow(),
			"한국투자증권_국내계좌!C2:D":  {{"000660", "SK하이닉스"}},

			"구포맷시트!A1:Q1": {{"일자", "구분", "종목명"}}, // 헤더 불일치
			"구포맷시트!C2:D":  {{"x", "y"}},
		},
		failRange: "한국투자증권_국내계좌!E2:F2", // 기존 값 조회 실패 → 스킵
	}
	w := newBackfillWriter(t, f)

	blank := func(a, b string) (string, string) { return "", "" } // 키 미설정
	summary, err := w.backfillSectors(context.Background(), blank, blank)
	require.NoError(t, err)

	assert.Equal(t, 1, summary.updated, "쓰기까지 끝난 시트")
	assert.Equal(t, 1, summary.skipped, "조회 실패 스킵만 센다(구포맷 스킵은 정상)")
	assert.Equal(t, 2, summary.keptRows, "기존 값을 유지한 행 합계")
	assert.True(t, summary.incomplete(err), "이 상태는 Error 로 남는다")
	assert.Len(t, f.writes, 1, "스킵한 시트는 쓰지 않는다")
}

// 전부 갱신되면 요약은 정상(Info) 판정.
func TestBackfillSectors_SummaryAllUpdated(t *testing.T) {
	f := &fakeBackfillSheets{valuesByRange: map[string][][]interface{}{
		"미래에셋증권_국내계좌!A1:Q1": domesticHeaderRow(),
		"미래에셋증권_국내계좌!C2:D":  {{"379780", "RISE 미국S&P500"}},
		"미래에셋증권_국내계좌!E2:F2": {{"ETF", "미국주식"}},
	}}
	w := newBackfillWriter(t, f)

	resolve := func(code, name string) (string, string) { return "ETF", "S&P500" }
	summary, err := w.backfillSectors(context.Background(), resolve, resolve)
	require.NoError(t, err)

	assert.Equal(t, backfillSummary{updated: 1}, summary)
	assert.False(t, summary.incomplete(err))
}
