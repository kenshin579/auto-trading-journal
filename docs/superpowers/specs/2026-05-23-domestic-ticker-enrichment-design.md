# 국내 종목 티커(종목코드) 추가 설계

- 작성일: 2026-05-23
- 브랜치: `feature/domestic-ticker-enrichment`

## 배경 / 목표

매매일지의 국내계좌 시트에 종목명만 있고 종목코드(티커)가 없다. 특히 **미래에셋증권 국내 CSV에는 종목코드 컬럼 자체가 없어** 파싱 단계에서 `stock_code=""`로 하드코딩된다(한국투자증권 국내 CSV는 종목코드 보유). 국내 거래에도 종목코드를 채워 시트에 별도 컬럼으로 표시한다.

## 핵심 결정 사항

| 항목 | 결정 | 이유 |
|------|------|------|
| 데이터 출처 | **KRX 공개 종목 마스터 직접 파싱** (Python 유지) | Go 전면 재작성(과대 비용)·moneyflow API 런타임 의존(취약) 회피. KIS SDK도 결국 이 마스터를 파싱하므로 매칭 품질은 동일(정본 KRX명)하면서 작업량은 최소·오프라인·무인증 |
| 표시 방법 | **종목코드 별도 컬럼 신규 추가** | 데이터 분리가 깔끔, 종목명 기반 중복키 그대로 유지 |
| 기존 시트 마이그레이션 | **수동 1회 처리** (사용자가 기존 국내 시트 삭제 후 재생성) | 자동 마이그레이션 로직 복잡도/위험 회피. 코드는 신규 10컬럼 포맷만 가정 |

### 대안 비교 (기록용)

- **1안 Go 전면 재작성 + KIS SDK**: 매칭 품질 최상이나 검증된 Python 앱(파서 3종·GSheets·요약·async)을 통째로 재작성 — 비용 과다. 채택 안 함.
- **2안 Python + moneyflow `/stock/search`**: 작업량 최소이나 moneyflow 서버 가동·네트워크·`symbols` 테이블 적재 상태에 런타임 의존 — 결합도/취약성 문제. 채택 안 함.
- **3안 Python + KRX 마스터 직접 파싱**: 채택. 1안의 매칭 품질을 2안 수준 작업량으로 확보.

## 데이터 출처 상세 — KRX 종목 마스터

KIS SDK(`korea-investment-stock/internal/krxmaster`)가 사용하는 것과 동일한 공개 파일을 Python에서 직접 처리한다.

- URL
  - KOSPI: `https://new.real.download.dws.co.kr/common/master/kospi_code.mst.zip`
  - KOSDAQ: `https://new.real.download.dws.co.kr/common/master/kosdaq_code.mst.zip`
- 형식: ZIP 내부 `.mst` 파일, **cp949 인코딩 + fixed-width**. 토큰 인증 불필요.
- 한 행 구조(앞부분): `단축코드[0:9]` + `표준코드(ISIN)[9:21]` + `한글명[21 : len-FWF]`
  - FWF 꼬리 길이: **KOSPI 227 byte, KOSDAQ 221 byte**
- 본 기능은 **이름→코드 매핑만** 필요 → fwf 70/64컬럼 전체 파싱 불필요. `code = line[0:9].strip()`, `name = line[21 : len(line)-fwf_len].strip()`만 추출.
- ETF는 KOSPI 마스터에 포함(그룹코드 `EF`). 개별주·KOSDAQ 종목 보강을 위해 두 마스터 모두 로드.

## 아키텍처

### 신규 모듈: `modules/symbol_master.py`

종목명→종목코드 리졸버. 책임:

1. KRX 마스터(KOSPI/KOSDAQ) 다운로드 → ZIP 해제 → cp949 디코드 → `{한글명: 단축코드}` dict 구성
2. **로컬 디스크 캐시** (예: `~/.cache/auto-trading-journal/{kospi,kosdaq}_code.mst.zip`, TTL 7일). 캐시 유효 시 네트워크 호출 생략, 만료/없음일 때만 다운로드. 다운로드 실패 시 만료 캐시라도 사용(graceful degrade)
3. `resolve(stock_name: str) -> str`: 정확 매칭. 미발견 시 `""` 반환 + 1회 경고 로그
4. 이름 충돌(동일명 다중 코드)은 드묾 — 발생 시 경고 후 첫 항목 사용(향후 필요하면 시장 우선순위 규칙 추가)

