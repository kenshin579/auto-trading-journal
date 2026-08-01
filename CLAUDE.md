# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Auto Trading Journal은 증권사별 CSV 파일을 파싱하여 구글 시트에 자동으로 매매일지를 작성하는 애플리케이션입니다. 국내/해외 주식을 지원하며, 증권사별 CSV 형식을 자동 감지합니다.

> **구현 언어: Go.** (Python 원본에서 1:1 포팅 후 Python 코드는 제거됨. 이력은 git `feature/go-porting` 브랜치 참조.) CSV는 CP949/UTF-8 모두 Go가 네이티브로 처리합니다(iconv 사전변환 불필요).

## Quick Start Commands (Go)

```bash
# 실행 (config/config.yaml + env GOOGLE_SPREADSHEET_ID / SERVICE_ACCOUNT_PATH 필요)
make run                 # = go run ./cmd/atj --log-level INFO
make dry                 # 드라이런 (시트 미반영)
make backfill-sectors    # 기존 국내 시트 행의 섹터/산업 열 일괄 채움(1회용)
go run ./cmd/atj --dry-run --log-level DEBUG

# 빌드 / 테스트
make build               # → ./atj 바이너리
make test                # = go test ./...
go test ./internal/parser/ -v
```

CLI 플래그: `--dry-run`, `--log-level DEBUG|INFO|WARNING|ERROR`.

## Architecture Overview

### Go 패키지 구조 (현재 메인)

```
cmd/atj/main.go              - 진입점 + 오케스트레이션 (StockDataProcessor 대응)
internal/
├── config   - config.yaml + env(GOOGLE_SPREADSHEET_ID / SERVICE_ACCOUNT_PATH) 로드
├── model    - Trade struct + 행 변환 + 중복키 (DupKey)
├── parser   - Parser 인터페이스 + registry(DetectParser) + mirae/hankook
│             (readCSVRows 가 CP949/UTF-8 네이티브 디코딩)
├── symbol   - KRX .mst → 종목코드 (fwf + cp949, 7일 캐시)
├── etfclass - ETF 라벨링 계약(섹터 판별자 + 고정 taxonomy + OpenAI 분류기). 국내/해외 공용
├── bizcat   - KIS 업종 조회 → 국내 섹터/산업 (InquirePrice + SearchStockInfo, 영구 캐시) — 거래행 섹터/산업 열용
├── fmpcat   - FMP 회사 프로필 → 해외 섹터/산업 (Company.Profile, 영구 캐시, 통화→거래소 접미사 US/JP) — 해외 거래행용
├── sector   - OpenAI 섹터 분류 (go-openai, JSON 캐시) — 요약 "섹터별 투자비중"용 (bizcat과 별개)
├── sheets   - Google Sheets v4 래퍼 (값/포맷/색상/필터/차트 + 레이트리밋 재시도)
├── writer   - 시트 생성/중복필터/삽입 + ReadAllTrades
└── summary  - 단일 "대시보드" 시트 (요약/지표/인사이트/추이/차트)
```

### Data Processing Pipeline

1. **CSV 스캔**: `input/{증권사명}/` 하위 CSV 파일 탐색 (`sample/` 제외)
2. **파서 감지**: CSV 헤더를 읽어 파서 자동 선택
3. **파싱**: 증권사 형식에 맞춰 Trade 객체 리스트 생성
4. **종목코드 보강**: 국내 거래 중 종목코드가 빈 항목을 KRX 마스터에서 조회해 채움
4b. **섹터/산업 보강**: 국내 거래는 KIS(`internal/bizcat`) — 섹터=InquirePrice 업종 한글명(bstp_kor_isnm), 산업=표준산업분류. ETF 는 섹터="ETF", 산업=종목명 OpenAI taxonomy. **해외 거래는 FMP(`internal/fmpcat`)** — 통화로 거래소 접미사(US/JP)를 붙여 `Company.Profile` 조회, 섹터/산업 영문 원본. 해외 ETF/펀드는 `IsEtf`/`IsFund` 플래그로 판별해 국내와 동일하게 섹터="ETF", 산업=`internal/etfclass` taxonomy 카테고리로 통일
5. **시트 확인**: 시트가 없으면 자동 생성 + 헤더 삽입
6. **중복 필터**: 기존 시트 데이터와 비교하여 중복 제거
7. **데이터 삽입**: 신규 거래 일괄 삽입 + 숫자/통화 포맷 적용 (거래 시트 날짜별 배경색은 현재 미적용)
8. **대시보드 갱신**: 단일 "대시보드" 시트 초기화 후 재작성 (포트폴리오/월별/종목별/투자지표/인사이트/추이 + 차트)

