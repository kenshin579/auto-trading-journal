# 대시보드 "지수 vs 나머지 투자" 구현 계획

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 대시보드에 시장대표 지수(S&P500·나스닥·코스피/코스닥 등)와 그 외에 각각 얼마를 투자하고 있는지 누적 매수금액과 보유 원금 두 기준으로 보여주는 섹션과 파이 차트를 추가한다.

**Architecture:** ETF 판별/분류를 데이터 소스 단계에서 정확하게 만들고(해외는 FMP `IsEtf`/`IsFund` 플래그, 국내는 기존 KIS 판별), 대시보드는 `(섹터, 산업) → (그룹, 버킷)` 순수 매핑만 한다. 거래 시트 스키마는 바뀌지 않는다.

**Tech Stack:** Go 1.x, `github.com/kenshin579/fmp-go`, `github.com/sashabaranov/go-openai`, Google Sheets API v4, testify

**설계 문서:** `docs/superpowers/specs/2026-08-01-dashboard-index-weight-design.md`

---

## 파일 구조

| 파일 | 책임 | 상태 |
|---|---|---|
| `internal/etfclass/classifier.go` | ETF 종목명 → 고정 taxonomy 카테고리 1개. 이것만 안다. | 신규(이동) |
| `internal/etfclass/classifier_test.go` | taxonomy 정규화·분류 규칙 테스트 | 신규 |
| `internal/bizcat/etfclass.go` | — | 삭제 |
| `internal/bizcat/resolver.go` | 국내 KIS 조회. 분류는 `etfclass` 에 위임 | 수정 |
| `internal/fmpcat/resolver.go` | 해외 FMP 조회 + ETF 판별/분류 | 수정 |
| `internal/summary/index_weight.go` | 버킷 매핑 · 집계 · 시트 작성 | 신규 |
| `internal/summary/index_weight_test.go` | 매핑/집계/행생성 테스트 | 신규 |
| `internal/summary/summary.go` | 섹션 삽입, 차트 범위 필드 | 수정 |
| `internal/summary/charts.go` | 파이 차트 1개 추가 | 수정 |
| `cmd/atj/main.go` | fmpcat 분류기 활성화 1줄 | 수정 |
| `CLAUDE.md` | 해외 ETF 설명 정정 + 새 섹션 기술 | 수정 |

**테스트 전략 메모:** 설계 문서는 fake Sheets 서버(`sheets.NewWithEndpoint`)로 섹션 작성을
검증한다고 적었지만, `internal/summary` 에는 그 테스트 인프라가 없다(현재 `writer`/`sheets`
패키지에만 있고, summary 테스트는 전부 순수 함수 단위다). 기존 패턴을 따라 셀 값 생성과
집계를 순수 함수(`indexWeightValues`, `indexWeightPieHelper`, `aggregateIndexWeight`)로 뽑아
검증하고, API 호출 배선은 `make dry` 로 확인한다.

---

### Task 1: `internal/etfclass` 패키지 분리 (동작 변경 없음)

국내(`bizcat`)와 해외(`fmpcat`) 가 같은 분류기를 써야 하므로 `bizcat` 밖으로 꺼낸다. 식별자 공개화와
**프롬프트 첫 줄에서 "한국 상장" 한정 문구를 빼는 것**(이제 해외 ETF 도 분류한다) 외에는 내용을
바꾸지 않는다 — taxonomy 와 나머지 프롬프트 문구는 Task 2 에서 바꾼다.

**Files:**
- Create: `internal/etfclass/classifier.go`
- Create: `internal/etfclass/classifier_test.go`
- Delete: `internal/bizcat/etfclass.go`
- Modify: `internal/bizcat/resolver.go:39-41`
- Modify: `internal/bizcat/resolver_test.go:124-132` (테스트 이동)

- [ ] **Step 1: 새 패키지 파일 생성**

`internal/etfclass/classifier.go`:

```go
// Package etfclass 는 ETF 종목명을 고정 taxonomy 의 카테고리 하나로 분류한다.
// 국내(internal/bizcat)와 해외(internal/fmpcat) 가 같은 체계를 공유하기 위해 분리돼 있다.
package etfclass

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

// Categories 는 모든 ETF 를 종목명으로 분류할 고정 taxonomy.
var Categories = []string{
	// 지역/시장대표
	"한국주식", "미국주식", "중국주식", "일본주식", "인도주식", "베트남주식", "글로벌주식",
	// 섹터/테마
	"반도체", "2차전지", "바이오·헬스케어", "AI·로봇", "신재생에너지", "원자력",
	"방위·우주항공", "자동차", "금융", "건설", "필수소비재", "IT·인터넷",
	// 자산군
	"배당", "리츠·부동산", "원자재", "채권", "통화·단기금리",
	// 기타
	"기타테마",
}

// FallbackCategory 는 taxonomy 밖 응답/분류 불가 시 사용.
const FallbackCategory = "기타테마"

var categorySet = func() map[string]bool {
	m := make(map[string]bool, len(Categories))
	for _, c := range Categories {
		m[c] = true
	}
	return m
}()

// Validate 는 분류 결과가 taxonomy 안에 있으면 그대로, 아니면 FallbackCategory 로 정규화한다.
func Validate(s string) string {
	s = strings.TrimSpace(s)
	if categorySet[s] {
		return s
	}
	return FallbackCategory
}

const classifyTimeout = 30 * time.Second

var systemPrompt = fmt.Sprintf(`당신은 ETF 분류 전문가입니다.
ETF 종목명을 보고 아래 카테고리 중 정확히 하나로 분류하세요.

사용 가능한 카테고리: %s

분류 기준:
- 특정 섹터·테마가 핵심이면 지역보다 해당 테마 우선
  (반도체/2차전지/바이오·헬스케어/AI·로봇/신재생에너지/원자력/방위·우주항공/자동차/금융/건설/필수소비재/IT·인터넷).
  예: "미국반도체"→반도체, "글로벌AI"→AI·로봇.
- 시장대표·종합 지수형은 투자 지역(한국주식/미국주식/중국주식/일본주식/인도주식/베트남주식/글로벌주식).
  예: "코스피200"/"코스닥150"→한국주식, "S&P500"/"나스닥100"→미국주식.
- 은행/보험/증권은 금융. 채권/국채/회사채는 채권. 단기자금/CD금리/통화는 통화·단기금리.
- 금·은·원유 등 상품은 원자재. 리츠·부동산·인프라는 리츠·부동산. 배당·커버드콜은 배당.
- 애매하거나 위에 없으면 기타테마.

반드시 JSON 으로만 응답: {"category": "<카테고리>"}`, strings.Join(Categories, ", "))

// New 는 ETF 종목명 → 카테고리 분류 함수를 만든다. apiKey 가 비면 nil 을 반환한다
// (호출부는 nil 이면 각자의 폴백을 사용). 결과는 호출부 영구 캐시에 저장돼 종목당 1회만 호출된다.
func New(apiKey, model string) func(name string) (string, error) {
	if apiKey == "" {
		return nil
	}
	if model == "" {
		model = "gpt-4o-mini"
	}
	client := openai.NewClient(apiKey)
	return func(name string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), classifyTimeout)
		defer cancel()
		resp, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
			Model: model,
			Messages: []openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
				{Role: openai.ChatMessageRoleUser, Content: "ETF 종목명: " + name},
			},
			Temperature: 0,
			ResponseFormat: &openai.ChatCompletionResponseFormat{
				Type: openai.ChatCompletionResponseFormatTypeJSONObject,
			},
		})
		if err != nil || len(resp.Choices) == 0 {
			slog.Warn("ETF OpenAI 분류 실패", "name", name, "err", err)
			return "", fmt.Errorf("etf classify: %w", err)
		}
		var out struct {
			Category string `json:"category"`
		}
		if err := json.Unmarshal([]byte(resp.Choices[0].Message.Content), &out); err != nil {
			slog.Warn("ETF OpenAI 응답 파싱 실패", "name", name, "err", err)
			return "", err
		}
		return Validate(out.Category), nil
	}
}
```

- [ ] **Step 2: 테스트 파일 생성 (bizcat 에서 이동)**

`internal/etfclass/classifier_test.go`:

```go
package etfclass

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidate(t *testing.T) {
	assert.Equal(t, "한국주식", Validate("한국주식"))
	assert.Equal(t, "금융", Validate("금융"))
	assert.Equal(t, "방위·우주항공", Validate("방위·우주항공"))
	assert.Equal(t, "반도체", Validate("  반도체  "), "공백 트림")
	assert.Equal(t, FallbackCategory, Validate("아무말"))
	assert.Equal(t, FallbackCategory, Validate(""))
}

// apiKey 가 비면 분류기를 만들지 않는다(호출부가 폴백을 쓰도록).
func TestNew_EmptyKeyReturnsNil(t *testing.T) {
	assert.Nil(t, New("", "gpt-4o-mini"))
	assert.NotNil(t, New("sk-test", ""))
}
```

- [ ] **Step 3: 테스트 실행 — 새 패키지가 통과하는지 확인**

Run: `go test ./internal/etfclass/ -v`
Expected: PASS (`TestValidate`, `TestNew_EmptyKeyReturnsNil`)

- [ ] **Step 4: bizcat 에서 옛 파일 삭제 후 호출부 교체**

```bash
rm internal/bizcat/etfclass.go
```

`internal/bizcat/resolver.go` 의 `EnableETFClassifier`(39-41행)를 교체:

```go
// EnableETFClassifier 는 ETF 종목명의 OpenAI 카테고리 분류를 활성화한다.
// apiKey 가 비면 분류기는 nil 로 남아 지수명 폴백을 사용한다.
func (r *Resolver) EnableETFClassifier(apiKey, model string) {
	r.classifyETF = etfclass.New(apiKey, model)
}
```

같은 파일 import 블록에 추가:

```go
	"github.com/kenshin579/auto-trading-journal/internal/etfclass"
```

- [ ] **Step 5: bizcat 로 옮겨간 테스트 삭제**

`internal/bizcat/resolver_test.go` 에서 `TestValidateETFCategory` 함수 전체(124-132행)를 삭제한다. `validateETFCategory`/`etfFallbackCategory` 는 이제 bizcat 에 없다.

- [ ] **Step 6: 전체 테스트 + vet**

Run: `make test && go vet ./...`
Expected: PASS. `internal/bizcat` 의 나머지 테스트(`TestResolveETFIndustry` 등)는 분류기를 함수로 주입받으므로 그대로 통과한다.

- [ ] **Step 7: 커밋**

```bash
git add internal/etfclass internal/bizcat
git commit -m "refactor(etfclass): ETF 분류기를 bizcat 밖 공용 패키지로 분리"
```

---

### Task 2: taxonomy 세분화 (미국주식 → S&P500 / 나스닥 / 미국주식(기타))

지수 비중을 S&P500·나스닥 단위로 보려면 카테고리를 쪼개야 한다. 동시에 "지수처럼 보이지만 지수가 아닌 것"(커버드콜·레버리지·팩터)을 지수로 오분류하지 않도록 프롬프트에 규칙을 넣는다.

**Files:**
- Modify: `internal/etfclass/classifier.go` (Categories, systemPrompt)
- Modify: `internal/etfclass/classifier_test.go`
- Modify: `internal/bizcat/resolver.go:25` (etfCacheVersion)
- Modify: `internal/bizcat/resolver_test.go` (TestNeedsRefresh)

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/etfclass/classifier_test.go` 에 추가:

```go
// 미국 시장대표는 추종 지수 단위로 쪼개진다. 옛 카테고리 "미국주식" 은 taxonomy 에서 빠졌다.
func TestValidate_USIndexSubdivided(t *testing.T) {
	assert.Equal(t, "S&P500", Validate("S&P500"))
	assert.Equal(t, "나스닥", Validate("나스닥"))
	assert.Equal(t, "미국주식(기타)", Validate("미국주식(기타)"))
	assert.Equal(t, FallbackCategory, Validate("미국주식"), "옛 카테고리는 taxonomy 밖")
}

