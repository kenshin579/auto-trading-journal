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

// 섹터 = InquirePrice 업종 한글명(bstp_kor_isnm), 산업 = 표준산업분류(std_idst_clsf_cd_name).
// bstp_kor_isnm 은 지수업종이 비는 일반 종목(클래시스 등)도 채워져 커버리지가 넓다.
func TestExtractSectorIndustry(t *testing.T) {
	// 대형주: 둘 다 채워짐
	s, i := extractSectorIndustry(
		&domestic.Price{BstpKorIsnm: "전기·전자"},
		&domestic.StockInfo{StdIdstClsfCdName: "반도체 제조업"},
	)
	assert.Equal(t, "전기·전자", s)
	assert.Equal(t, "반도체 제조업", i)

	// 지수업종 미분류 일반 종목(클래시스)도 bstp_kor_isnm 으로 섹터가 채워짐
	s, i = extractSectorIndustry(
		&domestic.Price{BstpKorIsnm: "의료·정밀기기"},
		&domestic.StockInfo{StdIdstClsfCdName: "의료용 기기 제조업"},
	)
	assert.Equal(t, "의료·정밀기기", s)
	assert.Equal(t, "의료용 기기 제조업", i)

	// ETF: bstp_kor_isnm 은 "ETF(...)", 표준산업분류는 빈값
	s, i = extractSectorIndustry(
		&domestic.Price{BstpKorIsnm: "ETF(실물복제/수익증권)"},
		&domestic.StockInfo{},
	)
	assert.Equal(t, "ETF(실물복제/수익증권)", s)
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
