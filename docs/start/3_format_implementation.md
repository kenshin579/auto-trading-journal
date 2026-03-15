# 대시보드 시트 숫자 포맷 수정 — 구현 문서

## 변경 파일 목록

| 파일 | 변경 내용 |
|------|----------|
| `modules/google_sheets_client.py` | `clear_number_formats()` 메서드 추가 |
| `modules/summary_generator.py` | 시트 초기화 시 포맷 리셋, 섹션 4·5 기본 포맷 선적용, 범위 계산 수정 |
| `tests/test_summary_format.py` | 포맷 offset 계산 및 범위 테스트 (신규) |

---

## 1. `clear_number_formats()` 메서드 추가

**파일**: `modules/google_sheets_client.py`

`clear_background_colors()`와 동일한 패턴으로, `numberFormat` 필드를 초기화하는 메서드를 추가한다.

```python
async def clear_number_formats(self, sheet_name: str, end_row: int = 1000, end_col: int = 26) -> bool:
    """시트 전체 숫자 포맷을 초기화합니다"""
    try:
        sheet_id = await self._get_sheet_id(sheet_name)
        if sheet_id is None:
            logger.error(f"시트 '{sheet_name}'를 찾을 수 없습니다")
            return False

        requests = [{
            'repeatCell': {
                'range': {
                    'sheetId': sheet_id,
                    'startRowIndex': 0,
                    'endRowIndex': end_row,
                    'startColumnIndex': 0,
                    'endColumnIndex': end_col,
                },
                'cell': {
                    'userEnteredFormat': {}
                },
                'fields': 'userEnteredFormat.numberFormat'
            }
        }]

        self.service.spreadsheets().batchUpdate(
            spreadsheetId=self.spreadsheet_id,
            body={'requests': requests}
        ).execute()

        logger.info(f"시트 '{sheet_name}' 숫자 포맷 초기화 완료")
        return True

    except HttpError as e:
        logger.error(f"시트 '{sheet_name}' 숫자 포맷 초기화 실패: {e}")
        return False
```

---

## 2. `_ensure_dashboard_sheet()`에 포맷 초기화 추가

**파일**: `modules/summary_generator.py` — `_ensure_dashboard_sheet()` 메서드

기존 `clear_background_colors()` 호출 다음에 `clear_number_formats()` 호출 추가:

```python
async def _ensure_dashboard_sheet(self):
    sheets = await self.client.list_sheets()
    if DASHBOARD_SHEET not in sheets:
        await self.client.create_sheet(DASHBOARD_SHEET)
    else:
        await self.client.clear_sheet(DASHBOARD_SHEET, start_row=1)
        await self.client.clear_background_colors(DASHBOARD_SHEET)
        await self.client.clear_number_formats(DASHBOARD_SHEET)  # 추가
```

---

## 3. 섹션 2·3 포맷 범위 계산 수정

**파일**: `modules/summary_generator.py` — `_apply_dashboard_formats()` 메서드

### 현재 코드 (버그)

```python
monthly_data_end = stock_start - 2   # 빈 행 1개 건너뛰기
stock_data_end = metrics_start - 2
```

### 문제

`generate_all()`에서의 행 흐름:
```
_write_monthly_summary() → returns end_row + 1 (데이터 마지막 행 + 1)
current_row += 1  (빈 행)
stock_start = current_row  (빈 행 다음)
```

즉 `_write_monthly_summary()`가 반환한 값이 `end_row + 1`이고, `current_row += 1` 이후 `stock_start`가 설정된다.
따라서 `stock_start - 2`는 **데이터 마지막 행**이 맞다.

예시: 데이터 24행일 때:
- `_write_monthly_summary` returns 25 (24+1)
- `current_row` = 25, `current_row += 1` → 26
- `stock_start` = 26
- `stock_start - 2` = 24 ✅

**결론**: 범위 계산 자체는 정확하다. 근본 원인은 **Task 1의 포맷 초기화 누락**이므로, 범위 계산은 그대로 유지하되 검증 테스트를 추가한다.

