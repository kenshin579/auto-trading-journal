package bizcat

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kenshin579/korea-investment-stock/domestic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolve_CacheHitSkipsFetch(t *testing.T) {
	calls := 0
	r := &Resolver{
		cache: map[string]entry{"005930": {Sector: "전기·전자", Industry: "반도체"}},
		fetch: func(code, name string) (string, string, error) { calls++; return "", "", nil },
	}
	s, i := r.Resolve("005930", "삼성전자")
	assert.Equal(t, "전기·전자", s)
	assert.Equal(t, "반도체", i)
	assert.Equal(t, 0, calls)
}

func TestResolve_MissCallsFetchAndCaches(t *testing.T) {
	calls := 0
	r := &Resolver{
		cache: map[string]entry{},
		fetch: func(code, name string) (string, string, error) { calls++; return "금융", "은행", nil },
	}
	s, i := r.Resolve("105560", "KB금융")
	assert.Equal(t, "금융", s)
	assert.Equal(t, "은행", i)
	r.Resolve("105560", "KB금융")
	assert.Equal(t, 1, calls)
}

// 같은 실행 내에서 한 번 실패한 코드는 재조회하지 않는다(negative cache).
// (영구 캐시엔 저장 안 함 — 다음 실행에는 다시 시도해야 하므로.)
func TestResolve_NegativeCacheSkipsRefetchOnError(t *testing.T) {
	calls := 0
	r := &Resolver{cache: map[string]entry{}, fetch: func(code, name string) (string, string, error) {
		calls++
		return "", "", assert.AnError
	}}
	s, i := r.Resolve("999999", "유령") // 1회차: fetch 실패
	assert.Equal(t, "", s)
	assert.Equal(t, "", i)
	r.Resolve("999999", "유령") // 2회차: negative cache 로 재조회 안 함
	assert.Equal(t, 1, calls, "실패 코드는 같은 실행 내 1회만 호출")
}

func TestResolve_EmptyCodeOrErrorReturnsBlank(t *testing.T) {
	r := &Resolver{cache: map[string]entry{}, fetch: func(code, name string) (string, string, error) { return "", "", assert.AnError }}
	s, i := r.Resolve("", "")
	assert.Equal(t, "", s)
	assert.Equal(t, "", i)
	s, i = r.Resolve("000000", "x")
	assert.Equal(t, "", s)
	assert.Equal(t, "", i)
}

// 섹터 = InquirePrice 업종 한글명(bstp_kor_isnm), 산업 = 표준산업분류(std_idst_clsf_cd_name).
// ETF 는 섹터="ETF", 산업 = 사전 결정된 etfIndustry(resolveETFIndustry 결과).
func TestExtractSectorIndustry(t *testing.T) {
	// 대형주: 둘 다 채워짐
	s, i := extractSectorIndustry(
		&domestic.Price{BstpKorIsnm: "전기·전자"},
		&domestic.StockInfo{StdIdstClsfCdName: "반도체 제조업"},
		"",
	)
	assert.Equal(t, "전기·전자", s)
	assert.Equal(t, "반도체 제조업", i)

	// 지수업종 미분류 일반 종목(클래시스)도 bstp_kor_isnm 으로 섹터가 채워짐
	s, i = extractSectorIndustry(
		&domestic.Price{BstpKorIsnm: "의료·정밀기기"},
		&domestic.StockInfo{StdIdstClsfCdName: "의료용 기기 제조업"},
		"",
	)
	assert.Equal(t, "의료·정밀기기", s)
	assert.Equal(t, "의료용 기기 제조업", i)

	// ETF: 섹터="ETF", 산업=사전 결정 분류
	s, i = extractSectorIndustry(
		&domestic.Price{BstpKorIsnm: "ETF(실물복제/수익증권)"},
		&domestic.StockInfo{EtfDvsnCd: "1"},
		"반도체",
	)
	assert.Equal(t, "ETF", s)
	assert.Equal(t, "반도체", i)
}

