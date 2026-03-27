# 월별 성과 추이 TODO

## Phase 1: 계산 로직 구현

- [x] `_calc_monthly_trend()` 정적 메서드 추가 (`summary_generator.py`)
  - 매도 거래를 월별로 그룹핑
  - 월별 승률, 평균 수익률/손실률, 손익비 계산
  - 월별 Profit Factor, 기대값 계산
  - 전월대비 손익 변화율 계산
  - Dict 리스트 반환

## Phase 2: 대시보드 섹션 작성

- [x] `_write_monthly_trend()` 메서드 추가 (`summary_generator.py`)
  - 헤더 11컬럼 (A~K) 작성
  - `_calc_monthly_trend()` 결과를 행 데이터로 변환
  - `batch_update_cells()`로 시트에 작성
- [x] `generate_all()` 수정
  - 섹션 순서 변경: 1→2→4→5→6(신규)→3(맨 아래로 이동)
  - `trend_start`, `stock_start` 위치에 맞게 포맷 메서드 호출 수정
- [x] `_apply_header_colors()` 수정
  - 섹션 6 헤더 행에 파란색 배경색 추가
  - 종목별 현황(섹션 3) 위치 변경 반영
- [x] `_apply_dashboard_formats()` 수정
  - 섹션 6 컬럼별 숫자 포맷 적용 (%, ₩, 소수)
  - 종목별 현황(섹션 3) 위치 변경 반영

## Phase 3: 단위 테스트

- [x] `tests/test_summary_monthly_trend.py` 생성
  - [x] `test_single_month`: 1개월 데이터 기본 지표 검증
  - [x] `test_two_months_mom_change`: 전월대비 변화율 검증
  - [x] `test_all_wins_month`: 손실 없는 월 엣지 케이스
  - [x] `test_all_losses_month`: 수익 없는 월 엣지 케이스
  - [x] `test_mixed_months`: 다수 월 독립 계산 검증
  - [x] `test_empty_sells`: 빈 리스트 처리
  - [x] `test_win_rate_calculation`: 승률 정확성
  - [x] `test_expectancy_calculation`: 기대값 공식 검증
- [x] `pytest tests/test_summary_monthly_trend.py` 전체 통과 확인 (12개)

## Phase 4: 검증

- [x] `pytest` 전체 테스트 통과 확인 (93개, 기존 81 + 신규 12)
- [ ] 실제 스프레드시트에서 대시보드 갱신 후 섹션 6 데이터 확인
