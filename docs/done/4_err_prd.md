# Google Sheets API Rate Limit (429) 개선 PRD

## 1. 배경

### 현재 상태

`run.sh` 실행 시 Google Sheets API **분당 60회 쓰기 제한**(WriteRequestsPerMinutePerUser)을 초과하여 `HttpError 429: RATE_LIMIT_EXCEEDED` 에러가 발생한다. 2026-04-04 실행 로그에서 총 **76회** 쓰기 요청(POST 75 + PUT 1)이 **46초** 안에 발생하여 대시보드 후반부(헤더 색상, 숫자 포맷, 차트 생성)가 모두 실패했다.

### 에러 발생 타임라인 (실제 로그 기준)

```
06:37:41 ~ 06:38:07  CSV 처리 단계        43회 쓰기 (7개 파일)
06:38:15 ~ 06:38:25  대시보드 단계         17회 쓰기 (초기화 + 데이터)
────────────────────── 여기서 60회 초과 ──────────────────────
06:38:25 ~ 06:38:27  대시보드 포맷/차트    16회 시도 → 전부 429 에러
```

### 영향

- 대시보드 차트 6개가 모두 사라짐 (삭제는 성공, 재생성은 실패)
- 대시보드 헤더 색상, 숫자 포맷 미적용
- CSV 파일이 많아질수록 (계좌 추가 등) 에러 확률 증가

## 2. 현재 API 호출 분석

### 2.1 CSV 처리 단계 (파일당 쓰기 횟수)

| 단계 | 메서드 | 쓰기 횟수 | 비고 |
|------|--------|-----------|------|
| 시트 포맷팅 | `freeze_rows()` | 1 | batchUpdate |
| 자동 필터 | `set_auto_filter()` | 2 | clearBasicFilter + setBasicFilter |
| 배경색 초기화 | `clear_background_colors()` | 1 | batchUpdate |
| 데이터 삽입 | `batch_update_cells()` | 1 | values:batchUpdate |
| 숫자 포맷 | `apply_number_format_to_columns()` | 1~N | 국내 1회, 해외 1+통화수 |
| **국내 계좌 합계** | | **6회** | |
| **해외 계좌 합계** | | **7~8회** | 통화 종류에 따라 증가 |

**7개 파일 실측**: 국내 6개 × 6회 + 해외 1개 × 7회 = **43회**

### 2.2 대시보드 단계 (쓰기 횟수)

| 단계 | 메서드 | 쓰기 횟수 | 비고 |
|------|--------|-----------|------|
| **초기화** | | **4회** | |
| └ 데이터 삭제 | `clear_sheet()` | 1 | values:clear |
| └ 배경색 초기화 | `clear_background_colors()` | 1 | batchUpdate |
| └ 숫자 포맷 초기화 | `clear_number_formats()` | 1 | batchUpdate |
| └ 차트 삭제 | `delete_all_charts()` | 1 | batchUpdate |
| **데이터 작성** | | **12회** | |
| └ 포트폴리오 요약 | `batch_update_cells()` | 1 | |
| └ 월별 성과 | `update_cells()` + `batch_update_cells()` | 2 | 헤더 + 데이터 분리 |
| └ 투자 지표 | `batch_update_cells()` × 2 | 2 | 파이 데이터 + 본문 |
| └ 매매 인사이트 | `batch_update_cells()` | 1 | |
| └ 월별 성과 추이 | `update_cells()` + `batch_update_cells()` | 2 | 헤더 + 데이터 분리 |
| └ 종목별 현황 | `update_cells()` + `batch_update_cells()` | 2 | 헤더 + 데이터 분리 |
| └ 차트용 데이터 | `batch_update_cells()` | 1 | Q~U열 |
| └ (소계: `update_cells` 3회 + `batch_update_cells` 9회) | | | |
| **포맷/색상** | | **17회** | ← 핵심 병목 |
| └ 헤더 배경색 | `batch_apply_colors()` | 1 | |
| └ 포트폴리오 포맷 | `apply_number_format_to_columns()` | 1 | |
| └ 월별 성과 포맷 | `apply_number_format_to_columns()` | 1 | |
| └ 투자 지표 포맷 | `_apply_metrics_formats()` | ~6 | 기본 1 + pct/krw 그룹별 |
| └ 매매 인사이트 포맷 | `_apply_metrics_formats()` | ~5 | 기본 1 + pct/krw 그룹별 |
| └ 월별 추이 포맷 | `apply_number_format_to_columns()` | 1 | |
| └ 종목별 포맷 | `apply_number_format_to_columns()` | 1 | |
| └ 차트 데이터 포맷 | `apply_number_format_to_columns()` | 1 | |
| **차트 생성** | `add_charts()` | **1회** | |
| **대시보드 합계** | | **~34회** | |

