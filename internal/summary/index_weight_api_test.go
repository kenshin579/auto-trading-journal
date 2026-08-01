package summary

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/kenshin579/auto-trading-journal/internal/model"
	"github.com/kenshin579/auto-trading-journal/internal/sheets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeIndexWeight 는 순수 함수가 아니라 Sheets 를 호출한다. 범위 문자열·시작 행 정렬·
// 반환 행 번호·실패 경로는 fake 엔드포인트로만 검증할 수 있다(make dry 는 대시보드 갱신을
// 통째로 건너뛰므로 이 코드를 한 줄도 실행하지 않는다).

// fakeValuesServer 는 Values.Update(PUT /v4/spreadsheets/{id}/values/{range}) 만 흉내내며
// 요청된 A1 범위를 순서대로 기록한다.
type fakeValuesServer struct {
	mu     sync.Mutex
	ranges []string
	// failFrom 번째(0-based) 호출부터 400 을 돌려준다. -1 이면 항상 성공.
	failFrom int
}

// newFakeValues 는 항상 성공하는 fake 를 만든다(failFrom 의 zero value 0 은
// "첫 호출부터 실패" 라 기본값으로 부적절하다).
func newFakeValues() *fakeValuesServer { return &fakeValuesServer{failFrom: -1} }

func (f *fakeValuesServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	idx := strings.Index(r.URL.Path, "/values/")
	if idx < 0 {
		http.Error(w, `{"error":{"code":404,"message":"unexpected path"}}`, http.StatusNotFound)
		return
	}
	// r.URL.Path 는 이미 percent-decoding 된 값이라 "대시보드!A5:E16" 그대로 얻는다.
	n := len(f.ranges)
	f.ranges = append(f.ranges, r.URL.Path[idx+len("/values/"):])

	w.Header().Set("Content-Type", "application/json")
	// 400 을 쓰는 이유: 500 은 executeWithRetry 의 재시도 대상이라 테스트가 60초 넘게 걸린다
	// (retrySleep 은 sheets 패키지 내부 변수라 여기서 스텁할 수 없다). 에러 전파 경로는 동일하다.
	if f.failFrom >= 0 && n >= f.failFrom {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":400,"message":"boom"}}`))
		return
	}
	_, _ = w.Write([]byte(`{}`))
}

func (f *fakeValuesServer) recorded() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.ranges...)
}

func newFakeGenerator(t *testing.T, f *fakeValuesServer) *Generator {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	c, err := sheets.NewWithEndpoint(context.Background(), "test-sheet", srv.URL)
	require.NoError(t, err)
	g := New(c, nil, nil) // writer/분류기는 이 경로에서 쓰이지 않는다
	g.dashboardSheetID = 7
	return g
}

// indexWeightFixture 는 지수 ETF 1종(부분 매도) + 개별종목 1종.
// → 표는 미분류를 뺀 indexWeightLayout 10줄, 헬퍼는 그룹 소계 2줄.
func indexWeightFixture() []model.Trade {
	return []model.Trade{
		{StockName: "SPYM", StockCode: "SPYM", Account: "해외", Currency: "USD",
			Sector: "ETF", Industry: "S&P500", TradeType: "매수", Quantity: 100, AmountKRW: 1_000_000},
		{StockName: "SPYM", StockCode: "SPYM", Account: "해외", Currency: "USD",
			Sector: "ETF", Industry: "S&P500", TradeType: "매도", Quantity: 50, AmountKRW: 600_000},
		{StockName: "삼성전자", StockCode: "005930", Account: "국내", Currency: "KRW",
			Sector: "전기·전자", Industry: "반도체 제조업", TradeType: "매수", Quantity: 10, AmountKRW: 1_000_000},
	}
}

// 표(A:E)와 파이 헬퍼(Y:Z)가 같은 startRow 에서 시작하고, 범위·반환값·차트 범위가 맞는지.
func TestWriteIndexWeight_RangesAndReturn(t *testing.T) {
	f := newFakeValues()
	g := newFakeGenerator(t, f)

	const startRow = 5
	next, err := g.writeIndexWeight(context.Background(), indexWeightFixture(), startRow)
	require.NoError(t, err)

	// 표: 제목행 + 안내행 + 컬럼헤더 + 버킷/그룹 10줄 = 13행 → 5..17
	// 헬퍼: 설명헤더 + 그룹 소계 2줄 = 3행 → 5..7
	assert.Equal(t, []string{"대시보드!A5:E17", "대시보드!Y5:Z7"}, f.recorded(),
		"UpdateCells 2회 — 표(A:E)와 파이 헬퍼(Y:Z)가 같은 행에서 시작한다")

	assert.Equal(t, 18, next, "반환값 = startRow + len(values) (다음 섹션의 시작 행)")

	// 차트는 헬퍼의 설명 헤더(5행)를 빼고 6..7 을 소스로 잡는다.
	assert.Equal(t, rowRange{start: startRow + 1, end: 7, ok: true}, g.indexWeightPie)
}

// 표 값이 실제로 그 범위에 들어가는지(행 수가 범위와 어긋나면 Sheets 가 잘라 쓴다).
func TestWriteIndexWeight_ValueRowCountMatchesRange(t *testing.T) {
	trades := indexWeightFixture()
	rows, diag := aggregateIndexWeight(trades)
	values, _ := indexWeightValues(rows, diag)

	assert.Len(t, values, 13, "제목행 + 안내행 + 컬럼헤더 + 버킷/그룹 10줄")
	assert.Len(t, indexWeightPieHelper(rows), 3, "설명헤더 + 그룹 소계 2줄")
}

