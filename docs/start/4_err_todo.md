# Google Sheets API Rate Limit (429) 개선 TODO

## Phase 1: google_sheets_client.py 인프라

- [x] `build_number_format_requests(sheet_id, column_formats, start_row, end_row)` static 메서드 추가
- [x] `build_color_requests(sheet_id, color_ranges)` static 메서드 추가
- [x] `execute_batch_requests(requests)` 메서드 추가
- [x] `apply_sheet_formatting_batch()` 메서드 추가 — freeze + filter + clear_bg 1회 호출
- [x] `_execute_with_retry(request_fn, max_retries=3)` 메서드 추가 — 429 exponential backoff
- [x] 기존 쓰기 메서드 5개에 `_execute_with_retry()` 적용

## Phase 2: CSV 시트 포맷팅 병합 (sheet_writer.py)

- [x] `apply_sheet_formatting()` → `apply_sheet_formatting_batch()` 호출로 변경
- [x] `ensure_sheet_exists()`에서 `clear_background_colors()` 별도 호출 제거

## Phase 3: 대시보드 초기화 병합 (summary_generator.py)

- [x] `_ensure_dashboard_sheet()` — clear_bg + clear_fmt + delete_charts를 1회 `execute_batch_requests()`로 병합

## Phase 4: 대시보드 포맷 일괄 처리 (summary_generator.py)

- [x] `__init__()`에 `self._pending_requests`, `self._dashboard_sheet_id` 추가
- [x] `_apply_header_colors()` → `_collect_header_colors()`로 변경 (async 제거, `_pending_requests`에 수집)
- [x] `_apply_dashboard_formats()` → `_collect_dashboard_formats()`로 변경 (async 제거, `_pending_requests`에 수집)
- [x] `_apply_metrics_formats()` → `_collect_metrics_formats()`로 변경 (async 제거, `_pending_requests`에 수집)
- [x] `_write_investment_metrics()` — `_collect_metrics_formats()` 호출로 변경
- [x] `_write_trading_insights()` — `_collect_metrics_formats()` 호출로 변경
- [x] `_write_trade_count_data()` — 포맷 요청을 `_pending_requests`에 수집으로 변경
- [x] `_flush_pending_requests()` 메서드 추가
- [x] `generate_all()` — sheet_id 캐시 + `_flush_pending_requests()` 호출 추가

## Phase 5: 대시보드 헤더/데이터 병합 (summary_generator.py)

- [x] `_write_monthly_summary()` — 헤더 + 데이터를 1회 `batch_update_cells()`로 병합
- [x] `_write_monthly_trend()` — 동일
- [x] `_write_stock_summary()` — 동일

## Phase 6: 테스트

- [ ] `tests/test_summary_format.py` — `_collect_metrics_formats` 호출 변경에 맞춰 수정
- [ ] `tests/test_rate_limit.py` 신규 — `build_number_format_requests()` 반환 구조 검증
- [ ] `tests/test_rate_limit.py` — `build_color_requests()` 반환 구조 검증
- [ ] `tests/test_rate_limit.py` — `_execute_with_retry()` 429 재시도 동작 검증 (mock)
- [ ] `tests/test_rate_limit.py` — `apply_sheet_formatting_batch()` 요청 구조 검증 (mock)
- [ ] `pytest` 전체 테스트 통과 확인

## Phase 7: 통합 검증

- [ ] `run.sh` 실행 시 429 에러 없이 완료 확인
- [ ] 대시보드 차트 6개 정상 생성 확인
- [ ] 대시보드 헤더 배경색, 숫자 포맷 정상 적용 확인
- [ ] `--log-level DEBUG`로 총 쓰기 요청 수 카운트 (목표: 30회 이하)
- [ ] 해외 계좌 CSV (다중 통화) 포맷 정상 적용 확인
