# auto-trading-journal Go 포팅 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 현재 Python auto-trading-journal(~3,760줄)을 동작 동등하게 Go로 전면 포팅한다.

**Architecture:** `cmd/atj` 진입점 + `internal/*` 패키지(config, model, parser, sheets, writer, summary, symbol, sector). 재작성 기간 동안 Python을 그대로 두고 단계별로 같은 `input/`을 양쪽에서 실행해 결과를 대조(Python = oracle)한 뒤, 마지막에 Python 제거.

**Tech Stack:** Go 1.25, `google.golang.org/api/sheets/v4`, `gopkg.in/yaml.v3`, `golang.org/x/text/encoding/korean`(cp949), `github.com/sashabaranov/go-openai`, `github.com/stretchr/testify`.

**참조 규칙:** 본 포팅은 **Python 소스가 동작 명세**다. 각 태스크는 명시된 Python 파일/라인을 읽고 동일 동작을 재현한다. 테스트가 동작 계약이다.

**브랜치:** `feature/go-porting` (이미 생성됨). 절대 main 직접 커밋 금지.

---

## 파일 구조

```
auto-trading-journal/
├── go.mod / go.sum                  # module github.com/kenshin579/auto-trading-journal
├── cmd/atj/main.go                  # 진입점 + StockDataProcessor 오케스트레이션
├── internal/
│   ├── config/config.go             # Config struct + Load (YAML + env override)
│   ├── model/trade.go               # Trade struct + 행 변환 + 중복키
│   ├── parser/
│   │   ├── parser.go                # Parser interface + 공용 헬퍼(parseFloat, convertDate)
│   │   ├── registry.go              # DetectParser (헤더 기반)
│   │   ├── mirae.go                 # MiraeDomestic / MiraeForeign
│   │   └── hankook.go               # HankookDomestic
│   ├── symbol/resolver.go           # KRX .mst → 종목코드 (fwf + cp949 + 7일 캐시)
│   ├── sector/classifier.go         # OpenAI 섹터 분류 + JSON 캐시
│   ├── sheets/client.go             # Google Sheets v4 래퍼 (값/포맷/색상/필터/차트/레이트리밋)
│   ├── writer/writer.go             # 시트 생성/중복필터/삽입/색상/포맷  (+ writer/headers.go)
│   └── summary/                     # 대시보드 생성 (단일 "대시보드" 시트)
│       ├── summary.go               # Generator + generateAll + 섹션 작성
│       ├── insights.go              # 지표/인사이트/추이 계산 헬퍼
│       └── charts.go                # basic/pie 차트 스펙 빌더
└── testdata/                        # CSV 픽스처 + 골든 출력
```

각 파일 단일 책임. 큰 모듈(sheets/summary)은 책임별로 파일 분리.

---

## Phase 1 — 스캐폴딩

### Task 1: Go 모듈 초기화

**Files:**
- Create: `go.mod`

- [ ] **Step 1: 모듈 초기화 + 의존성 추가**

Run:
```bash
cd /Users/user/src/workspace_moneyflow/auto-trading-journal
go mod init github.com/kenshin579/auto-trading-journal
go get google.golang.org/api/sheets/v4@latest
go get google.golang.org/api/option@latest
go get gopkg.in/yaml.v3@latest
go get golang.org/x/text/encoding/korean@latest
go get github.com/sashabaranov/go-openai@latest
go get github.com/stretchr/testify@latest
```

- [ ] **Step 2: 빌드 확인**

Run: `go build ./... 2>&1; echo "exit: $?"`
Expected: 빌드 대상 없음(패키지 미작성)이라도 에러 없이 종료. `go.mod`/`go.sum` 생성됨.

- [ ] **Step 3: `.gitignore` 보강** — `__pycache__/`, Go 바이너리 무시 항목 추가 (기존 .gitignore 있으면 append)

```
# Go
/atj
*.test
```

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum .gitignore
git commit -m "chore: Go 모듈 초기화 및 의존성 추가"
```

---

### Task 2: config 패키지

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

Python 참조: `main.py:load_config()` + `StockDataProcessor.__init__` 의 env 오버라이드 로직, `config/config.yaml` 스키마.

- [ ] **Step 1: 실패 테스트 작성**

```go
package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoad_YAMLAndEnvOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(
		"google_sheets:\n  spreadsheet_id: from_yaml\n  service_account_path: /sa.json\n"+
			"logging:\n  level: INFO\n"+
			"openai:\n  model: gpt-4o-mini\n  sector_cache_file: config/sector_cache.json\n"), 0o644))

	t.Setenv("GOOGLE_SPREADSHEET_ID", "from_env")
	t.Setenv("STOCK_DATA_OPENAI_API_KEY", "sk-test")

	cfg, err := Load(path)
	require.NoError(t, err)
	require.Equal(t, "from_env", cfg.SpreadsheetID())   // env 우선
	require.Equal(t, "/sa.json", cfg.ServiceAccountPath)
	require.Equal(t, "gpt-4o-mini", cfg.OpenAI.Model)
	require.Equal(t, "sk-test", cfg.OpenAIAPIKey())
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `go test ./internal/config/ -run TestLoad_YAMLAndEnvOverride -v`
Expected: FAIL (`undefined: Load`)

- [ ] **Step 3: 구현**

```go
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	GoogleSheets struct {
		SpreadsheetID      string `yaml:"spreadsheet_id"`
		ServiceAccountPath string `yaml:"service_account_path"`
	} `yaml:"google_sheets"`
	Logging struct {
		Level string `yaml:"level"`
	} `yaml:"logging"`
	OpenAI struct {
		Model           string `yaml:"model"`
		SectorCacheFile string `yaml:"sector_cache_file"`
	} `yaml:"openai"`
	BatchSize int `yaml:"batch_size"`

	// 편의 접근자용 (YAML 외부)
	ServiceAccountPath string `yaml:"-"`
}

// SpreadsheetID 는 env(GOOGLE_SPREADSHEET_ID) 우선, 없으면 YAML 값.
func (c *Config) SpreadsheetID() string {
	if v := os.Getenv("GOOGLE_SPREADSHEET_ID"); v != "" {
		return v
	}
	return c.GoogleSheets.SpreadsheetID
}

// OpenAIAPIKey 는 env(STOCK_DATA_OPENAI_API_KEY). 미설정이면 "".
func (c *Config) OpenAIAPIKey() string { return os.Getenv("STOCK_DATA_OPENAI_API_KEY") }

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("설정 파일을 찾을 수 없습니다: %s: %w", path, err)
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("config 파싱 실패: %w", err)
	}
	if c.Logging.Level == "" {
		c.Logging.Level = "INFO"
	}
	if c.OpenAI.Model == "" {
		c.OpenAI.Model = "gpt-4o-mini"
	}
	if c.OpenAI.SectorCacheFile == "" {
		c.OpenAI.SectorCacheFile = "config/sector_cache.json"
	}
	c.ServiceAccountPath = c.GoogleSheets.ServiceAccountPath
	return &c, nil
}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `go test ./internal/config/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat: config 로더 포팅 (YAML + env 오버라이드)"
```

---

### Task 3: model 패키지 (Trade)

**Files:**
- Create: `internal/model/trade.go`
- Test: `internal/model/trade_test.go`

Python 참조: `modules/models.py` 전체 (16필드, `to_domestic_row`/`to_foreign_row`/`to_sheet_row`/`duplicate_key`/`_num_str`).

- [ ] **Step 1: 실패 테스트 작성** (행 변환·중복키 계약)

```go
package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func sampleDomestic() Trade {
	return Trade{Date: "2026-02-13", TradeType: "매도", StockName: "삼성전자", StockCode: "005930",
		Quantity: 10, Price: 70000, Amount: 700000, Currency: "KRW", ExchangeRate: 1,
		AmountKRW: 700000, Fee: 1500, Profit: 50000, ProfitRate: 14.68, Account: "미래에셋증권_국내계좌"}
}

