# Google Sheets API Rate Limit (429) 개선 구현 문서

## 1. 구현 범위

PRD의 5가지 개선 방안을 모두 구현한다.

| 작업 | 대상 파일 | 절감 |
|------|----------|------|
| 대시보드 포맷 일괄 처리 | google_sheets_client.py, summary_generator.py | -16회 |
| CSV 시트 포맷팅 병합 | google_sheets_client.py, sheet_writer.py | -21회 |
| 대시보드 헤더/데이터 병합 | summary_generator.py | -3회 |
| 대시보드 초기화 병합 | summary_generator.py | -2회 |
| 429 재시도 로직 | google_sheets_client.py | 안전망 |

## 2. google_sheets_client.py 변경사항

### 2.1 신규: `execute_batch_requests()`

수집된 batchUpdate 요청 리스트를 1회 API 호출로 전송한다.

```python
async def execute_batch_requests(self, requests: List[Dict[str, Any]]) -> bool:
    """여러 batchUpdate 요청을 한 번에 실행"""
    if not requests:
        return True
    try:
        self.service.spreadsheets().batchUpdate(
            spreadsheetId=self.spreadsheet_id,
            body={'requests': requests}
        ).execute()
        logger.info(f"배치 요청 {len(requests)}개 실행 완료")
        return True
    except HttpError as e:
        logger.error(f"배치 요청 실행 실패: {e}")
        return False
```

### 2.2 신규: `build_number_format_requests()` (static)

`apply_number_format_to_columns()`의 요청 빌드 로직만 분리. API 호출 없이 `repeatCell` 요청 객체 리스트를 반환한다.

```python
@staticmethod
def build_number_format_requests(
    sheet_id: int,
    column_formats: List[Dict[str, Any]],
    start_row: int, end_row: int,
) -> List[Dict[str, Any]]:
    """숫자 포맷 repeatCell 요청 객체 생성 (API 호출 없음)"""
    requests = []
    for fmt in column_formats:
        requests.append({
            'repeatCell': {
                'range': {
                    'sheetId': sheet_id,
                    'startRowIndex': start_row - 1,
                    'endRowIndex': end_row,
                    'startColumnIndex': fmt['col'] - 1,
                    'endColumnIndex': fmt['col'],
                },
                'cell': {
                    'userEnteredFormat': {
                        'numberFormat': {
                            'type': fmt.get('type', 'NUMBER'),
                            'pattern': fmt['pattern'],
                        }
                    }
                },
                'fields': 'userEnteredFormat.numberFormat',
            }
        })
    return requests
```

### 2.3 신규: `build_color_requests()` (static)

`batch_apply_colors()`의 요청 빌드 로직만 분리.

```python
@staticmethod
def build_color_requests(
    sheet_id: int,
    color_ranges: List[Dict[str, Any]],
) -> List[Dict[str, Any]]:
    """배경색 repeatCell 요청 객체 생성 (API 호출 없음)"""
    requests = []
    for cr in color_ranges:
        requests.append({
            'repeatCell': {
                'range': {
                    'sheetId': sheet_id,
                    'startRowIndex': cr['start_row'] - 1,
                    'endRowIndex': cr['end_row'],
                    'startColumnIndex': cr['start_col'] - 1,
                    'endColumnIndex': cr['end_col'],
                },
                'cell': {
                    'userEnteredFormat': {
                        'backgroundColor': cr['color'],
                    }
                },
                'fields': 'userEnteredFormat.backgroundColor',
            }
        })
    return requests
```

### 2.4 신규: `apply_sheet_formatting_batch()`

`freeze_rows()` + `set_auto_filter()` + `clear_background_colors()`를 1회 batchUpdate로 병합.

