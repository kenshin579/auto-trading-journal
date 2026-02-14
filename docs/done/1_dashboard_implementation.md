# 대시보드 데이터 소스 변경 - 구현 문서

## 변경 파일 목록

| 파일 | 변경 유형 | 설명 |
|------|----------|------|
| `modules/sheet_writer.py` | 메서드 추가 | `read_all_trades()`, `_read_trades_from_sheet()` |
| `main.py` | 로직 변경 | 대시보드 데이터 소스를 시트 읽기로 변경 |
| `tests/test_sheet_reader.py` | 신규 | 시트 → Trade 변환 테스트 |

## 1. sheet_writer.py - 시트 데이터 읽기

### 1.1 `_read_trades_from_sheet(sheet_name, is_foreign)` 추가

개별 매매일지 시트에서 데이터를 읽어 Trade 리스트로 변환한다.

```python
async def _read_trades_from_sheet(self, sheet_name: str, is_foreign: bool) -> List[Trade]:
    """개별 매매일지 시트에서 Trade 리스트 반환"""
    data = await self.client.get_raw_grid_data(sheet_name, "A2:O10000")
    if not data or "sheets" not in data:
        return []

    rows = data["sheets"][0]["data"][0].get("rowData", [])
    trades = []
    for row in rows:
        values = row.get("values", [])
        # 국내 최소 9컬럼, 해외 최소 15컬럼
        min_cols = 15 if is_foreign else 9
        if len(values) < min_cols:
            continue

        # 날짜: formattedValue (시리얼 넘버 방지)
        date_val = values[0].get("formattedValue", "")
        if not date_val:
            continue

        trade = _row_to_trade(values, sheet_name, is_foreign, date_val)
        if trade:
            trades.append(trade)

    return trades
```

**셀 값 추출 헬퍼** - 기존 `get_existing_keys()`의 `_get_cell_value` 패턴을 재사용:

```python
def _get_cell_value(cell):
    """effectiveValue에서 값 추출 (문자열 또는 숫자)"""
    ev = cell.get("effectiveValue", {})
    return ev.get("stringValue") or ev.get("numberValue")

def _get_num(cell, default=0.0):
    """effectiveValue에서 숫자 추출"""
    ev = cell.get("effectiveValue", {})
    v = ev.get("numberValue")
    return float(v) if v is not None else default

def _get_str(cell, default=""):
    """effectiveValue에서 문자열 추출"""
    ev = cell.get("effectiveValue", {})
    v = ev.get("stringValue")
    return str(v) if v is not None else default
```

**국내계좌 행 → Trade 변환:**

```python
# 국내계좌: A~I (9컬럼)
# A:일자, B:구분, C:종목명, D:수량, E:단가, F:금액, G:수수료, H:손익금액, I:수익률(%)
Trade(
    date=date_val,                          # formattedValue
    trade_type=_get_str(values[1]),         # B: 매수/매도
    stock_name=_get_str(values[2]),         # C
    stock_code="",                          # 국내 시트에 없음
    quantity=_get_num(values[3]),           # D
    price=_get_num(values[4]),             # E
    amount=_get_num(values[5]),            # F (국내=원화)
    currency="KRW",
    exchange_rate=1.0,
    amount_krw=_get_num(values[5]),        # F (동일)
    fee=_get_num(values[6]),               # G
    tax=0.0,                               # 국내 시트에 없음
    profit=_get_num(values[7]),            # H (국내=원화)
    profit_krw=_get_num(values[7]),        # H (동일)
    profit_rate=_get_num(values[8]) * 100, # I: 0.1468 → 14.68
    account=sheet_name,
)
```

**해외계좌 행 → Trade 변환:**

```python
# 해외계좌: A~O (15컬럼)
# A:일자, B:구분, C:통화, D:종목코드, E:종목명, F:수량, G:단가,
# H:금액(외화), I:환율, J:금액(원화), K:수수료, L:세금,
# M:손익(외화), N:손익(원화), O:수익률(%)
Trade(
    date=date_val,                           # formattedValue
    trade_type=_get_str(values[1]),          # B
    stock_name=_get_str(values[4]),          # E
    stock_code=_get_str(values[3]),          # D
    quantity=_get_num(values[5]),            # F
    price=_get_num(values[6]),              # G
    amount=_get_num(values[7]),             # H
    currency=_get_str(values[3 - 1]),       # C (index 2)
    exchange_rate=_get_num(values[8]),       # I
    amount_krw=_get_num(values[9]),         # J
    fee=_get_num(values[10]),               # K
    tax=_get_num(values[11]),               # L
    profit=_get_num(values[12]),            # M
    profit_krw=_get_num(values[13]),        # N
    profit_rate=_get_num(values[14]) * 100, # O: 0.1468 → 14.68
    account=sheet_name,
)
```

### 1.2 `read_all_trades()` 추가

