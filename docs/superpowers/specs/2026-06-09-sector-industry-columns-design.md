# 매매일지 섹터/산업 열 추가 설계 (국내, KIS)

- 작성일: 2026-06-09
- 상태: 승인 대기

## 배경 / 목적

매매일지(거래 시트)의 각 종목 행에 **섹터**와 **산업** 정보를 별도 열로 추가한다.
출처는 KIS SDK(`github.com/kenshin579/korea-investment-stock`)의 국내주식 종목정보
조회 API다. 이 기능이 auto-trading-journal의 **첫 KIS SDK 의존**이며, Go 전환의
명분(워크스페이스 Go 정렬 + SDK 재사용)이 실현되는 지점이다.

## 결정사항 (브레인스토밍 합의)

- **출처**: KIS `client.Domestic.SearchStockInfo(ctx, 종목코드, "300")`
  - 섹터 = `IdxBztpLclsCdName` (지수업종 대분류명, 예 "전기·전자")
  - 산업 = `IdxBztpMclsCdName` (지수업종 중분류명)
  - 한글명 직접 제공 → 매핑 테이블/LLM 불필요
- **범위: 국내만 (MVP).** 해외는 섹터/산업 칸 공란. (해외 KIS 업종은 거래소코드(EXCD)
  추정이 필요하고 1단계·영문이라 별도 후속 작업으로 분리.)
- **컬럼 위치**: 종목명 바로 뒤 (가독성 우선) → 뒤따르는 컬럼 인덱스 재배치 필요.
- **기존 OpenAI 대시보드 섹터집계와 분리**: 대시보드 "섹터별 투자비중"(`internal/sector`,
  OpenAI, 국내+해외 GICS)은 **그대로 유지**. 신규 per-row 열은 KIS 지수업종(국내)만 사용.
  → 같은 국내 종목이 행에선 KIS 대분류("전기·전자"), 대시보드에선 GICS("IT")로 다르게
  보일 수 있으나 허용(향후 통합 시 정리).

## 아키텍처

### 신규 패키지 `internal/bizcat`
KIS 지수업종(대/중분류) 리졸버.

- `New() *Resolver` — lazy. 최초 `Resolve` 시 `kis.NewClientFromEnv()`로 KIS 클라이언트
  생성(가진 env 키: `KOREA_INVESTMENT_API_KEY` 등).
- `Resolve(code string) (sector, industry string)` — 캐시 우선, miss 시
  `Domestic.SearchStockInfo(ctx, code, "300")` 호출 → (`IdxBztpLclsCdName`,
  `IdxBztpMclsCdName`).
- **캐시**: `config/bizcat_cache.json`, `{종목코드: {"sector":..,"industry":..}}`.
  지수업종은 거의 안 바뀌므로 영구 캐시(miss 시에만 API). JSON `MarshalIndent` +
  `SetEscapeHTML(false)`로 한글 보존(기존 sector 캐시와 동일 방식).
- **회복력**: KIS 키 미설정 / 클라이언트 생성 실패 / API 오류 시 **빈 값 반환**(에러를
  전파하지 않고 로그). 매매일지 진행에 지장 없음 — `internal/symbol.Resolver` 패턴과 동일.
- go.mod에 `github.com/kenshin579/korea-investment-stock` 추가.

### 모델 (`internal/model`)
- `Trade`에 `Sector string`, `Industry string` 필드 추가.
- `ToDomesticRow()` (10→12컬럼): `일자, 구분, 종목코드, 종목명, **섹터, 산업**, 수량,
  단가, 금액, 수수료, 손익금액, 수익률(소수)`.
- `ToForeignRow()` (15→17컬럼): `일자, 구분, 통화, 종목코드, 종목명, **섹터, 산업**, 수량,
  단가, 금액(외화), 환율, 금액(원화), 수수료, 세금, 손익(외화), 손익(원화), 수익률(소수)`.
  (해외는 Sector/Industry 빈 문자열.)
- `DuplicateKey()` **불변**: (date, trade_type, stock_name, quantity, price). 섹터/산업은
  키에 포함하지 않음 → 재실행 시 중복 미발생.

### 헤더 / 마이그레이션 (`internal/writer/headers.go`)
- `DomesticHeaders` (12): `일자,구분,종목코드,종목명,섹터,산업,수량,단가,금액,수수료,손익금액,수익률(%)`
- `ForeignHeaders` (17): `일자,구분,통화,종목코드,종목명,섹터,산업,수량,단가,금액(외화),환율,금액(원화),수수료,세금,손익(외화),손익(원화),수익률(%)`
- 마이그레이션 감지용 구포맷 헤더 상수 추가:
  - `OldDomesticHeadersV2` = 현재 10컬럼(종목코드 포함, 섹터/산업 이전)
  - `OldForeignHeadersV1` = 현재 15컬럼
  - (기존 `OldDomesticHeadersV1` 9컬럼도 유지)
