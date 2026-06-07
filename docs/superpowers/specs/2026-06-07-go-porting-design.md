# auto-trading-journal Python → Go 전면 포팅 설계

- 작성일: 2026-06-07
- 상태: 승인됨 (구현 계획 대기)

## 배경 / 목적

auto-trading-journal은 증권사별 CSV 매매내역을 파싱해 Google Sheets에 매매일지를
자동 작성하는 도구다. 현재 Python(약 3,760줄 + 테스트 2,000줄)으로 구현돼 있다.

워크스페이스의 다른 SDK/서비스(`opendart-go`, `fmp-go`, `korea-investment-stock`,
moneyflow 백엔드)가 모두 Go로 정렬돼 있어, 본 도구도 Go로 전면 재작성한다.

이번 포팅의 정당성은 **워크스페이스 Go 정렬 + 향후 KIS SDK 기반 기능(섹터/산업 등)
추가를 위한 기반 마련**이다. 현재 Python 앱은 KIS SDK를 전혀 사용하지 않으므로
(자체 `.mst` 파서 + OpenAI만 사용), **이번 포팅도 KIS SDK를 import 하지 않는다.**

### 사전 검증 (완료)

Go `google.golang.org/api/sheets/v4`가 현재 Python(`google-api-python-client`)이
쓰는 **모든** Sheets 동작과 1:1 대응됨을 throwaway 스파이크로 경험적으로 증명했다.
가장 위험했던 **임베디드 차트(`addChart`)도 실제 스프레드시트에 정상 생성**됨을 확인.
둘 다 동일한 REST API v4를 감싼 생성 래퍼라 기능 차이가 없다.

## 범위

### 포함 (현재 Python 동작의 충실한 1:1 포팅)
- CSV 파서 3종 (미래에셋 국내/해외, 한국투자 국내) + 헤더 기반 자동 감지
- Trade 모델 + 국내(10컬럼)/해외(15컬럼) 행 변환 + 중복키
- Google Sheets v4 클라이언트 (배치/색상/포맷/필터/차트 + 레이트리밋/재시도)
- SheetWriter (시트 생성/중복필터/삽입/날짜별 색상/숫자·TEXT 포맷)
- SummaryGenerator (요약_월별, 요약_종목별, 대시보드 차트, 인사이트, **섹터별 투자비중**)
- SymbolResolver (KRX `.mst` → 종목코드, 7일 캐시)
- SectorClassifier (OpenAI 섹터 분류 + JSON 캐시) — *기존 요약 집계 유지를 위해 포팅*
- main 오케스트레이션 (`--dry-run`, `--log-level`)
- 기존 테스트 스위트 포팅

### 제외 (이번 작업 범위 밖)
- **신규 per-row 섹터/산업 열 추가** — 향후 별도 spec. 그때는 KIS SDK 공개 API
  `client.Domestic.SearchStockInfo(symbol, "300")`의 지수업종 3단계
  (`IdxBztpLclsCdName`=섹터 / `IdxBztpMclsCdName`=산업 / `IdxBztpSclsCdName`=세부)를
  그대로 사용한다 (한글명 직접 제공, 매핑/LLM 불필요). 해외는 `overseas` 시세 `EIcod`.
- KIS SDK import

## 아키텍처

### 패키지 구조
```
auto-trading-journal/
├── go.mod                    # module github.com/kenshin579/auto-trading-journal
├── cmd/atj/main.go           # 진입점 + 오케스트레이션
├── internal/
│   ├── config/               # YAML + env 오버라이드
│   ├── model/                # Trade struct + 행 변환 + 중복키
│   ├── parser/               # base(interface), registry, mirae, hankook
│   ├── sheets/               # Google Sheets v4 클라이언트 래퍼 (레이트리밋/재시도)
│   ├── writer/               # 시트 생성/중복필터/삽입/색상/포맷/필터
│   ├── summary/              # 월별·종목별·대시보드차트·인사이트·섹터집계
│   ├── symbol/               # KRX .mst → 종목코드 (fwf + cp949 직접 파싱)
│   └── sector/               # OpenAI 섹터 분류기 + JSON 캐시
└── testdata/                 # CSV 픽스처 + 골든 출력
```

재작성 기간 동안 **Python 파일은 그대로 둔다**(동등성 비교 oracle). 마지막 단계에서
Python 일체 제거. Go/Python은 한 저장소에 파일이 겹치지 않아 공존 가능.