### 2.3 전체 요약

```
CSV 처리:   43회 쓰기  (7개 파일, 489건)
대시보드:   34회 쓰기  (6개 섹션 + 6개 차트)
─────────────────────
합계:       77회 쓰기  → 60회 제한 초과 (17회 초과)
```

## 3. 개선 방안

### 3.1 대시보드 포맷 요청 일괄 처리 (절감: -16회)

**현재**: `apply_number_format_to_columns()`를 섹션마다 개별 호출 → 17회 batchUpdate

**개선**: 모든 포맷 요청(repeatCell)을 모아서 **1회** batchUpdate로 전송

```python
# 변경 전: 각 섹션에서 개별 API 호출
await self.client.apply_number_format_to_columns(DASHBOARD_SHEET, portfolio_formats, 2, 2)
await self.client.apply_number_format_to_columns(DASHBOARD_SHEET, monthly_formats, ...)
# ... 15회 더 반복

# 변경 후: 요청 객체만 수집 → 마지막에 한 번 전송
self._pending_format_requests.extend(
    build_format_requests(sheet_id, portfolio_formats, 2, 2)
)
self._pending_format_requests.extend(
    build_format_requests(sheet_id, monthly_formats, ...)
)
# ... 모든 섹션 수집 후
await self.client.execute_batch_requests(self._pending_format_requests)  # 1회
```

**구현 방법**:

1. `GoogleSheetsClient`에 신규 메서드 추가:
   - `build_number_format_requests(sheet_id, column_formats, start_row, end_row) → List[Dict]`: 요청 객체 생성만 (API 호출 안 함)
   - `build_color_requests(sheet_id, color_ranges) → List[Dict]`: 색상 요청 객체 생성만
   - `execute_batch_requests(requests) → bool`: 수집된 요청들을 1회 batchUpdate로 전송

2. `SummaryGenerator`에 요청 수집 패턴 도입:
   - `self._pending_format_requests: List[Dict] = []` 인스턴스 변수
   - `_apply_dashboard_formats()` → API 호출 대신 `_pending_format_requests`에 추가
   - `_apply_metrics_formats()` → 동일
   - `_apply_header_colors()` → 동일
   - `generate_all()` 끝에서 `_flush_pending_requests()` 호출

**결과**: 포맷/색상 17회 → 1회 (절감 16회)

### 3.2 대시보드 초기화 병합 (절감: -2회)

**현재**: `clear_background_colors()`, `clear_number_formats()`, `delete_all_charts()` 각각 별도 batchUpdate

**개선**: 3개를 **1회** batchUpdate로 병합

```python
# 변경 전
await self.client.clear_background_colors(DASHBOARD_SHEET)   # 1회
await self.client.clear_number_formats(DASHBOARD_SHEET)       # 1회
await self.client.delete_all_charts(DASHBOARD_SHEET)          # 1회 (+ 1 GET)

# 변경 후
requests = []
requests.append(build_clear_bg_request(sheet_id))
requests.append(build_clear_fmt_request(sheet_id))
requests.extend(build_delete_charts_requests(chart_ids))
await self.client.execute_batch_requests(requests)            # 1회
```

