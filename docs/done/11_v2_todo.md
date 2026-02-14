# v2 시트 포맷 개선 및 대시보드 - TODO

## Phase 1: Google Sheets 클라이언트 확장

- [x] `google_sheets_client.py`에 `_get_sheet_id()` 헬퍼 메서드 추가 (캐시 포함)
- [x] 기존 `apply_color_to_range()`, `apply_number_format()`, `batch_apply_colors()`에서 시트 ID 조회를 `_get_sheet_id()` 호출로 교체
- [x] `freeze_rows(sheet_name, row_count)` 메서드 추가
- [x] `set_auto_filter(sheet_name, start_row, start_col, end_col)` 메서드 추가
- [x] `apply_number_format_to_columns(sheet_name, column_formats, start_row, end_row)` 메서드 추가
- [x] `get_sheet_data()`에 `value_render_option` 파라미터 추가 (기본값: `FORMATTED_VALUE`)

## Phase 2: 데이터 모델 수정

- [x] `models.py`의 `to_domestic_row()`에서 `profit_rate`를 `profit_rate / 100`으로 변환
- [x] `models.py`의 `to_foreign_row()`에서 `profit_rate`를 `profit_rate / 100`으로 변환
- [x] `duplicate_key()`가 영향받지 않는지 확인 (변경 불필요)

## Phase 3: 매매일지 시트 포맷 적용

- [x] `sheet_writer.py`에 `DOMESTIC_FORMATS`, `FOREIGN_FORMATS` 상수 정의
- [x] `sheet_writer.py`에 `apply_sheet_formatting(sheet_name, is_foreign)` 메서드 추가 (freeze + filter)
- [x] `ensure_sheet_exists()`에서 `apply_sheet_formatting()` 호출 추가 (신규/기존 시트 모두)
- [x] `insert_trades()`에서 데이터 삽입 후 `_apply_number_formats()` 호출 추가
- [x] `_apply_number_formats(sheet_name, start_row, end_row, is_foreign)` 메서드 추가
- [x] `get_existing_keys()`에서 `value_render_option='UNFORMATTED_VALUE'` 사용하도록 변경

## Phase 4: 대시보드 시트 구현

- [x] `summary_generator.py`에서 `MONTHLY_SHEET`, `STOCK_SHEET` 상수를 `DASHBOARD_SHEET`로 교체
- [x] `_ensure_dashboard_sheet()` 메서드 구현 (시트 생성 또는 초기화)
- [x] `_write_portfolio_summary(trades, start_row)` 구현 (섹션 1: 핵심 수치)
- [x] `_write_monthly_summary(trades, start_row)` 구현 (섹션 2: 월별 성과 + 수익률 컬럼)
- [x] `_write_stock_summary(trades, start_row)` 구현 (섹션 3: 종목별 현황 + 수익률/투자비중)
- [x] `_write_investment_metrics(trades, start_row)` 구현 (섹션 4: 투자 지표)
- [x] `_apply_dashboard_formats(total_rows)` 구현 (freeze + 숫자 포맷)
- [x] `generate_all()` 메서드를 대시보드 단일 시트 로직으로 교체
- [x] 기존 `generate_monthly_summary()`, `generate_stock_summary()` 제거

## Phase 5: 테스트 및 검증

- [x] `tests/test_parsers.py` 업데이트: 수익률 퍼센트 소수 변환 검증 (국내/해외)
- [x] `pytest` 전체 실행 및 통과 확인 (26/26)
- [x] `python main.py --dry-run`으로 파이프라인 정상 동작 확인
- [x] 실제 실행하여 Google Sheets에서 결과 확인
  - [x] 헤더 고정 (freeze) 동작 확인
  - [x] 자동 필터 동작 확인
  - [x] 숫자 포맷 (천단위 구분, 퍼센트) 표시 확인
  - [x] 대시보드 시트 섹션별 데이터 정확성 확인