### Key Data Model

**Trade** (dataclass):
- 16개 필드: date, trade_type, stock_name, stock_code, quantity, price, amount, currency, exchange_rate, amount_krw, fee, tax, profit, profit_krw, profit_rate, account
- `to_domestic_row()`: 국내 12컬럼 (일자, 구분, 종목코드, 종목명, **섹터, 산업**, 수량, 단가, 금액, 수수료, 손익금액, 수익률)
- `to_foreign_row()`: 해외 17컬럼 (종목명 뒤 **섹터/산업** 공란)
- `duplicate_key()`: (date, trade_type, stock_name, quantity, price) 튜플

### Package Responsibilities

**internal/model** (`trade.go`):
- `Trade` struct (Sector/Industry 포함) + `ToDomesticRow`(12컬럼)/`ToForeignRow`(17컬럼)/`ToSheetRow` + `DuplicateKey`(DupKey; 섹터/산업 미포함)

**internal/bizcat** (`resolver.go`):
- 섹터 = `Domestic.InquirePrice(code)` 의 **업종 한글명**(`BstpKorIsnm`/bstp_kor_isnm, 예 "전기·전자"/"IT 서비스"/"의료·정밀기기"). 일반 종목 커버리지가 가장 넓고(지수업종 중분류는 대형주만 채워짐) moneyflow `sector_detail` 과 동일 소스. ETF 는 섹터를 짧게 `"ETF"` 로 정규화.
- 산업 = `Domestic.SearchStockInfo(code,"300")` 의 **표준산업분류**(`StdIdstClsfCdName`, 예 "의료용 기기 제조업"). ⚠️ 지수업종(대/중/소분류)은 커버리지가 낮아 미사용.
- **ETF 산업** = **펀드 종목명을 OpenAI taxonomy 로 분류**(`internal/etfclass`, 고정 28종(미국 시장대표는 S&P500/나스닥/미국주식(기타)로 분리, 팩터·스타일 포함): 한국/S&P500/나스닥/미국주식(기타)/중국/일본/인도/베트남/글로벌주식 · 반도체/2차전지/바이오·헬스케어/AI·로봇/신재생에너지/원자력/방위·우주항공/자동차/금융/건설/필수소비재/IT·인터넷 · 배당/팩터·스타일/리츠·부동산/원자재/채권/통화·단기금리 · 기타테마). 코드 분류 여부와 무관하게 종목명으로 통일(verbose 한 KRX 지수명도 정규화). 분류기 미설정/실패 시 `Domestic.InquireEtfPrice` 의 지수명(`EtfRprsBstpKorIsnm`, "KRX " 접두사 제거) 폴백. ETF 판별은 `EtfDvsnCd != ""`(+ bstp_kor_isnm "ETF…" 접두사 백업).
- 즉 코드당 InquirePrice + SearchStockInfo **2회 호출**(ETF 는 InquireEtfPrice 까지 **3회**). KIS 초당 제한(EGW00201) 회피를 위해 `WithRateLimit(kisCallsPerSec=3)` 로 호출을 페이싱하고, 같은 실행 내 실패 코드는 negative-cache(`failed`, 비영구)로 재조회를 막는다. 성공 결과는 `config/bizcat_cache.json` 에 영구 캐시(실행 간 유지, 스키마 `v5` — 구버전 항목은 1회 재조회). 구버전 캐시의 `{섹터:"ETF", 산업:""}` 항목은 자가치유로 재조회되어 산업이 채워진다(`needsRefresh`).
- 영구 캐시 `config/bizcat_cache.json`, lazy `kis.NewClientFromEnv()`. KIS 키 없거나 실패 시 빈 값(회복력).
- atj 는 `ensureKISFileToken`(main.go)으로 **파일 토큰 강제** — env가 redis 라도 Redis 의존 없이 동작.

