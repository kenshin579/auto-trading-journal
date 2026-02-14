# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Auto Trading Journal (v2)은 증권사별 CSV 파일을 파싱하여 구글 시트에 자동으로 매매일지를 작성하는 Python 애플리케이션입니다. 국내/해외 주식을 지원하며, 증권사별 CSV 형식을 자동 감지합니다.

## Quick Start Commands

### Setup
```bash
# Create and activate virtual environment
python3 -m venv .venv
source .venv/bin/activate

# Install dependencies
pip install -e .
```

### Running the Application
```bash
# Run with default settings
python main.py

# Run with dry-run mode (no actual sheet updates)
python main.py --dry-run

# Run with debug logging
python main.py --log-level DEBUG

# Run using the script (includes timestamps and logging)
./run.sh
```

### Testing
```bash
# Run all tests
pytest

# Run parser tests
pytest tests/test_parsers.py

# Run with verbose output
pytest -v
```

## Architecture Overview

### Core Components

```
StockDataProcessor (main.py)
├── ParserRegistry  - CSV 헤더 기반 파서 자동 감지
│   ├── MiraeDomesticParser  - 미래에셋 국내
│   ├── MiraeForeignParser   - 미래에셋 해외
│   └── HankookDomesticParser - 한국투자증권 국내
├── SheetWriter     - 시트 생성/중복필터/데이터삽입/색상적용
├── SummaryGenerator - 요약_월별, 요약_종목별 시트 생성
└── GoogleSheetsClient - Google Sheets API v4 래퍼
```

### Data Processing Pipeline

1. **CSV 스캔**: `input/{증권사명}/` 하위 CSV 파일 탐색 (`sample/` 제외)
2. **파서 감지**: CSV 헤더를 읽어 파서 자동 선택
3. **파싱**: 증권사 형식에 맞춰 Trade 객체 리스트 생성
4. **시트 확인**: 시트가 없으면 자동 생성 + 헤더 삽입
5. **중복 필터**: 기존 시트 데이터와 비교하여 중복 제거
6. **데이터 삽입**: 신규 거래 일괄 삽입 + 날짜별 색상 적용
7. **요약 갱신**: 월별/종목별 요약 시트 초기화 후 재작성

### Key Data Model

**Trade** (dataclass):
- 16개 필드: date, trade_type, stock_name, stock_code, quantity, price, amount, currency, exchange_rate, amount_krw, fee, tax, profit, profit_krw, profit_rate, account
- `to_domestic_row()`: 국내 9컬럼 행 반환
- `to_foreign_row()`: 해외 15컬럼 행 반환
- `duplicate_key()`: (date, trade_type, stock_name, quantity, price) 튜플

### Module Responsibilities

**modules/models.py**:
- Trade dataclass 정의
- 국내/해외 행 변환, 중복 키 생성

**modules/parsers/base_parser.py**:
- BaseParser ABC (can_parse, parse 추상 메서드)

**modules/parsers/mirae_parser.py**:
- MiraeDomesticParser: 헤더 `일자, 종목명, 기간 중 매수` 감지, 서브헤더 건너뜀
- MiraeForeignParser: 헤더 `매매일, 통화, 종목번호` 감지, 다중 통화 지원

**modules/parsers/hankook_parser.py**:
- HankookDomesticParser: 헤더 `매매일자, 종목코드, 매입단가` 감지
- 쌍따옴표/천단위 쉼표 처리

**modules/parser_registry.py**:
- detect_parser(): CSV 첫 행 읽어 파서 자동 선택

**modules/sheet_writer.py**:
- ensure_sheet_exists(): 시트 자동 생성 + 헤더 삽입
- get_existing_keys(): 중복 체크용 키 셋 반환
- insert_trades(): 데이터 삽입 + 날짜별 색상 적용
- 8색 팔레트, 국내/해외 헤더 상수

**modules/summary_generator.py**:
- generate_monthly_summary(): (연월, 계좌) 기준 집계 → 요약_월별 시트
- generate_stock_summary(): (종목명, 종목코드, 계좌, 통화) 기준 집계 → 요약_종목별 시트
- 매 실행 시 시트 초기화 후 재작성

