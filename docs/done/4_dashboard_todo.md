# 대시보드 차트 시각화 TODO

## Phase 1: 인프라 (google_sheets_client.py)

- [x] `get_charts(sheet_name)` 메서드 추가 — 시트의 차트 목록(chartId) 조회
- [x] `delete_all_charts(sheet_name)` 메서드 추가 — 시트의 모든 차트 배치 삭제
- [x] `add_charts(chart_requests)` 메서드 추가 — 여러 차트를 한 번의 batchUpdate로 추가

## Phase 2: 차트 빌더 (summary_generator.py)

- [x] `_build_basic_chart_spec()` 정적 메서드 추가 — Column/Line/Bar 차트 공용 스펙 빌더
- [x] `_build_pie_chart_spec()` 정적 메서드 추가 — Pie 차트 스펙 빌더

## Phase 3: 대시보드 생성 흐름 수정 (summary_generator.py)

- [x] `_ensure_dashboard_sheet()`에 `delete_all_charts()` 호출 추가
- [x] `_write_investment_metrics()`에서 계좌별 투자비중 데이터를 N~O열에 추가 작성
- [x] `_create_charts()` 메서드 추가 — 4개 차트 스펙 생성 후 `add_charts()` 호출
- [x] `generate_all()`에서 `_create_charts()` 호출 추가 (포맷 적용 후)

## Phase 4: 차트 생성

- [x] 차트 1: 월별 실현손익 추이 (Column Chart) — 섹션 5 A·C열 참조
- [x] 차트 2: 월별 승률·수익률 추이 (Line Chart) — 섹션 5 A·D·E열 참조
- [x] 차트 3: 계좌별 투자비중 (Pie Chart) — N·O열 참조
- [x] 차트 4: 손익비·Profit Factor 추이 (Line Chart) — 섹션 5 A·H·I열 참조

## Phase 5: 테스트

- [x] `_build_basic_chart_spec()` 단위 테스트 — 반환 dict 구조, 행 인덱스 검증
- [x] `_build_pie_chart_spec()` 단위 테스트 — 반환 dict 구조 검증
- [x] 통합 테스트 — 실제 시트에서 차트 4개 생성 및 재실행 시 초기화 확인