**internal/fmpcat** (`resolver.go`) — 해외 종목 섹터/산업:
- `Resolve(ticker, currency)` → FMP `Company.Profile(symbol)` 의 **영문 섹터/산업**(예 AAPL→Technology/Consumer Electronics). 표기는 영문 원본(나라별 비중 분석용).
- `exchangeSuffix(currency)`: 통화로 거래소 접미사. 현재 **US/JP 만**(`USD→""`, `JPY→".T"`), 그 외 통화는 미지원→공란. 캐시 키 = `ticker+suffix`(예 "7203.T").
- **ETF/펀드 판별**: `Profile` 의 `IsEtf`/`IsFund` 플래그(같은 응답 필드 — API 호출 증가 없음).
  참이면 국내와 동일 표기로 통일 — 섹터=`"ETF"`, 산업=`internal/etfclass` taxonomy 카테고리
  (회사명으로 분류, 분류기 없으면 FMP 원본 산업 폴백). 산업 문자열 `Asset Management` 로
  판별하면 BDC(MAIN·HTGC·BXSL)·자산운용사(BN)가 오분류되므로 플래그를 쓴다.
- 분류가 **일시 실패**하면 캐시하지 않고 다음 실행에 재시도한다(taxonomy 밖 값을 영구 캐시하면
  그 종목이 대시보드에서 영구히 미분류로 남는다).
- not-found(`fmp.ErrNotFound`)·일시적 오류는 빈 값 반환. 영구 캐시 `config/fmpcat_cache.json`
  (스키마 `v2` — 구버전 항목은 1회 재조회), lazy `fmp.NewClientFromEnv()`(`FMP_API_KEY`).

**internal/parser** (`parser.go`, `mirae.go`, `hankook.go`, `registry.go`):
- `Parser` 인터페이스(Name/CanParse/Parse), `DetectParser`(헤더 기반 자동 선택, 순서 Mirae국내→Mirae해외→Hankook)
- `MiraeDomestic`(헤더 `일자,종목명,기간 중 매수`, 서브헤더 스킵), `MiraeForeign`(`매매일,통화,종목번호`), `HankookDomestic`(`매매일자,종목코드,매입단가`)
- `readCSVRows`: CP949/UTF-8 네이티브 디코딩 + 천단위/쌍따옴표 처리

**internal/symbol** (`resolver.go`):
- KRX 공개 마스터(`kospi/kosdaq_code.mst.zip`) 다운로드/cp949/fwf 파싱, `~/.cache/auto-trading-journal` 7일 캐시
- `Resolver.Resolve`: 종목명→단축코드(lazy, 무인증·오프라인). 국내 CSV 코드 보강용

**internal/sheets** (`client.go`, `format.go`, `chart.go`):
- Google Sheets API v4 래퍼(서비스계정 인증), 값 I/O, 포맷/색상/필터/차트, 레이트리밋 재시도(`executeWithRetry`)
- ⚠️ **모든** API 호출(읽기 `Spreadsheets.Get`/`Values.Get`, 쓰기 `values.update`/`batchUpdate`)은
  `executeWithRetry` 를 거쳐야 한다. Sheets 쿼터는 **읽기/쓰기 각각 분당 60회(프로젝트·사용자 단위)** 이고
  버킷이 비면 최대 60초를 기다려야 회복되므로, 재시도 누적 대기가 60초를 넘도록 `maxRetries=6`
  (1+2+4+8+16+32≈63초)으로 잡혀 있다(`retry_test.go` 의 `TestRetryWaitBudgetCoversQuotaWindow` 가 이 불변식을 지킨다).
- `NewWithEndpoint` 는 테스트용 fake 서버(httptest)를 향하는 무인증 클라이언트다.

**internal/writer** (`writer.go`, `headers.go`, `reader.go`):
- `EnsureSheetExists`, `GetExistingKeys`(중복키), `InsertTrades`(포맷 적용), `ReadAllTrades`(대시보드 입력), 국내/해외 헤더 상수
- `ReadAllTrades` 는 읽기 쿼터를 아끼기 위해 **시트당 그리드 조회 1회**(`A1:Q10000`, 헤더=1행)만 쓴다.
  또한 조회 실패를 스킵하지 않고 **에러로 전파**한다 — 일부 시트만 실패한 채 진행하면 대시보드가
  부분 데이터로 통째로 재작성되어 기존 내용을 잃는다.

**internal/summary** (`summary.go`, `sections.go`, `insights.go`, `index_weight.go`, `country_sector.go`, `formats.go`, `charts.go`):
- 단일 "대시보드" 시트 생성: 포트폴리오/월별 요약 + 투자지표 + **지수 vs 나머지 투자** +
  매매인사이트/월별추이/나라별 섹터비중/종목별 현황 + basic/pie 차트. 매 실행 초기화 후 재작성
