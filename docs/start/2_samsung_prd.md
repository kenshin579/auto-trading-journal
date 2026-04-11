# 삼성증권 CSV 파서 추가 - PRD

## 1. 배경

`input/삼성증권/` 디렉토리에 삼성증권 국내계좌 CSV 파일이 추가되었다.
기존 미래에셋증권/한국투자증권 CSV와 포맷이 상이하여, 새로운 파서를 구현해야 한다.

## 2. 삼성증권 CSV 분석

### 2.1 파일 특성

| 항목 | 삼성증권 | 미래에셋증권 (국내) | 한국투자증권 |
|------|----------|---------------------|-------------|
| 인코딩 | **EUC-KR** | UTF-8 | UTF-8 |
| 메타데이터 | 4줄 (계좌정보, 조회기간 등) | 없음 | 없음 |
| 헤더 키워드 | `거래일자`, `거래명`, `종목명` | `일자`, `종목명`, `기간 중 매수` | `매매일자`, `종목코드`, `매입단가` |
| 컬럼 수 | 9 | 11 | 17 |
| 매매 구분 | `거래명` 컬럼에 매수/매도 명시 | 매수/매도 수량이 한 행에 공존 | 매수수량/매도수량 분리 |
| 종목코드 | 없음 | 없음 | 있음 |
| 손익 데이터 | 없음 | 있음 (손익금액, 수익률) | 있음 (실현손익, 손익률) |
| 페이지네이션 | 있음 (`1/2`, `2/2`) | 없음 | 없음 |
| 날짜 포맷 | `YYYY-MM-DD` (하이픈) | `YYYY/MM/DD` (슬래시) | `YYYY/MM/DD` (슬래시) |
| 숫자 포맷 | 쌍따옴표+쉼표 (`"20,360"`) | 쉼표 없음 | 쌍따옴표+쉼표 |

### 2.2 CSV 구조 상세

```
계좌거래내역                                          ← 메타 1줄: 제목
[2026-04-11 16:07:13]오용경()                         ← 메타 2줄: 타임스탬프
계좌번호,7113****-01 종합(평생혜택),고 객 명,오용경      ← 메타 3줄: 계좌정보
조회구분,매매,조회일자,2026/03/11 ~ 2026/04/11         ← 메타 4줄: 조회조건
거래일자,거래명,종목명,수 량,단 가,거래금액,수수료,제세금,처리점  ← 헤더
2026-04-08,매수,KODEX 미국AI전력핵심인프라,1,"20,360","20,360",1,,디지털업무팀
...
1/2                                                   ← 페이지 구분자
거래일자,거래명,종목명,수 량,단 가,거래금액,수수료,제세금,처리점  ← 헤더 반복
2026-04-08,매수,PLUS 고배당주,1,"25,165","25,165",,,디지털업무팀
...
- 출력 끝 -                                           ← 종료 마커
2/2                                                   ← 마지막 페이지 구분자
```

### 2.3 데이터 컬럼 매핑

| CSV 컬럼 (index) | 설명 | Trade 필드 매핑 |
|-------------------|------|-----------------|
| 거래일자 (0) | `YYYY-MM-DD` | `date` (변환 불필요) |
| 거래명 (1) | 매수/매도 | `trade_type` |
| 종목명 (2) | 종목명 | `stock_name` |
| 수 량 (3) | 수량 | `quantity` |
| 단 가 (4) | 단가 (쌍따옴표+쉼표) | `price` |
| 거래금액 (5) | 거래금액 (쌍따옴표+쉼표) | `amount`, `amount_krw` |
| 수수료 (6) | 수수료 | `fee` |
| 제세금 (7) | 세금 | `tax` |
| 처리점 (8) | 처리 지점명 (사용 안 함) | - |

### 2.4 미래에셋증권과의 핵심 차이점

1. **인코딩: EUC-KR** - 기존 파서/레지스트리는 모두 `encoding="utf-8"`로 파일을 연다. 삼성증권 CSV는 EUC-KR이므로 인코딩 자동 감지 또는 fallback 로직이 필요하다.
2. **메타데이터 4줄** - 헤더 행이 5번째 줄에 위치한다. 메타데이터를 건너뛰는 로직이 필요하다.
3. **페이지네이션** - 데이터 중간에 페이지 구분자(`1/2`)와 헤더 반복이 존재한다. 이를 무시하고 데이터 행만 추출해야 한다.
4. **종료 마커** - `- 출력 끝 -` 행을 무시해야 한다.
5. **거래명 컬럼** - 매수/매도가 별도 컬럼이 아니라 `거래명` 컬럼 값으로 구분된다.
6. **손익 데이터 없음** - 수수료/제세금만 있고, 손익금액/수익률은 CSV에 포함되지 않는다.

