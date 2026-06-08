# 매매일지 섹터/산업 열 추가 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 매매일지 거래 시트의 종목명 뒤에 섹터/산업 2열을 추가한다(국내=KIS 지수업종, 해외=공란).

**Architecture:** 신규 `internal/bizcat`(KIS `SearchStockInfo` 리졸버 + 영구 JSON 캐시)가 종목코드→(섹터,산업)을 제공. `model.Trade`에 Sector/Industry 필드 추가, 행 변환에 종목명 뒤로 삽입(국내 10→12, 해외 15→17). 종목명 뒤 삽입으로 밀린 컬럼 인덱스(중복키/포맷/rowToTrade/range)를 +2 재배치. `cmd/atj`가 종목코드 보강 직후 국내 거래에 업종을 채움.

**Tech Stack:** Go 1.25, `github.com/kenshin579/korea-investment-stock`(신규 의존), testify.

**참조 규칙:** spec = `docs/superpowers/specs/2026-06-09-sector-industry-columns-design.md`. 브랜치 `feature/sector-industry-columns`(생성됨). main 직접 커밋 금지.

**컬럼 인덱스 (0-based, 삽입 후) — 모든 태스크의 기준:**
- 국내(12): `0일자 1구분 2종목코드 3종목명 4섹터 5산업 6수량 7단가 8금액 9수수료 10손익 11수익률`
- 해외(17): `0일자 1구분 2통화 3종목코드 4종목명 5섹터 6산업 7수량 8단가 9금액외화 10환율 11금액원화 12수수료 13세금 14손익외화 15손익원화 16수익률`

---

## 파일 구조
- Create: `internal/bizcat/resolver.go`, `internal/bizcat/resolver_test.go`
- Modify: `internal/model/trade.go` (+test), `internal/writer/headers.go` (+test), `internal/writer/writer.go` (GetExistingKeys/포맷), `internal/writer/reader.go` (rowToTrade/분류/range), `cmd/atj/main.go` (+test), `go.mod`

---

## Task 1: bizcat 패키지 (KIS 지수업종 리졸버 + 캐시)

**Files:**
- Create: `internal/bizcat/resolver.go`, `internal/bizcat/resolver_test.go`
- Modify: `go.mod` (SDK 의존 추가)

- [ ] **Step 1: SDK 의존 추가**

Run:
```bash
cd /Users/user/src/workspace_moneyflow/auto-trading-journal
go get github.com/kenshin579/korea-investment-stock@latest
```
Expected: go.mod 에 모듈 추가. (검증: `grep korea-investment-stock go.mod`)

- [ ] **Step 2: 실패 테스트 작성** — `internal/bizcat/resolver_test.go`

`Resolver` 는 캐시(map) + `fetch` 함수(테스트에서 스텁 주입)로 구성한다. 캐시 hit 시 fetch 미호출, miss 시 fetch 호출 후 캐시 저장. 빈 코드/실패는 빈 값.

```go
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
	assert.Equal(t, 0, calls) // 캐시 hit → fetch 미호출
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
	r.Resolve("105560") // 두 번째는 캐시
	assert.Equal(t, 1, calls)
}

func TestResolve_EmptyCodeOrErrorReturnsBlank(t *testing.T) {
	r := &Resolver{cache: map[string]entry{}, fetch: func(string) (string, string, error) { return "", "", assert.AnError }}
	s, i := r.Resolve("")
	assert.Equal(t, "", s)
	assert.Equal(t, "", i)
	s, i = r.Resolve("000000") // fetch 에러 → 빈 값
	assert.Equal(t, "", s)
	assert.Equal(t, "", i)
}

func TestCacheSaveLoad_Roundtrip_PreservesKorean(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bizcat_cache.json")
	r := &Resolver{cache: map[string]entry{"005930": {Sector: "전기·전자", Industry: "반도체"}}, cachePath: path}
	require.NoError(t, r.saveCache())
	data, _ := os.ReadFile(path)
	assert.Contains(t, string(data), "전기·전자")    // 한글 보존(이스케이프 안 함)
	assert.NotContains(t, string(data), "\\u")

	r2 := New(path) // 파일에서 로드
	s, _ := r2.cacheLookup("005930")
	assert.Equal(t, "전기·전자", s)
}
```