**modules/google_sheets_client.py**:
- Google Sheets API v4 래퍼
- 서비스 계정 인증, 비동기 컨텍스트 관리자
- 시트 생성/삭제, 배치 업데이트, 색상 적용

## Configuration

### Required Files

**config/config.yaml**:
```yaml
google_sheets:
  spreadsheet_id: YOUR_SPREADSHEET_ID
  service_account_path: /path/to/service_account_key.json

logging:
  level: INFO
```

**Environment Variables** (optional override):
- `GOOGLE_SPREADSHEET_ID`: 스프레드시트 ID
- `SERVICE_ACCOUNT_PATH`: 서비스 계정 키 파일 경로

### Google Sheets Setup

1. Google Cloud Console에서 서비스 계정 생성
2. Google Sheets API 활성화
3. JSON 키 파일 다운로드
4. 대상 스프레드시트에 서비스 계정 이메일 편집자 권한 부여

### Sheet Structure

하나의 Google Spreadsheet 내에 모든 시트가 탭으로 존재:
```
[미래에셋증권_국내계좌] [미래에셋증권_해외계좌] [한국투자증권_국내계좌] [요약_월별] [요약_종목별]
```

시트 이름 = `{증권사 폴더명}_{CSV 파일명(확장자 제외)}`

## Input File Format

CSV 파일을 `input/{증권사명}/` 디렉토리에 배치:
```
input/
├── 미래에셋증권/
│   ├── 국내계좌.csv
│   └── 해외계좌.csv
├── 한국투자증권/
│   └── 국내계좌.csv
└── sample/          ← 처리 제외
```

## Git Branch Policy

**NEVER commit directly to main/master branch.** Always use feature branches.

1. **Create feature branch before making changes**:
   ```bash
   git checkout main && git pull origin main
   git checkout -b feature/{issue-number}-{feature-name}
   ```

2. **Branch naming conventions**:
   - `feature/{issue-number}-{name}` or `feat/{name}` - New features
   - `fix/{issue-number}-{name}` or `fix/{name}` - Bug fixes
   - `chore/{name}` - Maintenance tasks

3. **After completing work, create PR via `gh` CLI** (not GitHub MCP):
   ```bash
   gh pr create --assignee kenshin579 --title "type: 작업 요약" --body "$(cat <<'EOF'
   ## Summary
   - 변경 사항

   ## Test plan
   - [ ] 테스트 항목
   EOF
   )"
   ```

## Important Implementation Notes

### Async/Await Pattern
- `async with self.client` 컨텍스트 관리자 사용
- 시트 작업은 모두 async: `await self.sheet_writer.ensure_sheet_exists()`
- 진입점: `asyncio.run(processor.run())`

### Duplicate Detection
`(date, trade_type, stock_name, quantity, price)` 5-tuple로 중복 판별

### Color Coding
8색 팔레트가 날짜별로 순환. 같은 날짜 = 같은 색상

### Adding a New Broker Parser

1. `modules/parsers/` 에 새 파서 파일 생성
2. `BaseParser` 상속, `can_parse()` 와 `parse()` 구현
3. `modules/parsers/__init__.py` 에 export 추가
4. `modules/parser_registry.py` 의 `PARSERS` 리스트에 등록
5. `tests/test_parsers.py` 에 테스트 추가

### Text Encoding (Korean Content)

**Encoding Standard**: All files MUST be UTF-8 encoded (한글 콘텐츠 필수)

1. **Verify encoding after file creation**:
   ```bash
   file -I path/to/file.md
   ```

2. **If encoding is broken (charset=binary)**:
   ```bash
   cat > file.md << 'EOF'
   한글 내용...
   EOF
   ```

## Troubleshooting

### "파서 감지 실패"
CSV 헤더가 지원되는 형식과 일치하지 않음. `--log-level DEBUG`로 헤더 확인.

### "시트를 찾을 수 없습니다"
서비스 계정에 스프레드시트 편집 권한이 있는지 확인.

### Duplicate trades keep inserting
(date, trade_type, stock_name, quantity, price) 5개 필드의 정확한 일치 여부 확인.

### Service account authentication errors
1. `config/config.yaml`의 JSON 키 파일 경로 확인
2. 서비스 계정 이메일에 편집자 권한 부여 확인
3. Google Cloud Console에서 Sheets API 활성화 확인
