package sheets

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/googleapi"
)

// stubRetrySleep 은 테스트 동안 실제 대기를 없애고 호출된 대기 시간을 기록한다.
func stubRetrySleep(t *testing.T) *[]time.Duration {
	t.Helper()
	orig := retrySleep
	var waits []time.Duration
	retrySleep = func(ctx context.Context, d time.Duration) error {
		waits = append(waits, d)
		return nil
	}
	t.Cleanup(func() { retrySleep = orig })
	return &waits
}

func rateLimitErr() error {
	return &googleapi.Error{Code: 429, Message: "Quota exceeded for quota metric 'Read requests'"}
}

// Sheets 쿼터는 분당 리셋이므로, 재시도 대기 누적이 60초 이상이어야 버킷 회복을 기다릴 수 있다.
func TestRetryWaitBudgetCoversQuotaWindow(t *testing.T) {
	var total time.Duration
	for attempt := 0; attempt < maxRetries; attempt++ {
		total += retryWait(attempt)
	}
	assert.GreaterOrEqual(t, total, 60*time.Second,
		"재시도 누적 대기가 쿼터 창(60초)보다 짧으면 429 를 넘길 수 없다")
}

func TestExecuteWithRetryRetriesOnRateLimit(t *testing.T) {
	waits := stubRetrySleep(t)
	calls := 0

	err := executeWithRetry(context.Background(), func() error {
		calls++
		return rateLimitErr()
	})

	assert.Error(t, err)
	assert.Equal(t, maxRetries+1, calls, "최초 1회 + maxRetries 회 재시도해야 한다")
	assert.Len(t, *waits, maxRetries)
}

func TestExecuteWithRetrySucceedsAfterRateLimit(t *testing.T) {
	stubRetrySleep(t)
	calls := 0

	err := executeWithRetry(context.Background(), func() error {
		calls++
		if calls < 3 {
			return rateLimitErr()
		}
		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, 3, calls)
}

func TestExecuteWithRetryStopsOnNonRetryable(t *testing.T) {
	stubRetrySleep(t)
	calls := 0

	err := executeWithRetry(context.Background(), func() error {
		calls++
		return &googleapi.Error{Code: 403, Message: "forbidden"}
	})

	assert.Error(t, err)
	assert.Equal(t, 1, calls, "403 은 재시도 대상이 아니다")
}

func TestExecuteWithRetryHonorsContextCancel(t *testing.T) {
	orig := retrySleep
	retrySleep = func(ctx context.Context, d time.Duration) error { return context.Canceled }
	t.Cleanup(func() { retrySleep = orig })

	err := executeWithRetry(context.Background(), func() error { return rateLimitErr() })
	assert.ErrorIs(t, err, context.Canceled)
}

// isRetryable 은 래핑된 에러(fmt.Errorf %w)에서도 429 를 인식해야 한다.
func TestIsRetryableUnwrapsWrappedError(t *testing.T) {
	assert.True(t, isRetryable(fmt.Errorf("시트 데이터 조회 실패: %w", rateLimitErr())))
	assert.False(t, isRetryable(errors.New("plain")))
}

// ── 읽기 경로 재시도 (fake Sheets 엔드포인트) ──────────────────────

// newFakeClient 는 httptest 서버를 Sheets 엔드포인트로 쓰는 Client 를 만든다.
func newFakeClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c, err := NewWithEndpoint(context.Background(), "test-sheet", srv.URL)
	require.NoError(t, err)
	return c
}

func writeQuotaExceeded(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	_, _ = w.Write([]byte(`{"error":{"code":429,"message":"Quota exceeded","status":"RESOURCE_EXHAUSTED"}}`))
}

// GetValues 는 429 를 만나면 재시도해서 성공해야 한다(현재는 재시도 없이 즉시 실패).
func TestGetValuesRetriesOnRateLimit(t *testing.T) {
	stubRetrySleep(t)
	var hits int32

	c := newFakeClient(t, func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			writeQuotaExceeded(w)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"range":"A1:Q1","values":[["일자","구분"]]}`))
	})

	vals, err := c.GetValues(context.Background(), "시트!A1:Q1")
	require.NoError(t, err)
	assert.Equal(t, int32(2), atomic.LoadInt32(&hits))
	assert.Equal(t, [][]interface{}{{"일자", "구분"}}, vals)
}

// GetRawGridData / GetSpreadsheetMetadata 도 동일하게 재시도해야 한다.
func TestGridAndMetadataRetryOnRateLimit(t *testing.T) {
	stubRetrySleep(t)
	var hits int32

	c := newFakeClient(t, func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1)%2 == 1 {
			writeQuotaExceeded(w)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sheets":[{"properties":{"title":"시트","sheetId":7},` +
			`"data":[{"rowData":[{"values":[{"formattedValue":"2026-01-02"}]}]}]}]}`))
	})

	grid, err := c.GetRawGridData(context.Background(), "시트", "A1:Q10")
	require.NoError(t, err)
	require.Len(t, grid.RowData, 1)

	meta, err := c.GetSpreadsheetMetadata(context.Background())
	require.NoError(t, err)
	require.Len(t, meta.Sheets, 1)

	assert.Equal(t, int32(4), atomic.LoadInt32(&hits), "각 호출이 1회씩 재시도되어야 한다")
}

// 값 쓰기(ClearValues/UpdateCells/BatchUpdateValues)도 쓰기 쿼터(분당 60)를 공유하므로 재시도 대상이다.
func TestValueWritesRetryOnRateLimit(t *testing.T) {
	stubRetrySleep(t)
	var hits int32

	c := newFakeClient(t, func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1)%2 == 1 {
			writeQuotaExceeded(w)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	})

	ctx := context.Background()
	require.NoError(t, c.ClearValues(ctx, "시트!A1:Z"))
	require.NoError(t, c.UpdateCells(ctx, "시트!A1", [][]interface{}{{"a"}}))
	require.NoError(t, c.BatchUpdateValues(ctx, map[string][][]interface{}{"시트!A1": {{"a"}}}))

	assert.Equal(t, int32(6), atomic.LoadInt32(&hits))
}
