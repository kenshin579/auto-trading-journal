# Auto Trading Journal v2 - 구현 문서

## 1. 데이터 모델 (`modules/models.py`)

기존 `trade_models.py`의 `BaseTrade`/`DomesticTrade`/`ForeignTrade` 상속 구조를 단일 `Trade` 모델로 교체한다.

```python
@dataclass
class Trade:
    date: str              # YYYY-MM-DD
    trade_type: str        # 매수 / 매도
    stock_name: str        # 종목명
    stock_code: str        # 종목코드/티커 (없으면 "")
    quantity: float        # 수량
    price: float           # 단가 (외화 기준)
    amount: float          # 금액 (외화 기준)
    currency: str          # KRW / USD / JPY
    exchange_rate: float   # 환율 (국내는 1.0)
    amount_krw: float      # 원화 환산 금액
    fee: float             # 수수료
    tax: float             # 세금
    profit: float          # 실현손익 (외화)
    profit_krw: float      # 원화 실현손익
    profit_rate: float     # 수익률(%)
    account: str           # 증권사_계좌유형 (예: "미래에셋증권_국내계좌")

    def to_domestic_row(self) -> list:
        """국내계좌 시트 행 변환"""
        return [self.date, self.trade_type, self.stock_name,
                self.quantity, self.price, self.amount,
                self.fee, self.profit, self.profit_rate]

    def to_foreign_row(self) -> list:
        """해외계좌 시트 행 변환"""
        return [self.date, self.trade_type, self.currency, self.stock_code,
                self.stock_name, self.quantity, self.price, self.amount,
                self.exchange_rate, self.amount_krw,
                self.fee, self.tax, self.profit, self.profit_krw, self.profit_rate]

    def duplicate_key(self) -> tuple:
        """중복 체크 키"""
        return (self.date, self.trade_type, self.stock_name, self.quantity, self.price)
```

계좌 유형 판별:

```python
def is_domestic(self) -> bool:
    return "국내" in self.account

def is_foreign(self) -> bool:
    return "해외" in self.account
```

## 2. CSV 파서

### 2.1 파서 추상 클래스 (`modules/parsers/base_parser.py`)

```python
from abc import ABC, abstractmethod

class BaseParser(ABC):
    @staticmethod
    @abstractmethod
    def can_parse(header_row: list[str]) -> bool:
        """헤더를 보고 이 파서가 처리 가능한지 판단"""

    @abstractmethod
    def parse(self, file_path: Path, account: str) -> list[Trade]:
        """CSV 파일을 파싱하여 Trade 리스트 반환"""
```

### 2.2 미래에셋 국내 파서 (`modules/parsers/mirae_parser.py`)

감지 조건: 첫 번째 헤더 행에 `"일자"`, `"종목명"`, `"기간 중 매수"` 포함

파싱 로직:
- 2번째 행은 서브헤더 → 건너뜀
- 3번째 행부터 데이터
- `매수금액(col 4) > 0` → 매수 Trade 생성
- `매도금액(col 7) > 0` → 매도 Trade 생성
- 한 행에서 매수/매도 동시 발생 가능 → 2개 Trade 생성
- 날짜 포맷: `2026/02/13` → `2026-02-13` 변환

### 2.3 미래에셋 해외 파서 (`modules/parsers/mirae_parser.py`)

감지 조건: 첫 번째 헤더 행에 `"매매일"`, `"통화"`, `"종목번호"` 포함

파싱 로직:
- 1번째 행이 헤더, 2번째 행부터 데이터
- `매수 수량(col 7) > 0` → 매수 Trade 생성
- `매도 수량(col 11) > 0` → 매도 Trade 생성
- `통화(col 1)`: USD, JPY 등 → Trade.currency에 매핑
- `매매일환율(col 6)`: Trade.exchange_rate
- `원화매수금액(col 10)`, `원화매도금액(col 14)` → Trade.amount_krw
- 손익: `매매손익(col 19)` → profit, `총평가손익(col 22)` → profit_krw, `손익률(col 23)` → profit_rate

