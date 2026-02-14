# Auto Trading Journal v2 - TODO

## Phase 1: 데이터 모델 및 파서

- [x] `modules/models.py` - Trade 데이터 모델 작성
  - Trade dataclass 정의 (15개 필드)
  - `to_domestic_row()`, `to_foreign_row()` 메서드
  - `duplicate_key()` 메서드
  - `is_domestic()`, `is_foreign()` 헬퍼

- [x] `modules/parsers/__init__.py` 생성

- [x] `modules/parsers/base_parser.py` - 파서 추상 클래스
  - `can_parse(header)` 정적 메서드
  - `parse(file_path, account)` 추상 메서드

- [x] `modules/parsers/mirae_parser.py` - 미래에셋 국내 파서
  - 헤더 감지: `일자`, `종목명`, `기간 중 매수`
  - 서브헤더(2행) 건너뜀
  - 매수금액 > 0 → 매수 Trade, 매도금액 > 0 → 매도 Trade
  - 날짜 변환: `2026/02/13` → `2026-02-13`

- [x] `modules/parsers/mirae_parser.py` - 미래에셋 해외 파서
  - 헤더 감지: `매매일`, `통화`, `종목번호`
  - 다중 통화 지원 (USD, JPY)
  - 환율, 원화금액 매핑

- [x] `modules/parsers/hankook_parser.py` - 한국투자증권 국내 파서
  - 헤더 감지: `매매일자`, `종목코드`, `매입단가`
  - 쌍따옴표, 천단위 쉼표 처리
  - 빈 행(종목명 없음) 건너뜀

- [x] `modules/parser_registry.py` - 파서 레지스트리
  - CSV 헤더 읽기 → 파서 자동 선택
  - 미감지 시 에러 로그 + skip

- [x] 파서 단위 테스트 작성
  - `stocks/sample/` 데이터로 각 파서 검증
  - 매수/매도 동시 발생 행 테스트
  - 빈 행/잘못된 행 건너뜀 테스트

## Phase 2: Google Sheets 처리

- [x] `modules/google_sheets_client.py` 확장
  - `create_sheet(title)` 메서드 추가
  - `clear_sheet(sheet_name)` 메서드 추가

- [x] `modules/sheet_writer.py` - 시트 Writer 작성
  - `ensure_sheet_exists()`: 시트 없으면 생성 + 헤더 삽입
  - `get_existing_keys()`: 중복 체크용 키 셋 반환
  - `insert_trades()`: 신규 거래 삽입
  - 날짜별 색상 적용 (기존 8색 팔레트 재사용)
  - 국내/해외 헤더 상수 정의

- [x] `main.py` 재작성
  - `scan_csv_files()`: stocks/ 스캔 → (증권사, 계좌유형, 경로) 리스트
  - CSV 파싱 → 시트 생성 → 중복 필터 → 삽입 파이프라인
  - `--dry-run`, `--log-level` 옵션 유지

- [x] 통합 테스트 (dry-run 모드로 파이프라인 검증)

## Phase 3: 요약 시트

- [x] `modules/summary_generator.py` - 요약 시트 생성
  - `generate_monthly_summary()`: 요약_월별 시트 작성
    - (연월, 계좌) 기준 그룹핑
    - 매수건수/매수금액(원)/매도건수/매도금액(원)/실현손익(원) 집계
  - `generate_stock_summary()`: 요약_종목별 시트 작성
    - (종목명, 계좌) 기준 그룹핑
    - 누적 매수수량/매수금액/매도수량/매도금액/실현손익 집계
  - 매 실행 시 시트 초기화 후 재작성

- [x] main.py에 요약 시트 갱신 단계 연결

## Phase 4: 정리

- [ ] 기존 불필요 모듈 삭제
  - `modules/stock_classifier.py`
  - `modules/report_generator.py`
  - `modules/data_validator.py`
  - `modules/file_parser.py`
  - `modules/trade_models.py`
  - `modules/test_foreign.py`
  - `modules/test_sheet_manager.py`
  - `stock_type_cache.json`

- [ ] CLAUDE.md 업데이트 (v2 구조 반영)