전체 시트를 순회하며 매매일지 시트를 헤더 검증으로 식별하고 Trade 리스트를 반환한다.

```python
async def read_all_trades(self) -> List[Trade]:
    """모든 매매일지 시트에서 Trade 리스트를 읽어 반환

    헤더 행(1행)을 검증하여 매매일지 시트만 식별.
    DOMESTIC_HEADERS 일치 → 국내, FOREIGN_HEADERS 일치 → 해외.
    """
    sheets = await self.client.list_sheets()
    all_trades = []

    for sheet_name in sheets:
        # 1행 헤더 읽기
        header_data = await self.client.get_sheet_data(
            sheet_name, "A1:O1"
        )
        header_row = _extract_header_row(header_data)
        if not header_row:
            continue

        # 헤더 검증으로 시트 유형 판별
        if header_row == DOMESTIC_HEADERS:
            is_foreign = False
        elif header_row == FOREIGN_HEADERS:
            is_foreign = True
        else:
            logger.debug(f"시트 '{sheet_name}' 스킵 (매매일지 헤더 불일치)")
            continue

        # 데이터 읽기
        trades = await self._read_trades_from_sheet(sheet_name, is_foreign)
        all_trades.extend(trades)
        logger.info(f"시트 '{sheet_name}'에서 {len(trades)}건 읽음 ({'해외' if is_foreign else '국내'})")

    logger.info(f"전체 매매일지 시트에서 총 {len(all_trades)}건 읽음")
    return all_trades
```

**헤더 추출 헬퍼:**

```python
def _extract_header_row(data) -> List[str]:
    """get_sheet_data() 결과에서 1행 헤더를 문자열 리스트로 추출"""
    if not data or "sheets" not in data:
        return []
    row_data = data["sheets"][0]["data"][0].get("rowData", [])
    if not row_data:
        return []
    values = row_data[0].get("values", [])
    return [
        (cell.get("effectiveValue", {}).get("stringValue") or "")
        for cell in values
    ]
```

## 2. main.py - 대시보드 데이터 소스 변경

### 변경 위치: `run()` 메서드의 요약 시트 갱신 부분 (기존 191-196행)

**기존:**
```python
# 3. 요약 시트 갱신
if all_trades and not self.dry_run:
    logger.info("=== 요약 시트 갱신 중 ===")
    await self.summary_generator.generate_all(all_trades)
elif self.dry_run and all_trades:
    logger.info(f"[DRY-RUN] 요약 시트 갱신 예정 (총 {len(all_trades)}건)")
```

**변경:**
```python
# 3. 대시보드 갱신 (매매일지 시트에서 전체 데이터 읽기)
if not self.dry_run:
    logger.info("=== 대시보드 갱신 중 ===")
    all_trades_from_sheets = await self.sheet_writer.read_all_trades()
    if all_trades_from_sheets:
        await self.summary_generator.generate_all(all_trades_from_sheets)
    else:
        logger.warning("매매일지 시트에 데이터가 없어 대시보드를 갱신하지 않습니다")
elif self.dry_run:
    logger.info("[DRY-RUN] 대시보드 갱신 예정")
```

**핵심 변경점:**
- `all_trades` (CSV 파싱 결과) → `all_trades_from_sheets` (시트에서 읽은 전체 데이터)
- CSV 파일 유무와 관계없이 시트에 데이터가 있으면 대시보드 갱신
- `all_trades` 변수는 CSV 처리 루프에서 여전히 사용하되 (결과 출력용), 대시보드에는 전달하지 않음

## 3. 테스트

### 3.1 `tests/test_sheet_reader.py`

`_read_trades_from_sheet()` 의 행 → Trade 변환 로직을 테스트한다. Google Sheets API를 직접 호출하지 않고 `get_raw_grid_data()` 반환값을 모킹한다.

**테스트 케이스:**

| 테스트 | 검증 내용 |
|--------|----------|
| `test_domestic_row_to_trade` | 국내 9컬럼 행 → Trade 16필드 정확히 매핑 |
| `test_foreign_row_to_trade` | 해외 15컬럼 행 → Trade 16필드 정확히 매핑 |
| `test_profit_rate_conversion` | 수익률 0.1468 → profit_rate 14.68 변환 |
| `test_skip_empty_row` | 빈 행 스킵 처리 |
| `test_skip_incomplete_row` | 컬럼 부족 행 스킵 처리 |
| `test_header_validation_domestic` | DOMESTIC_HEADERS 일치 시 국내로 인식 |
| `test_header_validation_foreign` | FOREIGN_HEADERS 일치 시 해외로 인식 |
| `test_header_validation_skip` | 헤더 불일치 시 스킵 |

**모킹 구조:**
```python
@pytest.fixture
def mock_client():
    client = AsyncMock(spec=GoogleSheetsClient)
    return client

@pytest.fixture
def sheet_writer(mock_client):
    return SheetWriter(mock_client)
```
