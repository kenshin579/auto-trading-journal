package sector

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenshin579/auto-trading-journal/internal/summary"
)

func TestValidateSectors(t *testing.T) {
	out := validateSectors(
		map[string]string{"삼성전자": "IT", "X": "없는섹터"},
		[]string{"삼성전자", "X"},
	)
	assert.Equal(t, "IT", out["삼성전자"])
	assert.Equal(t, "기타", out["X"]) // 알 수 없는 섹터 → 기타
}

func TestValidateSectors_MissingName(t *testing.T) {
	// 응답에 없는 종목은 "기타"로 채운다.
	out := validateSectors(
		map[string]string{"삼성전자": "IT"},
		[]string{"삼성전자", "카카오"},
	)
	assert.Equal(t, "IT", out["삼성전자"])
	assert.Equal(t, "기타", out["카카오"])
}

func TestValidateSectors_AllValid(t *testing.T) {
	in := map[string]string{"삼성전자": "IT", "현대차": "경기소비재", "KB금융": "금융"}
	out := validateSectors(in, []string{"삼성전자", "현대차", "KB금융"})
	assert.Equal(t, in, out)
}

func TestCacheRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "sector_cache.json")

	c := &Classifier{cachePath: path, cache: map[string]string{
		"삼성전자": "IT",
		"현대차":  "경기소비재",
	}}
	require.NoError(t, c.saveCache())

	// 한글이 escape 되지 않고 그대로 저장되는지 (ensure_ascii=False 호환).
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.True(t, strings.Contains(string(raw), "삼성전자"), "Korean should be unescaped")
	assert.False(t, strings.Contains(string(raw), `\u`), "no unicode escapes expected")

	// {종목명: 섹터} 형식 유지.
	var parsed map[string]string
	require.NoError(t, json.Unmarshal(raw, &parsed))
	assert.Equal(t, "IT", parsed["삼성전자"])

	// New 로 다시 로드하면 동일 맵.
	c2 := New("dummy-key", "", path)
	assert.Equal(t, "IT", c2.cache["삼성전자"])
	assert.Equal(t, "경기소비재", c2.cache["현대차"])
}

func TestLoadCache_MissingFile(t *testing.T) {
	got := loadCache(filepath.Join(t.TempDir(), "does-not-exist.json"))
	assert.Empty(t, got)
	assert.NotNil(t, got)
}

func TestLoadCache_Corrupt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corrupt.json")
	require.NoError(t, os.WriteFile(path, []byte("{ not json"), 0o644))
	got := loadCache(path)
	assert.Empty(t, got) // 손상 → 빈 맵
}

// TestClassify_FullyCached 는 모든 종목이 캐시에 있을 때
// OpenAI 클라이언트를 전혀 건드리지 않고 캐시에서 반환하는지 검증한다.
// client 를 nil 로 둔 채 호출해도 패닉이 없어야 한다(네트워크 미접근).
func TestClassify_FullyCached(t *testing.T) {
	c := &Classifier{
		client: nil, // 사용되면 nil 역참조로 패닉 → 네트워크 미접근 보장
		cache: map[string]string{
			"삼성전자": "IT",
			"애플":   "IT",
		},
	}
	got, err := c.Classify(context.Background(), []summary.SectorStock{
		{Name: "삼성전자", Code: "005930", Currency: "KRW"},
		{Name: "애플", Code: "AAPL", Currency: "USD"},
	})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"삼성전자": "IT", "애플": "IT"}, got)
}

// TestInterfaceSatisfied 는 *Classifier 가 summary.SectorClassifier 를 만족함을 확인.
func TestInterfaceSatisfied(t *testing.T) {
	var _ summary.SectorClassifier = (*Classifier)(nil)
}