// resolveETFIndustry: 코드 분류(≠9999)는 KRX 분류명, 9999 는 분류기 결과, 분류기 실패/미설정 시 지수명 폴백.
func TestResolveETFIndustry(t *testing.T) {
	// 코드 분류된 국내 섹터 ETF → KRX 접두사 제거된 분류명
	got := resolveETFIndustry("4003", "KRX 반도체", "KODEX 반도체", nil)
	assert.Equal(t, "반도체", got)

	// 9999 + 분류기 → 분류기 결과
	got = resolveETFIndustry("9999", "S&P 500", "KODEX 미국S&P500",
		func(name string) (string, error) { return "미국주식", nil })
	assert.Equal(t, "미국주식", got)

	// 9999 + 분류기 실패 → 지수명 폴백
	got = resolveETFIndustry("9999", "S&P 500", "KODEX 미국S&P500",
		func(name string) (string, error) { return "", assert.AnError })
	assert.Equal(t, "S&P 500", got)

	// 9999 + 분류기 미설정(nil) → 지수명 폴백
	got = resolveETFIndustry("9999", "Nikkei 225", "TIGER 일본니케이225", nil)
	assert.Equal(t, "Nikkei 225", got)
}

func TestStripKRXPrefix(t *testing.T) {
	assert.Equal(t, "반도체", stripKRXPrefix("KRX 반도체"))
	assert.Equal(t, "금현물지수", stripKRXPrefix("KRX 금현물지수"))
	assert.Equal(t, "종합", stripKRXPrefix("종합"))
	assert.Equal(t, "", stripKRXPrefix(""))
}

func TestValidateETFCategory(t *testing.T) {
	assert.Equal(t, "미국주식", validateETFCategory("미국주식"))
	assert.Equal(t, "방위·우주항공", validateETFCategory("방위·우주항공"))
	assert.Equal(t, etfFallbackCategory, validateETFCategory("아무말"))
	assert.Equal(t, etfFallbackCategory, validateETFCategory(""))
}

// needsRefresh 는 구버전 ETF 캐시(version < etfCacheVersion)만 재조회 대상으로 본다.
func TestNeedsRefresh(t *testing.T) {
	assert.True(t, needsRefresh(entry{Sector: "ETF", Industry: "S&P 500", Version: 0}), "구버전 ETF 재조회")
	assert.True(t, needsRefresh(entry{Sector: "ETF", Industry: ""}), "산업 빈 구버전 ETF 재조회")
	assert.False(t, needsRefresh(entry{Sector: "ETF", Industry: "미국주식", Version: etfCacheVersion}), "신버전 ETF 유지")
	assert.False(t, needsRefresh(entry{Sector: "전기·전자", Industry: "반도체"}), "비-ETF 는 재조회 안 함")
}

// 구버전 ETF 캐시(지수명 산업, version 0)는 재조회되어 새 분류로 갱신되고, 이후엔 캐시 사용.
func TestResolve_StaleETFRefetchesThenCaches(t *testing.T) {
	calls := 0
	r := &Resolver{
		cache: map[string]entry{"379780": {Sector: "ETF", Industry: "S&P 500"}}, // version 0
		fetch: func(code, name string) (string, string, error) { calls++; return "ETF", "미국주식", nil },
	}
	s, i := r.Resolve("379780", "RISE 미국S&P500")
	assert.Equal(t, "ETF", s)
	assert.Equal(t, "미국주식", i)
	r.Resolve("379780", "RISE 미국S&P500") // 갱신 후엔 캐시 히트
	assert.Equal(t, 1, calls, "재조회는 1회만")
}

// 자가치유 재조회가 실패하면 기존 ETF 캐시(섹터+산업)는 보존한다.
func TestResolve_StaleETFRefetchFailureKeepsCached(t *testing.T) {
	r := &Resolver{
		cache: map[string]entry{"379780": {Sector: "ETF", Industry: "S&P 500"}},
		fetch: func(code, name string) (string, string, error) { return "", "", assert.AnError },
	}
	s, i := r.Resolve("379780", "RISE 미국S&P500")
	assert.Equal(t, "ETF", s)
	assert.Equal(t, "S&P 500", i)
}

func TestCacheSaveLoad_Roundtrip_PreservesKorean(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bizcat_cache.json")
	r := &Resolver{cache: map[string]entry{"005930": {Sector: "전기·전자", Industry: "반도체"}}, cachePath: path}
	require.NoError(t, r.saveCache())
	data, _ := os.ReadFile(path)
	assert.Contains(t, string(data), "전기·전자")
	assert.NotContains(t, string(data), "\\u")

	r2 := New(path)
	s, _ := r2.cacheLookup("005930")
	assert.Equal(t, "전기·전자", s)
}