- [ ] **Step 3: 실패 확인**

Run: `go test ./internal/bizcat/ -v`
Expected: FAIL (undefined Resolver/entry/New)

- [ ] **Step 4: 구현** — `internal/bizcat/resolver.go`

```go
// Package bizcat 는 KIS 지수업종(대분류=섹터 / 중분류=산업)을 종목코드로 조회한다.
// (기존 internal/sector(OpenAI, 대시보드 GICS)와 별개 — per-row 열 전용.)
package bizcat

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"sync"

	kis "github.com/kenshin579/korea-investment-stock"
)

type entry struct {
	Sector   string `json:"sector"`
	Industry string `json:"industry"`
}

// Resolver 는 종목코드 → (섹터, 산업) 리졸버. 영구 JSON 캐시 + lazy KIS 클라이언트.
type Resolver struct {
	mu        sync.Mutex
	cache     map[string]entry
	cachePath string
	fetch     func(code string) (sector, industry string, err error) // nil 이면 최초 호출 시 KIS 로 초기화
	dirty     bool
}

// New 는 cachePath 의 캐시를 로드한 리졸버를 만든다. (없거나 손상 시 빈 캐시)
func New(cachePath string) *Resolver {
	r := &Resolver{cache: map[string]entry{}, cachePath: cachePath}
	if data, err := os.ReadFile(cachePath); err == nil {
		var m map[string]entry
		if json.Unmarshal(data, &m) == nil && m != nil {
			r.cache = m
		} else {
			slog.Warn("bizcat 캐시 로드 실패, 빈 캐시로 시작", "path", cachePath)
		}
	}
	return r
}

func (r *Resolver) cacheLookup(code string) (string, bool) {
	e, ok := r.cache[code]
	return e.Sector, ok
}

// Resolve 는 종목코드의 (섹터, 산업)을 반환. 캐시 우선, miss 시 fetch. 실패/빈 코드는 ("","").
func (r *Resolver) Resolve(code string) (string, string) {
	if code == "" {
		return "", ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.cache[code]; ok {
		return e.Sector, e.Industry
	}
	if r.fetch == nil {
		r.fetch = kisFetch()
	}
	sector, industry, err := r.fetch(code)
	if err != nil {
		slog.Warn("업종 조회 실패, 빈 값 처리", "code", code, "err", err)
		return "", ""
	}
	r.cache[code] = entry{Sector: sector, Industry: industry}
	r.dirty = true
	return sector, industry
}

// Save 는 변경이 있으면 캐시를 파일에 기록한다. (run 종료 시 1회 호출)
func (r *Resolver) Save() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.dirty {
		return
	}
	if err := r.saveCache(); err != nil {
		slog.Warn("bizcat 캐시 저장 실패", "err", err)
	} else {
		r.dirty = false
	}
}

func (r *Resolver) saveCache() error {
	f, err := os.Create(r.cachePath)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(r.cache)
}

// kisFetch 는 KIS SDK 로 종목코드 → (대분류, 중분류) 를 조회하는 fetch 함수를 만든다.
// KIS 키 미설정/클라이언트 생성 실패 시 항상 ("","") 를 반환하는 함수를 돌려준다(회복력).
func kisFetch() func(string) (string, string, error) {
	client, err := kis.NewClientFromEnv()
	if err != nil {
		slog.Warn("KIS 클라이언트 생성 실패, 업종 보강 비활성화", "err", err)
		return func(string) (string, string, error) { return "", "", nil }
	}
	return func(code string) (string, string, error) {
		info, err := client.Domestic.SearchStockInfo(context.Background(), code, "300")
		if err != nil {
			return "", "", err
		}
		return info.IdxBztpLclsCdName, info.IdxBztpMclsCdName, nil
	}
}
```

- [ ] **Step 5: 통과 확인 + Commit**

```bash
go test ./internal/bizcat/ -v && go build ./...
git add internal/bizcat/ go.mod go.sum
git commit -m "feat: bizcat — KIS 지수업종(섹터/산업) 리졸버 + 영구 캐시"
```

---

## Task 2: model — Sector/Industry 필드 + 행 변환

**Files:**
- Modify: `internal/model/trade.go`
- Test: `internal/model/trade_test.go`

