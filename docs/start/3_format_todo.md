# 대시보드 시트 숫자 포맷 수정 — TODO

## Phase 1: 근본 원인 해결 — 숫자 포맷 초기화

- [x] `google_sheets_client.py`에 `clear_number_formats()` 메서드 추가
  - `clear_background_colors()`와 동일 패턴, `fields: 'userEnteredFormat.numberFormat'`
- [x] `summary_generator.py`의 `_ensure_dashboard_sheet()`에 `clear_number_formats()` 호출 추가
- [ ] 실행하여 기존 수동 포맷이 제거되는지 확인 (`--dry-run` 불가, 실제 시트 확인)

## Phase 2: 섹션 4·5 기본 포맷 선적용

- [x] `_apply_metrics_formats()`에 `total_rows` 파라미터 추가
- [x] 메서드 내 1단계: 섹션 전체 B열을 기본 NUMBER 포맷(`#,##0.##`)으로 초기화
- [x] 메서드 내 2·3단계: 기존 pct/krw 개별 포맷 적용 (로직 유지)
- [x] `_write_investment_metrics()` 호출부에 `len(rows)` 전달
- [x] `_write_trading_insights()` 호출부에 `len(rows)` 전달

## Phase 3: 테스트 추가

- [x] `tests/test_summary_format.py` 신규 생성
- [x] `_group_consecutive_rows()` 단위 테스트 (single, consecutive, gaps, empty)
- [x] `_write_investment_metrics` offset 검증 테스트 (mock client로 pct_rows/krw_rows 값 확인)
- [x] `pytest` 전체 테스트 통과 확인 (81 passed)

## Phase 4: 실제 시트 검증

- [ ] `python main.py` 실행
- [ ] 대시보드 > 월별 성과: 수익률(%) 컬럼이 `0.00%` 포맷인지 확인
- [ ] 대시보드 > 월별 성과: 매수금액/매도금액/실현손익이 `₩#,##0` 포맷인지 확인
- [ ] 대시보드 > 투자 지표 > 섹터별 투자비중: `2.26%` 형태로 표시되는지 확인
- [ ] 대시보드 > 투자 지표 > 상위 5종목 집중도 / 평균 수익률 / 평균 손실률: `%` 포맷 확인
- [ ] 대시보드 > 투자 지표 > 수익 Top 10: `₩#,##0` 포맷 확인
- [ ] 대시보드 > 투자 지표 > 손실 Top 10: `₩#,##0` 포맷 확인 (음수도 정상 표시)
