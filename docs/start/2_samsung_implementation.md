# 삼성증권 CSV 파서 추가 - 구현 문서

## 1. parser_registry.py 수정 — 인코딩 자동 감지 + 다중 행 헤더 탐색

### 1.1 인코딩 fallback

현재 `detect_parser()`는 `encoding="utf-8"` 고정. EUC-KR 파일을 위해 fallback 추가.

```python
ENCODINGS = ["utf-8", "euc-kr"]

def _open_csv(file_path: Path):
    """인코딩을 자동 감지하여 파일을 열고 (file_handle, encoding) 반환"""
    for enc in ENCODINGS:
        try:
            f = open(file_path, "r", encoding=enc)
            f.readline()  # 디코딩 가능한지 확인
            f.seek(0)
            return f, enc
        except UnicodeDecodeError:
            continue
    raise ValueError(f"CSV 인코딩 감지 실패: {file_path}")
```

### 1.2 다중 행 헤더 탐색

삼성증권 CSV는 메타데이터 4줄 뒤에 헤더가 있으므로, 첫 행만으로 파서를 감지할 수 없다.
최대 10행까지 순회하며 각 행을 헤더 후보로 매칭 시도.

```python
def detect_parser(file_path: Path) -> BaseParser:
    f, encoding = _open_csv(file_path)
    with f:
        reader = csv.reader(f)
        for row in itertools.islice(reader, 10):
            header_clean = [h.strip().strip('"') for h in row]
            for parser_cls in PARSERS:
                if parser_cls.can_parse(header_clean):
                    logger.info(f"파서 선택: {parser_cls.__name__} ({file_path.name})")
                    return parser_cls()
    raise ValueError(f"지원되지 않는 CSV 포맷: {file_path}")
```

### 1.3 PARSERS 리스트에 등록

```python
from .parsers.samsung_parser import SamsungDomesticParser

PARSERS = [
    MiraeDomesticParser,
    MiraeForeignParser,
    HankookDomesticParser,
    SamsungDomesticParser,  # 추가
]
```

## 2. samsung_parser.py — 삼성증권 국내계좌 파서

### 2.1 파일 위치

`modules/parsers/samsung_parser.py`

### 2.2 can_parse

```python
@staticmethod
def can_parse(header_row: List[str]) -> bool:
    keywords = {"거래일자", "거래명", "종목명"}
    header_set = set(h.strip() for h in header_row)
    return keywords.issubset(header_set)
```

**주의**: 컬럼명에 공백 있음 (`수 량`, `단 가`). 키워드에는 공백 없는 `거래일자`, `거래명`, `종목명`만 사용하면 충분.

### 2.3 스킵 로직

파싱 시 아래 행들을 건너뛴다:

| 패턴 | 판별 방법 |
|------|----------|
| 메타데이터 4줄 | 헤더 행(`거래일자`)을 찾을 때까지 건너뜀 |
| 헤더 행 | `row[0].strip() == "거래일자"` |
| 페이지 구분자 | `re.match(r"^\d+/\d+$", row[0].strip())` |
| 종료 마커 | `row[0].strip().startswith("- ")` 또는 `"출력 끝"` 포함 |
| 빈 행 | `len(row) < 6` 또는 `row[0].strip() == ""` |

### 2.4 인코딩 처리

파서 내부에서 EUC-KR fallback 로직 적용. `_open_csv()` 헬퍼를 `parser_registry.py`에서 공유하거나, 파서 자체에 인코딩 감지 로직을 둔다.

**방안**: `_open_csv()`를 모듈 레벨 유틸로 분리하여 `parser_registry.py`와 `samsung_parser.py`가 공유.

### 2.5 parse 메서드 핵심 로직

```python
def parse(self, file_path: Path, account: str) -> List[Trade]:
    trades = []
    f, encoding = _open_csv(file_path)
    with f:
        reader = csv.reader(f)
        header_found = False
        for row in reader:
            # 헤더 탐색
            if not header_found:
                if len(row) >= 6 and row[0].strip() == "거래일자":
                    header_found = True
                continue

            # 스킵 대상
            first = row[0].strip() if row else ""
            if not first:
                continue
            if re.match(r"^\d+/\d+$", first):   # 페이지 구분자
                continue
            if first == "거래일자":               # 반복 헤더
                continue
            if "출력 끝" in first or first.startswith("-"):
                continue

            # 데이터 파싱
            date = first  # 이미 YYYY-MM-DD
            trade_type = row[1].strip()
            stock_name = row[2].strip()
            quantity = _parse_float(row[3])
            price = _parse_float(row[4])
            amount = _parse_float(row[5])
            fee = _parse_float(row[6]) if len(row) > 6 else 0.0
            tax = _parse_float(row[7]) if len(row) > 7 else 0.0

            trades.append(Trade(
                date=date,
                trade_type=trade_type,
                stock_name=stock_name,
                stock_code="",
                quantity=quantity,
                price=price,
                amount=amount,
                currency="KRW",
                exchange_rate=1.0,
                amount_krw=amount,
                fee=fee,
                tax=tax,
                profit=0.0,
                profit_krw=0.0,
                profit_rate=0.0,
                account=account,
            ))
    return trades
```