- [ ] **Step 1: 실패 테스트 추가** — `internal/model/trade_test.go` 에 추가

```go
func TestToDomesticRow_WithSectorIndustry(t *testing.T) {
	tr := sampleDomestic()
	tr.Sector = "전기·전자"
	tr.Industry = "반도체"
	r := tr.ToDomesticRow()
	require.Len(t, r, 12)
	assert.Equal(t, "삼성전자", r[3]) // 종목명
	assert.Equal(t, "전기·전자", r[4]) // 섹터
	assert.Equal(t, "반도체", r[5])   // 산업
	assert.Equal(t, 10.0, r[6])       // 수량 (밀림)
	assert.Equal(t, 70000.0, r[7])    // 단가
}

func TestToForeignRow_SectorIndustryBlankCols(t *testing.T) {
	tr := sampleDomestic()
	tr.Account = "미래에셋증권_해외계좌"
	tr.Currency = "USD"
	r := tr.ToForeignRow()
	require.Len(t, r, 17)
	assert.Equal(t, "삼성전자", r[4]) // 종목명
	assert.Equal(t, "", r[5])         // 섹터(해외 공란)
	assert.Equal(t, "", r[6])         // 산업
	assert.Equal(t, 10.0, r[7])       // 수량
}
```
(`require` import 가 trade_test.go 에 없으면 추가.)

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/model/ -run "SectorIndustry" -v`
Expected: FAIL (필드 없음 / 길이 10·15)

- [ ] **Step 3: 구현** — `internal/model/trade.go`

struct 에 필드 추가(Account 위, StockName 근처 아무 곳):
```go
	Account      string
	Sector       string // 지수업종 대분류 (국내만; 해외 "")
	Industry     string // 지수업종 중분류
```
행 변환 교체:
```go
// ToDomesticRow: 국내 12컬럼 (종목명 뒤 섹터/산업)
func (t Trade) ToDomesticRow() []any {
	return []any{t.Date, t.TradeType, t.StockCode, t.StockName, t.Sector, t.Industry,
		t.Quantity, t.Price, t.Amount, t.Fee, t.Profit, rate(t.ProfitRate)}
}

// ToForeignRow: 해외 17컬럼 (종목명 뒤 섹터/산업; 해외는 공란)
func (t Trade) ToForeignRow() []any {
	return []any{t.Date, t.TradeType, t.Currency, t.StockCode, t.StockName, t.Sector, t.Industry,
		t.Quantity, t.Price, t.Amount, t.ExchangeRate, t.AmountKRW,
		t.Fee, t.Tax, t.Profit, t.ProfitKRW, rate(t.ProfitRate)}
}
```
`DuplicateKey()` 는 변경 없음(섹터/산업 미포함).

- [ ] **Step 4: 통과 확인** (기존 TestToDomesticRow 가 10컬럼 단언이면 12로 갱신 — 인덱스 r[6]=수량 등 위 테스트와 일치하게 수정)

Run: `go test ./internal/model/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/model/
git commit -m "feat: Trade에 Sector/Industry 필드 + 행 변환(국내 12/해외 17컬럼)"
```

---

## Task 3: writer/headers — 신규 헤더 + 구포맷 상수 + 포맷 인덱스 시프트

**Files:**
- Modify: `internal/writer/headers.go`
- Test: `internal/writer/headers_test.go`

- [ ] **Step 1: 실패 테스트 갱신/추가** — `internal/writer/headers_test.go`

```go
func TestHeaders_WithSectorIndustry(t *testing.T) {
	assert.Len(t, DomesticHeaders, 12)
	assert.Equal(t, "종목명", DomesticHeaders[3])
	assert.Equal(t, "섹터", DomesticHeaders[4])
	assert.Equal(t, "산업", DomesticHeaders[5])
	assert.Equal(t, "수량", DomesticHeaders[6])
	assert.Len(t, ForeignHeaders, 17)
	assert.Equal(t, "섹터", ForeignHeaders[5])
	assert.Equal(t, "산업", ForeignHeaders[6])
	// 구포맷 감지 상수
	assert.Len(t, OldDomesticHeadersV2, 10)
	assert.Len(t, OldForeignHeadersV1, 15)
}
```

- [ ] **Step 2: 실패 확인** → `go test ./internal/writer/ -run Headers -v` FAIL

- [ ] **Step 3: 구현** — `internal/writer/headers.go`

헤더 교체 + 구포맷 상수 추가:
```go
var DomesticHeaders = []string{
	"일자", "구분", "종목코드", "종목명", "섹터", "산업", "수량", "단가", "금액",
	"수수료", "손익금액", "수익률(%)",
}