// 프롬프트에 지수 오분류 방지 규칙이 들어있는지 확인한다.
// (LLM 응답 자체는 테스트하지 않는다 — 규칙 누락만 잡는다.)
func TestSystemPrompt_ContainsIndexGuards(t *testing.T) {
	for _, want := range []string{"커버드콜", "레버리지", "인버스", "팩터", "S&P500", "나스닥", "미국주식(기타)"} {
		assert.Contains(t, systemPrompt, want)
	}
}
```

- [ ] **Step 2: 테스트 실행 — 실패 확인**

Run: `go test ./internal/etfclass/ -run 'TestValidate_USIndexSubdivided|TestSystemPrompt' -v`
Expected: FAIL — `Validate("S&P500")` 이 `기타테마` 를 반환하고, systemPrompt 에 "커버드콜" 규칙 문자열이 없다.

- [ ] **Step 3: taxonomy 와 프롬프트 수정**

`internal/etfclass/classifier.go` 의 `Categories` 를 교체:

```go
// Categories 는 모든 ETF 를 종목명으로 분류할 고정 taxonomy.
var Categories = []string{
	// 지역/시장대표 (미국은 추종 지수 단위로 분리)
	"한국주식", "S&P500", "나스닥", "미국주식(기타)",
	"중국주식", "일본주식", "인도주식", "베트남주식", "글로벌주식",
	// 섹터/테마
	"반도체", "2차전지", "바이오·헬스케어", "AI·로봇", "신재생에너지", "원자력",
	"방위·우주항공", "자동차", "금융", "건설", "필수소비재", "IT·인터넷",
	// 자산군
	"배당", "리츠·부동산", "원자재", "채권", "통화·단기금리",
	// 기타
	"기타테마",
}
```

같은 파일의 `systemPrompt` 를 교체:

```go
var systemPrompt = fmt.Sprintf(`당신은 ETF 분류 전문가입니다.
ETF 종목명을 보고 아래 카테고리 중 정확히 하나로 분류하세요.

사용 가능한 카테고리: %s

분류 기준:
- 특정 섹터·테마가 핵심이면 지역보다 해당 테마 우선
  (반도체/2차전지/바이오·헬스케어/AI·로봇/신재생에너지/원자력/방위·우주항공/자동차/금융/건설/필수소비재/IT·인터넷).
  예: "미국반도체"→반도체, "글로벌AI"→AI·로봇.
- 시장 전체를 담는 대표 지수형은 추종 지수로 분류합니다.
  "S&P500"/"S&P 500"/"SPDR S&P 500"→S&P500,  "나스닥100"/"NASDAQ 100"/"QQQ"→나스닥,
  "코스피200"/"코스닥150"/"KRX300"→한국주식,
  러셀2000·다우존스·미국 토탈마켓 등 그 외 미국 시장대표→미국주식(기타),
  그 외 국가·지역은 중국주식/일본주식/인도주식/베트남주식/글로벌주식.
- 다음은 지수를 그대로 추종하지 않으므로 시장대표로 분류하지 마세요.
  · 커버드콜·프리미엄인컴(JEPI/JEPQ/"커버드콜"/"타겟프리미엄")→배당
  · 레버리지·인버스("2X"/"3X"/"UltraPro"/"곱버스"/"인버스")→기타테마
  · 팩터·스타일(성장주/가치주/퀄리티/모멘텀/저변동/동일가중)→기타테마,
    단 배당 팩터(배당성장·고배당·SCHD)는 배당
- 은행/보험/증권은 금융. 채권/국채/회사채는 채권. 단기자금/CD금리/통화는 통화·단기금리.
- 금·은·원유 등 상품은 원자재. 리츠·부동산·인프라는 리츠·부동산.
- 애매하거나 위에 없으면 기타테마.

반드시 JSON 으로만 응답: {"category": "<카테고리>"}`, strings.Join(Categories, ", "))
```

- [ ] **Step 4: 테스트 실행 — 통과 확인**

Run: `go test ./internal/etfclass/ -v`
Expected: PASS

- [ ] **Step 5: bizcat 캐시 버전 올리기 (기존 ETF 재분류 유도)**

`internal/bizcat/resolver.go:23-25` 를 교체:

```go
// etfCacheVersion 은 현재 ETF 산업 분류 스키마 버전. 이보다 낮은 ETF 캐시는 재조회한다.
// v2: 코드분류(KRX명)+9999(OpenAI) 하이브리드. v3: 전부 OpenAI taxonomy 로 통일.
// v4: 미국 시장대표를 S&P500/나스닥/미국주식(기타)로 세분(지수 비중 집계용).
const etfCacheVersion = 4
```

`internal/bizcat/resolver_test.go` 의 `TestNeedsRefresh` 에 한 줄 추가:

```go
	assert.True(t, needsRefresh(entry{Sector: "ETF", Industry: "미국주식", Version: 3}), "v3 는 미국 지수 세분 전이라 재분류")
```

- [ ] **Step 6: 공유 계약에 이름 붙이기 + 낡은 주석 정리**

`bizcat` 과 `fmpcat` 이 같은 함수 시그니처를 각자 선언하는 대신, 계약을 `etfclass` 한 곳에 둔다.
`internal/etfclass/classifier.go` 의 `New` 위에 타입을 추가하고 반환 타입을 바꾼다:

```go
// Classifier 는 ETF 종목명을 카테고리로 분류하는 함수. nil 은 "분류기 비활성"을 뜻하며,
// 호출부는 nil 일 때 각자의 폴백을 쓴다.
type Classifier func(name string) (string, error)

