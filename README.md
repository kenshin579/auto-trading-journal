# Auto Trading Journal (v2)

증권사별 CSV 파일을 파싱하여 구글 시트에 자동으로 매매일지를 작성하는 Python 애플리케이션입니다.

## 개요

증권사에서 다운로드한 CSV 파일을 자동으로 파싱하여 구글 시트에 매매일지를 작성합니다. 증권사별 CSV 형식을 자동 감지하며, 대시보드를 통해 투자 현황을 한눈에 파악할 수 있습니다.

### 주요 기능

- **CSV 자동 파싱**: 증권사별 CSV 헤더를 분석하여 파서 자동 선택
- **다중 증권사 지원**: 미래에셋증권(국내/해외), 한국투자증권(국내)
- **중복 방지**: (날짜, 매매유형, 종목명, 수량, 단가) 5-tuple 기반 중복 감지
- **대시보드**: 포트폴리오 요약, 월별 성과, 종목별 현황, 투자 지표 자동 생성
- **섹터 분류**: OpenAI 기반 종목 섹터 자동 분류 및 투자비중 분석
- **다중 계좌 지원**: 주식, ISA, IRP, 연금저축 등 여러 계좌 동시 관리
- **국내외 거래 지원**: 국내 주식/ETF 및 해외 주식/ETF (다중 통화 지원)

## 시스템 요구사항

- Python 3.10 이상
- Google Sheets API 접근 권한
- (선택) OpenAI API 키 (섹터 분류용)

## 설치

```bash
# 저장소 클론
git clone https://github.com/kenshin579/auto-trading-journal.git
cd auto-trading-journal

# 가상환경 생성 및 활성화
python3 -m venv .venv
source .venv/bin/activate

# 의존성 설치
pip install -e .
```

## 설정

### 1. Google Sheets API 설정

