# 대시보드 데이터 소스 변경 - TODO

## Phase 1: 시트 데이터 읽기 구현

### sheet_writer.py 헬퍼 함수

- [x] `_get_num(cell, default)` 헬퍼 추가 - effectiveValue에서 숫자 추출
- [x] `_get_str(cell, default)` 헬퍼 추가 - effectiveValue에서 문자열 추출
- [x] `_extract_header_row(data)` 헬퍼 추가 - get_sheet_data 결과에서 1행 헤더 문자열 리스트 추출
- [x] `_row_to_trade(values, sheet_name, is_foreign, date_val)` 헬퍼 추가 - 행 데이터 → Trade 변환

### sheet_writer.py 메서드

- [x] `_read_trades_from_sheet(sheet_name, is_foreign)` 구현
  - get_raw_grid_data()로 A2:O10000 범위 데이터 조회
  - 날짜: formattedValue 사용 (시리얼 넘버 방지)
  - 숫자: effectiveValue.numberValue 사용 (원본 정밀도)
  - 수익률: 0.1468 → 14.68 역변환 (시트 저장 시 /100 했으므로 *100)
  - 빈 행, 불완전한 행 스킵
- [x] `read_all_trades()` 구현
  - list_sheets()로 전체 시트 목록 조회
  - 각 시트 1행 헤더를 DOMESTIC_HEADERS / FOREIGN_HEADERS와 비교
  - 일치하는 시트만 매매일지로 인식
  - 헤더 종류로 국내/해외 판별
  - _read_trades_from_sheet() 호출하여 전체 Trade 리스트 반환

## Phase 2: 대시보드 데이터 소스 변경

### main.py

- [x] `run()` 메서드의 요약 시트 갱신 부분 변경
  - 기존: `summary_generator.generate_all(all_trades)` (CSV 파싱 결과)
  - 변경: `sheet_writer.read_all_trades()` → `generate_all()`에 전달
  - CSV 파일 유무와 관계없이 시트 데이터가 있으면 대시보드 갱신
  - dry-run 모드 로그 메시지 조정

## Phase 3: 테스트

### tests/test_sheet_reader.py

- [x] `test_domestic_row_to_trade` - 국내 9컬럼 → Trade 16필드 매핑 검증
- [x] `test_foreign_row_to_trade` - 해외 15컬럼 → Trade 16필드 매핑 검증
- [x] `test_profit_rate_conversion` - 수익률 역변환 (0.1468 → 14.68)
- [x] `test_skip_empty_row` - 빈 행 스킵
- [x] `test_skip_incomplete_row` - 컬럼 부족 행 스킵
- [x] `test_header_validation_domestic` - DOMESTIC_HEADERS 일치 시 국내 인식
- [x] `test_header_validation_foreign` - FOREIGN_HEADERS 일치 시 해외 인식
- [x] `test_header_validation_skip` - 헤더 불일치 시 스킵

### 검증

- [x] `pytest` 전체 테스트 통과 확인 (49/49)
- [ ] dry-run 모드로 실행하여 정상 동작 확인
