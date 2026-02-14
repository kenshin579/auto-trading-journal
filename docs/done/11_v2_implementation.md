# v2 시트 포맷 개선 및 대시보드 - 구현 문서

## 1. google_sheets_client.py - 신규 메서드 추가

### 1.1 `freeze_rows(sheet_name, row_count)`

Google Sheets API `updateSheetProperties`를 사용하여 행을 고정한다.

```python
async def freeze_rows(self, sheet_name: str, row_count: int = 1) -> bool:
    """시트의 상단 N행을 고정"""
    sheet_id = await self._get_sheet_id(sheet_name)
    requests = [{
        'updateSheetProperties': {
            'properties': {
                'sheetId': sheet_id,
                'gridProperties': {
                    'frozenRowCount': row_count
                }
            },
            'fields': 'gridProperties.frozenRowCount'
        }
    }]
    self.service.spreadsheets().batchUpdate(
        spreadsheetId=self.spreadsheet_id,
        body={'requests': requests}
    ).execute()
```

### 1.2 `set_auto_filter(sheet_name, start_row, end_row, start_col, end_col)`

`setBasicFilter` API를 사용한다. 기존 필터가 있으면 `clearBasicFilter` 후 재설정한다.

```python
async def set_auto_filter(self, sheet_name: str, start_row: int,
                          start_col: int, end_col: int) -> bool:
    """시트에 자동 필터를 설정 (기존 필터 제거 후)"""
    sheet_id = await self._get_sheet_id(sheet_name)
    requests = [
        {'clearBasicFilter': {'sheetId': sheet_id}},
        {'setBasicFilter': {
            'filter': {
                'range': {
                    'sheetId': sheet_id,
                    'startRowIndex': start_row - 1,
                    'startColumnIndex': start_col - 1,
                    'endColumnIndex': end_col
                }
            }
        }}
    ]
    # clearBasicFilter는 필터가 없으면 에러가 나므로 try-except로 감싼다
```

### 1.3 `_get_sheet_id(sheet_name)` - 헬퍼 메서드 추출

현재 `apply_color_to_range()`, `apply_number_format()`, `batch_apply_colors()` 모두 시트 ID를 가져오기 위해 매번 스프레드시트 메타데이터를 조회한다. 이를 헬퍼 메서드로 추출하고 캐시한다.

```python
async def _get_sheet_id(self, sheet_name: str) -> int:
    """시트 이름으로 sheetId를 반환 (캐시)"""
    if not hasattr(self, '_sheet_id_cache'):
        self._sheet_id_cache = {}
    if sheet_name not in self._sheet_id_cache:
        metadata = await self.get_spreadsheet_metadata()
        for sheet in metadata.get('sheets', []):
            title = sheet['properties']['title']
            sid = sheet['properties']['sheetId']
            self._sheet_id_cache[title] = sid
    return self._sheet_id_cache[sheet_name]
```

### 1.4 `apply_number_format_to_columns(sheet_name, column_formats, start_row, end_row)`

기존 `apply_number_format()`은 동일 패턴의 컬럼만 배치 처리한다. 새 메서드는 컬럼별로 다른 패턴을 한 번의 batchUpdate로 적용한다.

```python
async def apply_number_format_to_columns(
    self, sheet_name: str,
    column_formats: List[Dict],  # [{'col': 4, 'pattern': '#,##0', 'type': 'NUMBER'}, ...]
    start_row: int, end_row: int
) -> bool:
    """여러 컬럼에 각각 다른 숫자 포맷을 한 번에 적용"""
    sheet_id = await self._get_sheet_id(sheet_name)
    requests = []
    for fmt in column_formats:
        requests.append({
            'repeatCell': {
                'range': {
                    'sheetId': sheet_id,
                    'startRowIndex': start_row - 1,
                    'endRowIndex': end_row,
                    'startColumnIndex': fmt['col'] - 1,
                    'endColumnIndex': fmt['col']
                },
                'cell': {
                    'userEnteredFormat': {
                        'numberFormat': {
                            'type': fmt.get('type', 'NUMBER'),
                            'pattern': fmt['pattern']
                        }
                    }
                },
                'fields': 'userEnteredFormat.numberFormat'
            }
        })
    # 한 번의 batchUpdate로 전송
```