## 3. 구현 범위

### 3.1 신규 파일

| 파일 | 설명 |
|------|------|
| `modules/parsers/samsung_parser.py` | 삼성증권 국내계좌 파서 (`SamsungDomesticParser`) |

### 3.2 수정 파일

| 파일 | 변경 내용 |
|------|----------|
| `modules/parser_registry.py` | 인코딩 자동 감지 로직 추가 + `SamsungDomesticParser` 등록 |
| `modules/parsers/__init__.py` | `SamsungDomesticParser` export 추가 |
| `tests/test_parsers.py` | 삼성증권 파서 테스트 케이스 추가 |

### 3.3 CLAUDE.md 업데이트

파서 등록 목록과 Architecture 섹션에 삼성증권 파서 추가

## 4. 상세 설계

### 4.1 인코딩 처리 (`parser_registry.py`)

현재 `detect_parser()`는 `encoding="utf-8"`로 고정되어 있다. 삼성증권 CSV는 EUC-KR이므로 UTF-8로 열면 실패한다.

**방안**: UTF-8 → EUC-KR 순서로 fallback 시도

```python
def _read_header(file_path: Path) -> List[str]:
    """CSV 첫 번째 행을 읽어 헤더 반환 (인코딩 자동 감지)"""
    for encoding in ["utf-8", "euc-kr"]:
        try:
            with open(file_path, "r", encoding=encoding) as f:
                reader = csv.reader(f)
                header = next(reader)
            return header, encoding
        except (UnicodeDecodeError, StopIteration):
            continue
    raise ValueError(f"CSV 인코딩 감지 실패: {file_path}")
```

- 감지된 인코딩 정보를 파서의 `parse()` 메서드에 전달하거나, 파서 내부에서도 동일한 fallback 로직을 적용해야 한다.
- `detect_parser()`에서 메타데이터 행(제목, 계좌정보 등)이 헤더로 읽힐 수 있으므로, 헤더 매칭 실패 시 다음 행들도 확인하는 로직이 필요하다.

### 4.2 삼성증권 파서 (`samsung_parser.py`)

```python
class SamsungDomesticParser(BaseParser):
    """삼성증권 국내계좌 파서

    CSV 구조 (9컬럼):
        메타데이터 4줄 건너뜀
        col 0: 거래일자 (YYYY-MM-DD)
        col 1: 거래명 (매수/매도)
        col 2: 종목명
        col 3: 수 량
        col 4: 단 가 (쌍따옴표+쉼표)
        col 5: 거래금액 (쌍따옴표+쉼표)
        col 6: 수수료
        col 7: 제세금
        col 8: 처리점

    특이사항:
        - 인코딩: EUC-KR
        - 메타데이터 4줄 + 헤더 1줄 = 5줄 건너뜀
        - 페이지 구분자 (N/M), 반복 헤더, 종료 마커 무시
        - 손익 데이터 없음 (profit=0, profit_rate=0)
    """
```

**스킵해야 할 행 패턴**:
- 메타데이터 (처음 4줄)
- 헤더 행 (`거래일자,거래명,...`)
- 페이지 구분자 (`숫자/숫자` 패턴, 예: `1/2`)
- 종료 마커 (`- 출력 끝 -`)
- 빈 행

**Trade 매핑**:

| Trade 필드 | 값 |
|------------|------|
| `date` | col 0 그대로 (이미 `YYYY-MM-DD`) |
| `trade_type` | col 1 (`매수` / `매도`) |
| `stock_name` | col 2 |
| `stock_code` | `""` (CSV에 없음) |
| `quantity` | col 3 |
| `price` | col 4 (쉼표/따옴표 제거) |
| `amount` | col 5 (쉼표/따옴표 제거) |
| `currency` | `"KRW"` |
| `exchange_rate` | `1.0` |
| `amount_krw` | col 5와 동일 |
| `fee` | col 6 (매수: 수수료만, 매도: 수수료) |
| `tax` | col 7 (매도 시 제세금) |
| `profit` | `0.0` (CSV에 없음) |
| `profit_krw` | `0.0` |
| `profit_rate` | `0.0` |
| `account` | `"삼성증권_{파일명}"` |