### 2.6 _parse_float 헬퍼

한국투자증권 파서(`hankook_parser.py`)에 이미 동일한 로직이 있으므로 동일 패턴 적용:

```python
def _parse_float(value: str) -> float:
    cleaned = value.strip().strip('"').replace(",", "")
    if not cleaned:
        return 0.0
    return float(cleaned)
```

## 3. parsers/__init__.py 수정

```python
from .samsung_parser import SamsungDomesticParser

__all__ = [
    "BaseParser",
    "MiraeDomesticParser",
    "MiraeForeignParser",
    "HankookDomesticParser",
    "SamsungDomesticParser",  # 추가
]
```

## 4. _open_csv 유틸 위치 결정

`parser_registry.py`에 `_open_csv()` 정의하고, `samsung_parser.py`에서 import:

```python
# samsung_parser.py
from ..parser_registry import _open_csv
```

**순환 import 주의**: `parser_registry.py`가 `samsung_parser.py`를 import하고, 역방향 import가 발생한다.

**해결**: `_open_csv()`를 별도 유틸 모듈(`modules/encoding_utils.py`)로 분리하거나, `samsung_parser.py` 내부에 동일 로직을 중복 정의한다.

→ 파일 하나 추가하는 것보다 **`samsung_parser.py` 내부에 로직 포함**이 간결. 향후 다른 EUC-KR 파서가 추가되면 그때 분리.

## 5. 테스트 (`tests/test_parsers.py`)

### 5.1 TestSamsungDomesticParser 클래스

기존 테스트 패턴(미래에셋, 한국투자증권)을 따르되, 삼성증권 고유 엣지 케이스 추가.

```python
class TestSamsungDomesticParser:

    def test_can_parse(self):
        header = ["거래일자", "거래명", "종목명", "수 량", "단 가", "거래금액", "수수료", "제세금", "처리점"]
        assert SamsungDomesticParser.can_parse(header) is True

    def test_can_parse_rejects_mirae(self):
        header = ["일자", "종목명", "기간 중 매수"]
        assert SamsungDomesticParser.can_parse(header) is False

    def test_parse_sample(self, parser, sample_file):
        """실제 삼성증권 CSV 파싱 (EUC-KR, 메타데이터, 페이지네이션 포함)"""

    def test_skip_metadata_and_pagination(self, parser, tmp_path):
        """메타데이터 4줄, 페이지 구분자, 반복 헤더, 종료 마커가 올바르게 스킵되는지"""

    def test_buy_trade_fields(self, parser, sample_file):
        """매수 거래의 필드값 검증"""

    def test_fee_and_tax(self, parser, sample_file):
        """수수료/제세금 파싱"""

    def test_profit_is_zero(self, parser, sample_file):
        """삼성증권은 손익 데이터 없음 → 항상 0"""

    def test_quoted_comma_numbers(self, parser, tmp_path):
        """쌍따옴표+쉼표 숫자 파싱: "20,360" → 20360.0"""

    def test_date_format_unchanged(self, parser, sample_file):
        """날짜가 YYYY-MM-DD 형식 유지"""

    def test_to_domestic_row(self, parser, sample_file):
        """to_domestic_row() 9컬럼 반환"""
```

### 5.2 TestParserRegistry 추가

```python
def test_detect_samsung_domestic(self):
    """삼성증권 CSV에서 SamsungDomesticParser 자동 감지"""

def test_detect_samsung_euckr_encoding(self):
    """EUC-KR 인코딩 CSV 파일을 정상 감지"""
```

### 5.3 기존 테스트 회귀 확인

`parser_registry.py` 변경 후 기존 미래에셋/한국투자증권 테스트가 모두 통과하는지 확인.

## 6. CLAUDE.md 업데이트 내용

### Architecture 섹션

```
ParserRegistry  - CSV 헤더 기반 파서 자동 감지
├── MiraeDomesticParser   - 미래에셋 국내
├── MiraeForeignParser    - 미래에셋 해외
├── HankookDomesticParser - 한국투자증권 국내
└── SamsungDomesticParser - 삼성증권 국내    ← 추가
```

### Module Responsibilities 섹션

```
**modules/parsers/samsung_parser.py**:
- SamsungDomesticParser: 헤더 `거래일자, 거래명, 종목명` 감지
- EUC-KR 인코딩, 메타데이터/페이지네이션 스킵 처리
```

### Input File Format 섹션

```
input/
├── 미래에셋증권/
├── 한국투자증권/
├── 삼성증권/          ← 추가
│   └── 주식1.csv
└── sample/
```