```python
async def apply_sheet_formatting_batch(
    self, sheet_name: str,
    freeze_row_count: int = 1,
    filter_start_row: int = 1,
    filter_start_col: int = 1,
    filter_end_col: int = 9,
    clear_bg_end_row: int = 1000,
    clear_bg_end_col: int = 26,
) -> bool:
    """시트 포맷팅(행고정 + 필터 + 배경색초기화)을 1회 batchUpdate로 적용"""
    sheet_id = await self.get_sheet_id(sheet_name)
    if sheet_id is None:
        return False

    requests = [
        # 1. 행 고정
        {
            'updateSheetProperties': {
                'properties': {
                    'sheetId': sheet_id,
                    'gridProperties': {'frozenRowCount': freeze_row_count},
                },
                'fields': 'gridProperties.frozenRowCount',
            }
        },
        # 2. 기존 필터 제거
        {'clearBasicFilter': {'sheetId': sheet_id}},
        # 3. 새 필터 설정
        {
            'setBasicFilter': {
                'filter': {
                    'range': {
                        'sheetId': sheet_id,
                        'startRowIndex': filter_start_row - 1,
                        'startColumnIndex': filter_start_col - 1,
                        'endColumnIndex': filter_end_col,
                    }
                }
            }
        },
        # 4. 배경색 초기화
        {
            'repeatCell': {
                'range': {
                    'sheetId': sheet_id,
                    'startRowIndex': 0,
                    'endRowIndex': clear_bg_end_row,
                    'startColumnIndex': 0,
                    'endColumnIndex': clear_bg_end_col,
                },
                'cell': {'userEnteredFormat': {}},
                'fields': 'userEnteredFormat.backgroundColor',
            }
        },
    ]

    try:
        self.service.spreadsheets().batchUpdate(
            spreadsheetId=self.spreadsheet_id,
            body={'requests': requests},
        ).execute()
        logger.info(f"시트 '{sheet_name}' 포맷팅 일괄 적용 완료")
        return True
    except HttpError as e:
        logger.error(f"시트 포맷팅 일괄 적용 실패 ({sheet_name}): {e}")
        return False
```

### 2.5 신규: `_execute_with_retry()`

429 에러 시 exponential backoff 재시도. Google 공식 권장 패턴을 따른다.

```python
import asyncio
import random

async def _execute_with_retry(self, request_fn, max_retries: int = 3):
    """API 요청 실행 + 429 에러 시 exponential backoff 재시도"""
    for attempt in range(max_retries + 1):
        try:
            return request_fn().execute()
        except HttpError as e:
            if e.resp.status == 429 and attempt < max_retries:
                wait = min(2 ** attempt + random.uniform(0, 1), 64)
                logger.warning(
                    f"Rate limit 도달, {wait:.1f}초 후 재시도 "
                    f"({attempt + 1}/{max_retries})"
                )
                await asyncio.sleep(wait)
            else:
                raise
```

기존 API 호출 지점(`batch_update_cells`, `execute_batch_requests`, `add_charts` 등)에서 `.execute()` 대신 `_execute_with_retry(lambda: request_fn)` 을 사용하도록 변경.

적용 대상 메서드:
- `batch_update_cells()` — values:batchUpdate
- `execute_batch_requests()` — batchUpdate
- `add_charts()` — batchUpdate
- `apply_number_format_to_columns()` — batchUpdate (sheet_writer에서 여전히 직접 호출됨)
- `apply_sheet_formatting_batch()` — batchUpdate

## 3. summary_generator.py 변경사항

### 3.1 `__init__()` — 요청 수집용 인스턴스 변수 추가

```python
def __init__(self, ...):
    ...
    self._pending_requests: List[Dict[str, Any]] = []
    self._dashboard_sheet_id: Optional[int] = None
```

### 3.2 `generate_all()` — sheet_id 캐시 + flush 호출

```python
async def generate_all(self, all_trades: List[Trade]):
    self._pending_requests = []  # 초기화

    await self._ensure_dashboard_sheet()

    # sheet_id 1회 조회 후 캐시
    self._dashboard_sheet_id = await self.client.get_sheet_id(DASHBOARD_SHEET)

    # ... (기존 6개 섹션 데이터 작성 — 동일)

    # 포맷/색상 → API 호출 대신 _pending_requests에 수집
    self._collect_header_colors(monthly_start, trend_start, stock_start)
    self._collect_dashboard_formats(monthly_start, metrics_start,
                                    insights_start, trend_start,
                                    stock_start, current_row)

    # 차트용 데이터 작성 (포맷 요청도 수집됨)
    await self._write_trade_count_data(all_trades)

    # 수집된 포맷/색상 요청을 1회로 전송
    await self._flush_pending_requests()

    # 차트 생성
    await self._create_charts(trend_start=trend_start, trend_end=stock_start - 1)
```

### 3.3 `_ensure_dashboard_sheet()` — 초기화 병합

