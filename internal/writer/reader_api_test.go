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

// fakeSheets 는 Sheets API 를 흉내내며 어떤 경로가 몇 번 호출됐는지 기록한다.
type fakeSheets struct {
	mu sync.Mutex
	// gridCalls: ranges 파라미터가 있는 Spreadsheets.Get (그리드 조회) 횟수
	gridCalls int
	// metaCalls: ranges 없는 Spreadsheets.Get (메타데이터/시트목록) 횟수
	metaCalls int
	// valuesCalls: Values.Get (values 엔드포인트) 횟수
	valuesCalls int

	sheetNames []string
	gridBody   func(sheetName string) (int, string)
}

func (f *fakeSheets) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")

	if idx := indexOfSubstr(r.URL.Path, "/values/"); idx >= 0 {
		f.valuesCalls++
		_, _ = w.Write([]byte(`{"values":[]}`))
		return
	}

	ranges := r.URL.Query()["ranges"]
	if len(ranges) == 0 {
		f.metaCalls++
		ss := &gsheets.Spreadsheet{}
		for i, n := range f.sheetNames {
			ss.Sheets = append(ss.Sheets, &gsheets.Sheet{
				Properties: &gsheets.SheetProperties{Title: n, SheetId: int64(i)},
			})
		}
		b, _ := json.Marshal(ss)
		_, _ = w.Write(b)
		return
	}

	f.gridCalls++
	sheetName := ranges[0]
	if i := indexOfSubstr(sheetName, "!"); i >= 0 {
		sheetName = sheetName[:i]
	}
	status, body := f.gridBody(sheetName)
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func indexOfSubstr(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// gridJSON 은 헤더 행 + 데이터 행들을 GridData 응답 JSON 으로 만든다.
func gridJSON(header []string, rows [][]string) string {
	toRow := func(cells []string) *gsheets.RowData {
		rd := &gsheets.RowData{}
		for _, c := range cells {
			v := c
			rd.Values = append(rd.Values, &gsheets.CellData{
				FormattedValue: v,
				EffectiveValue: &gsheets.ExtendedValue{StringValue: &v},
			})
		}
		return rd
	}
	data := &gsheets.GridData{RowData: []*gsheets.RowData{toRow(header)}}
	for _, r := range rows {
		data.RowData = append(data.RowData, toRow(r))
	}
	ss := &gsheets.Spreadsheet{Sheets: []*gsheets.Sheet{{Data: []*gsheets.GridData{data}}}}
	b, _ := json.Marshal(ss)
	return string(b)
}

func newFakeWriter(t *testing.T, f *fakeSheets) *Writer {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	c, err := sheets.NewWithEndpoint(context.Background(), "test-sheet", srv.URL)
	require.NoError(t, err)
	return New(c)
}

func domesticRow() []string {
	return []string{"2026-02-13", "매도", "005930", "삼성전자", "전기·전자", "반도체",
		"10", "75000", "750000", "1500", "50000", "0.0714"}
}

// ReadAllTrades 는 시트당 그리드 1회만 읽어야 한다(헤더 전용 Values.Get 을 따로 호출하지 않는다).
// 읽기 쿼터(분당 60)를 아끼기 위한 핵심 불변식.
func TestReadAllTradesReadsOneRequestPerSheet(t *testing.T) {
	f := &fakeSheets{
		sheetNames: []string{"대시보드", "미래에셋증권_IRP", "미래에셋증권_ISA"},
		gridBody: func(sheetName string) (int, string) {
			if sheetName == "대시보드" {
				return http.StatusOK, gridJSON([]string{"포트폴리오 요약"}, nil)
			}
			return http.StatusOK, gridJSON(DomesticHeaders, [][]string{domesticRow(), domesticRow()})
		},
	}
	w := newFakeWriter(t, f)

	trades, err := w.ReadAllTrades(context.Background())
	require.NoError(t, err)

	assert.Len(t, trades, 4, "매매일지 시트 2개 × 2행")
	assert.Equal(t, 0, f.valuesCalls, "헤더 전용 Values.Get 은 더 이상 호출되지 않아야 한다")
	assert.Equal(t, 3, f.gridCalls, "시트당 그리드 조회 1회")
	assert.Equal(t, 1, f.metaCalls, "시트 목록 조회 1회")
}

// 시트 읽기가 실패하면 부분 데이터로 대시보드를 재작성하지 않도록 에러를 반환해야 한다.
func TestReadAllTradesFailsFastOnReadError(t *testing.T) {
	f := &fakeSheets{
		sheetNames: []string{"미래에셋증권_IRP", "미래에셋증권_ISA"},
		gridBody: func(sheetName string) (int, string) {
			if sheetName == "미래에셋증권_ISA" {
				return http.StatusBadRequest, `{"error":{"code":400,"message":"boom"}}`
			}
			return http.StatusOK, gridJSON(DomesticHeaders, [][]string{domesticRow()})
		},
	}
	w := newFakeWriter(t, f)

	trades, err := w.ReadAllTrades(context.Background())

	require.Error(t, err, "한 시트라도 읽기에 실패하면 에러여야 한다")
	assert.Contains(t, err.Error(), "미래에셋증권_ISA")
	assert.Nil(t, trades)
}

// 헤더 뒤에 빈 셀이 패딩되어 오더라도 헤더 매칭에 성공해야 한다(A1:Q 범위로 읽기 때문).
func TestReadAllTradesTrimsTrailingEmptyHeaderCells(t *testing.T) {
	padded := append(append([]string{}, DomesticHeaders...), "", "", "", "", "")
	f := &fakeSheets{
		sheetNames: []string{"미래에셋증권_IRP"},
		gridBody: func(string) (int, string) {
			return http.StatusOK, gridJSON(padded, [][]string{domesticRow()})
		},
	}
	w := newFakeWriter(t, f)

	trades, err := w.ReadAllTrades(context.Background())
	require.NoError(t, err)
	assert.Len(t, trades, 1)
}