인터페이스:
- 입력: 종목명 문자열
- 출력: 6자리 단축코드 문자열(또는 `""`)
- 의존: 네트워크(최초/캐시 만료 시), 로컬 캐시 파일

### 파이프라인 enrichment 훅

`StockDataProcessor`(main.py)에서 **파싱 직후·시트 삽입 전** 보강 단계 추가:

- 대상: **국내 거래 중 `stock_code == ""`** 인 항목만 (`Trade.is_domestic()`)
- 한국투자 국내처럼 CSV에 이미 코드가 있으면 그대로 둔다(보강하지 않음)
- 리졸버를 1회 초기화(마스터 1회 로드)해 전체 거래에 재사용
- 파서에 비종속 — 미래에셋·향후 다른 증권사 국내 파서 모두 자동 적용

## 시트 컬럼 변경 (국내 전용)

기존 9컬럼 → **10컬럼**, 종목명 뒤에 `종목코드` 삽입:

```
일자 | 구분 | 종목명 | 종목코드 | 수량 | 단가 | 금액 | 수수료 | 손익금액 | 수익률(%)
```

해외 시트(`종목코드` 기보유)는 변경 없음.

### 영향 코드 (컬럼 인덱스 시프트)

| 위치 | 변경 |
|------|------|
| `modules/models.py` `to_domestic_row()` | `stock_code`를 `stock_name` 뒤에 삽입 (9→10 항목) |
| `modules/sheet_writer.py` `DOMESTIC_HEADERS` | `"종목코드"`를 `"종목명"` 뒤에 추가 |
| `sheet_writer.py` `_row_to_trade()` 국내 분기 | 인덱스 시프트(새 헤더 순서): `stock_name=values[2]`, `stock_code=values[3]`(기존 `""` 하드코딩 제거), `quantity=values[4]`, `price=values[5]`, `amount=values[6]`, `fee=values[7]`, `profit=values[8]`, `rate=values[9]*100` |
| `sheet_writer.py` `get_existing_keys()` 국내 | 수량/단가 컬럼 인덱스 3,4 → 4,5 (중복키 값 자체는 종목명 기반이라 불변) |
| `sheet_writer.py` `_read_trades_from_sheet()` | 국내 `min_cols` 9 → 10 |
| 색상/숫자포맷 적용 | 국내 수익률 등 대상 컬럼 인덱스 +1 시프트 |
| `detect_account_type()` | 새 `DOMESTIC_HEADERS`와 비교. **옛 9컬럼 헤더 감지 시 명확한 경고/에러 로그**로 수동 마이그레이션 유도 |

**중복키 불변**: `duplicate_key()`는 `(date, trade_type, stock_name, quantity, price)` 값 기반이므로 종목코드 컬럼 추가가 중복 판정에 영향 없음.

### 요약 시트

`summary_generator.generate_stock_summary()`는 이미 `(종목명, 종목코드, 계좌, 통화)`로 집계 → 국내 거래에 실제 코드가 채워지면서 자연히 개선됨. 추가 변경 불필요(검증만).

## 마이그레이션 (수동 1회)

- 사용자가 기존 국내 시트(`미래에셋증권_국내계좌` 등)를 **삭제** 후 재실행 → 신규 10컬럼 헤더로 재생성. 또는 D열에 `종목코드` 컬럼을 수동 삽입.
- 코드는 신규 10컬럼 포맷만 가정. 옛 9컬럼 헤더를 만나면 read-back을 시도하지 않고 경고 로그로 사용자에게 마이그레이션 필요를 알린다(데이터 깨짐 방지 안전망).

## 테스트 전략

- `tests/test_symbol_master.py` (신규)
  - 소형 `.mst` fixture(cp949 fwf 모사)로 이름→코드 파싱 검증
  - 캐시 TTL: 유효 캐시 시 네트워크 미호출, 만료 시 재다운로드(다운로드 함수 mock)
  - 미발견 종목 → `""` 반환
- `tests/test_parsers.py` / 시트 관련 테스트
  - 국내 행 변환·헤더·read-back 인덱스 10컬럼 기준으로 갱신
  - enrichment: 빈 코드 국내 거래만 채워지고 한국투자(기보유) 코드는 불변임을 검증
- 샘플(`sample/미래에셋증권/국내계좌.csv`) 종목명이 KRX 정본명과 매칭되는지 통합 확인

## 범위 밖 (YAGNI)

- moneyflow API 연동, Go 재작성
- 기존 시트 자동 마이그레이션
- 이름→코드 fuzzy 매칭(정확 매칭만; 미발견은 빈 코드로 두고 로그)