```python
async def _ensure_dashboard_sheet(self):
    sheets = await self.client.list_sheets()
    if DASHBOARD_SHEET not in sheets:
        await self.client.create_sheet(DASHBOARD_SHEET)
    else:
        # 1) 데이터 삭제 (values:clear — 별도 엔드포인트)
        await self.client.clear_sheet(DASHBOARD_SHEET, start_row=1)

        # 2) 배경색 + 숫자포맷 + 차트를 1회 batchUpdate로 삭제
        sheet_id = await self.client.get_sheet_id(DASHBOARD_SHEET)
        requests = [
            # 배경색 초기화
            {
                'repeatCell': {
                    'range': {'sheetId': sheet_id, 'startRowIndex': 0,
                              'endRowIndex': 1000, 'startColumnIndex': 0,
                              'endColumnIndex': 26},
                    'cell': {'userEnteredFormat': {}},
                    'fields': 'userEnteredFormat.backgroundColor',
                }
            },
            # 숫자 포맷 초기화
            {
                'repeatCell': {
                    'range': {'sheetId': sheet_id, 'startRowIndex': 0,
                              'endRowIndex': 1000, 'startColumnIndex': 0,
                              'endColumnIndex': 26},
                    'cell': {'userEnteredFormat': {}},
                    'fields': 'userEnteredFormat.numberFormat',
                }
            },
        ]
        # 차트 삭제 요청 추가
        charts = await self.client.get_charts(DASHBOARD_SHEET)
        for c in charts:
            requests.append(
                {'deleteEmbeddedObject': {'objectId': c['chartId']}}
            )
        await self.client.execute_batch_requests(requests)
```

### 3.4 `_apply_header_colors()` → `_collect_header_colors()`

`async` 제거. API 호출 대신 `_pending_requests`에 추가.

```python
def _collect_header_colors(self, monthly_start, trend_start, stock_start):
    header_color = {'red': 0.24, 'green': 0.52, 'blue': 0.78}
    header_rows = [
        {'row': 1, 'end_col': 7},
        {'row': monthly_start, 'end_col': 8},
        {'row': trend_start, 'end_col': 11},
        {'row': stock_start, 'end_col': 11},
    ]
    color_ranges = [
        {'start_row': h['row'], 'end_row': h['row'],
         'start_col': 1, 'end_col': h['end_col'], 'color': header_color}
        for h in header_rows
    ]
    self._pending_requests.extend(
        GoogleSheetsClient.build_color_requests(self._dashboard_sheet_id, color_ranges)
    )
```

### 3.5 `_apply_dashboard_formats()` → `_collect_dashboard_formats()`

`async` 제거. 기존 `await self.client.apply_number_format_to_columns(...)` 호출을 `self._pending_requests.extend(GoogleSheetsClient.build_number_format_requests(...))` 로 대체.

4개 섹션(포트폴리오, 월별성과, 월별추이, 종목별) 모두 동일 패턴으로 변환.

### 3.6 `_apply_metrics_formats()` → `_collect_metrics_formats()`

`async` 제거. 기존 루프 내 `await self.client.apply_number_format_to_columns(...)` 를 `self._pending_requests.extend(...)` 로 대체.

```python
def _collect_metrics_formats(self, start_row, pct_offsets, krw_offsets, total_rows):
    sheet_id = self._dashboard_sheet_id

    # 기본 포맷
    default_fmt = [{'col': 2, 'pattern': '#,##0.##'}]
    self._pending_requests.extend(
        GoogleSheetsClient.build_number_format_requests(
            sheet_id, default_fmt, start_row, start_row + total_rows - 1
        )
    )

    # pct/krw 그룹별 포맷
    pct_fmt = [{'col': 2, 'pattern': '0.00%', 'type': 'PERCENT'}]
    krw_fmt = [{'col': 2, 'pattern': '₩#,##0'}]

    for offsets, fmt in [(pct_offsets, pct_fmt), (krw_offsets, krw_fmt)]:
        if not offsets:
            continue
        abs_rows = sorted(start_row + o for o in offsets)
        for r_start, r_end in self._group_consecutive_rows(abs_rows):
            self._pending_requests.extend(
                GoogleSheetsClient.build_number_format_requests(
                    sheet_id, fmt, r_start, r_end
                )
            )
```

### 3.7 `_flush_pending_requests()` — 신규

```python
async def _flush_pending_requests(self):
    if self._pending_requests:
        await self.client.execute_batch_requests(self._pending_requests)
        logger.info(f"대시보드 포맷/색상 {len(self._pending_requests)}개 요청 일괄 적용 완료")
        self._pending_requests = []
```

### 3.8 `_write_*` 헤더/데이터 병합

`_write_monthly_summary()`, `_write_monthly_trend()`, `_write_stock_summary()` 3개 메서드에서:

```python
# 변경 전
await self.client.update_cells(DASHBOARD_SHEET, f"A{start_row}", [headers])
if rows:
    await self.client.batch_update_cells(
        DASHBOARD_SHEET, {f"A{start_row + 1}:K{end_row}": rows}
    )

# 변경 후
if rows:
    all_rows = [headers] + rows
    await self.client.batch_update_cells(
        DASHBOARD_SHEET, {f"A{start_row}:K{end_row}": all_rows}
    )
else:
    await self.client.batch_update_cells(
        DASHBOARD_SHEET, {f"A{start_row}:K{start_row}": [headers]}
    )
```

### 3.9 `_write_investment_metrics()`, `_write_trading_insights()` — 포맷 수집

`_apply_metrics_formats()` → `_collect_metrics_formats()` 호출로 변경.

```python
# 변경 전
await self._apply_metrics_formats(start_row, pct_rows, krw_rows, len(rows))

# 변경 후
self._collect_metrics_formats(start_row, pct_rows, krw_rows, len(rows))
```

### 3.10 `_write_trade_count_data()` — 포맷 수집

```python
# 변경 전
await self.client.apply_number_format_to_columns(
    DASHBOARD_SHEET, amount_formats, start_row + 1, end_row
)

# 변경 후
self._pending_requests.extend(
    GoogleSheetsClient.build_number_format_requests(
        self._dashboard_sheet_id, amount_formats, start_row + 1, end_row
    )
)
```

## 4. sheet_writer.py 변경사항

### 4.1 `apply_sheet_formatting()` — 병합 호출

```python
# 변경 전
async def apply_sheet_formatting(self, sheet_name: str, is_foreign: bool = False):
    num_cols = len(FOREIGN_HEADERS) if is_foreign else len(DOMESTIC_HEADERS)
    await self.client.freeze_rows(sheet_name, 1)
    await self.client.set_auto_filter(sheet_name, 1, 1, num_cols)

# 변경 후
async def apply_sheet_formatting(self, sheet_name: str, is_foreign: bool = False):
    num_cols = len(FOREIGN_HEADERS) if is_foreign else len(DOMESTIC_HEADERS)
    await self.client.apply_sheet_formatting_batch(
        sheet_name,
        freeze_row_count=1,
        filter_start_row=1,
        filter_start_col=1,
        filter_end_col=num_cols,
    )
```

### 4.2 `ensure_sheet_exists()` — clear_background_colors 제거

`apply_sheet_formatting_batch()`에 배경색 초기화가 포함되므로 별도 호출 제거.

```python
# 변경 전
async def ensure_sheet_exists(self, sheet_name, is_foreign=False):
    sheets = await self._get_sheets()
    if sheet_name in sheets:
        await self.apply_sheet_formatting(sheet_name, is_foreign)
        await self.client.clear_background_colors(sheet_name)  # ← 제거
        return False
    ...
    await self.apply_sheet_formatting(sheet_name, is_foreign)
    return True

# 변경 후
async def ensure_sheet_exists(self, sheet_name, is_foreign=False):
    sheets = await self._get_sheets()
    if sheet_name in sheets:
        await self.apply_sheet_formatting(sheet_name, is_foreign)
        return False
    ...
    await self.apply_sheet_formatting(sheet_name, is_foreign)
    return True
```

## 5. `_execute_with_retry()` 적용 지점

기존 메서드에서 `.execute()` 호출을 `_execute_with_retry()`로 교체:

| 메서드 | 교체 대상 |
|--------|----------|
| `batch_update_cells()` | `self.service.spreadsheets().values().batchUpdate(...).execute()` |
| `execute_batch_requests()` | `self.service.spreadsheets().batchUpdate(...).execute()` |
| `add_charts()` | `self.service.spreadsheets().batchUpdate(...).execute()` |
| `apply_number_format_to_columns()` | `self.service.spreadsheets().batchUpdate(...).execute()` |
| `apply_sheet_formatting_batch()` | `self.service.spreadsheets().batchUpdate(...).execute()` |

> 읽기 전용 메서드(get_sheet_data, get_raw_grid_data 등)에는 적용하지 않는다. 429는 쓰기 쿼터에서만 발생.

## 6. 테스트 변경사항

### 6.1 `tests/test_summary_format.py`

- `_apply_metrics_formats` → `_collect_metrics_formats` 호출 변경에 맞춰 테스트 수정
- `_pending_requests`에 올바른 repeatCell 요청이 수집되는지 검증

### 6.2 신규: `tests/test_rate_limit.py`

- `build_number_format_requests()` 반환 dict 구조 검증
- `build_color_requests()` 반환 dict 구조 검증
- `_execute_with_retry()` 429 재시도 동작 검증 (mock)
- `apply_sheet_formatting_batch()` 요청 구조 검증 (mock)