### 4.3 헤더 감지 (`can_parse`)

삼성증권 CSV는 메타데이터 4줄 뒤에 헤더가 있으므로, `detect_parser()`에서 첫 행만 읽으면 메타데이터(`계좌거래내역`)가 반환된다.

**방안**: `detect_parser()`에서 첫 N행까지 순회하며 파서 매칭 시도

```python
# 최대 10행까지 읽어서 헤더 찾기
for row in itertools.islice(reader, 10):
    header_clean = [h.strip().strip('"') for h in row]
    for parser_cls in PARSERS:
        if parser_cls.can_parse(header_clean):
            return parser_cls()
```

**can_parse 키워드**: `{"거래일자", "거래명", "종목명"}`

### 4.4 테스트 케이스

```python
class TestSamsungDomesticParser:
    """삼성증권 국내계좌 파서 테스트"""

    # 1. can_parse - 헤더 인식
    # 2. can_parse - 다른 증권사 헤더 거부
    # 3. 메타데이터 4줄 건너뜀
    # 4. 페이지 구분자/반복 헤더 건너뜀
    # 5. 종료 마커 무시
    # 6. 매수 거래 파싱 (trade_type, quantity, price, amount)
    # 7. 매도 거래 파싱 (현재 샘플에 매도 없으나, 향후 대비)
    # 8. 수수료/제세금 파싱
    # 9. 쌍따옴표+쉼표 숫자 처리 ("20,360" → 20360.0)
    # 10. 날짜 포맷 확인 (이미 YYYY-MM-DD)
    # 11. EUC-KR 인코딩 처리
    # 12. 빈 행 건너뜀
    # 13. to_domestic_row() 9컬럼 반환
```

## 5. 작업 목록

### Task 1: 인코딩 자동 감지 (`parser_registry.py`)
- `detect_parser()`에서 UTF-8 → EUC-KR fallback 인코딩 감지
- 메타데이터를 건너뛰고 최대 10행까지 헤더 매칭 시도
- 감지된 인코딩을 파서에 전달하는 방법 결정

### Task 2: 삼성증권 파서 구현 (`samsung_parser.py`)
- `SamsungDomesticParser` 클래스 작성
- EUC-KR 인코딩으로 파일 열기
- 메타데이터 4줄 + 헤더 1줄 건너뛰기
- 페이지 구분자, 반복 헤더, 종료 마커 무시
- 쌍따옴표+쉼표 숫자 파싱
- Trade 객체 생성 (손익 데이터는 0으로 설정)

### Task 3: 파서 등록
- `modules/parsers/__init__.py`에 export 추가
- `modules/parser_registry.py`의 `PARSERS` 리스트에 등록

### Task 4: 테스트 작성 (`tests/test_parsers.py`)
- `TestSamsungDomesticParser` 클래스 추가
- 샘플 CSV 기반 테스트 (EUC-KR 인코딩)
- 엣지 케이스 테스트 (페이지네이션, 빈 행, 종료 마커)

### Task 5: CLAUDE.md 업데이트
- Architecture 섹션에 `SamsungDomesticParser` 추가
- CSV 포맷 설명에 삼성증권 추가

## 6. 리스크 및 고려사항

### 6.1 인코딩 변경의 영향 범위
`parser_registry.py`의 인코딩 감지 로직 변경은 기존 미래에셋/한국투자증권 파서에도 영향을 준다. UTF-8을 먼저 시도하므로 기존 파서 동작에는 영향이 없어야 하지만, 기존 테스트를 반드시 재실행해야 한다.

### 6.2 삼성증권 매도 데이터 미확인
현재 샘플 CSV에는 매수 거래만 포함되어 있다. 매도 시 CSV 포맷이 동일한지 확인이 필요하다 (향후 매도 데이터 포함된 CSV로 검증 필요).

### 6.3 손익 데이터 부재
삼성증권 CSV에는 손익금액/수익률이 없다. 이로 인해 요약 시트의 실현손익 집계에서 삼성증권 데이터는 항상 0으로 표시된다. 향후 매도 데이터가 추가되면 별도의 손익 계산 로직이 필요할 수 있다.

### 6.4 페이지네이션 패턴 일반화
현재 샘플은 2페이지(`1/2`, `2/2`)이지만, 거래가 많으면 N페이지까지 가능하다. 정규식 `^\d+/\d+$` 패턴으로 일반화해야 한다.