## 2. models.py - 숫자 타입 반환으로 변경

### 2.1 `to_domestic_row()` 변경

현재 모든 필드가 원시 타입(float)으로 이미 반환되고 있다. `profit_rate`만 퍼센트 소수 변환이 필요하다.

```python
def to_domestic_row(self) -> list:
    """국내계좌 시트 행 변환 (9컬럼)"""
    return [
        self.date, self.trade_type, self.stock_name,
        self.quantity, self.price, self.amount,
        self.fee, self.profit,
        self.profit_rate / 100 if self.profit_rate else 0,  # 14.68 → 0.1468
    ]
```

### 2.2 `to_foreign_row()` 변경

```python
def to_foreign_row(self) -> list:
    """해외계좌 시트 행 변환 (15컬럼)"""
    return [
        self.date, self.trade_type, self.currency, self.stock_code,
        self.stock_name, self.quantity, self.price, self.amount,
        self.exchange_rate, self.amount_krw,
        self.fee, self.tax, self.profit, self.profit_krw,
        self.profit_rate / 100 if self.profit_rate else 0,  # 퍼센트 소수
    ]
```

### 2.3 `duplicate_key()` 영향 확인

`duplicate_key()`는 `quantity`, `price`를 `_num_str()`로 변환한다. `to_*_row()`의 변경은 `duplicate_key()`에 영향을 주지 않는다. 단, 기존 시트에서 읽어온 데이터(FORMATTED_VALUE)와의 비교는 여전히 문자열 기반이므로 `duplicate_key()` 로직은 변경하지 않는다.

## 3. sheet_writer.py - 포맷 적용

### 3.1 `ensure_sheet_exists()` 변경

시트 생성 후 freeze + filter를 적용한다. 이미 존재하는 시트에도 매 실행 시 freeze + filter가 적용되도록 별도 메서드로 분리한다.

```python
async def ensure_sheet_exists(self, sheet_name: str, is_foreign: bool = False) -> bool:
    sheets = await self._get_sheets()
    if sheet_name not in sheets:
        headers = FOREIGN_HEADERS if is_foreign else DOMESTIC_HEADERS
        await self.client.create_sheet(sheet_name)
        await self.client.update_cells(sheet_name, "A1", [headers])
        self._invalidate_cache()
        logger.info(f"시트 '{sheet_name}' 생성 및 헤더 삽입 완료")
        created = True
    else:
        created = False

    # freeze + filter 매 실행 시 적용
    await self.apply_sheet_formatting(sheet_name, is_foreign)
    return created

async def apply_sheet_formatting(self, sheet_name: str, is_foreign: bool = False):
    """시트에 freeze + filter 적용"""
    num_cols = len(FOREIGN_HEADERS) if is_foreign else len(DOMESTIC_HEADERS)
    await self.client.freeze_rows(sheet_name, 1)
    await self.client.set_auto_filter(sheet_name, 1, 1, num_cols)
```

### 3.2 `insert_trades()` 변경 - 숫자 포맷 적용

데이터 삽입 후 컬럼별 numberFormat을 적용한다.

```python
async def insert_trades(self, sheet_name, trades, is_foreign=False) -> int:
    # ... 기존 데이터 삽입 로직 ...

    # 숫자 포맷 적용
    await self._apply_number_formats(sheet_name, start_row, end_row, is_foreign)

    # 날짜별 색상 적용
    self._apply_date_colors(...)

    return len(trades)
```

### 3.3 `_apply_number_formats()` 신규 메서드

컬럼별 포맷 정의를 상수로 관리한다.