**결과**: 초기화 4회 → 2회 (clear_sheet는 다른 API 엔드포인트라 분리 유지)

### 3.3 대시보드 헤더를 데이터와 병합 (절감: -3회)

**현재**: 월별 성과, 월별 추이, 종목별 현황에서 헤더와 데이터를 별도 API로 작성

```python
# 현재: 헤더 PUT + 데이터 POST = 2회
await self.client.update_cells(DASHBOARD_SHEET, f"A{start_row}", [headers])
await self.client.batch_update_cells(DASHBOARD_SHEET, {f"A{start_row+1}:K{end}": rows})
```

**개선**: 헤더 행을 데이터 행과 합쳐서 1회로 작성

```python
# 변경 후: 1회
all_rows = [headers] + rows
await self.client.batch_update_cells(DASHBOARD_SHEET, {f"A{start_row}:K{end}": all_rows})
```

**결과**: 데이터 12회 → 9회 (절감 3회)

### 3.4 CSV 처리 시트 포맷팅 병합 (절감: -CSV 파일 수 × 2회)

**현재**: `ensure_sheet_exists()`에서 `freeze_rows()` + `set_auto_filter()` + `clear_background_colors()` 각각 호출

```python
# 현재: 4회 (freeze 1 + filter 2 + clear_bg 1)
await self.client.freeze_rows(sheet_name, 1)
await self.client.set_auto_filter(sheet_name, 1, 1, num_cols)
await self.client.clear_background_colors(sheet_name)
```

**개선**: 3개를 1회 batchUpdate로 병합, `set_auto_filter`의 clearBasicFilter도 동일 요청에 포함

```python
# 변경 후: 1회
requests = [
    build_freeze_request(sheet_id, 1),
    {'clearBasicFilter': {'sheetId': sheet_id}},
    build_filter_request(sheet_id, 1, 1, num_cols),
    build_clear_bg_request(sheet_id),
]
await self.client.execute_batch_requests(requests)
```

**결과**: 파일당 4회 → 1회 (7개 파일 기준 28회 → 7회, 절감 21회)

### 3.5 429 에러 재시도 (안전망)

최적화 후에도 파일 수가 많아지면 제한에 도달할 수 있으므로, **exponential backoff 재시도** 로직을 추가한다.

```python
# google_sheets_client.py에 데코레이터/유틸 추가
async def _execute_with_retry(self, request_fn, max_retries=3):
    for attempt in range(max_retries + 1):
        try:
            return request_fn().execute()
        except HttpError as e:
            if e.resp.status == 429 and attempt < max_retries:
                wait = 2 ** attempt * 10  # 10s, 20s, 40s
                logger.warning(f"Rate limit 도달, {wait}초 후 재시도 ({attempt+1}/{max_retries})")
                await asyncio.sleep(wait)
            else:
                raise
```

## 4. 기대 효과

### 최적화 전후 쓰기 횟수 비교

| 구분 | 현재 | 최적화 후 | 절감 |
|------|------|----------|------|
| CSV 처리 (7파일) | 43회 | 15회 | -28회 |
| └ 파일당 시트 포맷팅 | 4회 | 1회 | -3회 × 7 |
| └ 파일당 데이터 삽입 | 1회 | 1회 | 변동 없음 |
| └ 파일당 숫자 포맷 | 1~N회 | 1~N회 | 변동 없음 |
| 대시보드 초기화 | 4회 | 2회 | -2회 |
| 대시보드 데이터 | 12회 | 9회 | -3회 |
| 대시보드 포맷/색상 | 17회 | 1회 | -16회 |
| 대시보드 차트 | 1회 | 1회 | 변동 없음 |
| **합계** | **77회** | **28회** | **-49회** |

> 60회 제한 대비 **28회**로 충분한 여유 확보. 계좌(CSV 파일)가 2배로 늘어도 ~42회로 안전.

