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
	r := &Resolver{cache: map[string]entry{}, fetch: func(symbol string) (profile, error) {
		calls++
		return profile{Sector: "X", Industry: "Y"}, nil
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
	r := &Resolver{cache: map[string]entry{}, fetch: func(symbol string) (profile, error) {
		calls++
		gotSymbol = symbol
		return profile{Sector: "Technology", Industry: "Consumer Electronics"}, nil
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
	r := &Resolver{cache: map[string]entry{}, fetch: func(symbol string) (profile, error) {
		gotSymbol = symbol
		return profile{Sector: "Consumer Cyclical", Industry: "Auto - Manufacturers"}, nil
	}}
	s, i := r.Resolve("7203", "JPY")
	assert.Equal(t, "7203.T", gotSymbol, "JPY 는 .T 접미사")
	assert.Equal(t, "Consumer Cyclical", s)
	assert.Equal(t, "Auto - Manufacturers", i)
}

// not-found(fetch 가 zero profile+nil 반환)는 같은 실행 안에서는 재조회하지 않는다.
// 파일에는 남지 않으므로(saveCache 가 빈 항목 제외) 다음 실행에 1회 재시도된다.
func TestResolve_NotFoundCachedEmpty(t *testing.T) {
	calls := 0
	r := &Resolver{cache: map[string]entry{}, fetch: func(symbol string) (profile, error) {
		calls++
		return profile{}, nil
	}}
	s, i := r.Resolve("1321", "JPY")
	assert.Equal(t, "", s)
	assert.Equal(t, "", i)
	r.Resolve("1321", "JPY")
	assert.Equal(t, 1, calls, "빈 결과도 같은 실행 내 재조회 안 함")
}

// 일시적 오류는 negative-cache(이번 실행만) → 같은 실행 내 재조회 안 함.
func TestResolve_ErrorNegativeCache(t *testing.T) {
	calls := 0
	r := &Resolver{cache: map[string]entry{}, fetch: func(symbol string) (profile, error) {
		calls++
		return profile{}, assert.AnError
	}}
	s, i := r.Resolve("AAPL", "USD")
	assert.Equal(t, "", s)
	assert.Equal(t, "", i)
	r.Resolve("AAPL", "USD")
	assert.Equal(t, 1, calls, "실패 심볼은 같은 실행 내 1회만")
}

// FMP 클라이언트가 없으면(키 미설정) 조회는 "성공+빈 값"이 아니라 실패여야 한다.
// 빈 값을 성공으로 캐시하면 캐시 항목이 전부 빈 값으로 덮이고, saveCache 가 빈 항목을
// 제외하므로 git 추적 캐시 파일이 통째로 비워진다(시트도 백필 전까지 공란).
func TestNoClientFetch_ReturnsError(t *testing.T) {
	_, err := noClientFetch("AAPL")
	require.Error(t, err, "클라이언트 없음은 성공이 아니다")
	assert.ErrorIs(t, err, errNoFMPClient)
}

// 클라이언트가 없으면 기존 캐시 항목을 빈 값으로 덮지 않는다(구버전 항목도 그대로 둔다).
func TestResolve_NoClientDoesNotOverwriteCache(t *testing.T) {
	old := entry{Sector: "Financial Services", Industry: "Asset Management - Global"} // v0 → 재조회 대상
	r := &Resolver{cache: map[string]entry{"QQQM": old}, fetch: noClientFetch}
	r.Resolve("QQQM", "USD")
	assert.Equal(t, old, r.cache["QQQM"], "캐시가 덮이지 않음")
	assert.False(t, r.dirty, "실패는 파일에 기록되지 않음")
}

// 캐시가 없던 심볼도 빈 값으로 캐시되지 않는다(다음 실행에 재시도해야 한다).
func TestResolve_NoClientDoesNotCacheEmpty(t *testing.T) {
	r := &Resolver{cache: map[string]entry{}, fetch: noClientFetch}
	s, i := r.Resolve("AAPL", "USD")
	assert.Equal(t, "", s)
	assert.Equal(t, "", i)
	assert.NotContains(t, r.cache, "AAPL", "빈 값을 캐시하지 않는다")
	assert.False(t, r.dirty)
}

// staleSafe: 구버전 캐시 값은 ETF 판별(isEtf) 이전 스키마라 실패 시 그대로 쓰면
// ETF 가 개별종목으로 오분류된다(섹터 "Financial Services" → bucketOf 는 개별종목).
// 현재 버전 값만 보존하고 구버전은 빈 값(미분류)으로 내보낸다.
func TestStaleSafe(t *testing.T) {
	s, i := staleSafe(entry{Sector: "Financial Services", Industry: "Asset Management - Global"}) // v0
	assert.Equal(t, "", s, "구버전 ETF 값은 개별종목으로 오분류되므로 미분류로 보낸다")
	assert.Equal(t, "", i)

	s, i = staleSafe(entry{Sector: "ETF", Industry: "나스닥", IsETF: true, Version: cacheVersion})
	assert.Equal(t, "ETF", s, "현재 버전 값은 보존")
	assert.Equal(t, "나스닥", i)

	s, i = staleSafe(entry{}) // 캐시 없음
	assert.Equal(t, "", s)
	assert.Equal(t, "", i)
}

// 구버전 캐시 + 재조회 실패 → 미분류(빈 값). 캐시 항목 자체는 손대지 않는다(다음 실행 자가치유).
// 그대로 돌려주면 QQQM 같은 지수 ETF 가 조용히 개별종목으로 시트·대시보드에 기록되고,
// logIndexWeightDiag 는 미분류만 경고하므로 아무 신호도 남지 않는다.
func TestResolve_StaleCacheFetchFailureReturnsBlank(t *testing.T) {
	old := entry{Sector: "Financial Services", Industry: "Asset Management - Global"} // v0 ETF
	r := &Resolver{cache: map[string]entry{"QQQM": old}, fetch: func(string) (profile, error) {
		return profile{}, assert.AnError
	}}
	s, i := r.Resolve("QQQM", "USD")
	assert.Equal(t, "", s)
	assert.Equal(t, "", i)
	assert.Equal(t, old, r.cache["QQQM"], "캐시 항목은 그대로 둔다")

	// negative-cache 경유(같은 실행 2회차)도 같은 값이어야 한다 —
	// 같은 티커가 행마다 다른 값으로 시트에 들어가면 안 된다.
	s, i = r.Resolve("QQQM", "USD")
	assert.Equal(t, "", s)
	assert.Equal(t, "", i)
}

// 분류 실패 경로도 마찬가지 — 구버전 캐시 값으로 되돌아가지 않는다.
func TestResolve_StaleCacheClassifyFailureReturnsBlank(t *testing.T) {
	old := entry{Sector: "Financial Services", Industry: "Asset Management - Income"} // v0 ETF
	r := &Resolver{
		cache: map[string]entry{"SCHD": old},
		fetch: func(string) (profile, error) {
			return profile{Sector: "Financial Services", Industry: "Asset Management - Income",
				Name: "Schwab US Dividend Equity ETF", IsETF: true}, nil
		},
		classifyETF: func(string) (string, error) { return "", assert.AnError },
	}
	s, i := r.Resolve("SCHD", "USD")
	assert.Equal(t, "", s)
	assert.Equal(t, "", i)
	assert.Equal(t, old, r.cache["SCHD"], "캐시 항목은 그대로 둔다")
}

// 빈 결과(미커버/미지원)는 파일에 영구화하지 않는다(노이즈 방지).
func TestSaveCache_OmitsEmptyEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fmpcat_cache.json")
	r := &Resolver{cache: map[string]entry{
		"AAPL":   {Sector: "Technology", Industry: "Consumer Electronics"},
		"1321.T": {Sector: "", Industry: ""},
	}, cachePath: path}
	require.NoError(t, r.saveCache())
	data, _ := os.ReadFile(path)
	assert.Contains(t, string(data), "AAPL")
	assert.NotContains(t, string(data), "1321.T", "빈 항목은 파일에 안 씀")
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

// ETF 는 국내(bizcat)와 같은 표기로 통일한다 — 섹터="ETF", 산업=taxonomy 카테고리.
func TestResolve_ETFUsesClassifier(t *testing.T) {
	var gotName string
	r := &Resolver{
		cache: map[string]entry{},
		fetch: func(symbol string) (profile, error) {
			return profile{
				Sector:   "Financial Services",
				Industry: "Asset Management - Income",
				Name:     "Schwab US Dividend Equity ETF",
				IsETF:    true,
			}, nil
		},
		classifyETF: func(name string) (string, error) {
			gotName = name
			return "배당", nil
		},
	}
	s, i := r.Resolve("SCHD", "USD")
	assert.Equal(t, "ETF", s)
	assert.Equal(t, "배당", i)
	assert.Equal(t, "Schwab US Dividend Equity ETF", gotName, "회사명으로 분류")
	assert.True(t, r.cache["SCHD"].IsETF)
}

// IsFund 도 ETF 로 취급한다(뮤추얼펀드형 상품).
func TestResolve_FundTreatedAsETF(t *testing.T) {
	r := &Resolver{
		cache: map[string]entry{},
		fetch: func(symbol string) (profile, error) {
			return profile{Sector: "Financial Services", Industry: "Asset Management", Name: "Some Fund", IsFund: true}, nil
		},
		classifyETF: func(name string) (string, error) { return "글로벌주식", nil },
	}
	s, i := r.Resolve("XXXX", "USD")
	assert.Equal(t, "ETF", s)
	assert.Equal(t, "글로벌주식", i)
}

// 분류기가 없으면(키 미설정) FMP 원본 산업으로 폴백한다(섹터는 ETF 유지).
func TestResolve_ETFWithoutClassifierFallsBack(t *testing.T) {
	r := &Resolver{cache: map[string]entry{}, fetch: func(symbol string) (profile, error) {
		return profile{Sector: "Financial Services", Industry: "Asset Management - Bonds", Name: "iShares 20+ Year Treasury Bond ETF", IsETF: true}, nil
	}}
	s, i := r.Resolve("TLT", "USD")
	assert.Equal(t, "ETF", s)
	assert.Equal(t, "Asset Management - Bonds", i, "분류기 nil → FMP 산업 폴백")
}

// 분류기가 있는데 실패하면 캐시하지 않는다 — taxonomy 밖 값을 영구 캐시하면
// 그 종목이 다음 실행에도 재조회되지 않아 대시보드에서 영구히 미분류로 남는다.
func TestResolve_ClassifyFailureDoesNotPoisonCache(t *testing.T) {
	r := &Resolver{cache: map[string]entry{}, fetch: func(symbol string) (profile, error) {
		return profile{Sector: "Financial Services", Industry: "Asset Management - Bonds", Name: "iShares 20+ Year Treasury Bond ETF", IsETF: true}, nil
	}, classifyETF: func(name string) (string, error) { return "", assert.AnError }}
	s, i := r.Resolve("TLT", "USD")
	assert.Equal(t, "", s)
	assert.Equal(t, "", i)
	assert.NotContains(t, r.cache, "TLT", "실패는 캐시에 남지 않는다")
	assert.True(t, r.failed["TLT"], "같은 실행 내 재조회는 막는다")
}

// 일반 종목(BDC·자산운용사 포함)은 기존대로 FMP 섹터/산업을 쓴다.
func TestResolve_NonETFKeepsFMPValues(t *testing.T) {
	r := &Resolver{cache: map[string]entry{}, fetch: func(symbol string) (profile, error) {
		return profile{Sector: "Financial Services", Industry: "Asset Management", Name: "Main Street Capital Corporation"}, nil
	}}
	s, i := r.Resolve("MAIN", "USD")
	assert.Equal(t, "Financial Services", s, "BDC 는 ETF 가 아니다")
	assert.Equal(t, "Asset Management", i)
}

// 버전 없는 구캐시(isEtf 정보 없음)는 1회 재조회된다.
func TestNeedsRefresh_OldCacheVersion(t *testing.T) {
	assert.True(t, needsRefresh(entry{Sector: "Financial Services", Industry: "Asset Management"}), "v0 재조회")
	assert.False(t, needsRefresh(entry{Sector: "Technology", Industry: "Semiconductors", Version: cacheVersion}))
}

func TestResolve_StaleCacheRefetches(t *testing.T) {
	calls := 0
	r := &Resolver{
		cache: map[string]entry{"SCHD": {Sector: "Financial Services", Industry: "Asset Management - Income"}},
		fetch: func(symbol string) (profile, error) {
			calls++
			return profile{Sector: "Financial Services", Industry: "Asset Management - Income", Name: "Schwab US Dividend Equity ETF", IsETF: true}, nil
		},
		classifyETF: func(name string) (string, error) { return "배당", nil },
	}
	s, i := r.Resolve("SCHD", "USD")
	assert.Equal(t, 1, calls, "구버전 캐시는 재조회")
	assert.Equal(t, "ETF", s)
	assert.Equal(t, "배당", i)
	r.Resolve("SCHD", "USD")
	assert.Equal(t, 1, calls, "갱신 후엔 캐시 히트")
}