```python
# 국내계좌 컬럼별 포맷 (1-based index)
DOMESTIC_FORMATS = [
    {'col': 4, 'pattern': '#,##0'},       # D: 수량
    {'col': 5, 'pattern': '#,##0'},       # E: 단가
    {'col': 6, 'pattern': '#,##0'},       # F: 금액
    {'col': 7, 'pattern': '#,##0'},       # G: 수수료
    {'col': 8, 'pattern': '#,##0'},       # H: 손익금액
    {'col': 9, 'pattern': '0.00%', 'type': 'PERCENT'},  # I: 수익률
]

# 해외계좌 컬럼별 포맷 (1-based index)
FOREIGN_FORMATS = [
    {'col': 6, 'pattern': '#,##0'},        # F: 수량
    {'col': 7, 'pattern': '#,##0.00'},     # G: 단가
    {'col': 8, 'pattern': '#,##0.00'},     # H: 금액(외화)
    {'col': 9, 'pattern': '#,##0.00'},     # I: 환율
    {'col': 10, 'pattern': '#,##0'},       # J: 금액(원화)
    {'col': 11, 'pattern': '#,##0.00'},    # K: 수수료
    {'col': 12, 'pattern': '#,##0.00'},    # L: 세금
    {'col': 13, 'pattern': '#,##0.00'},    # M: 손익(외화)
    {'col': 14, 'pattern': '#,##0'},       # N: 손익(원화)
    {'col': 15, 'pattern': '0.00%', 'type': 'PERCENT'},  # O: 수익률
]

async def _apply_number_formats(self, sheet_name, start_row, end_row, is_foreign):
    formats = FOREIGN_FORMATS if is_foreign else DOMESTIC_FORMATS
    await self.client.apply_number_format_to_columns(
        sheet_name, formats, start_row, end_row
    )
```

### 3.4 `get_existing_keys()` 영향 확인

기존 시트에는 문자열 데이터가 이미 들어가 있다. 새로 삽입되는 데이터는 숫자 타입이므로, `FORMATTED_VALUE`로 읽어올 때 `"28230"` 같은 문자열로 반환된다.

`duplicate_key()`에서 `_num_str()`을 사용하는데, 이는 `28230.0` → `"28230"`으로 변환하므로 일관성이 유지된다. **포맷(`#,##0`)이 적용된 후에도 `FORMATTED_VALUE`는 `"28,230"`이 된다.** 따라서 `get_sheet_data()`의 `valueRenderOption`을 `UNFORMATTED_VALUE`로 변경하거나, 중복 체크 로직을 숫자 비교로 변경해야 한다.

**해결 방안**: `get_existing_keys()`에서 사용하는 `get_sheet_data()` 호출 시 `valueRenderOption='UNFORMATTED_VALUE'`를 사용하도록 파라미터를 추가한다.

```python
async def get_sheet_data(self, sheet_name, range_str=None,
                         value_render_option='FORMATTED_VALUE'):
    result = self.service.spreadsheets().values().get(
        spreadsheetId=self.spreadsheet_id,
        range=range_name,
        valueRenderOption=value_render_option
    ).execute()
```

`get_existing_keys()`에서는:
```python
data = await self.client.get_sheet_data(
    sheet_name, "A2:O10000",
    value_render_option='UNFORMATTED_VALUE'
)
```

## 4. summary_generator.py - 대시보드 시트 통합

### 4.1 시트 구조 변경

기존 `요약_월별`, `요약_종목별` 2개 시트를 `대시보드` 1개 시트로 통합한다.

```python
DASHBOARD_SHEET = "대시보드"

# 기존 상수 제거
# MONTHLY_SHEET = "요약_월별"
# STOCK_SHEET = "요약_종목별"
```

### 4.2 `generate_all()` 변경