- `index_weight.go`: ETF 카테고리를 지수(S&P500/나스닥/한국/기타지역)와 나머지(개별종목/테마·섹터/
  배당·전략/채권·금)로 매핑해 **누적 매수금액**과 **보유 원금** 두 기준으로 집계. 차트 데이터는
  `Y:Z`(초기화 범위 `A1:Z` 안이어야 한다)
- 보유원금 = 잔여수량 × 전 기간 평균매수단가(**시세가 아니다**). 매수가 매도보다 앞선 종목은
  정확하고, 매도 후 재매수한 종목은 실제 취득원가와 차이가 날 수 있다
- taxonomy 밖 값이나 섹터가 빈 거래는 **미분류**로 따로 표시한다(임의로 다른 칸에 넣지 않는다).
  미분류 종목명과 `매도수량 > 매수수량` 종목은 실행 로그에 경고로 남는다

**internal/sector** (`classifier.go`):
- OpenAI(go-openai) 섹터 분류 + JSON 캐시(`config/sector_cache.json`). `STOCK_DATA_OPENAI_API_KEY` 있을 때만 활성

**internal/config** (`config.go`):
- `config.yaml` + env(`GOOGLE_SPREADSHEET_ID`, `SERVICE_ACCOUNT_PATH`, `STOCK_DATA_OPENAI_API_KEY`) 로드

## Configuration

### Required Files

**config/config.yaml**:
```yaml
google_sheets:
  spreadsheet_id: YOUR_SPREADSHEET_ID
  service_account_path: /path/to/service_account_key.json

logging:
  level: INFO
```

**Environment Variables** (optional override):
- `GOOGLE_SPREADSHEET_ID`: 스프레드시트 ID
- `SERVICE_ACCOUNT_PATH`: 서비스 계정 키 파일 경로
- `STOCK_DATA_OPENAI_API_KEY`: OpenAI 키(국내 ETF 산업 분류 + 대시보드 섹터 분류). 없으면 해당 기능 비활성
- `FMP_API_KEY`: FMP 키(해외 종목 섹터/산업). 없으면 해외 보강 비활성(공란)

### Google Sheets Setup

1. Google Cloud Console에서 서비스 계정 생성
2. Google Sheets API 활성화
3. JSON 키 파일 다운로드
4. 대상 스프레드시트에 서비스 계정 이메일 편집자 권한 부여

### Sheet Structure

하나의 Google Spreadsheet 내에 모든 시트가 탭으로 존재:
```
[미래에셋증권_국내계좌] [미래에셋증권_해외계좌] [한국투자증권_국내계좌] [요약_월별] [요약_종목별]
```

시트 이름 = `{증권사 폴더명}_{CSV 파일명(확장자 제외)}`

### 시트 컬럼 구조 (섹터/산업 포함)

국내계좌 시트는 **12컬럼**: 일자, 구분, 종목코드, 종목명, **섹터, 산업**, 수량, 단가, 금액,
수수료, 손익금액, 수익률(%). 해외계좌 시트는 **17컬럼**(종목명 뒤 섹터=F/산업=G, FMP 영문).
- 종목코드: CSV에 없으면 KRX 마스터(`internal/symbol`)에서 조회.
- 섹터/산업: 국내는 KIS(`internal/bizcat`, 섹터=업종 한글명 / 산업=표준산업분류), 해외는 FMP(`internal/fmpcat`, 영문 섹터/산업, 통화→거래소 접미사 US/JP). 해외 ETF/펀드는 `IsEtf`/`IsFund` 플래그로 국내와 동일하게 섹터="ETF"/산업=taxonomy 카테고리로 통일된다. 미지원 통화·미커버는 공란.

**기존 시트 마이그레이션**: 섹터/산업(또는 종목코드) 컬럼 도입 이전 포맷(10/15/9컬럼) 시트는
헤더 불일치로 **경고 로그와 함께 스킵**됩니다(자동 변환 안 함). 시트를 삭제 후 재실행하거나
종목명 뒤에 `섹터`,`산업` 컬럼을 수동 삽입하세요.

## Input File Format

CSV 파일을 `input/{증권사명}/` 디렉토리에 배치:
```
input/
├── 미래에셋증권/
│   ├── 국내계좌.csv
│   └── 해외계좌.csv
├── 한국투자증권/
│   └── 국내계좌.csv
└── sample/          ← 처리 제외
```

