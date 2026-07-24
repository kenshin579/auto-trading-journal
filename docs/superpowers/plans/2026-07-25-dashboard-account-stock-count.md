# 대시보드 계좌별 종목수 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 대시보드 "투자 지표" 섹션에 계좌별 보유종목수 / 거래종목수 블록(합계 행 포함)을 추가한다.

**Architecture:** 순수 집계 함수 `aggregateAccountStockCount` 를 `internal/summary/insights.go` 에 추가하고, `writeInvestmentMetrics` 가 기존 "계좌별 투자비중" 블록 바로 뒤에 3열 행들을 끼워 넣는다. 이 블록만 C열을 쓰므로 섹션 쓰기 범위를 `A:B` → `A:C` 로 넓힌다. 서식은 건드리지 않는다(종목수는 정수 그대로).

**Tech Stack:** Go, 표준 `testing` 패키지, Google Sheets API v4 (`internal/sheets`)

**Spec:** `docs/superpowers/specs/2026-07-25-dashboard-account-stock-count-design.md`

---

## File Structure

- Modify: `internal/summary/insights.go`
  - 집계 함수 `aggregateAccountStockCount` 추가
  - `writeInvestmentMetrics` 안에 블록 생성 + 쓰기 범위 `A:C` 로 변경
- Modify: `internal/summary/insights_test.go`
  - 집계 함수 단위 테스트 추가

새 파일은 만들지 않는다. `insights.go` 는 880줄이지만 이 변경은 섹션 4 로컬이라
분리하지 않고 기존 구조를 따른다.

---

### Task 1: 계좌별 종목수 집계 함수

**Files:**
- Modify: `internal/summary/insights.go` (섹션 4 블록 `── 섹션 4: 투자 지표 ──` 주석 아래, `writeInvestmentMetrics` 함수 **위**에 타입/함수 추가)
- Test: `internal/summary/insights_test.go` (파일 끝에 추가)

- [ ] **Step 1: Write the failing test**

`internal/summary/insights_test.go` 파일 맨 끝에 아래를 추가한다.

```go
// tr 는 테스트용 거래를 만든다(계좌별 종목수 집계용).
func tr(account, tradeType, code, name, currency string, qty float64) model.Trade {
	return model.Trade{
		Account: account, TradeType: tradeType,
		StockCode: code, StockName: name, Currency: currency, Quantity: qty,
	}
}

func TestAggregateAccountStockCount(t *testing.T) {
	trades := []model.Trade{
		// 한국투자: 삼성전자 10주 매수 후 10주 전량 매도 → 거래O, 보유X
		tr("한국투자증권_국내계좌", "매수", "005930", "삼성전자", "KRW", 10),
		tr("한국투자증권_국내계좌", "매도", "005930", "삼성전자", "KRW", 10),
		// 한국투자: SK하이닉스 5주 매수 후 2주 매도 → 부분매도, 보유O
		tr("한국투자증권_국내계좌", "매수", "000660", "SK하이닉스", "KRW", 5),
		tr("한국투자증권_국내계좌", "매도", "000660", "SK하이닉스", "KRW", 2),
		// 미래에셋: AAPL 3주 매수(미매도) → 보유O
		tr("미래에셋증권_해외계좌", "매수", "", "AAPL", "USD", 3),
		// 미래에셋: TSLA 1주 매수(미매도), 코드 없음 → 이름으로 구분되어야 함
		tr("미래에셋증권_해외계좌", "매수", "", "TSLA", "USD", 1),
	}

	got := aggregateAccountStockCount(trades)

	want := []accountStockCount{
		{account: "미래에셋증권_해외계좌", held: 2, total: 2},
		{account: "한국투자증권_국내계좌", held: 1, total: 2},
	}
	if len(got) != len(want) {
		t.Fatalf("len: got %d, want %d (%+v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d]: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestAggregateAccountStockCountEmpty(t *testing.T) {
	if got := aggregateAccountStockCount(nil); len(got) != 0 {
		t.Errorf("empty input: got %+v, want empty", got)
	}
}
```

정렬 검증은 `want` 의 순서(미래에셋 → 한국투자, 사전식)로 함께 확인된다.
종목코드가 빈 해외 거래 두 건(AAPL/TSLA)이 `total: 2` 로 나오는지가 이름 기반 구분 검증이다.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/summary/ -run TestAggregateAccountStockCount -v`
Expected: 컴파일 실패 — `undefined: aggregateAccountStockCount`, `undefined: accountStockCount`

- [ ] **Step 3: Write minimal implementation**

`internal/summary/insights.go` 에서 `// ── 섹션 4: 투자 지표 ──` 주석 줄 바로 아래,
`writeInvestmentMetrics` 함수 정의 **위에** 다음을 추가한다.

```go
// accountStockCount 는 계좌별 종목수 집계 결과.
// held = 잔량이 남은(매수수량 > 매도수량) 종목 수, total = 한 번이라도 거래한 종목 수.
type accountStockCount struct {
	account     string
	held, total int
}

// aggregateAccountStockCount 는 계좌별로 보유/거래 종목수를 세고 계좌명 사전식으로 정렬한다.
// 종목 식별 키는 (종목코드, 종목명, 통화) — 해외처럼 코드가 비어도 이름으로 구분된다.
func aggregateAccountStockCount(trades []model.Trade) []accountStockCount {
	type stockKey struct{ code, name, currency string }
	// 계좌 → 종목 → 순수량(매수 - 매도).
	netQty := map[string]map[stockKey]float64{}
	for _, t := range trades {
		if t.TradeType != "매수" && t.TradeType != "매도" {
			continue
		}
		byStock := netQty[t.Account]
		if byStock == nil {
			byStock = map[stockKey]float64{}
			netQty[t.Account] = byStock
		}
		k := stockKey{t.StockCode, t.StockName, t.Currency}
		if t.TradeType == "매수" {
			byStock[k] += t.Quantity
		} else {
			byStock[k] -= t.Quantity
		}
	}

	accounts := make([]string, 0, len(netQty))
	for a := range netQty {
		accounts = append(accounts, a)
	}
	sort.Strings(accounts)

	result := make([]accountStockCount, 0, len(accounts))
	for _, a := range accounts {
		c := accountStockCount{account: a, total: len(netQty[a])}
		for _, qty := range netQty[a] {
			if qty > 1e-9 { // 부동소수 오차 방지
				c.held++
			}
		}
		result = append(result, c)
	}
	return result
}
```