// New 는 ETF 종목명 → 카테고리 분류기를 만든다. apiKey 가 비면 nil 을 반환한다.
// 결과는 호출부 영구 캐시에 저장돼 종목당 1회만 호출된다.
func New(apiKey, model string) Classifier {
```

`Categories` 주석에 불변 조건을 명시한다(런타임 수정 시 `categorySet`/`systemPrompt` 와 조용히 어긋난다):

```go
// Categories 는 모든 ETF 를 종목명으로 분류할 고정 taxonomy.
// 국내 시장/섹터 ETF 와 해외·테마 ETF 를 하나의 일관된 카테고리 체계로 통일한다.
// 읽기 전용 — 런타임에 수정하지 말 것(categorySet/systemPrompt 가 init 시점에 이 값으로 고정된다).
```

`internal/bizcat/resolver.go:34` 의 필드 타입과 낡은 주석을 교체한다. "9999 미분류 ETF" 는 v2
하이브리드 시절 표현으로, 지금은 코드 분류 여부와 무관하게 전부 분류기를 탄다:

```go
	classifyETF etfclass.Classifier // ETF 종목명 → taxonomy 카테고리 분류기(optional, nil 가능)
```

- [ ] **Step 7: 전체 테스트**

Run: `make test && go vet ./...`
Expected: PASS

- [ ] **Step 8: 커밋**

```bash
git add internal/etfclass internal/bizcat
git commit -m "feat(etfclass): 미국 시장대표를 S&P500/나스닥/기타로 세분하고 지수 오분류 방지 규칙 추가"
```

---

### Task 2B: 분류 정확도 보강 (영문 큐 · 표기 정규화 · 팩터 카테고리)

Task 2 리뷰에서 드러난 정확도 구멍 넷을 메운다. 전부 `etfclass` 한 패키지 안에서 끝난다.
Task 2C 가 캐시 버전을 올리므로 **taxonomy 변경은 이 태스크에서 마무리해야 한다**(버전을 두 번
올리지 않기 위해).

1. **가드가 티커로 쓰여 있는데 분류기는 이름만 받는다.** 국내는 CSV 한글 펀드명, 해외(Task 3)는
   FMP `CompanyName` 이 입력이다. `JEPQ` 의 실제 입력은 `JPMorgan Nasdaq Equity Premium Income ETF`
   로 **"Nasdaq" 이 들어있어** 나스닥 버킷을 부풀릴 개연성이 크다. 한글 큐만으로는 영문 펀드명을
   못 잡으므로 영문 큐를 병기한다.
2. **`Validate` 가 exact match 라 `미국주식(기타)` 의 괄호 표기 흔들림에 취약하다.** 전각 괄호나
   공백이 섞이면 `기타테마` 로 떨어지는데, 그러면 지수 쪽이 **과소평가**된다(실패가 한 방향으로
   쏠린다). 정규화와 경고 로그를 넣는다.
3. **`원자재` 가 채굴·에너지 주식 펀드까지 흡수한다.** XLE·XLB·SIL·COPX 가 `원자재` 로 분류되면
   Task 5 매핑에서 **채권·금·현금성 ETF** 버킷에 들어간다 — 주식 익스포저가 현금성으로 표시된다.
   `원자재` 를 실물·선물 직접 투자로 한정한다.
4. **팩터·스타일 펀드가 갈 곳이 없다.** SCHG(미국 대형 성장주)를 `기타테마` 로 보내면 반도체·2차전지와
   같은 줄에 놓인다. 성격은 전략형이므로 `팩터·스타일` 카테고리를 추가해 "배당·전략 ETF" 버킷으로
   보낸다.

**Files:**
- Modify: `internal/etfclass/classifier.go`
- Modify: `internal/etfclass/classifier_test.go`
- Modify: `internal/bizcat/resolver_test.go` (fixture 정리)

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/etfclass/classifier_test.go` 의 기존 `TestSystemPrompt_ContainsIndexGuards` 를 교체한다.
기존 목록의 `"S&P500"`, `"나스닥"`, `"미국주식(기타)"` 는 프롬프트가 `Categories` 목록을 그대로
끼워넣으므로 **분류 규칙을 통째로 지워도 통과하는 동어반복**이다. 규칙 문구만 검증한다:

```go
// 프롬프트에 지수 오분류 방지 규칙이 들어있는지 확인한다.
// (LLM 응답 자체는 테스트하지 않는다 — 규칙 누락만 잡는다.)
// 카테고리 이름은 Categories 목록이 프롬프트에 그대로 들어가 항상 통과하므로 넣지 않는다.
func TestSystemPrompt_ContainsIndexGuards(t *testing.T) {
	for _, want := range []string{"커버드콜", "레버리지", "인버스", "팩터", "실물·선물"} {
		assert.Contains(t, systemPrompt, want)
	}
}

// 입력이 영문 펀드명(FMP CompanyName)인 경우가 많으므로 영문 큐도 있어야 한다.
// 예: JEPQ 의 입력은 "JPMorgan Nasdaq Equity Premium Income ETF" — 한글 큐로는 안 잡힌다.
func TestSystemPrompt_ContainsEnglishCues(t *testing.T) {
	for _, want := range []string{"Covered Call", "Equity Premium Income", "Growth", "Value", "Momentum", "Inverse", "Equal Weight"} {
		assert.Contains(t, systemPrompt, want)
	}
}

// 카테고리 표기 흔들림(전각 괄호·공백)은 정규화해서 받는다.
// 정규화 실패 시 기타테마로 떨어지면 지수 비중이 과소평가되므로 중요하다.
func TestValidate_NormalizesPunctuation(t *testing.T) {
	assert.Equal(t, "미국주식(기타)", Validate("미국주식（기타）"), "전각 괄호")
	assert.Equal(t, "미국주식(기타)", Validate("미국주식 (기타)"), "내부 공백")
	assert.Equal(t, "바이오·헬스케어", Validate(" 바이오·헬스케어 "))
	assert.Equal(t, FallbackCategory, Validate("미국주식"), "정규화해도 없는 카테고리")
}

// 팩터·스타일 펀드(SCHG 등)를 담을 카테고리.
func TestValidate_FactorStyleCategory(t *testing.T) {
	assert.Equal(t, "팩터·스타일", Validate("팩터·스타일"))
}
```

- [ ] **Step 2: 테스트 실행 — 실패 확인**

Run: `go test ./internal/etfclass/ -v`
Expected: FAIL — `Validate("미국주식（기타）")` 가 `기타테마` 반환, `팩터·스타일` 이 taxonomy 밖,
프롬프트에 영문 큐와 `실물·선물` 없음

- [ ] **Step 3: taxonomy 에 `팩터·스타일` 추가**

`internal/etfclass/classifier.go` 의 `Categories` 자산군 줄을 교체(총 28종):

```go
	// 자산군
	"배당", "팩터·스타일", "리츠·부동산", "원자재", "채권", "통화·단기금리",
```

- [ ] **Step 4: `Validate` 정규화 + 관측 로그**

`internal/etfclass/classifier.go` 의 `Validate` 를 교체(파일 import 에 `log/slog` 는 이미 있다):

```go
// categoryNormalizer 는 모델 응답의 표기 흔들림을 흡수한다(전각 괄호, 내부 공백).
// taxonomy 의 어떤 카테고리도 내부 공백을 갖지 않으므로 공백 제거는 안전하다.
var categoryNormalizer = strings.NewReplacer(
	"（", "(", "）", ")", " ", "", "\t", "", " ", "",
)

// Validate 는 분류 결과가 taxonomy 안에 있으면 그대로, 표기만 다르면 정규화해서,
// 그래도 없으면 FallbackCategory 로 돌려준다.
// taxonomy 밖 응답은 경고로 남긴다 — 조용히 기타테마로 흡수되면 지수 비중이 과소평가되는데
// 그 사실을 알 방법이 없어진다.
func Validate(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return FallbackCategory
	}
	if categorySet[s] {
		return s
	}
	if n := categoryNormalizer.Replace(s); categorySet[n] {
		slog.Warn("ETF 카테고리 표기 정규화", "raw", s, "normalized", n)
		return n
	}
	slog.Warn("ETF 카테고리가 taxonomy 밖 — 기타테마로 처리", "raw", s)
	return FallbackCategory
}
```

- [ ] **Step 5: 프롬프트 교체 (예외를 먼저, 영문 큐 병기)**

LLM 은 뒤에 오는 부정문("…로 분류하지 마세요")을 일관되지 않게 적용하므로 예외를 앞으로 옮긴다.
`internal/etfclass/classifier.go` 의 `systemPrompt` 를 교체:

```go
var systemPrompt = fmt.Sprintf(`당신은 ETF 분류 전문가입니다.
ETF 종목명(한글 펀드명 또는 영문 펀드명)을 보고 아래 카테고리 중 정확히 하나로 분류하세요.

사용 가능한 카테고리: %s

먼저 아래 예외에 해당하는지 보세요. 해당하면 이름에 지수명이 들어 있어도 시장대표 지수로
분류하지 마세요(지수를 그대로 추종하지 않기 때문입니다).
- 커버드콜·프리미엄인컴: "커버드콜"/"타겟프리미엄"/Covered Call/Equity Premium Income/
  Enhanced Income/Buffer → 배당.  예: "JPMorgan Nasdaq Equity Premium Income ETF"→배당
- 레버리지·인버스: "곱버스"/"인버스"/2X/3X/Ultra/UltraPro/Bull/Bear/Inverse → 기타테마
- 팩터·스타일: 성장주/가치주/퀄리티/모멘텀/저변동/동일가중/Growth/Value/Quality/Momentum/
  Low Volatility/Equal Weight → 팩터·스타일.
  단 배당 팩터(고배당·배당성장·Dividend Growth)는 배당
- 소수 종목 집중 바스켓("Magnificent Seven"/TOP10 등) → 기타테마

예외가 아니면 아래 기준으로 분류하세요.
- 특정 섹터·테마가 핵심이면 지역보다 해당 테마 우선
  (반도체/2차전지/바이오·헬스케어/AI·로봇/신재생에너지/원자력/방위·우주항공/자동차/금융/건설/필수소비재/IT·인터넷).
  예: "미국반도체"→반도체, "글로벌AI"→AI·로봇.
- 시장 전체를 담는 대표 지수형은 추종 지수로 분류합니다.
  "S&P500"/"S&P 500"/"SPDR S&P 500"→S&P500,  "나스닥100"/"NASDAQ 100"/"QQQ"→나스닥,
  "코스피200"/"코스닥150"/"KRX300"→한국주식,
  러셀2000·다우존스·미국 토탈마켓 등 그 외 미국 시장대표→미국주식(기타),
  그 외 국가·지역은 중국주식/일본주식/인도주식/베트남주식/글로벌주식.
- 은행/보험/증권은 금융. 채권/국채/회사채는 채권. 단기자금/CD금리/통화는 통화·단기금리.
- 원자재는 금·은·원유 등 실물·선물에 직접 투자하는 경우만입니다. 채굴·정유·에너지·소재 등
  관련 기업 주식에 투자하면 원자재가 아니라 해당 테마(원자력 등) 또는 기타테마입니다.
- 리츠·부동산·인프라는 리츠·부동산.
- 애매하거나 위에 없으면 기타테마.

반드시 JSON 으로만 응답: {"category": "<카테고리>"}`, strings.Join(Categories, ", "))
```

- [ ] **Step 6: bizcat 테스트 fixture 정리**

`internal/bizcat/resolver_test.go` 의 `TestNeedsRefresh` 에서 **현재 버전** 항목의 산업이
taxonomy 밖 값(`"미국주식"`)이라 "v4 가 미국주식을 저장한다"는 잘못된 인상을 준다. 교체:

```go
	assert.False(t, needsRefresh(entry{Sector: "ETF", Industry: "S&P500", Version: etfCacheVersion}), "현재 버전 ETF 유지")
```

- [ ] **Step 7: 테스트 실행 + 커밋**

Run: `go test ./internal/etfclass/ ./internal/bizcat/ -v && make test && go vet ./...`
Expected: PASS

```bash
git add internal/etfclass internal/bizcat
git commit -m "fix(etfclass): 영문 펀드명 큐·표기 정규화·팩터 카테고리로 지수 오분류 축소"
```

---

### Task 2C: 분류 일시 실패를 영구 캐시하지 않기 (bizcat)

지금은 OpenAI 분류가 일시적으로 실패하면 `resolveETFIndustry` 가 조용히 KIS 지수명으로 폴백하고,
그 값이 **현재 캐시 버전으로** 저장된다(`resolver.go:87`). `needsRefresh` 가 false 라 다음 실행에
재시도되지 않으므로, **한 번 삐끗한 종목은 taxonomy 밖 값으로 영구 고정되어 대시보드에서 계속
미분류 줄에 남는다.** 지수 비중을 보고 배분을 정하는 기능에서 이건 조용한 왜곡이다.

구분해야 할 두 경우:
- **분류기 nil**(OpenAI 키 없음) → 설정 상태이므로 매 실행 동일. 지수명 폴백을 캐시해도 된다.
- **분류기 있는데 실패**(레이트리밋·타임아웃) → 일시적. 캐시하면 안 되고 다음 실행에 재시도해야 한다.

두 번째를 KIS 조회 실패와 같은 경로(negative-cache, 기존 캐시 보존)로 보낸다.

**동시에 캐시 버전을 v5 로 올린다.** Task 2 가 v4 로 올리면서 ETF 89건을 한꺼번에 OpenAI 로
재분류하는데, 일시 실패 확률이 가장 높은 그 순간에 오염된 값이 **v4 로 기록**된다. 코드만 고치고
버전을 그대로 두면 이미 오염된 항목은 `needsRefresh` 가 false 라 소급 복구되지 않는다.

**Files:**
- Modify: `internal/bizcat/resolver.go` (`resolveETFIndustry`, `kisFetch`, `etfCacheVersion`)
- Modify: `internal/bizcat/resolver_test.go`

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/bizcat/resolver_test.go` 의 기존 `TestResolveETFIndustry` 를 새 시그니처에 맞게 교체한다
(반환값이 2개가 된다):

```go
func TestResolveETFIndustry(t *testing.T) {
	// 분류기 있으면 종목명 OpenAI 결과 사용
	got, err := resolveETFIndustry("KRX 반도체", "KODEX 반도체",
		func(name string) (string, error) { return "반도체", nil })
	assert.NoError(t, err)
	assert.Equal(t, "반도체", got)

	// 코드 분류였지만 verbose 한 KRX 지수명도 OpenAI 로 정규화된다
	got, err = resolveETFIndustry("S&P 500 Future Index TR", "KODEX 미국S&P500선물",
		func(name string) (string, error) { return "S&P500", nil })
	assert.NoError(t, err)
	assert.Equal(t, "S&P500", got)

	// 분류기 미설정(nil) → KIS 지수명 폴백(접두사 제거). 설정 상태이므로 에러가 아니다.
	got, err = resolveETFIndustry("KRX 반도체", "KODEX 반도체", nil)
	assert.NoError(t, err)
	assert.Equal(t, "반도체", got)

	// 분류기가 있는데 실패 → 에러 전파(일시 오류를 영구 캐시하지 않도록)
	_, err = resolveETFIndustry("S&P 500", "KODEX 미국S&P500",
		func(name string) (string, error) { return "", assert.AnError })
	assert.Error(t, err, "일시적 분류 실패는 fetch 실패로 전파")
}
```

이어서 캐시 오염 방지를 검증하는 테스트를 추가한다:

```go
// 분류 실패는 기존 캐시를 덮어쓰지 않고, 다음 실행에 재시도할 수 있게 둔다.
func TestResolve_ClassifyFailureDoesNotPoisonCache(t *testing.T) {
	calls := 0
	r := &Resolver{
		cache: map[string]entry{},
		fetch: func(code, name string) (string, string, error) {
			calls++
			return "", "", assert.AnError // 분류 실패가 fetch 실패로 전파된 상황
		},
	}
	s, i := r.Resolve("069500", "KODEX 200")
	assert.Equal(t, "", s)
	assert.Equal(t, "", i)
	assert.NotContains(t, r.cache, "069500", "실패는 캐시에 남지 않는다")
	assert.Equal(t, 1, calls)
}
```

- [ ] **Step 2: 테스트 실행 — 실패 확인**

Run: `go test ./internal/bizcat/ -run 'TestResolveETFIndustry|TestResolve_ClassifyFailure' -v`
Expected: FAIL — `resolveETFIndustry` 가 값 1개만 반환해 컴파일 에러

- [ ] **Step 3: 구현**

`internal/bizcat/resolver.go` 의 `resolveETFIndustry` 를 교체:

```go
// resolveETFIndustry 는 ETF 산업 분류를 결정한다.
//   - 분류기가 있으면 코드 분류 여부와 무관하게 펀드 종목명을 taxonomy 로 분류해 통일한다
//     (예 "S&P500"/"반도체"/"원자재"). 코드 분류된 verbose 한 지수명도 정규화됨.
//   - 분류기 미설정(nil) → KIS 지수명(stripKRXPrefix) 폴백. 설정 상태이므로 에러가 아니다.
//   - 분류기가 있는데 실패 → 에러를 전파한다. 폴백 값(taxonomy 밖 지수명)을 영구 캐시하면
//     그 종목이 다음 실행에도 재조회되지 않아 대시보드에서 영구히 미분류로 남기 때문이다.
func resolveETFIndustry(rprsName, fundName string, classify etfclass.Classifier) (string, error) {
	if classify == nil {
		return stripKRXPrefix(rprsName), nil
	}
	cat, err := classify(fundName)
	if err != nil {
		return "", err
	}
	if cat == "" {
		return stripKRXPrefix(rprsName), nil
	}
	return cat, nil
}
```

같은 파일 `kisFetch` 안의 호출부를 교체:

```go
		var etfIndustry string
		if isETF(price, info) {
			etf, err := client.Domestic.InquireEtfPrice(ctx, domestic.InquireEtfPriceParams{Symbol: code})
			if err != nil {
				// 일시적 실패(레이트리밋 등)를 빈 값으로 영구 캐시하지 않도록 fetch 실패로 전파한다
				// (negative-cache 처리 → 다음 실행에 재시도).
				return "", "", err
			}
			etfIndustry, err = resolveETFIndustry(etf.Output.EtfRprsBstpKorIsnm, name, classifyETF)
			if err != nil {
				return "", "", err
			}
		}
```

`kisFetch` 의 파라미터 타입도 `classifyETF etfclass.Classifier` 로 바꾼다.

- [ ] **Step 3B: 빈 분류 결과의 처리를 정리**

`Validate` 는 taxonomy 밖 응답을 `기타테마` 로 바꾸므로 **절대 빈 문자열을 반환하지 않는다.** 그래서
`resolveETFIndustry` 의 `cat != ""` 가드는 `etfclass.New` 로 만든 분류기에 대해서는 죽은 가지다.
가드는 남긴다 — `classifyETF` 는 주입되는 함수(`etfclass.Classifier`)라 호출부가 `Validate` 를
거쳤다고 가정할 수 없고, 테스트 스텁이 실제로 빈 값을 돌려준다. 대신 그 이유를 주석으로 남긴다.

빈 응답 자체는 관측 가능해야 하므로 `internal/etfclass/classifier.go` 의 `Validate` 에서
빈 문자열 조기 반환에 경고를 붙인다:

```go
	if s == "" {
		slog.Warn("ETF 카테고리가 비어 있음 — 기타테마로 처리")
		return FallbackCategory
	}
```

`resolveETFIndustry` 의 `cat == ""` 분기에 주석 한 줄:

```go
	// etfclass.New 로 만든 분류기는 Validate 를 거쳐 빈 값을 반환하지 않지만,
	// classifyETF 는 주입되는 함수라 그 보장을 가정하지 않는다.
```

- [ ] **Step 3C: 예외 블록에 우선순위 문장 추가**

예외 5개 중 둘 이상에 매칭되는 펀드가 있는데(DGRO `iShares Core Dividend Growth ETF` 는
배당 bullet 의 `Dividend Growth` 와 팩터 bullet 의 `Growth` 양쪽에 걸린다) 프롬프트가 그때 무엇을
적용할지 말하지 않는다. 지금은 `배당` 과 `팩터·스타일` 이 같은 버킷으로 가서 재무 영향이 없지만,
`배당` + `TOP10 바스켓`(한국 ETF 작명에 흔하다)처럼 버킷이 갈리는 겹침이 생기면 결과가 흔들린다.

`internal/etfclass/classifier.go` 의 `classifyRules` 첫 문장에 한 절을 더한다:

```
먼저 아래 예외에 해당하는지 보세요. 해당하면 이름에 지수명이 들어 있어도
시장대표 지수로 분류하지 마세요(지수를 그대로 추종하지 않기 때문입니다).
여러 예외에 해당하면 위에 있는 것을 적용하세요.
```

`categoryNormalizer` 주석도 실제 커버 범위에 맞춘다(현재 "전각 괄호, 내부 공백" 이라고만 쓰여 있으나
실제로는 ASCII 공백·탭·NBSP·전각 공백 넷을 다룬다):

```go
// categoryNormalizer 는 모델 응답의 표기 흔들림을 흡수한다
// (전각 괄호, ASCII 공백·탭, NBSP U+00A0, 전각 공백 U+3000).
```

테스트로 예외 내부 순서를 고정한다(`internal/etfclass/classifier_test.go`):

```go
// 여러 예외에 걸리는 펀드(DGRO 는 "Dividend Growth" 와 "Growth" 양쪽)가 있으므로
// 우선순위 진술과 예외 순서를 함께 고정한다.
func TestClassifyRules_ExceptionPriorityIsStated(t *testing.T) {
	assert.Contains(t, classifyRules, "여러 예외에 해당하면 위에 있는 것을 적용하세요")
	assert.Less(t, strings.Index(classifyRules, "배당이 핵심인 펀드"),
		strings.Index(classifyRules, "팩터·스타일:"), "배당 예외가 팩터보다 앞에 와야 한다")
}
```

- [ ] **Step 4: 캐시 버전 v5 로 올리기**

Task 2 의 v4 재분류 버스트 중 오염됐을 수 있는 항목을 무효화한다.
`internal/bizcat/resolver.go` 의 버전 선언을 교체:

```go
// etfCacheVersion 은 현재 ETF 산업 분류 스키마 버전. 이보다 낮은 ETF 캐시는 재조회한다.
// v2: 코드분류(KRX명)+9999(OpenAI) 하이브리드. v3: 전부 OpenAI taxonomy 로 통일.
// v4: 미국 시장대표를 S&P500/나스닥/미국주식(기타)로 세분(지수 비중 집계용).
// v5: 분류 실패 시 폴백 값을 영구 캐시하던 버그 이전 항목 무효화 + 팩터·스타일 카테고리 추가.
const etfCacheVersion = 5
```

- [ ] **Step 5: 테스트 실행 — 통과 확인**

Run: `go test ./internal/bizcat/ -v`
Expected: PASS (기존 테스트 포함 전부. `TestNeedsRefresh` 의 v3 케이스는 v5 기준으로도 여전히 true)

- [ ] **Step 6: 전체 테스트 + 커밋**

Run: `make test && go vet ./...`

```bash
git add internal/bizcat
git commit -m "fix(bizcat): ETF 분류 일시 실패를 영구 캐시하지 않고 다음 실행에 재시도"
```

---

### Task 3: fmpcat — `IsEtf`/`IsFund` 로 해외 ETF 판별 + 카테고리 통일

FMP 프로필은 ETF 를 `Financial Services / Asset Management*` 로 준다. 문자열로 가르면 BDC(MAIN·HTGC·BXSL)와 자산운용사(BN)가 오분류되므로 공식 플래그를 쓴다. `Profile` 응답에 이미 들어있는 필드라 **API 호출 횟수는 그대로**다.

**Files:**
- Modify: `internal/fmpcat/resolver.go` (전면)
- Modify: `internal/fmpcat/resolver_test.go` (fetch 스텁 시그니처 변경 + 신규 테스트)

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/fmpcat/resolver_test.go` 의 **기존 5개 테스트의 fetch 스텁을 새 시그니처로 교체**한다.

`TestResolve_UnsupportedCurrencySkipsFetch` (30-33행):

```go
	r := &Resolver{cache: map[string]entry{}, fetch: func(symbol string) (profile, error) {
		calls++
		return profile{Sector: "X", Industry: "Y"}, nil
	}}
```

`TestResolve_USMissCachesThenHit` (51-55행):

```go
	r := &Resolver{cache: map[string]entry{}, fetch: func(symbol string) (profile, error) {
		calls++
		gotSymbol = symbol
		return profile{Sector: "Technology", Industry: "Consumer Electronics"}, nil
	}}
```

`TestResolve_JPYAppendsSuffix` (67-70행):

```go
	r := &Resolver{cache: map[string]entry{}, fetch: func(symbol string) (profile, error) {
		gotSymbol = symbol
		return profile{Sector: "Consumer Cyclical", Industry: "Auto - Manufacturers"}, nil
	}}
```

`TestResolve_NotFoundCachedEmpty` (80-83행):

```go
	r := &Resolver{cache: map[string]entry{}, fetch: func(symbol string) (profile, error) {
		calls++
		return profile{}, nil
	}}
```

`TestResolve_ErrorNegativeCache` (94-97행):

```go
	r := &Resolver{cache: map[string]entry{}, fetch: func(symbol string) (profile, error) {
		calls++
		return profile{}, assert.AnError
	}}
```

`TestSaveCache_OmitsEmptyEntries` 와 `TestCacheSaveLoad_Roundtrip` 의 `entry{...}` 리터럴은 그대로 둔다(새 필드는 zero value).

이어서 **신규 테스트**를 파일 끝에 추가:

```go
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
```

- [ ] **Step 2: 테스트 실행 — 컴파일 실패 확인**

Run: `go test ./internal/fmpcat/ -v`
Expected: FAIL — `undefined: profile`, `unknown field classifyETF`, `undefined: needsRefresh`, `undefined: cacheVersion`

- [ ] **Step 3: 구현**

`internal/fmpcat/resolver.go` 전체를 교체:

```go
// Package fmpcat 는 FMP(Financial Modeling Prep) 회사 프로필로 해외 종목의
// 섹터/산업을 티커로 조회한다. (국내 internal/bizcat 의 해외 대응판.)
//
// ETF/펀드는 FMP 의 IsEtf/IsFund 플래그로 판별해 국내와 같은 표기로 통일한다
// (섹터="ETF", 산업=etfclass taxonomy 카테고리). 산업 문자열("Asset Management")로
// 판별하면 BDC·자산운용사가 ETF 로 오분류되므로 플래그를 쓴다.
package fmpcat

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"sync"

	fmp "github.com/kenshin579/fmp-go"

	"github.com/kenshin579/auto-trading-journal/internal/etfclass"
)

// cacheVersion 은 현재 캐시 스키마 버전. 이보다 낮은 항목은 1회 재조회한다.
// v2: ETF 판별(isEtf)과 taxonomy 산업 도입. v0(버전 필드 없음)=구버전.
const cacheVersion = 2

type entry struct {
	Sector   string `json:"sector"`
	Industry string `json:"industry"`
	IsETF    bool   `json:"isEtf,omitempty"`
	Version  int    `json:"v,omitempty"`
}

// profile 은 FMP 프로필에서 이 패키지가 쓰는 필드만 추린 것.
type profile struct {
	Sector   string
	Industry string
	Name     string
	IsETF    bool
	IsFund   bool
}

type Resolver struct {
	mu          sync.Mutex
	cache       map[string]entry
	failed      map[string]bool // 이번 실행에서 실패한 심볼(negative cache, 비영구)
	cachePath   string
	fetch       func(symbol string) (profile, error)
	classifyETF etfclass.Classifier // ETF 종목명 → taxonomy 카테고리 분류기(optional, nil 가능)
	dirty       bool
}

func New(cachePath string) *Resolver {
	r := &Resolver{cache: map[string]entry{}, cachePath: cachePath}
	if data, err := os.ReadFile(cachePath); err == nil {
		var m map[string]entry
		if json.Unmarshal(data, &m) == nil && m != nil {
			r.cache = m
		} else {
			slog.Warn("fmpcat 캐시 로드 실패, 빈 캐시로 시작", "path", cachePath)
		}
	}
	return r
}

// EnableETFClassifier 는 해외 ETF 종목명의 OpenAI 카테고리 분류를 활성화한다.
// apiKey 가 비면 분류기는 nil 로 남아 FMP 원본 산업을 그대로 쓴다.
func (r *Resolver) EnableETFClassifier(apiKey, model string) {
	r.classifyETF = etfclass.New(apiKey, model)
}

func (r *Resolver) cacheLookup(symbol string) (string, bool) {
	e, ok := r.cache[symbol]
	return e.Sector, ok
}

// needsRefresh 는 캐시 히트라도 재조회가 필요한지 판단한다.
// 구버전 캐시는 ETF 판별 정보(isEtf)가 없어 지수 집계에 쓸 수 없다.
func needsRefresh(e entry) bool {
	return e.Version < cacheVersion
}

// exchangeSuffix 는 통화로 FMP 거래소 접미사를 정한다. 현재 US/JP 만 지원
// (그 외 통화는 미지원 → 공란). 보유 국가 추가 시 여기에 매핑을 늘린다.
func exchangeSuffix(currency string) (string, bool) {
	switch currency {
	case "USD":
		return "", true // 미국: 접미사 없음
	case "JPY":
		return ".T", true // 도쿄
	default:
		return "", false
	}
}

// Resolve 는 해외 티커+통화로 (섹터, 산업)을 조회한다. 미지원 통화/빈 티커는 공란.
// ETF/펀드는 ("ETF", taxonomy 카테고리), 일반 종목은 FMP 영문 섹터/산업.
func (r *Resolver) Resolve(ticker, currency string) (string, string) {
	if ticker == "" {
		return "", ""
	}
	suffix, ok := exchangeSuffix(currency)
	if !ok {
		return "", "" // 미지원 통화 → FMP 호출 안 함
	}
	symbol := ticker + suffix

	r.mu.Lock()
	defer r.mu.Unlock()
	cached, hasCached := r.cache[symbol]
	if hasCached && !needsRefresh(cached) {
		return cached.Sector, cached.Industry
	}
	if r.failed[symbol] {
		// 이번 실행에서 이미 실패 → 재조회 안 함. 자가치유 대상이면 기존 캐시 보존.
		return cached.Sector, cached.Industry
	}
	if r.fetch == nil {
		r.fetch = fmpFetch()
	}
	p, err := r.fetch(symbol)
	if err != nil {
		if r.failed == nil {
			r.failed = map[string]bool{}
		}
		r.failed[symbol] = true
		slog.Warn("FMP 프로필 조회 실패, 빈 값 처리", "symbol", symbol, "err", err)
		return cached.Sector, cached.Industry // 자가치유 재조회 실패 시 기존 캐시 보존
	}

	// not-found 는 fetch 가 zero profile+nil 로 반환 → 빈 값 캐시(saveCache 가 파일에서 제외).
	e := entry{Sector: p.Sector, Industry: p.Industry, Version: cacheVersion}
	if p.IsETF || p.IsFund {
		industry, err := r.etfIndustry(p, symbol)
		if err != nil {
			if r.failed == nil {
				r.failed = map[string]bool{}
			}
			r.failed[symbol] = true
			slog.Warn("ETF 카테고리 분류 실패, 캐시하지 않음(다음 실행 재시도)", "symbol", symbol, "err", err)
			return cached.Sector, cached.Industry
		}
		e.IsETF = true
		e.Sector = "ETF"
		e.Industry = industry
	}
	r.cache[symbol] = e
	r.dirty = true
	return e.Sector, e.Industry
}

// etfIndustry 는 해외 ETF 의 산업(taxonomy 카테고리)을 정한다.
//   - 분류기 미설정(nil) → FMP 원본 산업 폴백. 설정 상태이므로 에러가 아니다.
//   - 분류기가 있는데 실패 → 에러 전파. taxonomy 밖 폴백 값을 영구 캐시하면 그 종목이
//     다음 실행에도 재조회되지 않아 대시보드에서 영구히 미분류로 남기 때문이다.
func (r *Resolver) etfIndustry(p profile, symbol string) (string, error) {
	if r.classifyETF == nil {
		return p.Industry, nil
	}
	name := p.Name
	if name == "" {
		name = symbol
	}
	cat, err := r.classifyETF(name)
	if err != nil {
		return "", err
	}
	if cat == "" {
		return p.Industry, nil
	}
	return cat, nil
}

func (r *Resolver) Save() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.dirty {
		return
	}
	if err := r.saveCache(); err != nil {
		slog.Warn("fmpcat 캐시 저장 실패", "err", err)
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
	// 빈 결과(미커버/미지원 통화)는 파일에 영구화하지 않는다(노이즈 방지).
	// 인메모리 캐시엔 남아 같은 실행 내 재조회는 막고, 다음 실행에 1회 재시도된다.
	out := make(map[string]entry, len(r.cache))
	for k, v := range r.cache {
		if v.Sector == "" && v.Industry == "" {
			continue
		}
		out[k] = v
	}
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func fmpFetch() func(string) (profile, error) {
	client, err := fmp.NewClientFromEnv()
	if err != nil {
		slog.Warn("FMP 클라이언트 생성 실패, 해외 섹터 보강 비활성화", "err", err)
		return func(string) (profile, error) { return profile{}, nil }
	}
	return func(symbol string) (profile, error) {
		p, err := client.Company.Profile(context.Background(), symbol)
		if err != nil {
			if errors.Is(err, fmp.ErrNotFound) {
				return profile{}, nil // 미커버 → 빈 값(재조회는 다음 실행에 1회)
			}
			return profile{}, err // 일시적 오류 → negative-cache, 다음 실행 재시도
		}
		return profile{
			Sector:   p.Sector,
			Industry: p.Industry,
			Name:     p.CompanyName,
			IsETF:    p.IsEtf,
			IsFund:   p.IsFund,
		}, nil
	}
}
```

- [ ] **Step 4: 테스트 실행 — 통과 확인**

Run: `go test ./internal/fmpcat/ -v`
Expected: PASS (기존 7개 + 신규 6개)

- [ ] **Step 5: 커밋**

```bash
git add internal/fmpcat
git commit -m "feat(fmpcat): IsEtf/IsFund 로 해외 ETF 판별하고 국내와 같은 카테고리 체계로 통일"
```

---

### Task 4: main.go 에서 해외 ETF 분류기 활성화

**Files:**
- Modify: `cmd/atj/main.go:189` 부근

- [ ] **Step 1: 한 줄 추가**

`cmd/atj/main.go` 에서 `fc := fmpcat.New("config/fmpcat_cache.json")` 바로 다음에 추가:

```go
	// 해외 ETF 는 종목명 기반 OpenAI 분류로 국내와 같은 카테고리 체계를 쓴다.
	// 키 없으면 no-op(FMP 원본 산업 폴백).
	fc.EnableETFClassifier(cfg.OpenAIAPIKey(), cfg.OpenAI.Model)
```

- [ ] **Step 2: 빌드 + 전체 테스트**

Run: `make build && make test`
Expected: 빌드 성공, 전체 PASS

- [ ] **Step 3: 커밋**

```bash
git add cmd/atj/main.go
git commit -m "feat(atj): 해외 ETF 카테고리 분류기 활성화"
```

---

### Task 5: `bucketOf` — (섹터, 산업) → (그룹, 버킷) 순수 매핑

**Files:**
- Create: `internal/summary/index_weight.go`
- Create: `internal/summary/index_weight_test.go`

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/summary/index_weight_test.go`:

```go
package summary

import (
	"testing"

	"github.com/kenshin579/auto-trading-journal/internal/model"
	"github.com/stretchr/testify/assert"
)

func TestBucketOf_Index(t *testing.T) {
	cases := map[string]string{
		"S&P500":    bucketSP500,
		"나스닥":       bucketNasdaq,
		"한국주식":      bucketKorea,
		"미국주식(기타)":  bucketOtherIndex,
		"중국주식":      bucketOtherIndex,
		"일본주식":      bucketOtherIndex,
		"인도주식":      bucketOtherIndex,
		"베트남주식":     bucketOtherIndex,
		"글로벌주식":     bucketOtherIndex,
	}
	for industry, wantBucket := range cases {
		g, b := bucketOf("ETF", industry)
		assert.Equal(t, groupIndex, g, industry)
		assert.Equal(t, wantBucket, b, industry)
	}
}

func TestBucketOf_Other(t *testing.T) {
	theme := []string{"반도체", "2차전지", "바이오·헬스케어", "AI·로봇", "신재생에너지", "원자력",
		"방위·우주항공", "자동차", "금융", "건설", "필수소비재", "IT·인터넷", "리츠·부동산", "기타테마"}
	for _, industry := range theme {
		g, b := bucketOf("ETF", industry)
		assert.Equal(t, groupOther, g, industry)
		assert.Equal(t, bucketTheme, b, industry)
	}

	// 배당·전략 계열: 배당(고배당·배당성장·커버드콜)과 팩터·스타일(성장/가치/퀄리티)
	for _, industry := range []string{"배당", "팩터·스타일"} {
		g, b := bucketOf("ETF", industry)
		assert.Equal(t, groupOther, g, industry)
		assert.Equal(t, bucketDividend, b, industry)
	}

	for _, industry := range []string{"채권", "원자재", "통화·단기금리"} {
		g, b := bucketOf("ETF", industry)
		assert.Equal(t, groupOther, g, industry)
		assert.Equal(t, bucketBondGold, b, industry)
	}
}

// 일반 종목은 섹터가 무엇이든 개별종목.
func TestBucketOf_Stock(t *testing.T) {
	g, b := bucketOf("전기·전자", "반도체 제조업")
	assert.Equal(t, groupOther, g)
	assert.Equal(t, bucketStock, b)

	g, b = bucketOf("Financial Services", "Asset Management")
	assert.Equal(t, groupOther, g, "BDC·자산운용사도 개별종목")
	assert.Equal(t, bucketStock, b)
}

// 섹터가 비었거나 taxonomy 밖 ETF 산업은 미분류 — 개별종목으로 밀어넣지 않는다.
func TestBucketOf_Unknown(t *testing.T) {
	g, b := bucketOf("", "")
	assert.Equal(t, groupUnknown, g)
	assert.Equal(t, bucketUnknown, b)

	g, b = bucketOf("ETF", "S&P 500 Future Index TR") // 분류기 실패 시의 지수명 폴백
	assert.Equal(t, groupUnknown, g)
	assert.Equal(t, bucketUnknown, b)

	g, b = bucketOf("ETF", "")
	assert.Equal(t, groupUnknown, g)
	assert.Equal(t, bucketUnknown, b)
}

// taxonomy 전체가 매핑 표에 있어야 한다(카테고리 추가 시 매핑 누락 방지).
func TestETFBuckets_CoversTaxonomy(t *testing.T) {
	for _, c := range etfclass.Categories {
		_, ok := etfBuckets[c]
		assert.True(t, ok, "매핑 누락: %s", c)
	}
}
```

이 단계의 테스트 파일 import 는 다음 둘뿐이다(`model` 은 Task 6 에서 추가한다):

```go
import (
	"testing"

	"github.com/kenshin579/auto-trading-journal/internal/etfclass"
	"github.com/stretchr/testify/assert"
)
```

- [ ] **Step 2: 테스트 실행 — 실패 확인**

Run: `go test ./internal/summary/ -run TestBucketOf -v`
Expected: FAIL — `undefined: bucketOf`, `undefined: groupIndex` 등

- [ ] **Step 3: 구현**

`internal/summary/index_weight.go` (이 단계에선 import 없음 — Task 6/7 에서 추가한다):

```go
package summary

// 그룹(상위 묶음). 미분류는 지수/나머지 어디에도 속하지 않는 독립 그룹이다.
const (
	groupIndex   = "지수"
	groupOther   = "나머지"
	groupUnknown = "미분류"
)

// 버킷(표시 줄 라벨).
const (
	bucketSP500      = "S&P500"
	bucketNasdaq     = "나스닥"
	bucketKorea      = "한국(코스피·코스닥)"
	bucketOtherIndex = "기타 지역·전세계"
	bucketStock      = "개별종목"
	bucketTheme      = "테마·섹터 ETF"
	bucketDividend   = "배당·전략 ETF"
	bucketBondGold   = "채권·금·현금성 ETF"
	bucketUnknown    = "미분류"
)

// etfBuckets 는 ETF 카테고리(etfclass.Categories) → (그룹, 버킷) 매핑.
// etfclass 에 카테고리를 추가하면 여기에도 넣어야 한다(TestETFBuckets_CoversTaxonomy 가 강제).
var etfBuckets = map[string][2]string{
	"S&P500":     {groupIndex, bucketSP500},
	"나스닥":        {groupIndex, bucketNasdaq},
	"한국주식":       {groupIndex, bucketKorea},
	"미국주식(기타)":   {groupIndex, bucketOtherIndex},
	"중국주식":       {groupIndex, bucketOtherIndex},
	"일본주식":       {groupIndex, bucketOtherIndex},
	"인도주식":       {groupIndex, bucketOtherIndex},
	"베트남주식":      {groupIndex, bucketOtherIndex},
	"글로벌주식":      {groupIndex, bucketOtherIndex},
	"반도체":        {groupOther, bucketTheme},
	"2차전지":       {groupOther, bucketTheme},
	"바이오·헬스케어":   {groupOther, bucketTheme},
	"AI·로봇":      {groupOther, bucketTheme},
	"신재생에너지":     {groupOther, bucketTheme},
	"원자력":        {groupOther, bucketTheme},
	"방위·우주항공":    {groupOther, bucketTheme},
	"자동차":        {groupOther, bucketTheme},
	"금융":         {groupOther, bucketTheme},
	"건설":         {groupOther, bucketTheme},
	"필수소비재":      {groupOther, bucketTheme},
	"IT·인터넷":     {groupOther, bucketTheme},
	"리츠·부동산":     {groupOther, bucketTheme},
	"기타테마":       {groupOther, bucketTheme},
	"배당":         {groupOther, bucketDividend},
	"팩터·스타일":     {groupOther, bucketDividend},
	"채권":         {groupOther, bucketBondGold},
	"원자재":        {groupOther, bucketBondGold},
	"통화·단기금리":    {groupOther, bucketBondGold},
}

// bucketOf 는 거래의 (섹터, 산업)으로 표시 그룹/버킷을 정한다.
//   - 섹터가 비면 미분류(FMP 미커버/미지원 통화/키 없음).
//   - 섹터가 "ETF" 가 아니면 개별종목.
//   - ETF 인데 산업이 taxonomy 밖이면(분류기 실패 폴백 등) 미분류 — 임의로 테마에 넣지 않는다.
func bucketOf(sector, industry string) (group, bucket string) {
	if sector == "" {
		return groupUnknown, bucketUnknown
	}
	if sector != "ETF" {
		return groupOther, bucketStock
	}
	if gb, ok := etfBuckets[industry]; ok {
		return gb[0], gb[1]
	}
	return groupUnknown, bucketUnknown
}
```

`etfBuckets` 는 `etfclass.Categories` 를 그대로 따라가야 하지만, 구현 파일은 `etfclass` 를
import 하지 않는다(런타임 의존이 없다). taxonomy 와의 동기화는 테스트
`TestETFBuckets_CoversTaxonomy` 가 강제한다 — 카테고리를 추가하고 매핑을 빠뜨리면 테스트가 깨진다.

- [ ] **Step 4: 테스트 실행 — 통과 확인**

Run: `go test ./internal/summary/ -run 'TestBucketOf|TestETFBuckets' -v`
Expected: PASS

- [ ] **Step 5: 커밋**

```bash
git add internal/summary/index_weight.go internal/summary/index_weight_test.go
git commit -m "feat(summary): 지수/나머지 버킷 매핑 함수 추가"
```

---

### Task 6: `aggregateIndexWeight` — 누적 매수금액 + 보유 원금 집계

**Files:**
- Modify: `internal/summary/index_weight.go`
- Modify: `internal/summary/index_weight_test.go`

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/summary/index_weight_test.go` 에 추가:

```go
// 표는 그룹 소계 + 버킷 줄이 고정 순서로 나온다. 금액 0 인 버킷도 표시된다.
func TestAggregateIndexWeight_LayoutAndTotals(t *testing.T) {
	trades := []model.Trade{
		// S&P500 ETF: 100주 매수 100만, 50주 매도 → 보유 50주 × 평균 1만 = 50만
		{StockName: "SPYM", StockCode: "SPYM", Account: "해외", Currency: "USD",
			Sector: "ETF", Industry: "S&P500", TradeType: "매수", Quantity: 100, AmountKRW: 1_000_000},
		{StockName: "SPYM", StockCode: "SPYM", Account: "해외", Currency: "USD",
			Sector: "ETF", Industry: "S&P500", TradeType: "매도", Quantity: 50, AmountKRW: 600_000},
		// 개별종목: 전량 보유
		{StockName: "삼성전자", StockCode: "005930", Account: "국내", Currency: "KRW",
			Sector: "전기·전자", Industry: "반도체 제조업", TradeType: "매수", Quantity: 10, AmountKRW: 1_000_000},
	}
	rows := aggregateIndexWeight(trades)

	byLabel := map[string]indexWeightRow{}
	for _, r := range rows {
		key := r.bucket
		if key == "" {
			key = "▸" + r.group
		}
		byLabel[key] = r
	}

	assert.Equal(t, 1_000_000.0, byLabel[bucketSP500].buy)
	assert.Equal(t, 500_000.0, byLabel[bucketSP500].held, "잔여 50주 × 평균단가 1만")
	assert.Equal(t, 1_000_000.0, byLabel[bucketStock].buy)
	assert.Equal(t, 1_000_000.0, byLabel[bucketStock].held)

	// 그룹 소계
	assert.Equal(t, 1_000_000.0, byLabel["▸"+groupIndex].buy)
	assert.Equal(t, 500_000.0, byLabel["▸"+groupIndex].held)
	assert.Equal(t, 1_000_000.0, byLabel["▸"+groupOther].buy)

	// 비중: 누적은 50/50, 보유는 1/3 : 2/3
	assert.InDelta(t, 0.5, byLabel["▸"+groupIndex].buyPct, 1e-9)
	assert.InDelta(t, 1.0/3.0, byLabel["▸"+groupIndex].heldPct, 1e-9)
	assert.InDelta(t, 2.0/3.0, byLabel["▸"+groupOther].heldPct, 1e-9)

	// 거래 없는 버킷도 0 으로 표시된다
	assert.Contains(t, byLabel, bucketNasdaq)
	assert.Equal(t, 0.0, byLabel[bucketNasdaq].buy)

	// 미분류가 0 이면 표시하지 않는다
	assert.NotContains(t, byLabel, "▸"+groupUnknown)
}

// 전량 매도한 종목은 보유 원금 0, 누적 매수금액은 남는다.
func TestAggregateIndexWeight_FullySoldHasZeroHeld(t *testing.T) {
	trades := []model.Trade{
		{StockName: "QQQM", StockCode: "QQQM", Account: "해외", Currency: "USD",
			Sector: "ETF", Industry: "나스닥", TradeType: "매수", Quantity: 10, AmountKRW: 1_000_000},
		{StockName: "QQQM", StockCode: "QQQM", Account: "해외", Currency: "USD",
			Sector: "ETF", Industry: "나스닥", TradeType: "매도", Quantity: 10, AmountKRW: 1_200_000},
	}
	rows := aggregateIndexWeight(trades)
	for _, r := range rows {
		if r.bucket == bucketNasdaq {
			assert.Equal(t, 1_000_000.0, r.buy)
			assert.Equal(t, 0.0, r.held)
		}
	}
}

// 매도 수량이 매수를 넘어도 보유 원금은 음수가 되지 않는다.
func TestAggregateIndexWeight_OversoldClampedToZero(t *testing.T) {
	trades := []model.Trade{
		{StockName: "TIGER 코스피", StockCode: "277630", Account: "국내", Currency: "KRW",
			Sector: "ETF", Industry: "한국주식", TradeType: "매수", Quantity: 10, AmountKRW: 100_000},
		{StockName: "TIGER 코스피", StockCode: "277630", Account: "국내", Currency: "KRW",
			Sector: "ETF", Industry: "한국주식", TradeType: "매도", Quantity: 30, AmountKRW: 330_000},
	}
	rows := aggregateIndexWeight(trades)
	for _, r := range rows {
		assert.GreaterOrEqual(t, r.held, 0.0, r.bucket)
	}
}

// 섹터가 빈 거래는 미분류 줄로 모이고, 분모에도 포함된다.
func TestAggregateIndexWeight_UnknownShownWhenNonZero(t *testing.T) {
	trades := []model.Trade{
		{StockName: "AAA", StockCode: "AAA", Account: "해외", Currency: "EUR",
			Sector: "", Industry: "", TradeType: "매수", Quantity: 1, AmountKRW: 1_000_000},
		{StockName: "SPYM", StockCode: "SPYM", Account: "해외", Currency: "USD",
			Sector: "ETF", Industry: "S&P500", TradeType: "매수", Quantity: 1, AmountKRW: 1_000_000},
	}
	rows := aggregateIndexWeight(trades)
	var unknown *indexWeightRow
	for i := range rows {
		if rows[i].group == groupUnknown && rows[i].bucket == "" {
			unknown = &rows[i]
		}
	}
	if assert.NotNil(t, unknown, "미분류 그룹 행이 있어야 한다") {
		assert.Equal(t, 1_000_000.0, unknown.buy)
		assert.InDelta(t, 0.5, unknown.buyPct, 1e-9, "분모에 미분류 포함")
	}
}

// 거래가 없으면 빈 슬라이스.
func TestAggregateIndexWeight_Empty(t *testing.T) {
	assert.Empty(t, aggregateIndexWeight(nil))
}
```

- [ ] **Step 2: 테스트 실행 — 실패 확인**

Run: `go test ./internal/summary/ -run TestAggregateIndexWeight -v`
Expected: FAIL — `undefined: aggregateIndexWeight`, `undefined: indexWeightRow`

- [ ] **Step 3: 구현**

`internal/summary/index_weight.go` 에 추가 (import 에 `"github.com/kenshin579/auto-trading-journal/internal/model"` 추가):

```go
// indexWeightRow 는 표의 한 줄. bucket 이 빈 줄은 그룹 소계 행이다.
type indexWeightRow struct {
	group   string
	bucket  string
	buy     float64 // 누적 매수금액(원)
	held    float64 // 보유 원금(원) = 잔여수량 × 평균매수단가
	buyPct  float64
	heldPct float64
}

// indexWeightLayout 은 표시 순서(고정). 거래가 없는 버킷도 0 으로 표시해 표 모양을 유지한다.
var indexWeightLayout = []struct{ group, bucket string }{
	{groupIndex, ""},
	{groupIndex, bucketSP500},
	{groupIndex, bucketNasdaq},
	{groupIndex, bucketKorea},
	{groupIndex, bucketOtherIndex},
	{groupOther, ""},
	{groupOther, bucketStock},
	{groupOther, bucketTheme},
	{groupOther, bucketDividend},
	{groupOther, bucketBondGold},
	{groupUnknown, ""},
}

// stockAgg 는 종목 단위(코드·이름·계좌·통화) 집계. 보유 원금 계산용.
type stockAgg struct {
	buyQty, buyAmount, sellQty float64
	sector, industry           string
}

// aggregateIndexWeight 는 거래를 지수/나머지 버킷별로 집계한다.
//
// 보유 원금 = max(0, 매수수량-매도수량) × (총매수금액/총매수수량).
// 버킷은 종목 단위로 한 번 정하므로 같은 종목의 거래는 모두 같은 칸에 들어간다.
// 비중 분모에는 미분류도 포함된다(지수+나머지+미분류 = 100%).
func aggregateIndexWeight(trades []model.Trade) []indexWeightRow {
	type key struct{ code, name, account, currency string }
	stocks := map[key]*stockAgg{}
	for _, t := range trades {
		if t.TradeType != "매수" && t.TradeType != "매도" {
			continue
		}
		k := key{t.StockCode, t.StockName, t.Account, t.Currency}
		a := stocks[k]
		if a == nil {
			a = &stockAgg{}
			stocks[k] = a
		}
		// 섹터/산업은 처음 만나는 비어 있지 않은 값을 쓴다.
		if a.sector == "" && t.Sector != "" {
			a.sector, a.industry = t.Sector, t.Industry
		}
		if t.TradeType == "매수" {
			a.buyQty += t.Quantity
			a.buyAmount += t.AmountKRW
		} else {
			a.sellQty += t.Quantity
		}
	}
	if len(stocks) == 0 {
		return nil
	}

	type sums struct{ buy, held float64 }
	byBucket := map[string]*sums{} // "group|bucket"
	byGroup := map[string]*sums{}
	var totalBuy, totalHeld float64

	add := func(m map[string]*sums, k string, buy, held float64) {
		s := m[k]
		if s == nil {
			s = &sums{}
			m[k] = s
		}
		s.buy += buy
		s.held += held
	}

	for _, a := range stocks {
		group, bucket := bucketOf(a.sector, a.industry)
		held := 0.0
		if a.buyQty > 0 {
			remain := a.buyQty - a.sellQty
			if remain > 0 {
				held = remain * (a.buyAmount / a.buyQty)
			}
		}
		add(byBucket, group+"|"+bucket, a.buyAmount, held)
		add(byGroup, group, a.buyAmount, held)
		totalBuy += a.buyAmount
		totalHeld += held
	}

	pct := func(v, total float64) float64 {
		if total == 0 {
			return 0
		}
		return v / total
	}

	rows := make([]indexWeightRow, 0, len(indexWeightLayout))
	for _, l := range indexWeightLayout {
		var s *sums
		if l.bucket == "" {
			s = byGroup[l.group]
		} else {
			s = byBucket[l.group+"|"+l.bucket]
		}
		if s == nil {
			s = &sums{}
		}
		// 미분류는 금액이 없으면 표시하지 않는다(정상 상태에서 노이즈 방지).
		if l.group == groupUnknown && s.buy == 0 && s.held == 0 {
			continue
		}
		rows = append(rows, indexWeightRow{
			group: l.group, bucket: l.bucket,
			buy: s.buy, held: s.held,
			buyPct: pct(s.buy, totalBuy), heldPct: pct(s.held, totalHeld),
		})
	}
	return rows
}
```

- [ ] **Step 4: 테스트 실행 — 통과 확인**

Run: `go test ./internal/summary/ -run 'TestAggregateIndexWeight|TestBucketOf|TestETFBuckets' -v`
Expected: PASS

- [ ] **Step 5: 커밋**

```bash
git add internal/summary/index_weight.go internal/summary/index_weight_test.go
git commit -m "feat(summary): 지수/나머지 누적매수·보유원금 집계 추가"
```

---

### Task 7: 시트 작성 (`writeIndexWeight`) — A:E 표 + Y:Z 차트 데이터 + 포맷

**Files:**
- Modify: `internal/summary/index_weight.go`
- Modify: `internal/summary/index_weight_test.go`
- Modify: `internal/summary/summary.go:50-54` (Generator 필드)

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/summary/index_weight_test.go` 에 추가:

```go
// 셀 값 생성: 제목행 + 컬럼헤더 + (그룹행은 "▸", 버킷행은 두 칸 들여쓰기).
func TestIndexWeightValues(t *testing.T) {
	rows := []indexWeightRow{
		{group: groupIndex, bucket: "", buy: 100, held: 50, buyPct: 0.5, heldPct: 0.5},
		{group: groupIndex, bucket: bucketSP500, buy: 100, held: 50, buyPct: 0.5, heldPct: 0.5},
		{group: groupOther, bucket: "", buy: 100, held: 50, buyPct: 0.5, heldPct: 0.5},
	}
	values, groupOffsets := indexWeightValues(rows)

	assert.Equal(t, "[지수 vs 나머지 투자]", values[0][0])
	assert.Equal(t, []any{"구분", "누적매수금액", "비중(%)", "보유원금", "비중(%)"}, values[1])
	assert.Equal(t, "▸ 지수", values[2][0])
	assert.Equal(t, "  S&P500", values[3][0])
	assert.Equal(t, "▸ 나머지", values[4][0])
	assert.Equal(t, 100.0, values[2][1])
	assert.Equal(t, 0.5, values[2][2])

	// 그룹 행 오프셋(0-based, 제목행 기준) — 배경색 적용에 쓰인다
	assert.Equal(t, []int{2, 4}, groupOffsets)
}

// 차트 헬퍼는 그룹 소계만, 보유원금 기준.
func TestIndexWeightPieHelper(t *testing.T) {
	rows := []indexWeightRow{
		{group: groupIndex, bucket: "", held: 50},
		{group: groupIndex, bucket: bucketSP500, held: 50},
		{group: groupOther, bucket: "", held: 30},
	}
	helper := indexWeightPieHelper(rows)
	assert.Equal(t, [][]any{
		{"[차트데이터] 지수 vs 나머지", "보유원금"},
		{"지수", 50.0},
		{"나머지", 30.0},
	}, helper)
}

// 거래가 없으면 헬퍼 데이터도 없다(제목 행만).
func TestIndexWeightPieHelper_Empty(t *testing.T) {
	assert.Len(t, indexWeightPieHelper(nil), 1)
}
```

- [ ] **Step 2: 테스트 실행 — 실패 확인**

Run: `go test ./internal/summary/ -run TestIndexWeight -v`
Expected: FAIL — `undefined: indexWeightValues`, `undefined: indexWeightPieHelper`

- [ ] **Step 3: 구현 — 순수 함수 두 개 + 시트 작성**

`internal/summary/index_weight.go` 에 추가 (import 에 `"context"`, `"fmt"`, `"log/slog"`, `gsheets "google.golang.org/api/sheets/v4"`, `"github.com/kenshin579/auto-trading-journal/internal/sheets"` 추가):

```go
// indexWeightValues 는 시트에 쓸 A:E 셀 값과 그룹 행 오프셋(0-based)을 만든다.
func indexWeightValues(rows []indexWeightRow) ([][]any, []int) {
	values := [][]any{
		{"[지수 vs 나머지 투자]", "", "", "", ""},
		{"구분", "누적매수금액", "비중(%)", "보유원금", "비중(%)"},
	}
	var groupOffsets []int
	for _, r := range rows {
		label := "  " + r.bucket
		if r.bucket == "" {
			label = "▸ " + r.group
			groupOffsets = append(groupOffsets, len(values))
		}
		values = append(values, []any{label, r.buy, r.buyPct, r.held, r.heldPct})
	}
	return values, groupOffsets
}

// indexWeightPieHelper 는 파이 차트용 (그룹, 보유원금) 데이터를 만든다(첫 행은 헤더).
func indexWeightPieHelper(rows []indexWeightRow) [][]any {
	helper := [][]any{{"[차트데이터] 지수 vs 나머지", "보유원금"}}
	for _, r := range rows {
		if r.bucket == "" {
			helper = append(helper, []any{r.group, r.held})
		}
	}
	return helper
}

// writeIndexWeight 는 "지수 vs 나머지 투자" 섹션을 작성한다.
// 표는 A:E, 파이 차트용 헬퍼 데이터는 Y:Z 에 쓴다
// (N:O=계좌별 파이, W:X=나라별 섹터 파이가 이미 쓰고 있고, 초기화 범위가 A1:Z 라 AA 이후는 안 지워진다).
func (g *Generator) writeIndexWeight(ctx context.Context, trades []model.Trade, startRow int) (int, error) {
	rows := aggregateIndexWeight(trades)
	values, groupOffsets := indexWeightValues(rows)

	endRow := startRow + len(values) - 1
	rng := fmt.Sprintf("%s!A%d:E%d", DashboardSheet, startRow, endRow)
	if err := g.client.UpdateCells(ctx, rng, values); err != nil {
		return 0, err
	}

	g.indexWeightPie = rowRange{}
	if helper := indexWeightPieHelper(rows); len(helper) > 1 {
		hEnd := startRow + len(helper) - 1
		hRng := fmt.Sprintf("%s!Y%d:Z%d", DashboardSheet, startRow, hEnd)
		if err := g.client.UpdateCells(ctx, hRng, helper); err != nil {
			return 0, err
		}
		g.indexWeightPie = rowRange{start: startRow + 1, end: hEnd, ok: true}
		// Z열(26) 보유원금 통화 포맷 — 차트 축의 과학적 표기 방지.
		g.pendingRequests = append(g.pendingRequests, sheets.BuildNumberFormatRequests(
			g.dashboardSheetID, []sheets.ColumnFormat{{Col: 26, Pattern: "₩#,##0"}}, startRow+1, hEnd)...)
	}

	g.collectIndexWeightFormats(startRow, endRow, groupOffsets)

	// 미분류가 남으면 그 금액만큼 지수/나머지 판단에서 빠진 것이므로 눈에 띄게 알린다.
	for _, r := range rows {
		if r.group == groupUnknown && r.bucket == "" && r.buy > 0 {
			slog.Warn("지수 분류 미분류 잔량 — OpenAI/FMP 키 또는 taxonomy 밖 카테고리 확인 필요",
				"누적매수", r.buy, "비중", r.buyPct)
		}
	}
	slog.Info("대시보드 지수 vs 나머지 작성", "rows", len(rows))
	return startRow + len(values), nil
}

// collectIndexWeightFormats 는 표의 숫자 포맷(B·D=원화, C·E=백분율)과 헤더/그룹 배경색을 수집한다.
func (g *Generator) collectIndexWeightFormats(startRow, endRow int, groupOffsets []int) {
	sid := g.dashboardSheetID
	build := sheets.BuildNumberFormatRequests
	krw := []sheets.ColumnFormat{{Col: 2, Pattern: "₩#,##0"}, {Col: 4, Pattern: "₩#,##0"}}
	pct := []sheets.ColumnFormat{
		{Col: 3, Pattern: "0.00%", Type: "PERCENT"},
		{Col: 5, Pattern: "0.00%", Type: "PERCENT"},
	}
	g.pendingRequests = append(g.pendingRequests, build(sid, krw, startRow, endRow)...)
	g.pendingRequests = append(g.pendingRequests, build(sid, pct, startRow, endRow)...)

	headerColor := &gsheets.Color{Red: 0.24, Green: 0.52, Blue: 0.78, ForceSendFields: []string{"Red", "Green", "Blue"}}
	groupColor := &gsheets.Color{Red: 0.85, Green: 0.92, Blue: 0.98, ForceSendFields: []string{"Red", "Green", "Blue"}}
	colorRanges := []sheets.ColorRange{
		{StartRow: startRow, EndRow: startRow, StartCol: 1, EndCol: 5, Color: headerColor},
	}
	for _, off := range groupOffsets {
		r := startRow + off
		colorRanges = append(colorRanges, sheets.ColorRange{StartRow: r, EndRow: r, StartCol: 1, EndCol: 5, Color: groupColor})
	}
	g.pendingRequests = append(g.pendingRequests, sheets.BuildColorRequests(sid, colorRanges)...)
}
```

- [ ] **Step 4: Generator 에 차트 범위 필드 추가**

`internal/summary/summary.go` 의 Generator 구조체(50-54행 부근) `countrySectorPies` 아래에 추가:

```go
	indexWeightPie      rowRange           // 지수 vs 나머지 pie (Y:Z 헬퍼)
```

- [ ] **Step 5: 테스트 실행 — 통과 확인**

Run: `go test ./internal/summary/ -v`
Expected: PASS (기존 테스트 포함 전부)

- [ ] **Step 6: 커밋**

```bash
git add internal/summary
git commit -m "feat(summary): 지수 vs 나머지 섹션 시트 작성 + 차트 데이터"
```

---

### Task 8: 대시보드에 섹션 삽입 + 파이 차트 생성

**Files:**
- Modify: `internal/summary/summary.go:92-100` (GenerateAll)
- Modify: `internal/summary/charts.go:200-211` 뒤

- [ ] **Step 1: GenerateAll 에 섹션 삽입**

`internal/summary/summary.go` 의 `writeInvestmentMetrics` 호출 다음, `writeTradingInsights` 앞에 삽입:

```go
	currentRow++ // 빈 행
	metricsStart := currentRow
	if currentRow, err = g.writeInvestmentMetrics(ctx, trades, currentRow); err != nil {
		return err
	}
	currentRow++ // 빈 행
	if currentRow, err = g.writeIndexWeight(ctx, trades, currentRow); err != nil {
		return err
	}
	currentRow++ // 빈 행
	insightsStart := currentRow
	if currentRow, err = g.writeTradingInsights(ctx, trades, currentRow); err != nil {
		return err
	}
```

`collectDashboardFormats` 는 월별 성과 범위를 `metricsStart-2` 로 잡고 `insightsStart` 는 쓰지 않으므로, 이 위치 삽입으로 기존 포맷 범위는 영향받지 않는다.

- [ ] **Step 2: 파이 차트 추가**

`internal/summary/charts.go` 의 나라별 섹터 pie 루프(200-211행) 다음에 추가:

```go
	// 차트: 지수 vs 나머지 (Pie, 보유원금 기준). Y열(24)=구분, Z열(25)=보유원금.
	if g.indexWeightPie.ok {
		specs = append(specs, buildPieChartSpec(
			sheetID, "지수 vs 나머지 (보유원금)",
			24, 25,
			g.indexWeightPie.start-1, g.indexWeightPie.end,
			chartRowSpacing*4, chartColSecondary, 450, 370,
		))
	}
```

- [ ] **Step 3: 차트 스펙 테스트 추가**

`internal/summary/index_weight_test.go` 에 추가:

```go
// 파이 차트가 Y:Z(0-based 24,25)를 소스로 잡는지 확인한다.
func TestIndexWeightPieChartSpec_UsesYZColumns(t *testing.T) {
	chart := buildPieChartSpec(7, "지수 vs 나머지 (보유원금)", 24, 25, 10, 13, 80, 20, 450, 370)
	domain := chart.Spec.PieChart.Domain.SourceRange.Sources[0]
	series := chart.Spec.PieChart.Series.SourceRange.Sources[0]
	assert.Equal(t, int64(24), domain.StartColumnIndex)
	assert.Equal(t, int64(25), domain.EndColumnIndex)
	assert.Equal(t, int64(25), series.StartColumnIndex)
	assert.Equal(t, int64(26), series.EndColumnIndex)
	assert.Equal(t, int64(10), domain.StartRowIndex)
	assert.Equal(t, int64(13), domain.EndRowIndex)
}
```

- [ ] **Step 4: 전체 테스트 + vet + 빌드**

Run: `make test && go vet ./... && make build`
Expected: 전부 PASS

- [ ] **Step 5: 드라이런으로 확인 (선택, 자격증명 있을 때)**

Run: `make dry`
Expected: 로그에 `대시보드 지수 vs 나머지 작성 rows=10` 형태가 찍히고 에러 없음. 시트에는 반영되지 않는다.

- [ ] **Step 6: 커밋**

```bash
git add internal/summary
git commit -m "feat(summary): 대시보드에 지수 vs 나머지 섹션과 파이 차트 배치"
```

---

### Task 9: 문서 갱신

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: 해외 ETF 설명 정정**

`CLAUDE.md` 의 `**internal/fmpcat** (resolver.go)` 항목에서 `not-found(fmp.ErrNotFound; 해외 ETF·미커버)는 빈 값으로 영구 캐시(재조회 안 함).` 로 시작하는 줄을 다음으로 교체:

```markdown
- **ETF/펀드 판별**: `Profile` 의 `IsEtf`/`IsFund` 플래그(같은 응답 필드 — API 호출 증가 없음).
  참이면 국내와 동일 표기로 통일 — 섹터=`"ETF"`, 산업=`internal/etfclass` taxonomy 카테고리
  (회사명으로 분류, 분류기 없으면 FMP 원본 산업 폴백). 산업 문자열 `Asset Management` 로
  판별하면 BDC(MAIN·HTGC·BXSL)·자산운용사(BN)가 오분류되므로 플래그를 쓴다.
- not-found(`fmp.ErrNotFound`)·일시적 오류는 빈 값 반환. 영구 캐시 `config/fmpcat_cache.json`
  (스키마 `v2` — 구버전 항목은 1회 재조회), lazy `fmp.NewClientFromEnv()`(`FMP_API_KEY`).
```

- [ ] **Step 2: etfclass 패키지를 구조도에 추가**

`CLAUDE.md` 의 Go 패키지 구조 블록에서 `bizcat` 줄 위에 추가:

```
├── etfclass - ETF 종목명 → 고정 taxonomy 카테고리(OpenAI). 국내/해외 공용
```

그리고 `**internal/bizcat**` 항목의 ETF 산업 설명에서 `internal/bizcat/etfclass.go` 를
`internal/etfclass` 로, `고정 25종` 을
`고정 28종(미국 시장대표는 S&P500/나스닥/미국주식(기타)로 분리, 팩터·스타일 포함)` 로 고친다.

- [ ] **Step 3: 대시보드 섹션 설명에 새 섹션 추가**

`**internal/summary**` 항목을 다음으로 교체:

```markdown
**internal/summary** (`summary.go`, `sections.go`, `insights.go`, `index_weight.go`, `country_sector.go`, `formats.go`, `charts.go`):
- 단일 "대시보드" 시트 생성: 포트폴리오/월별 요약 + 투자지표 + **지수 vs 나머지 투자** +
  매매인사이트/월별추이/나라별 섹터비중/종목별 현황 + basic/pie 차트. 매 실행 초기화 후 재작성
- `index_weight.go`: ETF 카테고리를 지수(S&P500/나스닥/한국/기타지역)와 나머지(개별종목/테마·섹터/
  배당·전략/채권·금)로 매핑해 **누적 매수금액**과 **보유 원금**(잔여수량×평균매수단가) 두 기준으로
  집계. 차트 데이터는 `Y:Z`(초기화 범위 `A1:Z` 안이어야 한다)
```

- [ ] **Step 3B: 캐시 재조회 한계를 Troubleshooting 에 명시**

`CLAUDE.md` 의 Troubleshooting 절에 항목을 추가한다. 이걸 모르면 사용자가 백필을 아무리 돌려도
안 고쳐지는 이유를 찾느라 오래 헤맨다:

```markdown
### OpenAI 키를 나중에 추가했는데 ETF 산업이 그대로다
`STOCK_DATA_OPENAI_API_KEY` 없이 실행하면 ETF 산업이 KIS 지수명으로 채워져 **현재 캐시 버전으로**
저장된다. 나중에 키를 넣어도 `needsRefresh` 가 버전만 보므로 자동 재조회되지 않는다.
`make backfill-sectors` 도 같은 캐시를 읽으므로 그것만으로는 갱신되지 않는다.
`config/bizcat_cache.json` 의 ETF 항목을 지우거나 파일을 통째로 삭제한 뒤 백필을 실행할 것.
```

- [ ] **Step 4: 인코딩 확인**

Run: `file -I CLAUDE.md`
Expected: `text/plain; charset=utf-8`

- [ ] **Step 5: 커밋**

```bash
git add CLAUDE.md
git commit -m "docs: 해외 ETF 판별 방식 정정 및 지수 vs 나머지 섹션 반영"
```

---

## 적용 (머지 후 1회)

캐시 버전이 올라가 자동 재조회되므로 캐시 파일을 지울 필요는 없다.

- [ ] `make backfill-sectors 2>&1 | tee /tmp/backfill.log` — 기존 시트 행의 섹터/산업 갱신.
      로그를 파일로 남겨야 아래 검수의 "경고 건수" 항목을 실제로 확인할 수 있다.
  - 국내 ETF 89건 KIS 재조회(코드당 3콜, 3 calls/s → 약 1~2분), 해외 105건 FMP 재조회,
    OpenAI ETF 재분류 약 124건
  - 로그에 `섹터/산업 백필 완료` 가 시트마다 찍히는지 확인
  - 분류가 일시 실패한 종목은 **그 실행에서만** 옛 값(taxonomy 밖)이 남아 대시보드에 `미분류` 로
    보이고 다음 실행에 자동 치유된다. 백필 직후 미분류가 조금 보이면 한 번 더 실행할 것
- [ ] **백필 직후 `config/bizcat_cache.json` · `config/fmpcat_cache.json` 을 눈으로 검수한다.**
      이 기능의 정확도는 전적으로 LLM 프롬프트에 달려 있는데 CI 로 잡히는 것이 없다. 캐시가 곧
      산출물이므로 실입력 약 194건을 1회 확인하는 것이 픽스처 테스트보다 커버리지가 넓다.
      확인 항목:
  - `배당` 건수가 백필 전(8건) 대비 급감하지 않았는가 — 급감했으면 배당 규칙이 지수 쪽으로 샌 것
  - `S&P500` + `나스닥` + `미국주식(기타)` 합계가 백필 전 `미국주식`(15건)과 정합적인가
  - 커버드콜(JEPI·JEPQ)·레버리지·팩터 펀드가 지수 3종 안에 하나도 없는가
  - 실행 로그에 `ETF 카테고리가 taxonomy 밖` / `표기 정규화` 경고가 몇 건인가
- [ ] `make run` — 대시보드에 새 섹션 반영
- [ ] 시트에서 확인: `미분류` 줄이 보이면 그 금액만큼 분류가 빠진 것 — 로그의 경고로 원인 확인

## PR

```bash
git push -u origin feature/dashboard-index-weight
gh pr create --assignee kenshin579 --title "feat: 대시보드 지수 vs 나머지 투자 섹션" --body "$(cat <<'EOF'
## Summary
- 대시보드에 시장대표 지수(S&P500·나스닥·코스피/코스닥 등) 투자와 그 외 투자를 누적 매수금액·보유 원금 두 기준으로 비교하는 섹션 추가
- 해외 ETF 판별을 FMP `IsEtf`/`IsFund` 플래그로 정확화(API 호출 증가 없음) — BDC·자산운용사 오분류 제거
- ETF 카테고리에서 미국 시장대표를 S&P500/나스닥/미국주식(기타)로 세분, 커버드콜·레버리지·팩터의 지수 오분류 방지 규칙 추가
- ETF 분류기를 `internal/etfclass` 공용 패키지로 분리(국내·해외 공용)

## Test plan
- [ ] `make test` 전체 통과
- [ ] `make backfill-sectors` 후 해외 ETF 행이 `ETF / 카테고리`로 갱신되는지 확인
- [ ] `make run` 후 대시보드 "지수 vs 나머지 투자" 섹션과 파이 차트 확인
- [ ] `미분류` 줄이 비어 있는지(분류 누락 없음) 확인

설계: `docs/superpowers/specs/2026-08-01-dashboard-index-weight-design.md`
EOF
)"
```