var ForeignHeaders = []string{
	"일자", "구분", "통화", "종목코드", "종목명", "섹터", "산업", "수량", "단가",
	"금액(외화)", "환율", "금액(원화)", "수수료", "세금",
	"손익(외화)", "손익(원화)", "수익률(%)",
}

// 마이그레이션 감지용 구포맷 (섹터/산업 추가 이전).
var OldDomesticHeadersV2 = []string{ // 10컬럼
	"일자", "구분", "종목코드", "종목명", "수량", "단가", "금액",
	"수수료", "손익금액", "수익률(%)",
}
var OldForeignHeadersV1 = []string{ // 15컬럼
	"일자", "구분", "통화", "종목코드", "종목명", "수량", "단가",
	"금액(외화)", "환율", "금액(원화)", "수수료", "세금",
	"손익(외화)", "손익(원화)", "수익률(%)",
}
```
(기존 `OldDomesticHeadersV1` 9컬럼은 유지.)

포맷 상수 인덱스 +2 시프트:
```go
var DomesticFormats = []sheets.ColumnFormat{
	{Col: 7, Pattern: "#,##0"},                    // G: 수량
	{Col: 8, Pattern: "₩#,##0"},                   // H: 단가
	{Col: 9, Pattern: "₩#,##0"},                   // I: 금액
	{Col: 10, Pattern: "₩#,##0"},                  // J: 수수료
	{Col: 11, Pattern: "₩#,##0"},                  // K: 손익금액
	{Col: 12, Pattern: "0.00%", Type: "PERCENT"},  // L: 수익률
}

var ForeignFormatsCommon = []sheets.ColumnFormat{
	{Col: 8, Pattern: "#,##0"},                    // H: 수량
	{Col: 11, Pattern: "#,##0.00"},                // K: 환율
	{Col: 12, Pattern: "₩#,##0"},                  // L: 금액(원화)
	{Col: 16, Pattern: "₩#,##0"},                  // P: 손익(원화)
	{Col: 17, Pattern: "0.00%", Type: "PERCENT"},  // Q: 수익률
}

// 단가/금액외화/수수료/세금/손익외화 (1-based)
var ForeignCurrencyCols = []int{9, 10, 13, 14, 15}
```

- [ ] **Step 4: 통과 확인 + Commit**

```bash
go test ./internal/writer/ -run Headers -v
git add internal/writer/headers.go internal/writer/headers_test.go
git commit -m "feat: 헤더에 섹터/산업(국내12/해외17) + 구포맷 감지 상수 + 포맷 인덱스 시프트"
```

---

## Task 4: writer GetExistingKeys — 중복키 컬럼 인덱스 시프트

**Files:**
- Modify: `internal/writer/writer.go`
- Test: `internal/writer/writer_test.go`

현재 `GetExistingKeys`: 국내 수량=4/단가=5, 해외 수량=5/단가=6, range `A2:O10000`, guards `len<6` / `len>=7`. 새 위치: 국내 수량=6/단가=7, 해외 수량=7/단가=8.

- [ ] **Step 1: 실패 테스트 추가** — grid 모킹이 어려우면, 키 추출 로직을 순수 함수로 분리해 테스트. `writer.go` 에 헬퍼 추가:
```go
// keyColsForGrid 는 (isForeign)에 따른 (종목명, 수량, 단가) 컬럼 인덱스를 반환한다.
func keyColsForGrid(isForeign bool) (nameCol, qtyCol, priceCol int) {
	if isForeign {
		return 4, 7, 8
	}
	return 3, 6, 7
}
```
테스트:
```go
func TestKeyColsForGrid(t *testing.T) {
	n, q, p := keyColsForGrid(false)
	assert.Equal(t, [3]int{3, 6, 7}, [3]int{n, q, p})
	n, q, p = keyColsForGrid(true)
	assert.Equal(t, [3]int{4, 7, 8}, [3]int{n, q, p})
}
```

- [ ] **Step 2: 실패 확인** → FAIL (undefined keyColsForGrid)

- [ ] **Step 3: 구현** — `keyColsForGrid` 추가하고 `GetExistingKeys` 본문을 그것을 쓰도록 교체:
```go
	grid, err := w.client.GetRawGridData(ctx, sheetName, "A2:Q10000") // 해외 17컬럼(Q)까지
	...
	nameCol, qtyCol, priceCol := keyColsForGrid(isForeign)
	maxCol := priceCol
	for _, row := range grid.RowData {
		values := row.Values
		if len(values) <= maxCol {
			continue
		}
		dateVal := values[0].FormattedValue
		if dateVal == "" {
			continue
		}
		tradeType := stringFromCell(cellEffective(values[1]))
		stockName := stringFromCell(cellEffective(values[nameCol]))
		key := model.DupKey{
			dateVal, tradeType, stockName,
			normalizeCellValue(cellEffective(values[qtyCol])),
			normalizeCellValue(cellEffective(values[priceCol])),
		}
		keys[key] = true
	}