func TestToDomesticRow(t *testing.T) {
	r := sampleDomestic().ToDomesticRow()
	// 일자, 구분, 종목코드, 종목명, 수량, 단가, 금액, 수수료, 손익, 수익률(소수)
	assert.Equal(t, []any{"2026-02-13", "매도", "005930", "삼성전자",
		10.0, 70000.0, 700000.0, 1500.0, 50000.0, 0.1468}, r)
}

func TestDuplicateKey_NumberNormalization(t *testing.T) {
	tr := sampleDomestic()
	tr.Quantity = 10
	tr.Price = 70000
	assert.Equal(t, DupKey{"2026-02-13", "매도", "삼성전자", "10", "70000"}, tr.DuplicateKey())
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `go test ./internal/model/ -v`
Expected: FAIL (`undefined: Trade`)

- [ ] **Step 3: 구현**

```go
package model

import "strconv"

type Trade struct {
	Date         string
	TradeType    string // 매수 / 매도
	StockName    string
	StockCode    string
	Quantity     float64
	Price        float64
	Amount       float64
	Currency     string // KRW / USD / JPY
	ExchangeRate float64
	AmountKRW    float64
	Fee          float64
	Tax          float64
	Profit       float64
	ProfitKRW    float64
	ProfitRate   float64 // 퍼센트(14.68)
	Account      string
}

// DupKey: (date, trade_type, stock_name, quantity_str, price_str)
type DupKey [5]string

func (t Trade) IsForeign() bool  { return containsHaewoe(t.Account) }
func (t Trade) IsDomestic() bool { return !t.IsForeign() }

func containsHaewoe(s string) bool {
	// Python: "해외" in self.account
	return indexOf(s, "해외") >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func rate(p float64) float64 {
	if p == 0 {
		return 0
	}
	return p / 100
}

// ToDomesticRow: 국내 10컬럼
func (t Trade) ToDomesticRow() []any {
	return []any{t.Date, t.TradeType, t.StockCode, t.StockName,
		t.Quantity, t.Price, t.Amount, t.Fee, t.Profit, rate(t.ProfitRate)}
}

// ToForeignRow: 해외 15컬럼
func (t Trade) ToForeignRow() []any {
	return []any{t.Date, t.TradeType, t.Currency, t.StockCode, t.StockName,
		t.Quantity, t.Price, t.Amount, t.ExchangeRate, t.AmountKRW,
		t.Fee, t.Tax, t.Profit, t.ProfitKRW, rate(t.ProfitRate)}
}

func (t Trade) ToSheetRow() []any {
	if t.IsForeign() {
		return t.ToForeignRow()
	}
	return t.ToDomesticRow()
}

// numStr: 정수면 소수점 제거 (Python _num_str)
func numStr(v float64) string {
	if v == float64(int64(v)) {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'g', -1, 64)
}

func (t Trade) DuplicateKey() DupKey {
	return DupKey{t.Date, t.TradeType, t.StockName, numStr(t.Quantity), numStr(t.Price)}
}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `go test ./internal/model/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/model/
git commit -m "feat: Trade 모델 포팅 (행 변환 + 중복키)"
```

---

## Phase 2 — 파서 + 종목 마스터

### Task 4: 파서 인터페이스 + 공용 헬퍼

**Files:**
- Create: `internal/parser/parser.go`
- Test: `internal/parser/parser_test.go`

Python 참조: `modules/parsers/base_parser.py`, `mirae_parser.py:_parse_float/_convert_date`, `hankook_parser.py:_parse_float/_convert_date`(쌍따옴표 제거 변형).

- [ ] **Step 1: 실패 테스트 작성**

```go
package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseFloat(t *testing.T) {
	assert.Equal(t, 0.0, parseFloat(""))
	assert.Equal(t, 1234.5, parseFloat(" 1,234.5 "))
	assert.Equal(t, 70000.0, parseFloat("\"70,000\"")) // 쌍따옴표+콤마
}

func TestConvertDate(t *testing.T) {
	assert.Equal(t, "2026-02-13", convertDate(" 2026/02/13 "))
	assert.Equal(t, "2026-02-13", convertDate("\"2026/02/13\""))
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/parser/ -run "TestParseFloat|TestConvertDate" -v`
Expected: FAIL (undefined)

- [ ] **Step 3: 구현** (Python의 두 변형을 하나로 통합 — 쌍따옴표 제거를 항상 수행해도 미래에셋 값에 영향 없음)

```go
package parser

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kenshin579/auto-trading-journal/internal/model"
)

// Parser: 증권사별 CSV 파서.
type Parser interface {
	Name() string
	CanParse(header []string) bool
	Parse(path string, account string) ([]model.Trade, error)
}

func parseFloat(v string) float64 {
	c := strings.ReplaceAll(strings.Trim(strings.TrimSpace(v), `"`), ",", "")
	if c == "" {
		return 0
	}
	f, err := strconv.ParseFloat(c, 64)
	if err != nil {
		return 0
	}
	return f
}

func convertDate(s string) string {
	return strings.ReplaceAll(strings.Trim(strings.TrimSpace(s), `"`), "/", "-")
}

func base(path string) string { return filepath.Base(path) }
```

- [ ] **Step 4: 통과 확인 + Commit**

```bash
go test ./internal/parser/ -v
git add internal/parser/parser.go internal/parser/parser_test.go
git commit -m "feat: 파서 인터페이스 + 공용 헬퍼 포팅"
```

---

### Task 5: 미래에셋 국내 파서

**Files:**
- Create: `internal/parser/mirae.go`
- Test: `internal/parser/mirae_test.go`
- Test fixture: `testdata/mirae_domestic.csv`

Python 참조: `modules/parsers/mirae_parser.py:MiraeDomesticParser` (11컬럼, 헤더+서브헤더 2행 스킵, 매수/매도 분리 생성, can_parse 키워드 `{일자, 종목명, 기간 중 매수}`).

- [ ] **Step 1: 픽스처 작성** `testdata/mirae_domestic.csv` (UTF-8). 1행 헤더(키워드 포함), 2행 서브헤더, 3행~ 데이터. 매수만/매도만/매수+매도 케이스 1행씩.

```
일자,종목명,기간 중 매수,,,,,,,,
,,수량,평균단가,매수금액,수량,평균단가,매도금액,매매비용,손익금액,수익률
2026/02/13,삼성전자,10,70000,700000,0,0,0,0,0,0
2026/02/14,삼성전자,0,0,0,10,75000,750000,1500,50000,7.14
```

- [ ] **Step 2: 실패 테스트 작성**

```go
package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMiraeDomestic_Parse(t *testing.T) {
	p := MiraeDomestic{}
	assert.True(t, p.CanParse([]string{"일자", "종목명", "기간 중 매수"}))

	trades, err := p.Parse("../../testdata/mirae_domestic.csv", "미래에셋증권_국내계좌")
	require.NoError(t, err)
	require.Len(t, trades, 2)
	assert.Equal(t, "매수", trades[0].TradeType)
	assert.Equal(t, 700000.0, trades[0].Amount)
	assert.Equal(t, "매도", trades[1].TradeType)
	assert.Equal(t, 1500.0, trades[1].Fee)
	assert.Equal(t, 50000.0, trades[1].Profit)
	assert.InDelta(t, 7.14, trades[1].ProfitRate, 0.001)
}
```

- [ ] **Step 3: 실패 확인**

Run: `go test ./internal/parser/ -run TestMiraeDomestic -v` → FAIL (undefined MiraeDomestic)

- [ ] **Step 4: 구현** — `mirae_parser.py:MiraeDomesticParser.parse` 를 그대로 옮긴다. 핵심 규칙:
  - `encoding/csv` 리더, `FieldsPerRecord = -1`(가변 컬럼 허용)
  - 헤더 1행 + 서브헤더 1행 스킵, 데이터는 3행부터
  - `len(row) < 11` 스킵; date·name 모두 빈 행 스킵; date 빈데 name 있으면 error; name 빈 행 스킵
  - 매수 수량>0 → 매수 Trade(fee/tax/profit 0), 매도 수량>0 → 매도 Trade(fee=col8, profit=col9, rate=col10)
  - 국내라 currency=KRW, exchange_rate=1, amount_krw=amount, stock_code=""

```go
package parser

import (
	"encoding/csv"
	"fmt"
	"os"

	"github.com/kenshin579/auto-trading-journal/internal/model"
)

type MiraeDomestic struct{}

func (MiraeDomestic) Name() string { return "MiraeDomesticParser" }

func (MiraeDomestic) CanParse(h []string) bool {
	return hasAll(h, "일자", "종목명", "기간 중 매수")
}

func (MiraeDomestic) Parse(path, account string) ([]model.Trade, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	var out []model.Trade
	for i := 2; i < len(rows); i++ { // 헤더+서브헤더 스킵
		row := rows[i]
		if len(row) < 11 {
			continue
		}
		dateRaw := trimSpace(row[0])
		name := trimSpace(row[1])
		if dateRaw == "" && name == "" {
			continue
		}
		if dateRaw == "" {
			return nil, fmt.Errorf("날짜가 비어있습니다: %s, %d행", base(path), i+1)
		}
		if name == "" {
			continue
		}
		date := convertDate(dateRaw)
		buyQty, buyPrice, buyAmt := parseFloat(row[2]), parseFloat(row[3]), parseFloat(row[4])
		sellQty, sellPrice, sellAmt := parseFloat(row[5]), parseFloat(row[6]), parseFloat(row[7])
		fee, profit, profitRate := parseFloat(row[8]), parseFloat(row[9]), parseFloat(row[10])

		if buyQty > 0 {
			out = append(out, model.Trade{Date: date, TradeType: "매수", StockName: name,
				Quantity: buyQty, Price: buyPrice, Amount: buyAmt, Currency: "KRW",
				ExchangeRate: 1, AmountKRW: buyAmt, Account: account})
		}
		if sellQty > 0 {
			out = append(out, model.Trade{Date: date, TradeType: "매도", StockName: name,
				Quantity: sellQty, Price: sellPrice, Amount: sellAmt, Currency: "KRW",
				ExchangeRate: 1, AmountKRW: sellAmt, Fee: fee, Profit: profit,
				ProfitKRW: profit, ProfitRate: profitRate, Account: account})
		}
	}
	return out, nil
}
```

`hasAll`/`trimSpace` 헬퍼를 `parser.go`에 추가:
```go
func trimSpace(s string) string { return strings.TrimSpace(s) }

func hasAll(header []string, keys ...string) bool {
	set := map[string]bool{}
	for _, h := range header {
		set[strings.Trim(strings.TrimSpace(h), `"`)] = true
	}
	for _, k := range keys {
		if !set[k] {
			return false
		}
	}
	return true
}
```

- [ ] **Step 5: 통과 확인 + Commit**

```bash
go test ./internal/parser/ -run TestMiraeDomestic -v
git add internal/parser/mirae.go internal/parser/mirae_test.go testdata/mirae_domestic.csv internal/parser/parser.go
git commit -m "feat: 미래에셋 국내 파서 포팅"
```

---

### Task 6: 미래에셋 해외 파서

**Files:**
- Modify: `internal/parser/mirae.go` (MiraeForeign 추가)
- Test: `internal/parser/mirae_test.go` (케이스 추가)
- Fixture: `testdata/mirae_foreign.csv`

Python 참조: `mirae_parser.py:MiraeForeignParser` (25컬럼, 헤더 1행 스킵, can_parse `{매매일, 통화, 종목번호}`). 컬럼 매핑은 Python docstring 그대로: exchange_rate=col6, buy=7/8/9/10(krw), sell=11/12/13/14(krw), fee=15, tax=16, profit=col19, profit_krw=col22(총평가손익), rate=col23. stock_code=col2.

- [ ] **Step 1: 픽스처** `testdata/mirae_foreign.csv` — 헤더 1행(키워드 포함) + 25컬럼 데이터 2행(매수 1, 매도 1).

- [ ] **Step 2: 실패 테스트**

```go
func TestMiraeForeign_Parse(t *testing.T) {
	p := MiraeForeign{}
	assert.True(t, p.CanParse([]string{"매매일", "통화", "종목번호"}))
	trades, err := p.Parse("../../testdata/mirae_foreign.csv", "미래에셋증권_해외계좌")
	require.NoError(t, err)
	require.NotEmpty(t, trades)
	assert.Equal(t, "USD", trades[0].Currency)
	assert.NotEqual(t, 1.0, trades[0].ExchangeRate) // 환율 반영
	assert.NotEmpty(t, trades[0].StockCode)         // 종목번호(티커)
}
```

- [ ] **Step 3: 실패 확인** → `go test ./internal/parser/ -run TestMiraeForeign -v` FAIL

- [ ] **Step 4: 구현** — `MiraeForeignParser.parse`를 옮긴다. 데이터는 1행 헤더만 스킵(인덱스 1부터), `len(row) < 25` 스킵, 동일한 빈행/에러 규칙. 매수 Trade는 fee/tax/profit 0, 매도 Trade는 fee=col15, tax=col16, profit=col19, profit_krw=col22, rate=col23. currency=col1, stock_code=col2, exchange_rate=col6, amount_krw=col10(매수)/col14(매도).

- [ ] **Step 5: 통과 확인 + Commit**

```bash
go test ./internal/parser/ -run "TestMirae" -v
git add internal/parser/mirae.go internal/parser/mirae_test.go testdata/mirae_foreign.csv
git commit -m "feat: 미래에셋 해외 파서 포팅"
```

---

### Task 7: 한국투자 국내 파서

**Files:**
- Create: `internal/parser/hankook.go`
- Test: `internal/parser/hankook_test.go`
- Fixture: `testdata/hankook_domestic.csv`

Python 참조: `hankook_parser.py:HankookDomesticParser` (17컬럼 쌍따옴표, 헤더 1행 스킵, can_parse `{매매일자, 종목코드, 매입단가}`). 매핑: name=col1, code=col2, buy_price=col6, buy_qty=col7, sell_price=col8, sell_qty=col9, buy_amount=col10, sell_amount=col11, realized_profit=col12, profit_rate=col13, commission=col14, tax=col16. **매수 조건 `buy_qty>0 AND buy_amount>0`, 매도 조건 `sell_qty>0 AND sell_amount>0`**(미래에셋과 다름). 매도 fee=commission+tax.

- [ ] **Step 1: 픽스처** (쌍따옴표 포함 17컬럼 2행)

- [ ] **Step 2: 실패 테스트**

```go
func TestHankookDomestic_Parse(t *testing.T) {
	p := HankookDomestic{}
	assert.True(t, p.CanParse([]string{"매매일자", "종목코드", "매입단가"}))
	trades, err := p.Parse("../../testdata/hankook_domestic.csv", "한국투자증권_국내계좌")
	require.NoError(t, err)
	require.NotEmpty(t, trades)
	for _, tr := range trades {
		assert.NotEmpty(t, tr.StockCode) // 한투는 CSV에 코드 있음
		assert.Equal(t, "KRW", tr.Currency)
	}
}
```

- [ ] **Step 3: 실패 확인** → FAIL

- [ ] **Step 4: 구현** — 위 매핑/조건 그대로. `encoding/csv`는 쌍따옴표를 자동 처리하므로 `parseFloat`의 추가 `"` 제거는 방어적으로 유지.

- [ ] **Step 5: 통과 확인 + Commit**

```bash
go test ./internal/parser/ -run TestHankook -v
git add internal/parser/hankook.go internal/parser/hankook_test.go testdata/hankook_domestic.csv
git commit -m "feat: 한국투자 국내 파서 포팅"
```

---

### Task 8: 파서 레지스트리

**Files:**
- Create: `internal/parser/registry.go`
- Test: `internal/parser/registry_test.go`

Python 참조: `modules/parser_registry.py` (첫 행 읽어 `strip().strip('"')` 정규화 후 PARSERS 순서대로 can_parse 매칭, 실패 시 error). **순서: MiraeDomestic, MiraeForeign, HankookDomestic.**

- [ ] **Step 1: 실패 테스트**

```go
func TestDetectParser(t *testing.T) {
	p, err := DetectParser("../../testdata/mirae_domestic.csv")
	require.NoError(t, err)
	assert.Equal(t, "MiraeDomesticParser", p.Name())

	p, err = DetectParser("../../testdata/hankook_domestic.csv")
	require.NoError(t, err)
	assert.Equal(t, "HankookDomesticParser", p.Name())

	_, err = DetectParser("../../testdata/unsupported.csv")
	require.Error(t, err)
}
```
픽스처 `testdata/unsupported.csv`: 매칭 안 되는 헤더 1행.

- [ ] **Step 2: 실패 확인** → FAIL

- [ ] **Step 3: 구현**

```go
package parser

import (
	"encoding/csv"
	"fmt"
	"os"
)

var registry = []Parser{MiraeDomestic{}, MiraeForeign{}, HankookDomestic{}}

func DetectParser(path string) (Parser, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("헤더 읽기 실패: %s: %w", base(path), err)
	}
	for _, p := range registry {
		if p.CanParse(header) {
			return p, nil
		}
	}
	return nil, fmt.Errorf("지원되지 않는 CSV 포맷: %s", path)
}
```

- [ ] **Step 4: 통과 확인 + Commit**

```bash
go test ./internal/parser/ -v
git add internal/parser/registry.go internal/parser/registry_test.go testdata/unsupported.csv
git commit -m "feat: 파서 레지스트리(헤더 기반 자동 감지) 포팅"
```

---

### Task 9: KRX 종목 마스터 리졸버

**Files:**
- Create: `internal/symbol/resolver.go`
- Test: `internal/symbol/resolver_test.go`

Python 참조: `modules/symbol_master.py` 전체. KOSPI/KOSDAQ `.mst.zip` 다운로드 → cp949 디코드 → 한 행 `[0:9]=단축코드, [9:21]=표준코드, [21:len-fwfLen]=한글명` 추출 → `{한글명: 코드}` (첫 항목 우선). FWF: KOSPI 227, KOSDAQ 221. 캐시 `~/.cache/auto-trading-journal`, TTL 7일. 다운로드 실패 시 만료 캐시 fallback. URL은 Python 상수 그대로.

- [ ] **Step 1: 실패 테스트** (네트워크 없이 파싱 로직만 — fixture .mst 텍스트로 `parseMstLines` 검증)

```go
package symbol

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseMstLines(t *testing.T) {
	// 단축코드(9) + 표준코드(12) + 한글명 + fwf(여기선 5바이트 가정)
	line := "005930   " + "KR7005930003" + "삼성전자" + "XXXXX"
	got := parseMstLines(line+"\n", 5)
	assert.Equal(t, "005930", got["삼성전자"])
}
```

- [ ] **Step 2: 실패 확인** → FAIL

- [ ] **Step 3: 구현** — `Resolver` struct(lazy 로드), `Resolve(name) string`, 내부 `parseMstLines(text, fwfLen)`, `fetchZip`(캐시 TTL + fallback), `loadAll`(KOSPI+KOSDAQ 병합). cp949 디코딩은 `korean.EUCKR.NewDecoder()` 사용. 미발견 시 경고 1회(중복 억제).

```go
package symbol

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/transform"
)

const (
	kospiURL      = "https://new.real.download.dws.co.kr/common/master/kospi_code.mst.zip"
	kosdaqURL     = "https://new.real.download.dws.co.kr/common/master/kosdaq_code.mst.zip"
	kospiFWFLen   = 227
	kosdaqFWFLen  = 221
	cacheTTL      = 7 * 24 * time.Hour
)

type Resolver struct {
	m      map[string]string
	loaded bool
	warned map[string]bool
}

func New() *Resolver { return &Resolver{warned: map[string]bool{}} }

func (r *Resolver) Resolve(name string) string {
	r.ensure()
	code := r.m[strings.TrimSpace(name)]
	return code
}

func (r *Resolver) ensure() {
	if r.loaded {
		return
	}
	r.loaded = true
	r.m = map[string]string{}
	for _, src := range []struct {
		url, cache string
		fwf        int
	}{{kospiURL, "kospi_code.mst.zip", kospiFWFLen}, {kosdaqURL, "kosdaq_code.mst.zip", kosdaqFWFLen}} {
		zipBytes, err := fetchZip(src.url, src.cache)
		if err != nil {
			continue // Python: 실패 시 보강 건너뜀
		}
		text, err := extractMst(zipBytes)
		if err != nil {
			continue
		}
		for name, code := range parseMstLines(text, src.fwf) {
			if _, ok := r.m[name]; !ok {
				r.m[name] = code
			}
		}
	}
}

func parseMstLines(text string, fwfLen int) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		if len(line) < fwfLen+21 {
			continue
		}
		code := strings.TrimSpace(line[0:9])
		name := strings.TrimSpace(line[21 : len(line)-fwfLen])
		if code != "" && name != "" {
			if _, ok := out[name]; !ok {
				out[name] = code
			}
		}
	}
	return out
}

func cacheDir() string {
	d, _ := os.UserHomeDir()
	p := filepath.Join(d, ".cache", "auto-trading-journal")
	os.MkdirAll(p, 0o755)
	return p
}

func fetchZip(url, cacheName string) ([]byte, error) {
	path := filepath.Join(cacheDir(), cacheName)
	if fi, err := os.Stat(path); err == nil && time.Since(fi.ModTime()) < cacheTTL {
		return os.ReadFile(path)
	}
	resp, err := http.Get(url)
	if err == nil {
		defer resp.Body.Close()
		data, rerr := io.ReadAll(resp.Body)
		if rerr == nil {
			if _, zerr := zip.NewReader(bytes.NewReader(data), int64(len(data))); zerr == nil {
				os.WriteFile(path, data, 0o644)
				return data, nil
			}
		}
	}
	if b, ferr := os.ReadFile(path); ferr == nil { // 만료 캐시 fallback
		return b, nil
	}
	return nil, err
}

func extractMst(zipBytes []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return "", err
	}
	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, ".mst") {
			rc, err := f.Open()
			if err != nil {
				return "", err
			}
			defer rc.Close()
			dec := transform.NewReader(rc, korean.EUCKR.NewDecoder())
			b, err := io.ReadAll(dec)
			if err != nil {
				return "", err
			}
			return string(b), nil
		}
	}
	return "", io.EOF
}
```

- [ ] **Step 4: 통과 확인 + Commit**

```bash
go test ./internal/symbol/ -v
git add internal/symbol/
git commit -m "feat: KRX 종목 마스터 리졸버 포팅 (.mst fwf + cp949 + 캐시)"
```

---

## Phase 3 — Sheets 클라이언트 + Writer

> 이 단계 완료 시 "CSV→거래 시트 쓰기"가 동작해야 한다. 단계 끝에서 dry-run 아닌 실제 쓰기로 Python 결과와 시트를 대조한다.

### Task 10: Sheets 클라이언트 — 인증 + 값 I/O

**Files:**
- Create: `internal/sheets/client.go`
- Test: `internal/sheets/client_test.go` (값 변환 등 순수 함수 단위테스트만; API 호출은 Phase 통합검증에서)

Python 참조: `modules/google_sheets_client.py` 의 `__init__`, `get_sheet_id`, `list_sheets`, `get_sheet_data`, `get_raw_grid_data`, `update_cells`, `batch_update_cells`, `clear_sheet`.

- [ ] **Step 1: 클라이언트 골격 + 인증 구현**

```go
package sheets

import (
	"context"

	"google.golang.org/api/option"
	gsheets "google.golang.org/api/sheets/v4"
)

type Client struct {
	svc           *gsheets.Service
	spreadsheetID string
	sheetIDCache  map[string]int64
}

func New(ctx context.Context, spreadsheetID, serviceAccountPath string) (*Client, error) {
	svc, err := gsheets.NewService(ctx, option.WithCredentialsFile(serviceAccountPath))
	if err != nil {
		return nil, err
	}
	return &Client{svc: svc, spreadsheetID: spreadsheetID, sheetIDCache: map[string]int64{}}, nil
}
```

- [ ] **Step 2: 값 I/O 메서드 포팅** — `ListSheets() ([]string,error)`, `GetSheetID(name) (int64,bool,error)`(+캐시/`InvalidateSheetIDCache`), `GetValues(rangeA1) ([][]interface{},error)`, `UpdateCells(rangeA1, [][]any) error`(USER_ENTERED), `BatchUpdateValues(map[string][][]any) error`, `ClearValues(rangeA1) error`, `GetRawGridData(rangeA1) (*gsheets.GridData,error)`. 각 메서드는 Python 동명 메서드의 인자/동작을 따른다.

- [ ] **Step 3: 빌드 확인**

Run: `go build ./internal/sheets/` → 에러 없음

- [ ] **Step 4: Commit**

```bash
git add internal/sheets/
git commit -m "feat: Sheets 클라이언트 인증 + 값 I/O 포팅"
```

---

### Task 11: Sheets 클라이언트 — 포맷/색상/필터/차트 + 레이트리밋

**Files:**
- Modify: `internal/sheets/client.go` (또는 `internal/sheets/format.go`, `internal/sheets/chart.go`로 분리)
- Test: `internal/sheets/format_test.go` (요청 빌더 순수 함수)

Python 참조: `google_sheets_client.py` 의 `apply_color_to_range`, `apply_number_format`, `apply_number_format_to_columns`, `batch_apply_colors`, `create_sheet`, `delete_sheet`, `clear_background_colors`, `clear_number_formats`, `freeze_rows`, `set_auto_filter`, `get_charts`, `delete_all_charts`, `add_charts`, `build_number_format_requests`, `build_text_format_requests`, `build_color_requests`, `execute_batch_requests`, `apply_sheet_formatting_batch`, `_execute_with_retry`(레이트리밋 재시도).

**중요 불변식:** #57 최적화 — 가능한 모든 포맷/색상 요청은 **단일 `batchUpdate`로 묶어** 보낸다(요청 수 최소화). `build_*_requests`는 API 호출 없는 순수 빌더로 유지.

- [ ] **Step 1: 요청 빌더 순수 함수 실패 테스트** (`buildNumberFormatRequests`, `buildTextFormatRequests`, `buildColorRequests` 가 올바른 `*gsheets.Request` 생성하는지)

```go
func TestBuildTextFormatRequests(t *testing.T) {
	reqs := buildTextFormatRequests(123, 3, 2, 5) // sheetID, col(1-based), startRow, endRow
	require.Len(t, reqs, 1)
	rc := reqs[0].RepeatCell
	assert.Equal(t, int64(123), rc.Range.SheetId)
	assert.Equal(t, int64(2), rc.Range.StartColumnIndex) // col-1
	assert.Equal(t, "@", rc.Cell.UserEnteredFormat.NumberFormat.Pattern)
}
```

- [ ] **Step 2: 실패 확인** → FAIL

- [ ] **Step 3: 구현** — 빌더 3종 + 위 API 메서드들. 0-인덱스 GridRange는 스파이크에서 검증된 대로 `ForceSendFields`로 0값 강제 전송. `executeWithRetry`는 429/5xx에 지수백오프 재시도(Python `_execute_with_retry`와 동일 정책). 차트 추가/삭제는 검증된 스파이크 패턴 사용.

- [ ] **Step 4: 통과 확인 + Commit**

```bash
go test ./internal/sheets/ -v
git add internal/sheets/
git commit -m "feat: Sheets 포맷/색상/필터/차트 + 레이트리밋 재시도 포팅"
```

---

### Task 12: Writer — 헤더/상수 + 시트 보장/포맷

**Files:**
- Create: `internal/writer/headers.go`, `internal/writer/writer.go`
- Test: `internal/writer/headers_test.go`

Python 참조: `modules/sheet_writer.py:14-117` (DOMESTIC_HEADERS 10컬럼, FOREIGN_HEADERS 15컬럼, OLD_DOMESTIC_HEADERS_V1 9컬럼, DOMESTIC_FORMATS, FOREIGN_FORMATS_COMMON, FOREIGN_CURRENCY_COLS=[7,8,11,12,13], CURRENCY_PATTERNS, ensure_sheet_exists, apply_sheet_formatting).

- [ ] **Step 1: 헤더 상수 테스트**

```go
func TestHeaders(t *testing.T) {
	assert.Len(t, DomesticHeaders, 10)
	assert.Equal(t, "종목코드", DomesticHeaders[2])
	assert.Equal(t, "종목명", DomesticHeaders[3])
	assert.Len(t, ForeignHeaders, 15)
	assert.Len(t, OldDomesticHeadersV1, 9)
}
```

- [ ] **Step 2: 실패 확인** → FAIL

- [ ] **Step 3: 구현** — 헤더/포맷 상수(Python 값 그대로) + `Writer` struct(`New(*sheets.Client)`) + `EnsureSheetExists(ctx, name, isForeign) (bool,error)`(없으면 생성+헤더 삽입+freeze+filter) + `ApplySheetFormatting(ctx, name, isForeign)`(종목코드 컬럼 TEXT@ 등). 헤더는 `client.GetSheetID` 캐시 무효화 처리 포함.

- [ ] **Step 4: 통과 확인 + Commit**

```bash
go test ./internal/writer/ -v
git add internal/writer/headers.go internal/writer/headers_test.go internal/writer/writer.go
git commit -m "feat: Writer 헤더/상수 + 시트 보장/포맷 포팅"
```

---

### Task 13: Writer — 중복키 + 삽입(색상/포맷)

**Files:**
- Modify: `internal/writer/writer.go`
- Test: `internal/writer/writer_test.go`

Python 참조: `sheet_writer.py:119-293` (`get_existing_keys` 국내/해외 컬럼 위치, `find_last_row`, `insert_trades` 날짜별 색상 8색 순환·숫자포맷·TEXT 종목코드, `_normalize_num`, `_group_consecutive_rows`, `_col_letter`).

**불변식:** 8색 팔레트 날짜별 순환(같은 날짜=같은 색), 종목코드 TEXT@, 삽입은 단일 batch 지향.

- [ ] **Step 1: 순수 헬퍼 실패 테스트** (`normalizeNum`, `groupConsecutiveRows`, `colLetter`, 그리고 `existingKeysFromGrid`가 국내/해외 컬럼에서 올바른 DupKey 추출)

```go
func TestColLetter(t *testing.T) {
	assert.Equal(t, "A", colLetter(1))
	assert.Equal(t, "Z", colLetter(26))
	assert.Equal(t, "AA", colLetter(27))
}
func TestGroupConsecutiveRows(t *testing.T) {
	assert.Equal(t, [][2]int{{2, 4}, {6, 6}}, groupConsecutiveRows([]int{2, 3, 4, 6}))
}
```

- [ ] **Step 2: 실패 확인** → FAIL

- [ ] **Step 3: 구현** — 헬퍼 + `GetExistingKeys(ctx, name, isForeign) (map[model.DupKey]bool, error)`(국내 cols 0,1,3,4,5 / 해외 0,1,4,5,6; 숫자는 정규화 문자열로 비교) + `FindLastRow` + `InsertTrades(ctx, name, trades, isForeign) (int,error)`(행 변환 → 단일 values write → 날짜별 색상 batch → 숫자/TEXT 포맷 batch). 색상 팔레트는 Python의 8색 RGB 그대로.

- [ ] **Step 4: 통과 확인 + Commit**

```bash
go test ./internal/writer/ -v
git add internal/writer/
git commit -m "feat: Writer 중복필터 + 삽입(날짜별 색상/포맷) 포팅"
```

---

### Task 14: Writer — read_all_trades (대시보드 입력)

**Files:**
- Modify: `internal/writer/writer.go` (또는 `internal/writer/reader.go`)
- Test: `internal/writer/reader_test.go`

Python 참조: `sheet_writer.py:295-534` (`read_all_trades` 헤더 검증으로 매매일지 시트 식별, 국내/해외 분기, OLD_V1 9컬럼은 경고 후 스킵; `_row_to_trade`, `_get_num/_get_str/_get_code`, `_extract_header_row`).

- [ ] **Step 1: `rowToTrade` 실패 테스트** (국내/해외 행 → Trade 역변환; 수익률 소수→퍼센트 복원 등 Python과 동일)

```go
func TestRowToTradeDomestic(t *testing.T) {
	row := []interface{}{"2026-02-13", "매도", "005930", "삼성전자", "10", "75000", "750000", "1500", "50000", "0.0714"}
	tr := rowToTrade(row, false, "미래에셋증권_국내계좌")
	assert.Equal(t, "삼성전자", tr.StockName)
	assert.Equal(t, "005930", tr.StockCode)
	assert.InDelta(t, 7.14, tr.ProfitRate, 0.01)
}
```

- [ ] **Step 2: 실패 확인** → FAIL

- [ ] **Step 3: 구현** — `rowToTrade` + getter 헬퍼 + `ReadAllTrades(ctx) ([]model.Trade, error)`(전 시트 순회 → 헤더 매칭 DOMESTIC/FOREIGN → 행 파싱; OLD_V1은 경고 로그+스킵).

- [ ] **Step 4: 통과 확인 + Commit**

```bash
go test ./internal/writer/ -v
git add internal/writer/
git commit -m "feat: Writer read_all_trades(대시보드 입력) 포팅"
```

- [ ] **Step 5: Phase 3 동등성 검증** — `input/`에 샘플 CSV를 두고 Python(`python main.py`)으로 거래 시트를 만든 뒤, 별도 테스트 스프레드시트에서 Go 미니 드라이버(임시 `cmd/atj` 부분 or 임시 테스트)로 동일 삽입 → 시트 값/색상/포맷이 일치하는지 육안+값 비교. 차이가 있으면 해당 Task로 돌아가 수정.

---

## Phase 4 — 대시보드(summary)

> 실제 코드는 단일 **"대시보드"** 시트로 통합(`DASHBOARD_SHEET="대시보드"`). 섹션: 포트폴리오 요약 / 월별 성과 / 종목별 현황 / 투자 지표 / 매매 인사이트 / 월별 추이 / (숨김)거래건수 데이터 / 차트(basic+pie). 섹터별 투자비중은 투자지표 섹션 내 OpenAI 사용.

### Task 15: summary 골격 + 대시보드 시트 확보 + generateAll

**Files:**
- Create: `internal/summary/summary.go`
- Test: `internal/summary/summary_test.go`

Python 참조: `summary_generator.py:1-126` (상수 DASHBOARD_SHEET, CHART_COL_START=13, CHART_COL_SECONDARY=20, CHART_ROW_SPACING=20; `__init__`, `generate_all`, `_ensure_dashboard_sheet`).

- [ ] **Step 1: 골격 구현** — `Generator` struct(`New(client, writer, sectorClassifier)`; sectorClassifier는 nil 허용), 상수, `EnsureDashboardSheet(ctx)`(없으면 생성, 있으면 초기화: clear values/colors/formats + 차트 삭제), `GenerateAll(ctx, trades)` 스켈레톤(섹션 호출 순서만, 각 섹션은 다음 태스크에서).

- [ ] **Step 2: 빌드 + 최소 테스트**(생성자/상수) 통과

- [ ] **Step 3: Commit**

```bash
git add internal/summary/summary.go internal/summary/summary_test.go
git commit -m "feat: summary 골격 + 대시보드 시트 확보 포팅"
```

---

### Task 16: 요약 섹션 — 포트폴리오/월별/종목별

**Files:**
- Modify: `internal/summary/summary.go`
- Test: `internal/summary/summary_test.go`

Python 참조: `summary_generator.py:127-267` (`_write_portfolio_summary`, `_write_monthly_summary`, `_write_stock_summary`). 각 함수는 trades + start_row → 작성 후 다음 start_row 반환. 집계 키/정렬/컬럼 구성을 Python 그대로.

- [ ] **Step 1: 집계 순수 함수 실패 테스트** — 월별 집계((연월,계좌)→매수/매도/손익), 종목별 집계((종목명,코드,계좌,통화)) 결과가 Python과 동일한지 작은 입력으로 검증.

- [ ] **Step 2: 실패 확인** → FAIL

- [ ] **Step 3: 구현** — 세 섹션 작성 함수 + 집계 헬퍼. 시트 쓰기는 `client.UpdateCells`/batch 사용. 반환 start_row 누적.

- [ ] **Step 4: 통과 확인 + Commit**

```bash
go test ./internal/summary/ -v
git add internal/summary/
git commit -m "feat: 대시보드 요약 섹션(포트폴리오/월별/종목별) 포팅"
```

---

### Task 17: 투자 지표 + 매매 인사이트 + 월별 추이

**Files:**
- Create: `internal/summary/insights.go`
- Test: `internal/summary/insights_test.go`

Python 참조: `summary_generator.py:269-686` (`_write_investment_metrics`, `_write_trading_insights`, `_write_monthly_trend`, `_calc_monthly_trend`, `_calc_streaks`, `_calc_day_of_week_stats`, `_calc_monthly_profits`, `_calc_trade_frequency`).

- [ ] **Step 1: 계산 헬퍼 실패 테스트** — `calcStreaks`(연승/연패), `calcDayOfWeekStats`, `calcMonthlyProfits`, `calcTradeFrequency`를 Python 결과와 동일하게. 매도 거래 정렬 입력 기준.

```go
func TestCalcStreaks(t *testing.T) {
	// 손익 부호 시퀀스로 최대연승/최대연패/현재연속 등 Python 반환과 동일 검증
}
```

- [ ] **Step 2: 실패 확인** → FAIL

- [ ] **Step 3: 구현** — 계산 헬퍼 + 3개 섹션 작성 함수. 부동소수/반올림은 Python과 동일 표현(시트 표시값 기준).

- [ ] **Step 4: 통과 확인 + Commit**

```bash
go test ./internal/summary/ -v
git add internal/summary/
git commit -m "feat: 투자 지표/인사이트/월별 추이 포팅"
```

---

### Task 18: 거래건수 데이터 + 포맷/색상 수집·일괄 적용

**Files:**
- Modify: `internal/summary/summary.go`
- Test: `internal/summary/summary_test.go`

Python 참조: `summary_generator.py:688-871` (`_collect_header_colors`, `_collect_dashboard_formats`, `_collect_metrics_formats`, `_group_consecutive_rows`, `_flush_pending_requests`, `_write_trade_count_data`).

**불변식:** 포맷/색상 요청을 `pendingRequests`에 모았다가 **한 번에 flush**(요청 수 최소화). 거래건수 데이터는 차트 소스용 숨김 영역.

- [ ] **Step 1: `groupConsecutiveRows` + 포맷 수집 단위 테스트**(writer의 동명 헬퍼와 동일 동작이면 공유 검토하되, 우선 summary 내 동작 검증)

- [ ] **Step 2: 실패 확인** → FAIL

- [ ] **Step 3: 구현** — pending 수집 메서드 + flush + 거래건수 데이터 작성.

- [ ] **Step 4: 통과 확인 + Commit**

```bash
go test ./internal/summary/ -v
git add internal/summary/
git commit -m "feat: 대시보드 포맷/색상 일괄 적용 + 거래건수 데이터 포팅"
```

---

### Task 19: 차트 (basic + pie)

**Files:**
- Create: `internal/summary/charts.go`
- Test: `internal/summary/charts_test.go`

Python 참조: `summary_generator.py:872-1107` (`_create_charts`, `_build_basic_chart_spec`, `_build_pie_chart_spec`, 내부 `source_range`). 차트 위치 상수(CHART_COL_START/SECONDARY/ROW_SPACING) 그대로.

- [ ] **Step 1: 차트 스펙 빌더 실패 테스트** — `buildBasicChartSpec`/`buildPieChartSpec`가 올바른 `*gsheets.EmbeddedChart`(타입/도메인/시리즈/위치) 생성. 스파이크 패턴과 동일 구조.

- [ ] **Step 2: 실패 확인** → FAIL

- [ ] **Step 3: 구현** — 빌더 2종 + `CreateCharts(ctx, ...)`(대시보드 시트 ID 조회 → 스펙 조립 → `client.AddCharts` 단일 호출). 사전 `DeleteAllCharts`로 중복 방지.

- [ ] **Step 4: 통과 확인 + Commit**

```bash
go test ./internal/summary/ -v
git add internal/summary/
git commit -m "feat: 대시보드 차트(basic/pie) 생성 포팅"
```

---

### Task 20: OpenAI 섹터 분류기 + 섹터 집계 연결

**Files:**
- Create: `internal/sector/classifier.go`
- Test: `internal/sector/classifier_test.go`
- Modify: `internal/summary/summary.go` (섹터별 투자비중 연결: `_get_sector_map`)

Python 참조: `modules/sector_classifier.py` 전체 + `summary_generator.py:312-324, 1108-1117`. SECTORS 12종, SYSTEM_PROMPT, 캐시 `{종목명: 섹터}` JSON, 국내/해외 분리 배치 호출, JSON mode·temperature 0, 유효섹터 필터·누락 "기타".

- [ ] **Step 1: 캐시 로드/저장 + 유효섹터 필터 실패 테스트** (네트워크 없이 — 캐시 hit 경로와 `validateSectors` 동작)

```go
func TestValidateSectors(t *testing.T) {
	in := map[string]string{"삼성전자": "IT", "이상한종목": "없는섹터"}
	out := validateSectors(in, []string{"삼성전자", "이상한종목"})
	assert.Equal(t, "IT", out["삼성전자"])
	assert.Equal(t, "기타", out["이상한종목"]) // 알 수 없는 섹터 → 기타
}
```

- [ ] **Step 2: 실패 확인** → FAIL

- [ ] **Step 3: 구현** — `Classifier` struct(`New(apiKey, model, cachePath)`; go-openai client), `Classify(ctx, stocks []Stock) (map[string]string, error)`(캐시 우선 → 미캐시만 국내/해외 분리 호출 → 검증 → 캐시 저장). 캐시 파일 포맷은 Python과 동일(JSON map, indent 2, ensure_ascii=false). `validateSectors` 순수 함수.

- [ ] **Step 4: summary 연결** — `Generator`가 nil 아닌 classifier 보유 시 투자지표 섹션에 "섹터별 투자비중" 블록 추가(매수 trades amount_krw 기준 비중, Python `_get_sector_map` + `summary:312-324`와 동일).

- [ ] **Step 5: 통과 확인 + Commit**

```bash
go test ./internal/sector/ ./internal/summary/ -v
git add internal/sector/ internal/summary/
git commit -m "feat: OpenAI 섹터 분류기 + 섹터별 투자비중 연결 포팅"
```

---

## Phase 5 — 통합 + 정리

### Task 21: cmd/atj main 오케스트레이션

**Files:**
- Create: `cmd/atj/main.go`
- Test: `cmd/atj/main_test.go` (scan/enrich 등 순수 로직)

Python 참조: `main.py` 전체 (`StockDataProcessor`, `scan_csv_files` NFC 정규화, `process_file`(파서감지→파싱→코드보강→정렬→시트보장→중복필터→삽입), `run`(스캔→삽입→대시보드 갱신), `enrich_domestic_codes`, argparse `--dry-run`/`--log-level`).

- [ ] **Step 1: scan/enrich 실패 테스트**

```go
func TestEnrichDomesticCodes(t *testing.T) {
	trades := []model.Trade{
		{StockName: "삼성전자", Account: "미래에셋증권_국내계좌"}, // 코드 빈→보강
		{StockName: "AAPL", StockCode: "AAPL", Account: "미래에셋증권_해외계좌"}, // 해외 미보강
	}
	enrichDomesticCodes(trades, stubResolver{"삼성전자": "005930"})
	assert.Equal(t, "005930", trades[0].StockCode)
	assert.Equal(t, "AAPL", trades[1].StockCode)
}
```

- [ ] **Step 2: 실패 확인** → FAIL

- [ ] **Step 3: 구현** — `flag` 패키지로 `--dry-run`/`--log-level`, config 로드, 클라이언트/writer/summary/sector/resolver 조립, `scanCSVFiles`(input/ 순회, NFC 정규화 — `golang.org/x/text/unicode/norm`), `processFile`, `run`. 로깅은 표준 `log/slog`로 레벨 매핑. 섹터는 `STOCK_DATA_OPENAI_API_KEY` 있을 때만 활성. dry-run 시 삽입/대시보드 스킵(로그만).

- [ ] **Step 4: 빌드 + 테스트 통과**

```bash
go build ./... && go test ./... -v
```

- [ ] **Step 5: Commit**

```bash
git add cmd/ internal/
git commit -m "feat: cmd/atj 메인 오케스트레이션 포팅"
```

---

### Task 22: 엔드투엔드 동등성 검증

**Files:** (코드 변경 없음; 검증 절차)

- [ ] **Step 1: dry-run 동등성** — 동일 `input/`으로 Python `python main.py --dry-run`과 Go `go run ./cmd/atj --dry-run` 로그/처리 건수 비교. 파서 선택·건수·중복 건너뜀 수치 일치 확인.

- [ ] **Step 2: 실쓰기 동등성(테스트 스프레드시트)** — 임시 스프레드시트 ID로 양쪽 실행 후 거래 시트 + 대시보드(요약/지표/인사이트/추이/차트)를 값·포맷·색상·차트 유무까지 대조. (섹터 집계는 `STOCK_DATA_OPENAI_API_KEY` 설정 시 포함.)

- [ ] **Step 3: 불일치 수정** — 차이 발견 시 해당 Task로 회귀 수정 후 재검증. 모두 일치할 때까지 반복.

- [ ] **Step 4: 검증 결과 기록** — `docs/superpowers/plans/2026-06-07-go-porting.md` 하단 또는 PR 본문에 비교 결과 요약.

---

### Task 23: Python 제거 + 문서 갱신

**Files:**
- Delete: `main.py`, `modules/`, `tests/`(Python), `pyproject.toml`, `run.sh`(또는 Go 실행용으로 교체)
- Modify: `CLAUDE.md`, `README`(있으면), 워크스페이스 루트 `CLAUDE.md`의 auto-trading-journal 섹션

- [ ] **Step 1: Go 실행 스크립트/Makefile 작성** — `Makefile`(`make run`, `make test`, `make build`) 또는 `run.sh`를 `go run ./cmd/atj`로 교체. CSV 인코딩 변환 사전단계는 Go가 cp949를 직접 처리하므로 제거.

- [ ] **Step 2: Python 자산 제거**

```bash
git rm -r main.py modules/ tests/ pyproject.toml
git rm run.sh   # Go 버전으로 교체했다면
```

- [ ] **Step 3: 문서 갱신** — `auto-trading-journal/CLAUDE.md`를 Go 구조/명령(`make run`, `go test ./...`)으로 갱신. 워크스페이스 루트 `CLAUDE.md` 표의 tech stack을 `Go 1.25, testify, Google Sheets API v4`로 수정. "신규 per-row 섹터/산업 열" 및 "KIS SDK 연동"은 향후 작업으로 명시.

- [ ] **Step 4: 최종 빌드/테스트 + 인코딩 확인**

```bash
go build ./... && go test ./...
file -I CLAUDE.md  # charset=utf-8 확인
```

- [ ] **Step 5: Commit + PR**

```bash
git add -A
git commit -m "chore: Python 자산 제거 + 문서 Go 기준 갱신"
git push -u origin feature/go-porting
gh pr create --title "feat: auto-trading-journal Go 전면 포팅" --body "$(cat <<'EOF'
## Summary
- Python(~3,760줄) → Go 전면 포팅 (동작 동등)
- config/model/parser/symbol/sheets/writer/summary/sector 패키지 + cmd/atj
- Google Sheets v4 패리티(차트 포함) 사전 스파이크로 검증
- #57 레이트리밋 최적화 보존, KRX .mst cp949 네이티브 처리(iconv 제거)

## 범위 밖
- 신규 per-row 섹터/산업 열 (향후 별도: KIS SearchStockInfo 지수업종 활용)
- KIS SDK import

## Test plan
- [ ] go test ./... 전체 통과
- [ ] Python vs Go dry-run 처리 건수 동등
- [ ] 테스트 스프레드시트에서 거래 시트 + 대시보드 + 차트 동등
EOF
)"
```

---

## 자기 점검 결과 (작성자)

- **Spec 커버리지:** spec의 모든 모듈(config/model/parser/sheets/writer/summary/symbol/sector/main)이 Task 1~21에 매핑됨. 단계별 동등성 검증 = Task 14 Step5 / Task 22. Python 제거·문서 = Task 23. 범위 밖(섹터열/SDK) 명시됨.
- **플레이스홀더:** 큰 모듈(sheets/summary)은 Python 소스를 동작 명세로 인용(라인 범위 명시) + Go 시그니처/테스트 계약 제시 — 포팅 특성상 정당. "TODO/적절히 처리" 류 없음.
- **타입 일관성:** `model.Trade`/`model.DupKey`, `parser.Parser`(Name/CanParse/Parse), `sheets.Client`/`writer.Writer`/`summary.Generator`/`sector.Classifier` 명칭이 태스크 간 일치. 빌더 함수명(`buildTextFormatRequests` 등) 일관.