## Git Branch Policy

**NEVER commit directly to main/master branch.** Always use feature branches.

1. **Create feature branch before making changes**:
   ```bash
   git checkout main && git pull origin main
   git checkout -b feature/{issue-number}-{feature-name}
   ```

2. **Branch naming conventions**:
   - `feature/{issue-number}-{name}` or `feat/{name}` - New features
   - `fix/{issue-number}-{name}` or `fix/{name}` - Bug fixes
   - `chore/{name}` - Maintenance tasks

3. **After completing work, create PR via `gh` CLI** (not GitHub MCP):
   ```bash
   gh pr create --assignee kenshin579 --title "type: 작업 요약" --body "$(cat <<'EOF'
   ## Summary
   - 변경 사항

   ## Test plan
   - [ ] 테스트 항목
   EOF
   )"
   ```

## Important Implementation Notes

### 실행 흐름 (cmd/atj)
- `context.Context` 를 모든 Sheets 호출에 전달. Sheets 작업은 순차 + 배치(레이트리밋 최소화)
- 진입점: `cmd/atj/main.go` → scan → 파일별 처리(파서감지/파싱/코드보강/중복필터/삽입) → 대시보드 갱신
- `STOCK_DATA_OPENAI_API_KEY` 설정 시에만 섹터 분류 활성

### Duplicate Detection
`(date, trade_type, stock_name, quantity, price)` 5-tuple로 중복 판별

### Color Coding
거래 시트의 날짜별 8색 배경 팔레트는 제거됨(현재 미적용). 배경색은 "대시보드" 시트의
헤더 행에만 적용된다(`summary` 패키지).

### Adding a New Broker Parser

1. `internal/parser/` 에 새 파서 파일 생성
2. `Parser` 인터페이스(`Name`/`CanParse`/`Parse`) 구현
3. `internal/parser/registry.go` 의 `registry` 슬라이스에 등록
4. `internal/parser/{broker}_test.go` 테스트 + 루트 `testdata/` CSV 픽스처 추가
5. `make test` 로 전체 통과 확인

### Text Encoding (Korean Content)

**Encoding Standard**: All files MUST be UTF-8 encoded (한글 콘텐츠 필수)

1. **Verify encoding after file creation**:
   ```bash
   file -I path/to/file.md
   ```

2. **If encoding is broken (charset=binary)**:
   ```bash
   cat > file.md << 'EOF'
   한글 내용...
   EOF
   ```

## Troubleshooting

### "파서 감지 실패"
CSV 헤더가 지원되는 형식과 일치하지 않음. `--log-level DEBUG`로 헤더 확인.

### "시트를 찾을 수 없습니다"
서비스 계정에 스프레드시트 편집 권한이 있는지 확인.

### Duplicate trades keep inserting
(date, trade_type, stock_name, quantity, price) 5개 필드의 정확한 일치 여부 확인.

### Service account authentication errors
1. `config/config.yaml`의 JSON 키 파일 경로 확인
2. 서비스 계정 이메일에 편집자 권한 부여 확인
3. Google Cloud Console에서 Sheets API 활성화 확인

### OpenAI 키를 나중에 추가했는데 ETF 산업이 그대로다
`STOCK_DATA_OPENAI_API_KEY` 없이 실행하면 ETF 산업이 KIS 지수명/FMP 원본 산업으로 채워져
**현재 캐시 버전으로** 저장된다. 나중에 키를 넣어도 `needsRefresh` 가 버전만 보므로 자동
재조회되지 않는다. `make backfill-sectors` 도 같은 캐시를 읽으므로 그것만으로는 갱신되지 않는다.
`config/bizcat_cache.json`·`config/fmpcat_cache.json` 의 해당 항목을 지우거나 파일을 통째로
삭제한 뒤 백필을 실행할 것.

### 대시보드 "지수 vs 나머지" 에 미분류가 크게 잡힌다
분류가 빠진 금액이다. 실행 로그의 `지수 분류 미분류 종목` 경고에 상위 종목명과 금액이 찍힌다.
흔한 원인: (1) `STOCK_DATA_OPENAI_API_KEY`/`FMP_API_KEY` 미설정, (2) US/JP 외 통화 보유분
(`fmpcat` 의 `exchangeSuffix` 미지원 → 섹터 공란), (3) 분류가 일시 실패해 옛 값이 남은 경우
— 이건 다음 실행에 자동 치유되므로 한 번 더 실행하면 된다.