```
(기존 `len(values) < 6` / `if isForeign && len(values) >= 7` 분기 제거.)

- [ ] **Step 4: 통과 확인 + Commit**

```bash
go test ./internal/writer/ -v
git add internal/writer/writer.go internal/writer/writer_test.go
git commit -m "feat: GetExistingKeys 중복키 컬럼 인덱스 시프트(국내6/7,해외7/8) + range Q"
```

---

## Task 5: writer reader — rowToTrade 시프트 + 헤더 분류 + read range

**Files:**
- Modify: `internal/writer/reader.go`
- Test: `internal/writer/reader_test.go`

- [ ] **Step 1: 실패 테스트 갱신** — 기존 `TestRowToTradeDomestic` 를 12컬럼 행으로 갱신 + 해외 17컬럼 추가:
```go
func TestRowToTradeDomestic_12cols(t *testing.T) {
	row := []interface{}{"2026-02-13", "매도", "005930", "삼성전자", "전기·전자", "반도체",
		"10", "75000", "750000", "1500", "50000", "0.0714"}
	tr := rowToTrade(row, false, "미래에셋증권_국내계좌")
	assert.Equal(t, "삼성전자", tr.StockName)
	assert.Equal(t, "005930", tr.StockCode)
	assert.Equal(t, 10.0, tr.Quantity)
	assert.Equal(t, 75000.0, tr.Price)
	assert.InDelta(t, 7.14, tr.ProfitRate, 0.01)
}

func TestRowToTradeForeign_17cols(t *testing.T) {
	row := []interface{}{"2026-03-15", "매도", "USD", "AAPL", "Apple", "", "",
		"5", "182", "910", "1365", "1241650", "910", "150", "32.5", "46700", "0.0371"}
	tr := rowToTrade(row, true, "미래에셋증권_해외계좌")
	assert.Equal(t, "AAPL", tr.StockCode)
	assert.Equal(t, "Apple", tr.StockName)
	assert.Equal(t, 5.0, tr.Quantity)
	assert.Equal(t, 46700.0, tr.ProfitKRW)
	assert.InDelta(t, 3.71, tr.ProfitRate, 0.01)
}
```

- [ ] **Step 2: 실패 확인** → FAIL

- [ ] **Step 3: 구현** — `rowToTrade` 인덱스 시프트(섹터/산업 4,5(국내)·5,6(해외)는 읽지 않음):

국내 분기:
```go
	// 국내 12컬럼: 0일자 1구분 2종목코드 3종목명 4섹터 5산업 6수량 7단가 8금액 9수수료 10손익 11수익률
	amount := getNum(row, 8)
	profit := getNum(row, 10)
	return model.Trade{
		Date: date, TradeType: getStr(row, 1), StockCode: getCode(row, 2), StockName: getStr(row, 3),
		Quantity: getNum(row, 6), Price: getNum(row, 7), Amount: amount,
		Currency: "KRW", ExchangeRate: 1.0, AmountKRW: amount,
		Fee: getNum(row, 9), Tax: 0.0, Profit: profit, ProfitKRW: profit,
		ProfitRate: getNum(row, 11) * 100, Account: account,
	}
