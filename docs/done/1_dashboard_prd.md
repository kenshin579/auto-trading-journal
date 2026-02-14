# 대시보드 데이터 소스 변경 PRD

## 1. 배경

### 현재 동작 방식

대시보드는 `main.py`의 실행 흐름에서 CSV 파일을 파싱한 인메모리 `Trade` 객체 리스트(`all_trades`)를 기반으로 생성된다.

```
CSV 파일 스캔 → 파서 감지 → Trade 객체 파싱 → 시트에 삽입
                                                    ↓
                                    all_trades (인메모리) → 대시보드 생성
```

**`main.py:191-194` 핵심 코드:**
```python
if all_trades and not self.dry_run:
    await self.summary_generator.generate_all(all_trades)
```

### 문제점

| 문제 | 설명 |
|------|------|
| CSV 파일 기반 대시보드 | 대시보드가 `input/` 디렉토리의 CSV 파일에서 파싱한 데이터만 반영 |
| 파일 1개일 때 부분 대시보드 | CSV 파일이 1개만 있으면 해당 파일의 거래만으로 대시보드가 생성됨 |
| 누적 데이터 미반영 | 이전 실행에서 시트에 이미 삽입된 거래 데이터가 대시보드에 포함되지 않음 |
| CSV 삭제 시 데이터 손실 | 처리 완료 후 CSV를 삭제하면 다음 실행 시 대시보드에서 해당 데이터가 사라짐 |

**예시 시나리오:**
1. 1차 실행: `미래에셋증권/국내계좌.csv` + `미래에셋증권/해외계좌.csv` → 대시보드에 2개 계좌 데이터 반영
2. 2차 실행: `한국투자증권/국내계좌.csv`만 추가 → 대시보드에 한국투자증권 데이터만 반영 (미래에셋 데이터 누락)

### 기대 동작

대시보드는 **매매일지 시트에 이미 저장된 전체 데이터**를 기준으로 생성되어야 한다. CSV 파일의 존재 여부와 무관하게, 시트에 축적된 모든 거래 이력이 대시보드에 반영되어야 한다.

```
CSV 파일 스캔 → 파서 감지 → Trade 객체 파싱 → 시트에 삽입
                                                    ↓
                                    매매일지 시트에서 전체 데이터 읽기 → 대시보드 생성
```

## 2. 요구사항

### 2.1 매매일지 시트에서 Trade 데이터 읽기

모든 매매일지 시트의 데이터를 읽어 `Trade` 객체 리스트로 변환하는 기능을 구현한다.

- **매매일지 시트 판별 (헤더 행 검증 방식)**:
  - 각 시트의 1행(헤더)을 읽어 `DOMESTIC_HEADERS` 또는 `FOREIGN_HEADERS`와 일치하는지 확인
  - 일치하면 매매일지 시트로 인식, 불일치하면 스킵
  - `대시보드` 시트 등 관계없는 시트가 있어도 안전하게 무시됨
- **시트 유형 판별**: 헤더가 `FOREIGN_HEADERS`와 일치하면 해외계좌, `DOMESTIC_HEADERS`와 일치하면 국내계좌

```python
# sheet_writer.py에 정의된 헤더 상수로 판별
DOMESTIC_HEADERS = ["일자", "구분", "종목명", "수량", "단가", "금액", "수수료", "손익금액", "수익률(%)"]
FOREIGN_HEADERS = ["일자", "구분", "통화", "종목코드", "종목명", "수량", "단가", ...]
```
- **데이터 범위**: 각 시트의 2행부터 마지막 데이터 행까지 (1행은 헤더)
- **변환 규칙**:

#### 국내계좌 시트 (9컬럼) → Trade 매핑

| 시트 컬럼 | Trade 필드 | 변환 |
|-----------|-----------|------|
| A: 일자 | date | 그대로 (YYYY-MM-DD) |
| B: 구분 | trade_type | 그대로 (매수/매도) |
| C: 종목명 | stock_name | 그대로 |
| D: 수량 | quantity | float |
| E: 단가 | price | float |
| F: 금액 | amount, amount_krw | float (국내는 동일) |
| G: 수수료 | fee | float |
| H: 손익금액 | profit, profit_krw | float (국내는 동일) |
| I: 수익률(%) | profit_rate | 퍼센트 소수 → 백분율 (0.1468 → 14.68) |
| - | currency | `"KRW"` 고정 |
| - | exchange_rate | `1.0` 고정 |
| - | stock_code | `""` (국내 시트에 코드 없음) |
| - | tax | `0.0` 고정 |
| - | account | 시트 이름 (예: `미래에셋증권_국내계좌`) |

#### 해외계좌 시트 (15컬럼) → Trade 매핑