// 거래가 없으면 헬퍼가 없으니 Y:Z 쓰기를 생략하고, 차트도 만들지 않는다.
func TestWriteIndexWeight_NoTradesSkipsHelperWrite(t *testing.T) {
	f := newFakeValues()
	g := newFakeGenerator(t, f)

	next, err := g.writeIndexWeight(context.Background(), nil, 5)
	require.NoError(t, err)

	// 제목행 + 안내행 + 컬럼헤더만 → 5..7
	assert.Equal(t, []string{"대시보드!A5:E7"}, f.recorded(), "Y:Z 쓰기는 생략된다")
	assert.Equal(t, 8, next)
	assert.False(t, g.indexWeightPie.ok)

	// 데이터 행이 없으니 숫자 포맷도 걸지 않는다(제목/헤더 행에 통화·백분율 포맷 금지).
	assert.Zero(t, countNumberFormatRequests(g), "데이터 행이 없으면 숫자 포맷 요청도 없다")
}

// 전량 매도라 보유원금이 전부 0 이면 빈 파이가 그려지므로 차트를 만들지 않는다.
// (헬퍼 행 자체는 있으므로 len(helper) > 1 만으로는 걸러지지 않는다.)
func TestWriteIndexWeight_AllSoldSkipsPieChart(t *testing.T) {
	f := newFakeValues()
	g := newFakeGenerator(t, f)

	trades := []model.Trade{
		{StockName: "QQQM", StockCode: "QQQM", Account: "해외", Currency: "USD",
			Sector: "ETF", Industry: "나스닥", TradeType: "매수", Quantity: 10, AmountKRW: 1_000_000},
		{StockName: "QQQM", StockCode: "QQQM", Account: "해외", Currency: "USD",
			Sector: "ETF", Industry: "나스닥", TradeType: "매도", Quantity: 10, AmountKRW: 1_200_000},
	}
	_, err := g.writeIndexWeight(context.Background(), trades, 5)
	require.NoError(t, err)

	assert.Len(t, f.recorded(), 1, "보유원금이 전부 0 이면 Y:Z 헬퍼를 쓰지 않는다")
	assert.False(t, g.indexWeightPie.ok)
}

// 숫자 포맷은 데이터 행(startRow+3)부터 — 제목행/안내행/컬럼헤더에는 걸지 않는다.
func TestWriteIndexWeight_NumberFormatsStartAtDataRow(t *testing.T) {
	f := newFakeValues()
	g := newFakeGenerator(t, f)

	const startRow = 5
	_, err := g.writeIndexWeight(context.Background(), indexWeightFixture(), startRow)
	require.NoError(t, err)

	// A:E 표 포맷(B·D 원화, C·E 백분율) 4건 + 헬퍼 Z열 1건.
	var tableRows []int64
	for _, req := range g.pendingRequests {
		if req.RepeatCell == nil || req.RepeatCell.Fields != "userEnteredFormat.numberFormat" {
			continue
		}
		gr := req.RepeatCell.Range
		if gr.StartColumnIndex >= 24 { // Y:Z 헬퍼는 별도 범위
			continue
		}
		tableRows = append(tableRows, gr.StartRowIndex)
	}
	require.Len(t, tableRows, 4, "B·D 원화 + C·E 백분율")
	for _, sr := range tableRows {
		// GridRange 는 0-based 이므로 1-based startRow+3 은 startRow+2.
		assert.Equal(t, int64(startRow+2), sr, "데이터 행(1-based %d)부터 포맷", startRow+3)
	}
}

// 표(A:E) 쓰기 실패는 삼켜지지 않고 전파되며, 이전 실행의 차트 범위도 남지 않는다.
func TestWriteIndexWeight_PropagatesTableWriteError(t *testing.T) {
	f := newFakeValues()
	f.failFrom = 0
	g := newFakeGenerator(t, f)
	g.indexWeightPie = rowRange{start: 1, end: 2, ok: true} // 이전 실행 잔재

	next, err := g.writeIndexWeight(context.Background(), indexWeightFixture(), 5)

	require.Error(t, err)
	assert.Zero(t, next)
	assert.False(t, g.indexWeightPie.ok, "표 쓰기가 실패하면 차트 범위를 남기지 않는다")
	assert.Len(t, f.recorded(), 1, "첫 쓰기에서 멈춘다")
}

// 헬퍼(Y:Z) 쓰기만 실패해도 전파된다(표는 이미 써진 뒤라 조용히 넘어가기 쉬운 지점).
func TestWriteIndexWeight_PropagatesHelperWriteError(t *testing.T) {
	f := newFakeValues()
	f.failFrom = 1 // 첫 쓰기(A:E)는 성공, 두 번째(Y:Z)부터 실패
	g := newFakeGenerator(t, f)

	next, err := g.writeIndexWeight(context.Background(), indexWeightFixture(), 5)

	require.Error(t, err)
	assert.Zero(t, next)
	assert.False(t, g.indexWeightPie.ok)
	assert.Equal(t, []string{"대시보드!A5:E17", "대시보드!Y5:Z7"}, f.recorded())
}

func countNumberFormatRequests(g *Generator) int {
	n := 0
	for _, req := range g.pendingRequests {
		if req.RepeatCell != nil && req.RepeatCell.Fields == "userEnteredFormat.numberFormat" {
			n++
		}
	}
	return n
}