```
해외 분기:
```go
	// 해외 17컬럼: ...4종목명 5섹터 6산업 7수량 8단가 9금액외화 10환율 11금액원화 12수수료 13세금 14손익외화 15손익원화 16수익률
	return model.Trade{
		Date: date, TradeType: getStr(row, 1), Currency: getStr(row, 2),
		StockCode: getCode(row, 3), StockName: getStr(row, 4),
		Quantity: getNum(row, 7), Price: getNum(row, 8), Amount: getNum(row, 9),
		ExchangeRate: getNum(row, 10), AmountKRW: getNum(row, 11),
		Fee: getNum(row, 12), Tax: getNum(row, 13), Profit: getNum(row, 14),
		ProfitKRW: getNum(row, 15), ProfitRate: getNum(row, 16) * 100, Account: account,
	}
```

헤더 분류(`ReadAllTrades`) — 신규 헤더 매칭 + 구포맷 경고/스킵 추가:
```go
	switch {
	case headersEqual(headerRow, DomesticHeaders):
		isForeign = false
	case headersEqual(headerRow, ForeignHeaders):
		isForeign = true
	case headersEqual(headerRow, OldDomesticHeadersV2),
		headersEqual(headerRow, OldForeignHeadersV1),
		headersEqual(headerRow, OldDomesticHeadersV1):
		slog.Warn("시트가 섹터/산업 추가 이전 포맷입니다. 시트를 삭제 후 재실행하거나 "+
			"종목명 뒤에 '섹터','산업' 컬럼을 수동 삽입하세요. (이번 실행에서는 스킵)", "sheet", sheetName)
		continue
	default:
		slog.Debug("시트 스킵(매매일지 헤더 불일치)", "sheet", sheetName)
		continue
	}
```

`readTradesFromSheet` 의 읽기 range: 국내/해외 각각 최소 L(12)/Q(17)까지 읽도록 확인·수정. 현재 `if isForeign` 분기에서 읽는 A1 range 를 국내 `A2:L`, 해외 `A2:Q` 로(또는 공통 `A2:Q`) 갱신. (읽은 뒤 rowToTrade 가 필요한 인덱스만 사용.)

- [ ] **Step 4: 통과 확인 + Commit**

```bash
go test ./internal/writer/ -v
git add internal/writer/reader.go internal/writer/reader_test.go
git commit -m "feat: rowToTrade 인덱스 시프트(12/17) + 구포맷 경고스킵 + read range 확장"
```

---

## Task 6: cmd/atj — bizcat 리졸버 배선 + 국내 업종 보강

**Files:**
- Modify: `cmd/atj/main.go`
- Test: `cmd/atj/main_test.go`

- [ ] **Step 1: 실패 테스트 추가** — `enrichSectors`(국내만 채움) 순수 함수:
```go
type stubBizcat map[string][2]string

func (s stubBizcat) Resolve(code string) (string, string) {
	v, ok := s[code]
	if !ok {
		return "", ""
	}
	return v[0], v[1]
}

func TestEnrichSectors_DomesticOnly(t *testing.T) {
	trades := []model.Trade{
		{StockCode: "005930", Account: "미래에셋증권_국내계좌"},
		{StockCode: "AAPL", Account: "미래에셋증권_해외계좌"}, // 해외 미보강
	}
	enrichSectors(trades, stubBizcat{"005930": {"전기·전자", "반도체"}})
	assert.Equal(t, "전기·전자", trades[0].Sector)
	assert.Equal(t, "반도체", trades[0].Industry)
	assert.Equal(t, "", trades[1].Sector)
}
```

- [ ] **Step 2: 실패 확인** → FAIL

- [ ] **Step 3: 구현** — `cmd/atj/main.go`

bizcat 인터페이스 + enrich 함수:
```go
// bizcatResolver 는 종목코드 → (섹터, 산업) 조회를 추상화한다(테스트 스텁 주입용).
type bizcatResolver interface {
	Resolve(code string) (sector, industry string)
}

