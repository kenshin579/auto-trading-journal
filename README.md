# Auto Trading Journal

주식 매매일지를 마크다운 파일에서 자동으로 파싱하여 구글 시트에 입력하는 시스템입니다.

## 개요

수동으로 거래 내역을 입력하는 번거로움을 없애고, 정확한 매매 기록을 자동으로 관리할 수 있도록 도와주는 Python 애플리케이션입니다.

### 주요 기능

- **자동 파싱**: 탭으로 구분된 마크다운 파일에서 거래 데이터 자동 추출
- **지능형 분류**: GPT-4 API와 키워드 기반 분류를 통한 주식/ETF 자동 구분
- **중복 방지**: 종목명과 날짜를 기준으로 중복 거래 자동 감지 및 제외
- **데이터 검증**: 날짜, 수량, 금액 등의 유효성 자동 검증
- **시각적 구분**: 날짜별 배경색 자동 지정으로 거래 시각화
- **다중 계좌 지원**: 일반 계좌, ISA, IRP, 연금 계좌 등 여러 계좌 동시 관리
- **국내외 거래 지원**: 국내 주식/ETF 및 해외 주식/ETF (다중 통화 지원)
- **상세 리포트**: 처리 결과 요약 및 상세 리포트 자동 생성

## 시스템 요구사항

- Python 3.8 이상
- Google Sheets API 접근 권한
- (선택) OpenAI API 키 (향상된 분류를 위해)

## 설치

1. 저장소 클론:
```bash
git clone https://github.com/your-username/auto-trading-journal.git
cd auto-trading-journal
```

2. 가상환경 생성 및 활성화:
```bash
python -m venv .venv
source .venv/bin/activate  # Windows: .venv\Scripts\activate
```

3. 의존성 설치:
```bash
pip install -r requirements.txt
```

## 설정

### 1. Google Sheets API 설정 

