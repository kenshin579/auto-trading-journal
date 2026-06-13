package bizcat

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolve_CacheHitSkipsFetch(t *testing.T) {
	calls := 0
	r := &Resolver{
		cache: map[string]entry{"005930": {Sector: "전기·전자", Industry: "반도체"}},
		fetch: func(code string) (string, string, error) { calls++; return "", "", nil },
	}
	s, i := r.Resolve("005930")
	assert.Equal(t, "전기·전자", s)
	assert.Equal(t, "반도체", i)
	assert.Equal(t, 0, calls)
}

func TestResolve_MissCallsFetchAndCaches(t *testing.T) {
	calls := 0
	r := &Resolver{
		cache: map[string]entry{},
		fetch: func(code string) (string, string, error) { calls++; return "금융", "은행", nil },
	}
	s, i := r.Resolve("105560")
	assert.Equal(t, "금융", s)
	assert.Equal(t, "은행", i)
	r.Resolve("105560")
	assert.Equal(t, 1, calls)
}

func TestResolve_EmptyCodeOrErrorReturnsBlank(t *testing.T) {
	r := &Resolver{cache: map[string]entry{}, fetch: func(string) (string, string, error) { return "", "", assert.AnError }}
	s, i := r.Resolve("")
	assert.Equal(t, "", s)
	assert.Equal(t, "", i)
	s, i = r.Resolve("000000")
	assert.Equal(t, "", s)
	assert.Equal(t, "", i)
}

// 섹터 = 지수업종 중분류, 산업 = 표준산업분류(일반 종목 커버리지가 넓음).
// 지수업종이 비어도(클래시스/솔루엠 등) 표준산업분류로 산업은 채워져야 한다.
func TestPickSectorIndustry(t *testing.T) {
	// 대형주: 둘 다 채워짐
	s, i := pickSectorIndustry("전기,전자", "통신 및 방송 장비 제조업")
	assert.Equal(t, "전기,전자", s)
	assert.Equal(t, "통신 및 방송 장비 제조업", i)

	// 지수업종 미분류 일반 종목(클래시스): 섹터 빈값이지만 산업은 표준산업분류로 채워짐
	s, i = pickSectorIndustry("", "의료용 기기 제조업")
	assert.Equal(t, "", s)
	assert.Equal(t, "의료용 기기 제조업", i)

	// ETF: 둘 다 빈값
	s, i = pickSectorIndustry("", "")
	assert.Equal(t, "", s)
	assert.Equal(t, "", i)
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