```python
async def generate_all(self, all_trades: List[Trade]):
    """대시보드 시트 생성 (초기화 후 재작성)"""
    await self._ensure_dashboard_sheet()

    current_row = 1
    current_row = await self._write_portfolio_summary(all_trades, current_row)
    current_row += 1  # 빈 행
    current_row = await self._write_monthly_summary(all_trades, current_row)
    current_row += 1  # 빈 행
    current_row = await self._write_stock_summary(all_trades, current_row)
    current_row += 1  # 빈 행
    current_row = await self._write_investment_metrics(all_trades, current_row)

    # 포맷 적용
    await self._apply_dashboard_formats(current_row)
```

### 4.3 섹션 1: 포트폴리오 요약 산출 로직

```python
async def _write_portfolio_summary(self, trades, start_row) -> int:
    buy_trades = [t for t in trades if t.trade_type == '매수']
    sell_trades = [t for t in trades if t.trade_type == '매도']

    total_buy = sum(t.amount_krw for t in buy_trades)
    total_sell = sum(t.amount_krw for t in sell_trades)
    total_profit = sum(t.profit_krw for t in sell_trades)
    total_return = total_profit / total_sell if total_sell else 0
    total_count = len(trades)

    # 승률: profit_krw > 0인 매도 건수 / 전체 매도 건수
    profitable = len([t for t in sell_trades if t.profit_krw > 0])
    win_rate = profitable / len(sell_trades) if sell_trades else 0

    # 행1: 헤더, 행2: 값
    headers = ["지표", "총 매수금액(원)", "총 매도금액(원)", "총 실현손익(원)",
               "총 수익률(%)", "총 거래건수", "승률(%)"]
    values = ["값", total_buy, total_sell, total_profit,
              total_return, total_count, win_rate]

    await self.client.update_cells(DASHBOARD_SHEET, f"A{start_row}", [headers])
    await self.client.update_cells(DASHBOARD_SHEET, f"A{start_row + 1}", [values])

    return start_row + 2  # 다음 섹션 시작 행
```

### 4.4 섹션 2: 월별 성과

기존 `generate_monthly_summary()` 로직에 수익률 컬럼 추가.

```python
# 수익률 계산
profit_rate = g["profit"] / g["sell_amount"] if g["sell_amount"] else 0

rows.append([
    month, account,
    g["buy_count"], g["buy_amount"],
    g["sell_count"], g["sell_amount"],
    g["profit"], profit_rate,
])
```

### 4.5 섹션 3: 종목별 현황

기존 `generate_stock_summary()` 로직에 수익률 + 투자 비중 추가.

```python
total_buy_amount = sum(g["buy_amount"] for g in groups.values())

for (name, code, account, currency), g in sorted(groups.items()):
    profit_rate = g["profit"] / g["sell_amount"] if g["sell_amount"] else 0
    weight = g["buy_amount"] / total_buy_amount if total_buy_amount else 0

    rows.append([
        name, code, account, currency,
        g["buy_qty"], g["buy_amount"],
        g["sell_qty"], g["sell_amount"],
        g["profit"], profit_rate, weight,
    ])
```

### 4.6 섹션 4: 투자 지표