### 모듈 매핑 (Python → Go)
| Python | Go 패키지 | 비고 |
|---|---|---|
| `main.py` | `cmd/atj` | asyncio→goroutine. Sheets 호출은 대부분 순차+배치 유지 |
| `modules/models.py` | `internal/model` | dataclass→struct |
| `modules/parsers/*` | `internal/parser` | cp949 네이티브 디코딩 (iconv 사전단계 제거) |
| `modules/parser_registry.py` | `internal/parser` | 헤더 기반 detect |
| `modules/google_sheets_client.py` | `internal/sheets` | **#57 레이트리밋 최적화(쓰기 77→28요청) 보존** |
| `modules/sheet_writer.py` | `internal/writer` | 8색 팔레트·날짜별 색상·TEXT@ 종목코드·마이그레이션 경고 보존 |
| `modules/summary_generator.py` | `internal/summary` | 차트·인사이트·섹터집계 전부 |
| `modules/symbol_master.py` | `internal/symbol` | `~/.cache/auto-trading-journal` 7일 캐시 동일 |
| `modules/sector_classifier.py` | `internal/sector` | go-openai, JSON mode, temperature 0, 캐시 포맷 동일 |

### 기술 스택
| 용도 | 선택 | 근거 |
|---|---|---|
| Sheets | `google.golang.org/api/sheets/v4` | 패리티 증명됨 |
| 인증 | `option.WithCredentialsFile` (서비스계정 JSON) | 동일 |
| YAML config | `gopkg.in/yaml.v3` | 표준 |
| CSV + 인코딩 | `encoding/csv` + `golang.org/x/text/encoding/korean`(cp949) | iconv 사전단계 제거 |
| OpenAI | `github.com/sashabaranov/go-openai` | JSON mode·temperature 지원, 커뮤니티 표준 |
| 테스트 | `testify` | opendart-go·fmp-go와 동일 관례 |
| KRX `.mst` | 직접 포팅 (fwf + cp949) | SDK `internal/krxmaster`는 import 불가 → 레이아웃만 참조 |

## 데이터 모델

Trade struct는 현재 dataclass 16필드를 그대로 옮긴다: date, trade_type, stock_name,
stock_code, quantity, price, amount, currency, exchange_rate, amount_krw, fee, tax,
profit, profit_krw, profit_rate, account. 메서드: `ToDomesticRow()`(10컬럼),
`ToForeignRow()`(15컬럼), `ToSheetRow()`, `DuplicateKey()`.

수익률은 시트 표현과 동일하게 퍼센트 소수로 변환(14.68 → 0.1468). 중복키는
`(date, trade_type, stock_name, quantity, price)` — 숫자는 문자열 정규화.

## 설정 / 비밀

기존 env 그대로 사용:
- `GOOGLE_SPREADSHEET_ID`, `SERVICE_ACCOUNT_PATH`
- `STOCK_DATA_OPENAI_API_KEY` (미설정 시 섹터 분류 비활성화 — 현재 동작 동일)

config.yaml 스키마 동일 (google_sheets, logging, openai, batch_size 등).

## 진행 순서 (단계별 + 동등성 검증)

각 단계마다 동일 `input/`으로 Python·Go를 실행하고 결과(시트 또는 dry-run 출력)를
대조한다. Python 검증본이 oracle.

1. **스캐폴딩** — go.mod, `internal/config`, `internal/model` + 단위테스트
2. **파서 + 레지스트리** — CSV→Trade, cp949, 골든 픽스처 비교
3. **Sheets 클라이언트 + writer** — 거래 시트 쓰기까지 (배치/색상/포맷/중복필터/레이트리밋)
4. **summary** — 요약_월별·종목별 + 대시보드 차트 + 인사이트 + 섹터별 투자비중(OpenAI)
5. **main 통합 + 최종 동등성 검증 + Python 제거 + 문서/CLAUDE.md 갱신**

## 테스트 전략

- 파서/모델/요약포맷/레이트리밋 등 기존 테스트(~2,000줄)를 Go testify로 포팅
- CSV 픽스처 + 골든 출력을 `testdata/`에 둠
- 각 단계의 동등성 검증을 통해 회귀 방지

## 위험 / 완화

| 위험 | 완화 |
|---|---|
| Sheets 동작 비대응 | 스파이크로 사전 증명 완료 (차트 포함) |
| 레이트리밋 회귀 | #57 배치 최적화(77→28) 로직을 명시적으로 보존·검증 |
| `.mst` fwf 파싱 오차 | SDK krxmaster.go + Python symbol_master.py 양쪽을 레이아웃 레퍼런스로 |
| OpenAI 응답 포맷 차이 | JSON mode + 유효 섹터 필터링 + 누락 종목 "기타" 처리(현재 로직 동일) |
| 포팅 중 회귀 | 단계별 Python 대조 (oracle) |