`sort` 는 `insights.go` 에 이미 import 되어 있다. 추가 import 는 없다.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/summary/ -run TestAggregateAccountStockCount -v`
Expected: `--- PASS: TestAggregateAccountStockCount`, `--- PASS: TestAggregateAccountStockCountEmpty`, `ok`

- [ ] **Step 5: Commit**

```bash
git add internal/summary/insights.go internal/summary/insights_test.go
git commit -m "feat(summary): 계좌별 보유/거래 종목수 집계 함수 추가"
```

---

### Task 2: 투자 지표 섹션에 블록 출력

**Files:**
- Modify: `internal/summary/insights.go` (`writeInvestmentMetrics` 내부 — "계좌별 투자비중" 블록 직후, "통화별 투자비중" 주석 직전 / 그리고 섹션 끝 쓰기 범위)

- [ ] **Step 1: 블록 생성 코드 삽입**

`writeInvestmentMetrics` 안에서 아래 기존 코드 블록을 찾는다.

```go
	// 통화별 투자비중.
	rows = append(rows, []any{"통화별 투자비중", ""})
```

그 **바로 앞에** 다음을 삽입한다.

```go
	// 계좌별 종목수 (보유 / 전체). 제목 행의 B·C 열이 헤더 역할을 한다.
	if counts := aggregateAccountStockCount(trades); len(counts) > 0 {
		rows = append(rows, []any{"계좌별 종목수", "보유", "전체"})
		var heldSum, totalSum int
		for _, c := range counts {
			rows = append(rows, []any{"  " + c.account, c.held, c.total})
			heldSum += c.held
			totalSum += c.total
		}
		rows = append(rows, []any{"  합계", heldSum, totalSum})
	}
```

`pctRows` / `krwRows` 에는 넣지 않는다 — 종목수는 정수 그대로 표시한다.

- [ ] **Step 2: 쓰기 범위를 A:C 로 확장**

같은 함수의 `// 데이터 작성.` 아래에서 다음 줄을 찾는다.

```go
	rng := fmt.Sprintf("%s!A%d:B%d", DashboardSheet, startRow, endRow)
```

다음으로 바꾼다.

```go
	// 계좌별 종목수 블록만 C열을 쓴다(나머지 행은 2열 ragged).
	rng := fmt.Sprintf("%s!A%d:C%d", DashboardSheet, startRow, endRow)
```

- [ ] **Step 3: 빌드 + 전체 테스트**

Run: `go build ./... && go test ./...`
Expected: 빌드 성공, 모든 패키지 `ok` 또는 `no test files`. 실패 시 다음 스텝으로 넘어가지 말 것.

- [ ] **Step 4: Commit**

```bash
git add internal/summary/insights.go
git commit -m "feat(summary): 투자 지표에 계좌별 종목수 블록(합계 포함) 표시"
```

---

### Task 3: 드라이런 확인

**Files:** 없음 (검증 전용)

- [ ] **Step 1: 드라이런 실행**

Run: `make dry`
Expected: 에러 없이 종료. 로그에 `대시보드 투자 지표 작성 rows=<N>` 이 찍히고,
N 이 변경 전보다 `계좌수 + 2` 만큼 커져 있어야 한다.

`input/` 에 CSV 가 없거나 자격증명이 없어 실행이 불가능하면 이 태스크는 건너뛰고,
그 사실을 결과 보고에 명시한다. 임의로 자격증명을 만들거나 설정을 바꾸지 말 것.

- [ ] **Step 2: 실제 시트 반영 (선택)**

사용자가 요청한 경우에만 `make run` 을 실행하고, 대시보드 투자 지표 섹션에
아래 형태가 나오는지 눈으로 확인한다.

```
계좌별 종목수      보유   전체
  미래에셋증권_국내계좌  12   30
  ...
  합계             25    56
```

---

### Task 4: PR 생성

**Files:** 없음

- [ ] **Step 1: 브랜치 푸시**

```bash
git push -u origin feature/dashboard-account-stock-count
```

- [ ] **Step 2: PR 생성**

```bash
gh pr create --assignee kenshin579 --title "feat(summary): 대시보드에 계좌별 종목수 표시" --body "$(cat <<'EOF'
## Summary
- 투자 지표 섹션에 `계좌별 종목수` 블록 추가 (보유 / 전체 / 합계)
- 보유 = 매수수량 − 매도수량 > 0 인 종목 수, 전체 = 한 번이라도 거래한 고유 종목 수
- 종목 식별 키는 (종목코드, 종목명, 통화) — 코드가 빈 해외 종목도 이름으로 구분
- 투자 지표 섹션 쓰기 범위를 A:B → A:C 로 확장

## Test plan
- [ ] `go test ./internal/summary/ -run TestAggregateAccountStockCount -v`
- [ ] `go test ./...`
- [ ] `make dry` 로 대시보드 생성 확인
EOF
)"
```

리뷰어는 지정하지 않는다.