1. [Google Cloud Console](https://console.cloud.google.com/)에서 프로젝트 생성
2. Google Sheets API 활성화
3. 서비스 계정 생성 및 JSON 키 파일 다운로드
4. JSON 키 파일을 안전한 위치에 저장 (예: `~/.config/google_sheet/service_account_key.json`)
5. Google Sheets에 서비스 계정 이메일 공유 (편집자 권한)

### 2. 설정 파일 구성

`config/config.yaml` 파일을 생성하고 다음과 같이 설정:

```yaml
google_sheets:
  spreadsheet_id: YOUR_SPREADSHEET_ID_HERE
  service_account_path: /path/to/service_account_key.json

logging:
  level: INFO

batch_size: 10
empty_row_threshold: 100
stock_type_cache_file: stock_type_cache.json
```

### 3. 환경 변수 설정 (선택)

더 나은 주식/ETF 분류를 위해 OpenAI API 키를 설정할 수 있습니다:

```bash
export OPENAI_API_KEY=your_openai_api_key_here
```

## 사용 방법

### 1. 매매일지 파일 준비

`stocks/` 디렉토리에 매매일지 마크다운 파일을 배치합니다.

**국내 주식 형식 예시** (`stocks/계좌1 국내.md`):

```
일자	종목명	기간 중 매수			기간 중 매도			매매비용	손익금액	수익률
		수량	평균단가	매수금액	수량	평균단가	매도금액
2025/10/17	삼성전자	100	50000	5000000	0	0	0	0	0	0.00
2025/10/17	KODEX 200	0	0	0	50	20000	1000000	150	50000	5.13
```

**해외 주식 형식 예시** (`stocks/계좌1 해외.md`):

```
일자	종목명	통화	티커	잔고수량	평균매입환율	거래환율	기간 중 매수				기간 중 매도				거래수수료 + 제세금		합계		손익
								수량	평균단가	매수금액	매수금액(원)	수량	평균단가	매도금액	매도금액(원)	거래수수료	제세금	합계(원)	평가손익	수익률
2025/10/15	APPLE INC	USD	AAPL	10	1300.00	1315.50	10	150.00	1500.00	1973250	0	0.00	0.00	0	25.00	0.00	25.00	32888	0.00	0.00
```

### 2. 실행

**기본 실행**:
```bash
./run.sh
```

또는 직접 Python으로:
```bash
python main.py
```

**드라이런 모드** (실제로 시트를 수정하지 않고 미리보기):
```bash
python main.py --dry-run
```

**디버그 로깅**:
```bash
python main.py --log-level DEBUG
```

### 3. 결과 확인

- **콘솔 출력**: 처리 진행 상황과 결과 요약
- **구글 시트**: 자동으로 입력된 거래 내역 (날짜별 색상 구분)
- **리포트 파일**: `reports/` 디렉토리에 자동 생성
  - `summary_report_YYYYMMDD_HHMMSS.txt`: 요약 리포트
  - `detailed_report_YYYYMMDD_HHMMSS.txt`: 상세 리포트

## 프로젝트 구조

```
auto-trading-journal/
├── main.py                      # 메인 진입점
├── test_main.py                 # 통합 테스트
├── run.sh                       # 실행 스크립트 (로깅 포함)
├── pyproject.toml               # 프로젝트 메타데이터
├── requirements.txt             # Python 의존성
├── config/
│   └── config.yaml              # 설정 파일
├── modules/                     # 핵심 모듈
│   ├── trade_models.py          # 거래 데이터 모델
│   ├── file_parser.py           # 파일 파싱
│   ├── google_sheets_client.py  # Google Sheets API 클라이언트
│   ├── stock_classifier.py      # 주식/ETF 분류
│   ├── data_validator.py        # 데이터 검증
│   ├── sheet_manager.py         # 시트 관리
│   └── report_generator.py      # 리포트 생성
├── stocks/                      # 매매일지 파일
├── reports/                     # 생성된 리포트
├── logs/                        # 실행 로그
├── docs/                        # 문서
│   ├── prd.md                   # 제품 요구사항 명세서
│   └── foreign_stock_design.md  # 해외 주식 설계 문서
└── stock_type_cache.json        # 주식 분류 캐시
```

## 아키텍처

시스템은 다음과 같은 모듈형 서비스 아키텍처로 구성되어 있습니다:

```
┌─────────────────────────────────────────┐
│     StockDataProcessor (메인 조정자)    │
└──────────┬──────────────────────────────┘
           │
    ┌──────┼──────┬──────┬──────┬──────┐
    ▼      ▼      ▼      ▼      ▼      ▼
  파일   데이터  주식   시트   리포트  Google
  파서   검증기  분류기  관리자  생성기  Sheets
```

### 데이터 처리 파이프라인

1. **파일 파싱**: MD 파일에서 거래 데이터 추출
2. **데이터 검증**: 날짜, 수량, 금액 등 유효성 검사
3. **주식 분류**: 캐시 → OpenAI API → 키워드 기반 순차 분류
4. **중복 확인**: 기존 시트와 비교하여 중복 거래 제거
5. **일괄 삽입**: 신규 거래 내역 구글 시트에 삽입
6. **리포트 생성**: 처리 결과 요약 및 상세 리포트 생성

## 지원하는 거래 유형

### 국내 거래
- 국내 주식 (KRX)
- 국내 ETF (KODEX, TIGER, SOL, KBSTAR, ARIRANG, HANARO, ACE 등)

### 해외 거래
- 해외 주식 (미국, 유럽, 일본, 중국, 홍콩 등)
- 해외 ETF (SPY, QQQ, IWM, VTI, TLT, GLD, JEPI 등)
- 지원 통화: USD, EUR, JPY, CNY, HKD, GBP, CAD, AUD

## 주요 기능 상세

### 지능형 주식/ETF 분류

3단계 분류 전략:
1. **캐시 조회**: 이미 분류된 800+ 증권 즉시 반환
2. **AI 분류**: GPT-4를 통한 정확한 분류
3. **키워드 기반**: ETF 키워드 매칭 (국내 100+, 해외 200+ 티커)

### 중복 방지 메커니즘

- (종목명 + 날짜) 조합으로 중복 확인
- 이미 입력된 거래는 자동으로 건너뜀
- 중복 건너뛴 거래 리포트에 기록

### 데이터 검증

- 날짜 형식 검증 (YYYY-MM-DD)
- 수량, 가격 양수 검증
- 총액 계산 검증 (±0.1% 또는 ±10원 ���용)
- 거래 유형 검증 (매수/매도)

### 시각적 조직화

- 8색 팔레트를 사용한 날짜별 배경색 지정
- 같은 날짜의 거래는 동일한 색상
- 거래 날짜를 시각적으로 빠르게 구분

## 개발

### 테스트 실행

```bash
pytest test_main.py
pytest modules/test_foreign.py
pytest modules/test_sheet_manager.py
```

### 로깅 레벨

- **DEBUG**: 상세한 작업 추적, API 응답
- **INFO**: 처리 진행 상황, 배치 요약
- **WARNING**: 잘못된 데이터, 재시도, 폴백
- **ERROR**: 주의가 필요한 치명적 오류

### 코드 스타일

- PEP 8 준수
- Type hints 사용
- Async/await 패턴 활용
- 단일 책임 원칙 (SRP)

## 의존성

주요 라이브러리:
- `google-api-python-client`: Google Sheets API 접근
- `google-auth`: Google 인증
- `openai`: GPT-4 API (선택)
- `pyyaml`: YAML 설정 파싱
- `colorlog`: 컬러 콘솔 로깅
- `ujson`: 빠른 JSON 처리
- `python-dateutil`: 날짜/시간 유틸리티

전체 의존성은 [requirements.txt](requirements.txt)를 참조하세요.

## 문서

- [PRD (제품 요구사항 명세서)](docs/prd.md) - 상세한 기능 명세 및 요구사항
- [해외 주식 설계 문서](docs/foreign_stock_design.md) - 해외 주식 기능 설계

## 향후 개선 계획

- [ ] 실시간 모니터링 대시보드
- [ ] 자동 손익 계산 및 차트
- [ ] 세금 계산 지원
- [ ] 다중 증권사 형식 지원
- [ ] 웹 UI 구성 인터페이스
- [ ] 완료 시 이메일 알림
- [ ] CI/CD 파이프라인 구축
- [ ] Docker 컨테이너화

## 문제 해결

### OpenAI API 오류

OpenAI API를 사용할 수 없는 경우, 시스템은 자동으로 키워드 기반 분류로 폴백합니다.

```
WARNING - OpenAI API를 사용할 수 없습니다. 키워드 기반 분류를 사용합니다.
```

### Google Sheets API 인증 오류

서비스 계정 JSON 파일 경로를 확인하고, 해당 서비스 계정에 Google Sheets 편집 권한이 있는지 확인하세요.

### 중복 거래가 계속 입력됨

종목명과 날짜가 정확히 일치하는지 확인하세요. 공백이나 대소문자 차이가 있으면 중복으로 인식되지 않습니다.

## 기여

기여는 언제나 환영합니다! Pull Request를 보내주세요.

1. Fork the Project
2. Create your Feature Branch (`git checkout -b feature/AmazingFeature`)
3. Commit your Changes (`git commit -m 'Add some AmazingFeature'`)
4. Push to the Branch (`git push origin feature/AmazingFeature`)
5. Open a Pull Request

## 라이선스

이 프로젝트는 MIT 라이선스를 따릅니다.

## 연락처

프로젝트 링크: [https://github.com/your-username/auto-trading-journal](https://github.com/your-username/auto-trading-journal)

---

**주의**: 이 프로젝트는 개인 투자 기록 관리용으로 설계되었습니다. 실제 투자 결정이나 세무 신고에 사용하기 전에 데이터의 정확성을 반드시 확인하세요.