### 2.4 한국투자증권 국내 파서 (`modules/parsers/hankook_parser.py`)

감지 조건: 첫 번째 헤더 행에 `"매매일자"`, `"종목코드"`, `"매입단가"` 포함

파싱 로직:
- 쌍따옴표 제거, 천단위 쉼표 제거 (`"18,609"` → `18609`)
- 종목명이 빈 행은 건너뜀
- `매수금액(col 10) > 0` → 매수 Trade 생성 (단가: `매입단가(col 6)`)
- `매도금액(col 11) > 0` → 매도 Trade 생성 (단가: `매도단가(col 8)`)
- `실현손익(col 12)`, `손익률(col 13)` → 매도 Trade에만 설정
- `수수료(col 14)` + `제세금(col 16)` → Trade.fee에 합산
- 통화: KRW 고정, 환율: 1.0

### 2.5 파서 레지스트리 (`modules/parser_registry.py`)

```python
PARSERS = [MiraeDomesticParser, MiraeForeignParser, HankookDomesticParser]

def detect_parser(file_path: Path) -> BaseParser:
    """CSV 첫 번째 행을 읽어 적합한 파서를 반환"""
    with open(file_path, 'r', encoding='utf-8') as f:
        reader = csv.reader(f)
        header = next(reader)
    # 쌍따옴표 제거 후 각 파서의 can_parse() 호출
    header_clean = [h.strip().strip('"') for h in header]
    for parser_cls in PARSERS:
        if parser_cls.can_parse(header_clean):
            return parser_cls()
    raise ValueError(f"지원되지 않는 CSV 포맷: {file_path}")
```

## 3. Google Sheets 처리

### 3.1 기존 `google_sheets_client.py` 확장

시트 생성 메서드 추가:

```python
async def create_sheet(self, title: str) -> bool:
    """새 시트(탭)를 추가"""
    requests = [{"addSheet": {"properties": {"title": title}}}]
    body = {"requests": requests}
    self.service.spreadsheets().batchUpdate(
        spreadsheetId=self.spreadsheet_id, body=body
    ).execute()

async def clear_sheet(self, sheet_name: str) -> bool:
    """시트 데이터 전체 삭제 (헤더 제외)"""
    self.service.spreadsheets().values().clear(
        spreadsheetId=self.spreadsheet_id,
        range=f"{sheet_name}!A2:Z"
    ).execute()
```

### 3.2 시트 Writer (`modules/sheet_writer.py`)

기존 `sheet_manager.py`를 재구성한다.

핵심 메서드:

```python
class SheetWriter:
    def __init__(self, client: GoogleSheetsClient):
        self.client = client
        self.color_palette = [...]  # 기존 8색 팔레트 유지

    async def ensure_sheet_exists(self, sheet_name: str, account_type: str):
        """시트가 없으면 생성하고 헤더를 삽입"""
        sheets = await self.client.list_sheets()
        if sheet_name not in sheets:
            await self.client.create_sheet(sheet_name)
            headers = DOMESTIC_HEADERS if "국내" in account_type else FOREIGN_HEADERS
            await self.client.update_cells(sheet_name, "A1", [headers])

    async def get_existing_keys(self, sheet_name: str) -> set[tuple]:
        """기존 데이터에서 중복 체크용 키 셋 반환"""
        # (일자, 구분, 종목명, 수량, 단가) 튜플 셋

    async def insert_trades(self, sheet_name: str, trades: list[Trade]):
        """신규 거래 삽입 + 날짜별 색상 적용"""
        # 1. 마지막 행 찾기
        # 2. 데이터 삽입 (batch_update_cells)
        # 3. 날짜별 색상 적용 (batch_apply_colors)
```

헤더 상수:

```python
DOMESTIC_HEADERS = ["일자", "구분", "종목명", "수량", "단가", "금액", "수수료", "손익금액", "수익률(%)"]
FOREIGN_HEADERS = ["일자", "구분", "통화", "종목코드", "종목명", "수량", "단가", "금액(외화)", "환율", "금액(원화)", "수수료", "세금", "손익(외화)", "손익(원화)", "수익률(%)"]
```

### 3.3 날짜별 색상 적용

기존 `sheet_manager.py`의 `apply_date_colors()` 로직을 그대로 재사용한다.

- 8색 팔레트
- 같은 날짜 → 같은 색
- `batch_apply_colors()`로 한번에 적용

## 4. 요약 시트 (`modules/summary_generator.py`)

### 4.1 월별 요약 (`요약_월별`)

```python
async def generate_monthly_summary(self, sheet_writer, all_trades: list[Trade]):
    """모든 매매일지 시트 데이터 → 월별 요약"""
    # 1. 시트 초기화 (clear + 헤더 삽입)
    # 2. (연월, 계좌) 기준으로 그룹핑
    # 3. 각 그룹별 매수건수/매수금액(원)/매도건수/매도금액(원)/실현손익(원) 집계
    # 4. 연월 오름차순 정렬 후 삽입
```

집계 로직:
- `연월`: `trade.date[:7]` (YYYY-MM)
- `매수금액(원)`: 매수 거래의 `amount_krw` 합산
- `매도금액(원)`: 매도 거래의 `amount_krw` 합산
- `실현손익(원)`: 매도 거래의 `profit_krw` 합산

### 4.2 종목별 요약 (`요약_종목별`)

```python
async def generate_stock_summary(self, sheet_writer, all_trades: list[Trade]):
    """모든 매매일지 시트 데이터 → 종목별 요약"""
    # 1. 시트 초기화
    # 2. (종목명, 계좌) 기준으로 그룹핑
    # 3. 누적 매수수량/매수금액/매도수량/매도금액/실현손익 집계
    # 4. 종목명 오름차순 정렬 후 삽입
```

## 5. 메인 파이프라인 (`main.py`)

```python
async def run(self):
    # 1. stocks/ 스캔 → (증권사, 계좌유형, CSV경로) 리스트
    # 2. 각 CSV별:
    #    a. 파서 감지 → 파싱 → Trade 리스트
    #    b. 시트 존재 확인 → 없으면 생성
    #    c. 기존 키 로드 → 중복 필터링
    #    d. 신규 Trade 삽입 + 색상 적용
    # 3. 전체 Trade 수집 → 요약 시트 갱신
    # 4. 결과 출력
```

디렉토리 스캔:

```python
def scan_csv_files(self) -> list[tuple[str, str, Path]]:
    """stocks/ 하위 CSV 파일 스캔"""
    stocks_dir = Path("stocks")
    results = []
    for broker_dir in stocks_dir.iterdir():
        if not broker_dir.is_dir() or broker_dir.name == "sample":
            continue
        for csv_file in broker_dir.glob("*.csv"):
            account_type = csv_file.stem  # "국내계좌" or "해외계좌"
            account = f"{broker_dir.name}_{account_type}"
            results.append((broker_dir.name, account_type, csv_file))
    return results
```

## 6. 기존 코드 정리 대상

| 삭제 대상 | 이유 |
|-----------|------|
| `modules/stock_classifier.py` | 주식/ETF 분류 불필요 |
| `modules/report_generator.py` | 요약 시트로 대체 |
| `modules/data_validator.py` | 파서 내부 검증으로 통합 |
| `modules/file_parser.py` | `parsers/` 디렉토리로 교체 |
| `modules/trade_models.py` | `models.py`로 교체 |
| `stock_type_cache.json` | stock_classifier 제거와 함께 불필요 |
| `modules/test_foreign.py` | 새 테스트로 교체 |
| `modules/test_sheet_manager.py` | 새 테스트로 교체 |