| 시트 컬럼 | Trade 필드 | 변환 |
|-----------|-----------|------|
| A: 일자 | date | 그대로 |
| B: 구분 | trade_type | 그대로 |
| C: 통화 | currency | 그대로 |
| D: 종목코드 | stock_code | 그대로 |
| E: 종목명 | stock_name | 그대로 |
| F: 수량 | quantity | float |
| G: 단가 | price | float |
| H: 금액(외화) | amount | float |
| I: 환율 | exchange_rate | float |
| J: 금액(원화) | amount_krw | float |
| K: 수수료 | fee | float |
| L: 세금 | tax | float |
| M: 손익(외화) | profit | float |
| N: 손익(원화) | profit_krw | float |
| O: 수익률(%) | profit_rate | 퍼센트 소수 → 백분율 (0.1468 → 14.68) |
| - | account | 시트 이름 (예: `미래에셋증권_해외계좌`) |

### 2.2 대시보드 데이터 소스 변경

대시보드 생성 시 인메모리 `all_trades` 대신, 매매일지 시트에서 읽은 전체 Trade 데이터를 사용한다.

- **기존**: `summary_generator.generate_all(all_trades)` (CSV 파싱 결과)
- **변경**: 매매일지 시트에서 전체 데이터를 읽어 `generate_all()`에 전달
- **대시보드 갱신 조건**: CSV 파일 유무와 관계없이, 매매일지 시트에 데이터가 있으면 대시보드를 갱신

### 2.3 기존 기능 유지

- CSV → 시트 삽입 파이프라인은 변경 없음
- 중복 필터링 로직은 변경 없음
- 대시보드의 4개 섹션(포트폴리오 요약, 월별 성과, 종목별 현황, 투자 지표) 구성은 변경 없음
- dry-run 모드 동작은 변경 없음

## 3. 기술 변경 사항

### 3.1 sheet_writer.py - 신규 메서드

| 메서드 | 용도 |
|--------|------|
| `read_all_trades()` | 모든 매매일지 시트에서 Trade 리스트를 읽어 반환 |
| `_read_trades_from_sheet(sheet_name, is_foreign)` | 개별 시트에서 Trade 리스트 변환 |

**`read_all_trades()` 동작:**
1. `list_sheets()`로 전체 시트 목록 조회
2. 각 시트의 1행(헤더)을 읽어 `DOMESTIC_HEADERS` / `FOREIGN_HEADERS`와 비교
3. 헤더가 일치하는 시트만 매매일지로 인식 (대시보드 등 기타 시트는 자동 스킵)
4. 헤더 종류로 국내/해외 판별
5. 각 시트에서 `get_raw_grid_data()`로 데이터 조회
6. 행 데이터를 `Trade` 객체로 변환
7. 전체 Trade 리스트 반환

**데이터 읽기 방식:**
- `get_raw_grid_data()`를 사용하여 `effectiveValue` + `formattedValue` 동시 조회
- 날짜는 `formattedValue` 사용 (시리얼 넘버 → "YYYY-MM-DD" 변환 방지)
- 숫자는 `effectiveValue.numberValue` 사용 (원본 정밀도 유지)

### 3.2 main.py - 변경 사항

**기존 코드 (`main.py:191-196`):**
```python
if all_trades and not self.dry_run:
    await self.summary_generator.generate_all(all_trades)
```

**변경 코드:**
```python
if not self.dry_run:
    all_trades_from_sheets = await self.sheet_writer.read_all_trades()
    if all_trades_from_sheets:
        await self.summary_generator.generate_all(all_trades_from_sheets)
```

### 3.3 summary_generator.py - 변경 없음

`generate_all(all_trades)` 인터페이스는 동일. 입력 데이터 소스만 변경.

## 4. 작업 목록

### Phase 1: 시트 데이터 읽기 구현

1. `sheet_writer.py`에 `_read_trades_from_sheet(sheet_name, is_foreign)` 구현
   - `get_raw_grid_data()`로 시트 데이터 조회
   - 행 데이터 → Trade 객체 변환 로직
   - 빈 행, 불완전한 행 스킵 처리
2. `sheet_writer.py`에 `read_all_trades()` 구현
   - 전체 시트 목록에서 대시보드 제외
   - 국내/해외 판별 후 개별 시트 읽기
   - 전체 Trade 리스트 반환

### Phase 2: 대시보드 데이터 소스 변경

3. `main.py`에서 대시보드 생성 로직 변경
   - `all_trades` (인메모리) 대신 `read_all_trades()` (시트) 사용
   - CSV 파일 유무와 관계없이 대시보드 갱신

### Phase 3: 테스트

4. `_read_trades_from_sheet()` 단위 테스트 추가
   - 국내계좌 시트 → Trade 변환 검증
   - 해외계좌 시트 → Trade 변환 검증
   - 빈 시트, 불완전한 행 처리 검증
5. 통합 테스트 (dry-run)
   - CSV 1개만 있을 때 대시보드에 전체 시트 데이터 반영 확인
   - CSV 없이 실행 시 기존 시트 데이터로 대시보드 갱신 확인
