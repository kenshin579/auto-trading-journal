# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Auto Trading Journal is a Python application that automatically parses stock trading logs from markdown files and uploads them to Google Sheets. It supports both domestic (Korean) and foreign stock markets, with intelligent stock/ETF classification using GPT-4 and keyword-based fallback.

## Quick Start Commands

### Setup
```bash
# Create and activate virtual environment
python3 -m venv .venv
source .venv/bin/activate  # On Windows: .venv\Scripts\activate

# Install dependencies (editable mode recommended for development)
pip install -e .

# Alternative: Install from requirements.txt
# pip install -r requirements.txt
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

# Run specific test files
pytest test_main.py
pytest modules/test_foreign.py
pytest modules/test_sheet_manager.py

# Run with verbose output
pytest -v
```

## Architecture Overview

### Core Components

The system follows a **modular service-oriented architecture** with `StockDataProcessor` as the main orchestrator:

```
StockDataProcessor (main.py)
├── FileParser - Parses MD files from stocks/ directory
├── DataValidator - Validates trade data integrity
├── StockClassifier - Classifies stocks vs ETFs (cache → OpenAI → keyword fallback)
├── SheetManager - Manages Google Sheets operations
└── ReportGenerator - Generates processing reports
```

### Data Processing Pipeline

1. **File Scanning**: Scan `stocks/` directory for `.md` files
2. **File Type Detection**: Detect domestic vs foreign based on filename/headers
3. **Parsing**: Extract trade data using tab-separated format
4. **Validation**: Verify dates, quantities, amounts (±0.1% or ±10원 tolerance)
5. **Classification**: Determine stock vs ETF (3-tier: cache → AI → keywords)
6. **Duplicate Check**: Check against existing sheet data (stock_name + date)
7. **Batch Insert**: Insert non-duplicate trades with color coding by date
8. **Report Generation**: Generate summary and detailed reports in `reports/`

### Key Data Models

**BaseTrade** (abstract base class):
- `stock_name`: 종목명
- `date`: YYYY-MM-DD format
- Methods: `to_sheet_row()`, `validate()`

**DomesticTrade** (국내 주식):
- Fields: `trade_type`, `quantity`, `price`, `total_amount`
- Output: 8 columns including broker (미래에셋증권)

**ForeignTrade** (해외 주식):
- Additional fields: `currency`, `ticker`, exchange rates, commission, tax, profit metrics
- Supports: USD, EUR, JPY, CNY, HKD, GBP, CAD, AUD
- Output: Same 8-column format as domestic for consistency

### Module Responsibilities

**file_parser.py**:
- Detects file type (domestic/foreign) from filename or headers
- Extracts prefix from filename (e.g., "계좌1 국내" → "계좌1")
- Parses tab-separated markdown tables
- Returns `TradingLog` dataclass with trades list

**stock_classifier.py**:
- Maintains cache file (`stock_type_cache.json`) with 800+ pre-classified securities
- Uses OpenAI GPT-4 for AI-based classification (batch processing)
- Falls back to keyword matching for ETFs
- Domestic ETF keywords: KODEX, TIGER, SOL, KBSTAR, ARIRANG, etc.
- Foreign ETF tickers: SPY, QQQ, IWM, JEPI, SCHD, etc. (200+ tickers)

**sheet_manager.py**:
- Finds target sheets by prefix and file type
- Manages 8-color palette for date-based row coloring
- Implements duplicate detection using (stock_name, date) tuples
- Handles batch insertions with `empty_row_threshold=100`
- Uses async/await for concurrent operations

**data_validator.py**:
- Validates date formats (YYYY-MM-DD)
- Checks positive values for quantity and price
- Verifies total amount calculations (±0.1% or ±10원 tolerance)
- Validates trade types (매수/매도)

**google_sheets_client.py**:
- Wraps Google Sheets API v4
- Handles authentication via service account
- Implements async context manager for connection lifecycle
- Provides batch operations for efficiency

## Configuration

### Required Files

**config/config.yaml**:
```yaml
google_sheets:
  spreadsheet_id: YOUR_SPREADSHEET_ID
  service_account_path: /path/to/service_account_key.json

logging:
  level: INFO  # DEBUG, INFO, WARNING, ERROR

batch_size: 10
empty_row_threshold: 100
stock_type_cache_file: stock_type_cache.json
```

**Environment Variables** (optional):
- `OPENAI_API_KEY`: For enhanced stock/ETF classification

### Google Sheets Setup

1. Create service account in Google Cloud Console
2. Enable Google Sheets API
3. Download JSON key file
4. Share target spreadsheet with service account email (editor access)

### Sheet Naming Convention

Sheets must follow these patterns for auto-detection:

**Domestic** (국내):
- `{prefix} 국내 주식 매수내역`
- `{prefix} 국내 주식 매도내역`
- `{prefix} 국내 ETF 매수내역`
- `{prefix} 국내 ETF 매도내역`

**Foreign** (해외):
- `{prefix} 해외 주식 매수내역`
- `{prefix} 해외 주식 매도내역`
- `{prefix} 해외 ETF 매수내역`
- `{prefix} 해외 ETF 매도내역`

Where `{prefix}` is extracted from the filename (e.g., "계좌1", "ISA", "IRP").

## Input File Format

### Domestic Stock Format

Place files in `stocks/` directory with names like `계좌1 국내.md`:

