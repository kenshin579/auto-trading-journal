package sheets

import "testing"

// TestInvalidateSheetIDCache 는 캐시 무효화가 맵을 비우고(재사용 가능한) 빈 맵으로
// 재초기화하는지 네트워크 없이 검증한다.
func TestInvalidateSheetIDCache(t *testing.T) {
	c := &Client{sheetIDCache: map[string]int64{"시트A": 1, "시트B": 2}}

	c.InvalidateSheetIDCache()

	if c.sheetIDCache == nil {
		t.Fatal("무효화 후 캐시는 nil 이 아니라 빈 맵이어야 한다")
	}
	if len(c.sheetIDCache) != 0 {
		t.Fatalf("무효화 후 캐시는 비어 있어야 한다, got %d entries", len(c.sheetIDCache))
	}

	// 재초기화된 맵에 쓰기가 가능해야 한다.
	c.sheetIDCache["시트C"] = 3
	if c.sheetIDCache["시트C"] != 3 {
		t.Fatal("무효화 후 캐시에 쓰기가 가능해야 한다")
	}
}

// TestInvalidateSheetIDCacheZeroValue 는 zero-value Client 에서도 패닉 없이
// 동작하는지 확인한다.
func TestInvalidateSheetIDCacheZeroValue(t *testing.T) {
	var c Client
	c.InvalidateSheetIDCache()
	if c.sheetIDCache == nil || len(c.sheetIDCache) != 0 {
		t.Fatal("zero-value Client 무효화 후 빈 맵이어야 한다")
	}
}
