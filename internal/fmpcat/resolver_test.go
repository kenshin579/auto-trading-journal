package fmpcat

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExchangeSuffix(t *testing.T) {
	s, ok := exchangeSuffix("USD")
	assert.Equal(t, "", s)
	assert.True(t, ok)

	s, ok = exchangeSuffix("JPY")
	assert.Equal(t, ".T", s)
	assert.True(t, ok)

	_, ok = exchangeSuffix("EUR")
	assert.False(t, ok, "미지원 통화")
	_, ok = exchangeSuffix("")
	assert.False(t, ok)
}

// 미지원 통화는 FMP 호출 없이 공란 반환.
func TestResolve_UnsupportedCurrencySkipsFetch(t *testing.T) {
	calls := 0
	r := &Resolver{cache: map[string]entry{}, fetch: func(symbol string) (string, string, error) {
		calls++
		return "X", "Y", nil
	}}
	s, i := r.Resolve("BABA", "HKD")
	assert.Equal(t, "", s)
	assert.Equal(t, "", i)
	assert.Equal(t, 0, calls, "미지원 통화는 fetch 안 함")
}

func TestResolve_EmptyTickerReturnsBlank(t *testing.T) {
	r := &Resolver{cache: map[string]entry{}}
	s, i := r.Resolve("", "USD")
	assert.Equal(t, "", s)
	assert.Equal(t, "", i)
}

// USD 는 접미사 없이 티커 그대로 조회, 결과 캐시.
func TestResolve_USMissCachesThenHit(t *testing.T) {
	calls := 0
	var gotSymbol string
	r := &Resolver{cache: map[string]entry{}, fetch: func(symbol string) (string, string, error) {
		calls++
		gotSymbol = symbol
		return "Technology", "Consumer Electronics", nil
	}}
	s, i := r.Resolve("AAPL", "USD")
	assert.Equal(t, "Technology", s)
	assert.Equal(t, "Consumer Electronics", i)
	assert.Equal(t, "AAPL", gotSymbol, "USD 는 접미사 없음")
	r.Resolve("AAPL", "USD")
	assert.Equal(t, 1, calls, "캐시 히트")
}

// JPY 는 .T 접미사를 붙여 조회.
func TestResolve_JPYAppendsSuffix(t *testing.T) {
	var gotSymbol string
	r := &Resolver{cache: map[string]entry{}, fetch: func(symbol string) (string, string, error) {
		gotSymbol = symbol
		return "Consumer Cyclical", "Auto - Manufacturers", nil
	}}
	s, i := r.Resolve("7203", "JPY")
	assert.Equal(t, "7203.T", gotSymbol, "JPY 는 .T 접미사")
	assert.Equal(t, "Consumer Cyclical", s)
	assert.Equal(t, "Auto - Manufacturers", i)
}

// not-found(fetch 가 빈 값+nil 반환)는 빈 값으로 캐시되어 재조회하지 않는다(해외 ETF 등).
func TestResolve_NotFoundCachedEmpty(t *testing.T) {
	calls := 0
	r := &Resolver{cache: map[string]entry{}, fetch: func(symbol string) (string, string, error) {
		calls++
		return "", "", nil
	}}
	s, i := r.Resolve("1321", "JPY")
	assert.Equal(t, "", s)
	assert.Equal(t, "", i)
	r.Resolve("1321", "JPY")
	assert.Equal(t, 1, calls, "빈 결과도 캐시되어 재조회 안 함")
}

// 일시적 오류는 negative-cache(이번 실행만) → 같은 실행 내 재조회 안 함.
func TestResolve_ErrorNegativeCache(t *testing.T) {
	calls := 0
	r := &Resolver{cache: map[string]entry{}, fetch: func(symbol string) (string, string, error) {
		calls++
		return "", "", assert.AnError
	}}
	s, i := r.Resolve("AAPL", "USD")
	assert.Equal(t, "", s)
	assert.Equal(t, "", i)
	r.Resolve("AAPL", "USD")
	assert.Equal(t, 1, calls, "실패 심볼은 같은 실행 내 1회만")
}

func TestCacheSaveLoad_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fmpcat_cache.json")
	r := &Resolver{cache: map[string]entry{"AAPL": {Sector: "Technology", Industry: "Consumer Electronics"}}, cachePath: path}
	require.NoError(t, r.saveCache())
	data, _ := os.ReadFile(path)
	assert.Contains(t, string(data), "Consumer Electronics")

	r2 := New(path)
	s, ok := r2.cacheLookup("AAPL")
	assert.True(t, ok)
	assert.Equal(t, "Technology", s)
}