### 확장성 안전선

| CSV 파일 수 | 현재 (예상) | 최적화 후 (예상) | 제한(60) 이내 |
|------------|------------|----------------|-------------|
| 7개 (현재) | 77회 | 28회 | O |
| 10개 | ~100회 | ~34회 | O |
| 15개 | ~140회 | ~44회 | O |
| 20개 | ~180회 | ~54회 | O |
| 25개 | ~220회 | ~64회 | X → 재시도로 처리 |

## 5. 기술 변경 사항

### 5.1 google_sheets_client.py - 신규/변경 메서드

| 메서드 | 유형 | 용도 |
|--------|------|------|
| `build_number_format_requests()` | 신규 (static) | repeatCell 요청 객체 생성 (API 미호출) |
| `build_color_requests()` | 신규 (static) | 색상 repeatCell 요청 객체 생성 |
| `execute_batch_requests(requests)` | 신규 | 수집된 요청 리스트를 1회 batchUpdate로 전송 |
| `apply_sheet_formatting_batch()` | 신규 | freeze + filter + clear_bg를 1회 호출로 병합 |
| `_execute_with_retry()` | 신규 | 429 에러 시 exponential backoff 재시도 |

### 5.2 summary_generator.py - 변경 사항

| 변경 대상 | 설명 |
|----------|------|
| `generate_all()` | sheet_id 1회 조회 후 캐시, 마지막에 `_flush_pending_requests()` 호출 |
| `_ensure_dashboard_sheet()` | clear_bg + clear_fmt + delete_charts를 1회 batchUpdate로 병합 |
| `_apply_dashboard_formats()` | API 호출 → `_pending_format_requests`에 수집 |
| `_apply_metrics_formats()` | API 호출 → `_pending_format_requests`에 수집 |
| `_apply_header_colors()` | API 호출 → `_pending_format_requests`에 수집 |
| `_write_monthly_summary()` | 헤더를 데이터 행과 합쳐서 1회로 작성 |
| `_write_monthly_trend()` | 동일 |
| `_write_stock_summary()` | 동일 |
| `_write_trade_count_data()` | 포맷 요청을 `_pending_format_requests`에 수집 |
| `_flush_pending_requests()` | 신규 - 수집된 포맷/색상 요청을 1회 전송 |

### 5.3 sheet_writer.py - 변경 사항

| 변경 대상 | 설명 |
|----------|------|
| `apply_sheet_formatting()` | freeze + filter + clear_bg → `apply_sheet_formatting_batch()` 1회 호출 |

## 6. 구현 우선순위

| 순위 | 작업 | 절감 | 난이도 | 위험도 |
|------|------|------|--------|--------|
| 1 | 대시보드 포맷 일괄 처리 (3.1) | -16회 | 중 | 낮음 |
| 2 | CSV 시트 포맷팅 병합 (3.4) | -21회 | 중 | 낮음 |
| 3 | 대시보드 헤더/데이터 병합 (3.3) | -3회 | 낮음 | 낮음 |
| 4 | 대시보드 초기화 병합 (3.2) | -2회 | 낮음 | 낮음 |
| 5 | 429 재시도 로직 (3.5) | 안전망 | 중 | 낮음 |

> 1~4번만 적용해도 77회 → 28회로 감소하여 문제 해결. 5번은 향후 파일 증가 대비 안전망.

## 7. 테스트 계획

- [ ] 최적화 후 `run.sh` 실행 시 429 에러 없이 완료 확인
- [ ] 대시보드 차트 6개가 정상 생성되는지 확인
- [ ] 대시보드 헤더 배경색, 숫자 포맷이 정상 적용되는지 확인
- [ ] `--log-level DEBUG`로 실행하여 총 쓰기 요청 수 카운트 (목표: 30회 이하)
- [ ] `pytest` 기존 테스트 통과 확인
- [ ] 해외 계좌 CSV (다중 통화) 포맷 정상 적용 확인