```
일자	종목명	기간 중 매수			기간 중 매도			매매비용	손익금액	수익률
		수량	평균단가	매수금액	수량	평균단가	매도금액
2025/10/17	삼성전자	100	50000	5000000	0	0	0	0	0	0.00
2025/10/17	KODEX 200	0	0	0	50	20000	1000000	150	50000	5.13
```

### Foreign Stock Format

Place files with names like `계좌1 해외.md`:

```
일자	종목명	통화	티커	잔고수량	평균매입환율	거래환율	기간 중 매수				기간 중 매도				거래수수료 + 제세금		합계		손익
								수량	평균단가	매수금액	매수금액(원)	수량	평균단가	매도금액	매도금액(원)	거래수수료	제세금	합계(원)	평가손익	수익률
2025/10/15	APPLE INC	USD	AAPL	10	1300.00	1315.50	10	150.00	1500.00	1973250	0	0.00	0.00	0	25.00	0.00	25.00	32888	0.00	0.00
```

## Important Implementation Notes

### Async/Await Pattern
The application uses async/await throughout:
- Always use `async with self.sheet_manager` context manager
- Sheet operations are async: `await self.sheet_manager.find_target_sheets()`
- Main entry point: `asyncio.run(processor.run())`

### Error Handling Strategy
- **OpenAI API failure**: Automatically falls back to keyword-based classification
- **Missing sheets**: Logs warning but continues processing other files
- **Invalid trades**: Separated during validation, logged in detailed report
- **Duplicate trades**: Skipped automatically, counted in summary

### Duplicate Detection
Uses `(stock_name, date)` tuple for uniqueness. Whitespace and case-sensitive - ensure exact matches.

### Color Coding
8-color palette cycles based on unique dates. Same date = same color across all rows.

### Logging Levels
- **DEBUG**: API responses, cache hits, detailed parsing steps
- **INFO**: Processing progress, batch summaries, results
- **WARNING**: Invalid data, retries, fallback mechanisms
- **ERROR**: Critical failures requiring attention

## Output and Reports

### Console Output
- Real-time processing progress (`[1/5] 파일 처리 중...`)
- Category-wise insertion counts (주식_매수: X건)
- Total processing summary

### Generated Reports
Located in `reports/` directory:
- `summary_report_YYYYMMDD_HHMMSS.txt`: High-level statistics
- `detailed_report_YYYYMMDD_HHMMSS.txt`: Full validation and processing details

### Logs
Located in `logs/` directory (when using `run.sh`):
- `run_YYYYMMDD_HHMMSS.log`: Complete execution trace with timestamps

## Development Guidelines

### Code Style
- Follow PEP 8
- Use type hints (typing module)
- Maintain single responsibility principle (SRP)
- Prefer composition over inheritance

### Adding New Features

**To support a new currency**:
1. Add to `ForeignTrade.validate()` in `modules/trade_models.py`

**To add new ETF keywords**:
1. Update `self.etf_keywords` (domestic) or `self.foreign_etf_tickers` in `modules/stock_classifier.py`
2. Consider adding to cache file directly for faster lookups

**To modify sheet column format**:
1. Update `to_sheet_row()` method in respective Trade class
2. Ensure consistency between DomesticTrade and ForeignTrade (both use 8 columns)

### Testing New Parsers
1. Add test file to `stocks/` directory
2. Run with `--dry-run` to verify parsing without modifying sheets
3. Check `reports/detailed_report_*.txt` for validation issues
4. Use `--log-level DEBUG` to trace processing steps

### Text Encoding (Korean Content)

**Encoding Standard**: All files MUST be UTF-8 encoded (한글 콘텐츠 필수)

**When creating Korean/emoji content with Claude Code**:

1. **Verify encoding after file creation**:
   ```bash
   file -I path/to/file.md
   # Expected: text/plain; charset=utf-8 ✅
   # Problem:  application/octet-stream; charset=binary ❌
   ```

2. **If encoding is broken (charset=binary)**:
   ```bash
   # Option 1: Use Bash heredoc (most reliable)
   cat > file.md << 'EOF'
   한글 내용...
   EOF

   # Option 2: Re-create with Write tool
   # (usually works, but verify with file -I afterward)
   ```

3. **Prevention tips**:
   - Write tool generally handles UTF-8 correctly
   - For very large files (>5000 lines), verify encoding
   - If using Cursor/VSCode, default encoding should be UTF-8
   - System locale (`.zshrc` settings) don't affect Claude Code tools

4. **Quick encoding check**:
   ```bash
   # Check encoding
   file -I docs/**/*.md

   # View Korean content
   cat file.md | head -20
   ```

**Note**: This project contains Korean content in:
- Documentation (`docs/`)
- Markdown blog posts (`contents/`)
- Code comments and commit messages

## Troubleshooting

### "OpenAI API를 사용할 수 없습니다"
Expected behavior when `OPENAI_API_KEY` not set. System automatically falls back to keyword-based classification.

### "시트를 찾을 수 없습니다"
Verify sheet naming matches the patterns above. Check prefix extraction in logs.

### Duplicate trades keep inserting
Ensure exact match of stock_name and date. Check for trailing spaces or date format inconsistencies.

### Service account authentication errors
1. Verify JSON key file path in `config/config.yaml`
2. Confirm service account email has editor access to spreadsheet
3. Check if Google Sheets API is enabled in Google Cloud Console