```python
async def _write_investment_metrics(self, trades, start_row) -> int:
    buy_trades = [t for t in trades if t.trade_type == '매수']
    sell_trades = [t for t in trades if t.trade_type == '매도']
    total_buy = sum(t.amount_krw for t in buy_trades)

    rows = [["[투자 지표]", ""]]

    # 계좌별 투자비중
    rows.append(["계좌별 투자비중", ""])
    account_buy = defaultdict(float)
    for t in buy_trades:
        account_buy[t.account] += t.amount_krw
    for account, amount in sorted(account_buy.items()):
        rows.append([f"  {account}", amount / total_buy if total_buy else 0])

    # 통화별 투자비중
    rows.append(["통화별 투자비중", ""])
    currency_buy = defaultdict(float)
    for t in buy_trades:
        currency_buy[t.currency] += t.amount_krw
    for currency, amount in sorted(currency_buy.items()):
        rows.append([f"  {currency}", amount / total_buy if total_buy else 0])

    # 상위 5종목 집중도
    stock_buy = defaultdict(float)
    for t in buy_trades:
        stock_buy[t.stock_name] += t.amount_krw
    top5 = sorted(stock_buy.values(), reverse=True)[:5]
    top5_ratio = sum(top5) / total_buy if total_buy else 0
    rows.append(["상위 5종목 집중도", top5_ratio])

    # 평균 수익률 / 평균 손실률
    profit_rates = [t.profit_rate for t in sell_trades if t.profit_rate > 0]
    loss_rates = [t.profit_rate for t in sell_trades if t.profit_rate < 0]
    avg_profit = (sum(profit_rates) / len(profit_rates) / 100) if profit_rates else 0
    avg_loss = (sum(loss_rates) / len(loss_rates) / 100) if loss_rates else 0
    rows.append(["평균 수익률", avg_profit])
    rows.append(["평균 손실률", avg_loss])

    # 손익비
    pl_ratio = abs(avg_profit / avg_loss) if avg_loss else 0
    rows.append(["손익비", pl_ratio])

    # 최대 수익/손실 종목
    if sell_trades:
        best = max(sell_trades, key=lambda t: t.profit_krw)
        worst = min(sell_trades, key=lambda t: t.profit_krw)
        rows.append(["최대 수익 종목", f"{best.stock_name} (+{best.profit_krw:,.0f}원)"])
        rows.append(["최대 손실 종목", f"{worst.stock_name} ({worst.profit_krw:,.0f}원)"])

    # 데이터 작성
    end_row = start_row + len(rows) - 1
    await self.client.batch_update_cells(
        DASHBOARD_SHEET, {f"A{start_row}:B{end_row}": rows}
    )

    return start_row + len(rows)
```

### 4.7 대시보드 포맷 적용

```python
async def _apply_dashboard_formats(self, total_rows):
    """대시보드 시트에 freeze + 숫자 포맷 적용"""
    await self.client.freeze_rows(DASHBOARD_SHEET, 1)

    # 포트폴리오 요약 (행 2): 금액 컬럼에 #,##0, 비율에 0.00%
    # 월별 성과 섹션: 금액에 #,##0, 수익률에 0.00%
    # 종목별 현황 섹션: 금액에 #,##0, 비율에 0.00%
    # 투자 지표 섹션: 비율에 0.00%
    # → 각 섹션의 시작/끝 행을 추적하여 apply_number_format_to_columns 호출
```

## 5. main.py - 변경 사항

### 5.1 기존 요약 시트 삭제 처리

기존 `요약_월별`, `요약_종목별` 시트가 존재할 경우 삭제하지 않고 그대로 둔다 (사용자가 수동으로 정리). `대시보드` 시트만 새로 생성/갱신한다.

### 5.2 `SummaryGenerator` 호출 변경 없음

`generate_all(all_trades)` 인터페이스는 동일하게 유지되므로 main.py 변경 불필요.

## 6. 주의 사항

### 6.1 기존 데이터 호환

- 기존 시트에 문자열로 입력된 데이터는 그대로 유지된다
- 새로 삽입되는 데이터만 숫자 타입 + 포맷이 적용된다
- 중복 체크 시 `UNFORMATTED_VALUE`를 사용하여 포맷 영향을 받지 않도록 한다

### 6.2 API 호출 최적화

- `_get_sheet_id()` 캐시를 도입하여 메타데이터 조회 횟수를 최소화한다
- freeze + filter + numberFormat을 하나의 `batchUpdate`로 묶어 API 호출을 최소화한다

### 6.3 수익률 데이터 변환

- 파서에서 `profit_rate`는 `14.68` (퍼센트 값)로 파싱된다
- `to_*_row()`에서 `0.1468` (소수)로 변환하여 시트에 삽입한다
- 시트에서 `0.00%` 포맷이 적용되면 `14.68%`로 표시된다