1. [Google Cloud Console](https://console.cloud.google.com/)에서 프로젝트 생성
2. Google Sheets API 활성화
3. 서비스 계정 생성 및 JSON 키 파일 다운로드
4. 대상 스프레드시트에 서비스 계정 이메일 편집자 권한 부여

### 2. 설정 파일

`config/config.yaml`:

```yaml
google_sheets:
  spreadsheet_id: YOUR_SPREADSHEET_ID
  service_account_path: /path/to/service_account_key.json

logging:
  level: INFO
```

환경변수로도 설정 가능합니다 (설정 파일보다 우선):

| 변수명 | 설명 |
|--------|------|
| `GOOGLE_SPREADSHEET_ID` | Google Sheets 문서 ID |
| `SERVICE_ACCOUNT_PATH` | 서비스 계정 키 파일 경로 |
| `OPENAI_API_KEY` | OpenAI API 키 (섹터 분류용, 선택) |

## 사용 방법

### 1. CSV 파일 준비

증권사에서 다운로드한 CSV 파일을 `input/{증권사명}/` 디렉토리에 배치합니다.

```
input/
├── 미래에셋증권/
│   ├── 주식1.csv
│   ├── 주식2.csv
│   ├── 해외주식1.csv
│   ├── ISA.csv
│   ├── IRP.csv
│   ├── 연금저축1.csv
│   └── 연금저축2.csv
└── 한국투자증권/
    └── 국내계좌.csv
```

> **참고**: CSV 파일이 EUC-KR 인코딩인 경우 UTF-8로 변환이 필요합니다.
> ```bash
> iconv -f euc-kr -t utf-8 input.csv > input_utf8.csv
> ```

### 2. 실행

```bash
# 기본 실행
python main.py

# 드라이런 모드 (시트 수정 없이 미리보기)
python main.py --dry-run

# 디버그 로깅
python main.py --log-level DEBUG

# 실행 스크립트 (타임스탬프 및 로깅 포함)
./run.sh
```

### 3. 결과 확인

구글 시트에 자동으로 생성되는 탭:

```
[미래에셋증권_주식1] [미래에셋증권_해외주식1] [미래에셋증권_ISA] ... [대시보드]
```

- 시트 이름 = `{증권사 폴더명}_{CSV 파일명(확장자 제외)}`
- **대시보드**: 포트폴리오 요약, 월별 성과, 종목별 현황, 투자 지표

## 프로젝트 구조

```
auto-trading-journal/
├── main.py                          # 메인 진입점
├── run.sh                           # 실행 스크립트
├── pyproject.toml                   # 프로젝트 메타데이터
├── config/
│   ├── config.yaml                  # 설정 파일
│   └── sector_cache.json            # 섹터 분류 캐시
├── modules/
│   ├── models.py                    # Trade 데이터 모델
│   ├── parser_registry.py           # CSV 파서 자동 감지
│   ├── parsers/
│   │   ├── base_parser.py           # 파서 추상 클래스
│   │   ├── mirae_parser.py          # 미래에셋증권 (국내/해외)
│   │   └── hankook_parser.py        # 한국투자증권 (국내)
│   ├── sheet_writer.py              # 시트 생성/중복필터/데이터삽입
│   ├── summary_generator.py         # 대시보드 시트 생성
│   ├── sector_classifier.py         # OpenAI 섹터 분류
│   └── google_sheets_client.py      # Google Sheets API v4 래퍼
├── tests/
│   └── test_parsers.py              # 파서 테스트
├── input/                           # CSV 입력 파일 (gitignore)
└── docs/                            # 문서
```

## 아키텍처

```
StockDataProcessor (main.py)
├── ParserRegistry     CSV 헤더 기반 파서 자동 감지
│   ├── MiraeDomesticParser    미래에셋 국내
│   ├── MiraeForeignParser     미래에셋 해외
│   └── HankookDomesticParser  한국투자증권 국내
├── SheetWriter        시트 생성 / 중복 필터 / 데이터 삽입
├── SummaryGenerator   대시보드 시트 생성
├── SectorClassifier   OpenAI 섹터 분류
└── GoogleSheetsClient Google Sheets API v4 래퍼
```

### 데이터 처리 파이프라인

1. **CSV 스캔**: `input/{증권사명}/` 하위 CSV 파일 탐색 (`sample/` 제외)
2. **파서 감지**: CSV 헤더를 읽어 파서 자동 선택
3. **파싱**: 증권사 형식에 맞춰 Trade 객체 리스트 생성
4. **시트 확인**: 시트가 없으면 자동 생성 + 헤더 삽입
5. **중복 필터**: 기존 시트 데이터와 비교하여 중복 제거
6. **데이터 삽입**: 신규 거래 일괄 삽입
7. **대시보드 갱신**: 포트폴리오 요약, 월별/종목별 현황, 투자 지표 재작성

### 대시보드 구성

| 섹션 | 내용 |
|------|------|
| 포트폴리오 요약 | 총 매수/매도 금액, 실현손익, 수익률, 승률 |
| 월별 성과 | 연월별 매수/매도 건수, 금액, 실현손익 |
| 종목별 현황 | 종목별 매수/매도 수량, 금액, 손익, 투자비중 |
| 투자 지표 | 계좌별/통화별/섹터별 투자비중, 수익/손실 Top 10 |

## 새 증권사 파서 추가

1. `modules/parsers/`에 새 파서 파일 생성
2. `BaseParser` 상속, `can_parse()`와 `parse()` 구현
3. `modules/parsers/__init__.py`에 export 추가
4. `modules/parser_registry.py`의 `PARSERS` 리스트에 등록
5. `tests/test_parsers.py`에 테스트 추가

## 테스트

```bash
# 전체 테스트
pytest

# 파서 테스트
pytest tests/test_parsers.py

# 상세 출력
pytest -v
```

## 문제 해결

### "파서 감지 실패"
CSV 헤더가 지원되는 형식과 일치하지 않습니다. `--log-level DEBUG`로 헤더를 확인하세요.

### Google Sheets API 인증 오류
1. `config/config.yaml`의 JSON 키 파일 경로 확인
2. 서비스 계정 이메일에 편집자 권한 부여 확인
3. Google Cloud Console에서 Sheets API 활성화 확인

### Rate Limit 오류 (429)
Google Sheets API는 분당 60회 쓰기 제한이 있습니다. 1~2분 후 재실행하세요.

### 중복 거래가 계속 입력됨
`(date, trade_type, stock_name, quantity, price)` 5개 필드의 정확한 일치 여부를 확인하세요.

### CSV 인코딩 깨짐
증권사 다운로드 CSV가 EUC-KR인 경우 UTF-8로 변환 후 사용하세요.

## 라이선스

MIT License

---

**주의**: 이 프로젝트는 개인 투자 기록 관리용으로 설계되었습니다. 실제 투자 결정이나 세무 신고에 사용하기 전에 데이터의 정확성을 반드시 확인하세요.