// enrichSectors 는 국내 거래(코드 있음)에 섹터/산업을 채운다(in-place). 해외는 스킵.
func enrichSectors(trades []model.Trade, r bizcatResolver) {
	for i := range trades {
		t := &trades[i]
		if t.IsDomestic() && t.StockCode != "" {
			t.Sector, t.Industry = r.Resolve(t.StockCode)
		}
	}
}
```
processor 에 필드 추가 + 생성:
```go
	bizcatRes bizcatResolver
```
`newProcessor` 에서 `bizcat.New("config/bizcat_cache.json")` 로 생성해 대입(import 추가).
`processFile` 의 `enrichDomesticCodes(trades, p.symbolRes)` **직후** 추가:
```go
	enrichSectors(trades, p.bizcatRes)
```
`run` 종료 시(대시보드 갱신 후) 캐시 저장: `p.bizcatRes` 가 `*bizcat.Resolver` 면 `Save()` 호출. 인터페이스에 `Save()` 를 넣거나, processor 가 구체 타입 `*bizcat.Resolver` 를 별도 보관해 `defer p.bizcatStore.Save()`. (간단히 구체 타입 필드 `bizcatStore *bizcat.Resolver` 를 두고 nil 체크 후 Save.)

- [ ] **Step 4: 통과 확인 + Commit**

```bash
go build ./... && go test ./cmd/atj/ -v
git add cmd/atj/main.go cmd/atj/main_test.go
git commit -m "feat: cmd — bizcat 배선 + 국내 거래 섹터/산업 보강"
```

---

## Task 7: 통합 검증 + 문서

**Files:** (코드 변경 없음 / 문서)

- [ ] **Step 1: 전체 빌드/테스트/vet**

```bash
go build ./... && go vet ./... && go test ./...
```
Expected: 전 패키지 PASS.

- [ ] **Step 2: dry-run 스모크** (config/env 있으면) — `go run ./cmd/atj --dry-run --log-level DEBUG` 로 파싱·보강 경로 확인(시트 미반영).

- [ ] **Step 3: 문서 갱신** — `CLAUDE.md`/`README.md` 에 섹터/산업 열(국내, KIS bizcat) 및 신규 헤더(12/17컬럼), 구포맷 시트 재생성 안내 반영. `internal/bizcat` 패키지 책임 추가.

- [ ] **Step 4: Commit**

```bash
git add CLAUDE.md README.md
git commit -m "docs: 섹터/산업 열 + bizcat 패키지 반영"
```

- [ ] **Step 5: PR** (사용자 확인 후)

```bash
git push -u origin feature/sector-industry-columns
gh pr create --title "feat: 매매일지 섹터/산업 열 추가 (국내, KIS bizcat)" --body "$(cat <<'EOF'
## Summary
- 거래 시트 종목명 뒤에 섹터/산업 2열 추가 (국내=KIS 지수업종 대/중분류, 해외=공란)
- 신규 internal/bizcat (SearchStockInfo + 영구 캐시), 첫 KIS SDK 의존
- 헤더 국내 10→12 / 해외 15→17, 중복키·포맷·rowToTrade 인덱스 시프트
- 대시보드 OpenAI 섹터집계는 무변경

## Test plan
- [ ] go test ./... 통과
- [ ] dry-run 스모크
- [ ] 구포맷 시트 경고스킵 확인 / 재생성 후 섹터·산업 채워짐 확인
EOF
)"
```

---

## 자기 점검 (작성자)
- **Spec 커버리지**: bizcat(Task1)/model(2)/headers+포맷(3)/중복키(4)/rowToTrade+분류+range(5)/cmd 배선(6)/검증·문서(7) — spec 전 항목 매핑. 마이그레이션=구포맷 경고스킵(Task5). 해외 공란=model(Task2 해외행 빈값)+cmd(Task6 국내만).
- **인덱스 일관성**: 국내 수량6/단가7, 해외 수량7/단가8 — Task2(행)/Task3(포맷 G7..L12, H8..Q17)/Task4(키 6,7·7,8)/Task5(rowToTrade) 전부 동일 기준. ForeignCurrencyCols [9,10,13,14,15].
- **플레이스홀더**: 없음. (Task5 read range는 "현재 분기 확인 후 L/Q로" — 실제 현재 코드 확인 지시 포함.)