- `ReadAllTrades`: 헤더가 신규(12/17)면 처리, 구포맷(10/15/9)이면 **경고 로그 + 스킵**
  (자동 컬럼삽입 없음). 사용자는 시트 삭제 후 재실행 또는 수동으로 섹터/산업 컬럼 삽입.

### 컬럼 인덱스 재배치 (`internal/writer`)
종목명 뒤 2열 삽입으로 이후 컬럼이 +2 시프트.

- **`GetExistingKeys`** (중복키 추출):
  - 국내: 일자=0, 구분=1, 종목명=3 (불변), 수량 4→**6**, 단가 5→**7**
  - 해외: 일자=0, 구분=1, 종목명=4 (불변), 수량 5→**7**, 단가 6→**8**
- **`DomesticFormats`**: 수량 5→7, 단가 6→8, 금액 7→9, 수수료 8→10, 손익 9→11, 수익률 10→12
- **`ForeignFormatsCommon`**: 수량 6→8, 환율 9→11, 금액(원화) 10→12, 손익(원화) 14→16, 수익률 15→17
- **`ForeignCurrencyCols`** (단가/금액외화/수수료/세금/손익외화): `[7,8,11,12,13]` → `[9,10,13,14,15]`
- **종목코드 TEXT@ 컬럼**: 국내 3→3(종목코드 위치 불변, 종목명 앞), 해외 4→4 — 헤더 인덱스로 계산하므로 `DomesticHeaders.index("종목코드")` 기반 유지
- **`rowToTrade`** (역매핑, 대시보드 입력용): 시프트된 인덱스로 읽음. 섹터/산업 2열은
  대시보드가 쓰지 않으므로 무시(읽되 버림). 수익률 복원(*100) 등 기존 로직 유지.

### 파이프라인 (`cmd/atj`)
- `processFile`에서 `enrichDomesticCodes`(종목코드 보강) **직후**, 국내 거래(`IsDomestic()`)에
  대해 `bizcat.Resolve(code)`로 `Sector`/`Industry` 채움(코드가 있는 경우만). 해외는 스킵.
- `bizcat.Resolver`는 `StockDataProcessor` 조립 시 생성(lazy). KIS 키 없으면 Resolve가 빈
  값 반환하므로 별도 분기 불필요(로그만).

## 데이터 흐름
CSV 파싱 → 종목코드 보강(KRX .mst) → **업종 보강(KIS, 국내)** → 날짜정렬 → 시트 보장
(신규 12/17 헤더) → 중복필터(새 인덱스) → 삽입(행에 섹터/산업 포함, 새 포맷 인덱스) →
대시보드 갱신(OpenAI 섹터집계 그대로).

## 오류 처리
- KIS 키 미설정/실패 → 섹터/산업 공란, 경고 로그, 매매일지 정상 진행.
- 캐시 파일 손상 → 무시하고 빈 캐시로 시작(기존 sector 캐시와 동일).
- 구포맷 시트 → 경고 후 스킵(데이터 손상 방지).

## 테스트
- `bizcat`: 캐시 hit(네트워크 미접촉)·miss·빈 값 폴백, 캐시 JSON 한글 보존. (live API 테스트 없음)
- `model`: ToDomesticRow 12컬럼·ToForeignRow 17컬럼 위치, 해외 섹터/산업 공란, DupKey 불변.
- `writer`: 새 헤더 상수 길이, GetExistingKeys 새 인덱스로 키 추출, rowToTrade 새 인덱스 역매핑, 구포맷(10/15) 스킵.
- `cmd`: 국내만 업종 보강(stub resolver), 해외 미보강.

## 범위 밖
- 해외 종목 섹터/산업 (EXCD 추정 + EIcod) — 후속.
- 대시보드 섹터집계를 KIS로 통합 — 후속.
- 기존 운영 시트 자동 마이그레이션(컬럼 자동 삽입) — 수동/재생성.

## 위험 / 완화
| 위험 | 완화 |
|---|---|
| 인덱스 재배치 누락 → 데이터 어긋남 | 중복키/포맷/rowToTrade 각각 테스트로 새 인덱스 고정 |
| 운영 시트 구포맷 | 경고+스킵으로 손상 방지, 사용자 재생성 안내 |
| KIS API 레이트리밋 | 영구 캐시로 distinct 종목코드만 1회 호출 |
| SDK 첫 의존 빌드 | go.mod 추가 후 `go build ./...` 검증 |