---

## 4. 섹션 4·5 기본 포맷 선적용

**파일**: `modules/summary_generator.py` — `_apply_metrics_formats()` 메서드

기존 수동 포맷이 남아있는 셀이 있을 수 있으므로, 섹션 전체 B열을 먼저 기본 NUMBER 포맷으로 초기화한 후 개별 행 포맷을 적용한다.

```python
async def _apply_metrics_formats(self, start_row: int,
                                 pct_offsets: List[int],
                                 krw_offsets: List[int],
                                 total_rows: int):
    """투자 지표 섹션 행별 포맷 적용 (연속 행 그룹핑)"""
    # 1단계: 섹션 전체 B열을 기본 NUMBER 포맷으로 초기화
    default_fmt = [{'col': 2, 'pattern': '#,##0.##'}]
    await self.client.apply_number_format_to_columns(
        DASHBOARD_SHEET, default_fmt, start_row, start_row + total_rows - 1
    )

    # 2단계: pct/krw 개별 포맷 적용
    pct_fmt = [{'col': 2, 'pattern': '0.00%', 'type': 'PERCENT'}]
    krw_fmt = [{'col': 2, 'pattern': '₩#,##0'}]

    for offsets, fmt in [(pct_offsets, pct_fmt), (krw_offsets, krw_fmt)]:
        if not offsets:
            continue
        abs_rows = sorted(start_row + o for o in offsets)
        for r_start, r_end in self._group_consecutive_rows(abs_rows):
            await self.client.apply_number_format_to_columns(
                DASHBOARD_SHEET, fmt, r_start, r_end
            )
```

### 호출부 변경

`_write_investment_metrics()`와 `_write_trading_insights()`에서 `total_rows` 인자 추가:

```python
# _write_investment_metrics 내
await self._apply_metrics_formats(start_row, pct_rows, krw_rows, len(rows))

# _write_trading_insights 내
await self._apply_metrics_formats(start_row, pct_rows, krw_rows, len(rows))
```

---

## 5. 테스트 추가

**파일**: `tests/test_summary_format.py` (신규)

### 5-1. `_group_consecutive_rows` 테스트

```python
class TestGroupConsecutiveRows:
    def test_single_row(self):
        assert SummaryGenerator._group_consecutive_rows([5]) == [(5, 5)]

    def test_consecutive(self):
        assert SummaryGenerator._group_consecutive_rows([3, 4, 5]) == [(3, 5)]

    def test_gaps(self):
        assert SummaryGenerator._group_consecutive_rows([3, 4, 7, 8, 9, 12]) == [
            (3, 4), (7, 9), (12, 12)
        ]

    def test_empty(self):
        assert SummaryGenerator._group_consecutive_rows([]) == []
```

### 5-2. offset 매핑 검증 테스트

`_write_investment_metrics`에서 `pct_rows`와 `krw_rows`의 offset이 실제 rows 인덱스와 일치하는지 검증.
모의 데이터를 넣고, `batch_update_cells` 호출을 mock하여 전달된 rows와 offset의 일관성 확인.

```python
class TestMetricsOffsetMapping:
    """투자 지표 섹션의 pct_rows/krw_rows offset이 정확한지 검증"""

    @pytest.mark.asyncio
    async def test_pct_offset_points_to_pct_value(self):
        """pct_rows의 각 offset이 실제로 비율 값(0~1 범위)을 가리키는지 확인"""
        # mock client, mock sector_classifier
        # _write_investment_metrics 호출
        # pct_rows의 각 offset → rows[offset][1]이 0~1 범위인지 assert

    @pytest.mark.asyncio
    async def test_krw_offset_points_to_krw_value(self):
        """krw_rows의 각 offset이 실제로 원화 금액을 가리키는지 확인"""
        # krw_rows의 각 offset → rows[offset][1]이 큰 정수 값인지 assert
```
