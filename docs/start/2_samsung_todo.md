# 삼성증권 CSV 파서 추가 - TODO

## Phase 1: 인코딩 및 헤더 감지 개선

- [ ] `parser_registry.py` — `_open_csv()` 인코딩 fallback 함수 추가 (UTF-8 → EUC-KR)
- [ ] `parser_registry.py` — `detect_parser()` 최대 10행까지 순회하며 헤더 매칭하도록 수정
- [ ] 기존 테스트 실행하여 미래에셋/한국투자증권 파서 회귀 없는지 확인

## Phase 2: 삼성증권 파서 구현

- [ ] `modules/parsers/samsung_parser.py` 파일 생성
- [ ] `SamsungDomesticParser.can_parse()` — 키워드 `{"거래일자", "거래명", "종목명"}` 매칭
- [ ] `SamsungDomesticParser.parse()` — EUC-KR 인코딩으로 파일 열기
- [ ] `parse()` — 메타데이터 4줄 건너뜀 (헤더 행 탐색)
- [ ] `parse()` — 페이지 구분자 (`^\d+/\d+$`) 건너뜀
- [ ] `parse()` — 반복 헤더 건너뜀
- [ ] `parse()` — 종료 마커 (`- 출력 끝 -`) 건너뜀
- [ ] `parse()` — 쌍따옴표+쉼표 숫자 파싱 (`"20,360"` → `20360.0`)
- [ ] `parse()` — Trade 객체 생성 (손익 필드 = 0.0)

## Phase 3: 파서 등록

- [ ] `modules/parsers/__init__.py` — `SamsungDomesticParser` export 추가
- [ ] `modules/parser_registry.py` — `PARSERS` 리스트에 `SamsungDomesticParser` 등록

## Phase 4: 테스트

- [ ] `tests/test_parsers.py` — `TestSamsungDomesticParser` 클래스 추가
  - [ ] `test_can_parse` — 삼성증권 헤더 인식
  - [ ] `test_can_parse_rejects_mirae` — 다른 증권사 헤더 거부
  - [ ] `test_parse_sample` — 실제 CSV 파싱 (EUC-KR)
  - [ ] `test_skip_metadata_and_pagination` — 메타데이터/페이지 구분자/종료 마커 스킵
  - [ ] `test_buy_trade_fields` — 매수 거래 필드 검증
  - [ ] `test_fee_and_tax` — 수수료/제세금 파싱
  - [ ] `test_profit_is_zero` — 손익 항상 0
  - [ ] `test_quoted_comma_numbers` — 쌍따옴표+쉼표 숫자 처리
  - [ ] `test_date_format_unchanged` — YYYY-MM-DD 유지
  - [ ] `test_to_domestic_row` — 9컬럼 반환
- [ ] `tests/test_parsers.py` — `TestParserRegistry`에 삼성증권 감지 테스트 추가
- [ ] 전체 테스트 실행 (`pytest`) — 기존 테스트 포함 전부 통과 확인

## Phase 5: 문서 업데이트

- [ ] `CLAUDE.md` — Architecture 섹션에 `SamsungDomesticParser` 추가
- [ ] `CLAUDE.md` — Module Responsibilities에 `samsung_parser.py` 설명 추가
- [ ] `CLAUDE.md` — Input File Format에 `삼성증권/` 디렉토리 추가
