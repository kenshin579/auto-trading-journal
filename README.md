# Auto Trading Journal

증권사별 CSV 파일을 파싱하여 구글 시트에 자동으로 매매일지를 작성하는 Go 애플리케이션입니다.

## 개요

증권사에서 다운로드한 CSV 파일을 자동으로 파싱하여 구글 시트에 매매일지를 작성합니다. 증권사별 CSV 형식을 자동 감지하며, 대시보드를 통해 투자 현황을 한눈에 파악할 수 있습니다.

> 원래 Python으로 작성되었다가 Go로 1:1 포팅되었습니다. Python 원본 이력은 git `feature/go-porting` 브랜치에서 확인할 수 있습니다.

### 주요 기능

- **CSV 자동 파싱**: 증권사별 CSV 헤더를 분석하여 파서 자동 선택
- **인코딩 자동 처리**: CP949(EUC-KR)/UTF-8 네이티브 디코딩 (별도 변환 불필요)
- **다중 증권사 지원**: 미래에셋증권(국내/해외), 한국투자증권(국내)
- **중복 방지**: (날짜, 매매유형, 종목명, 수량, 단가) 5-tuple 기반 중복 감지
- **대시보드**: 포트폴리오 요약, 월별 성과, 종목별 현황, 투자 지표, 매매 인사이트, 차트 자동 생성
- **섹터 분류**: OpenAI 기반 종목 섹터 자동 분류 및 투자비중 분석
- **다중 계좌 지원**: 주식, ISA, IRP, 연금저축 등 여러 계좌 동시 관리
- **국내외 거래 지원**: 국내 주식/ETF 및 해외 주식/ETF (다중 통화 지원)

## 시스템 요구사항

- Go 1.25 이상
- Google Sheets API 접근 권한
- (선택) OpenAI API 키 (섹터 분류용)

## 설치

```bash
git clone https://github.com/kenshin579/auto-trading-journal.git
cd auto-trading-journal
go build -o atj ./cmd/atj   # 또는 make build
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
| `STOCK_DATA_OPENAI_API_KEY` | OpenAI API 키 (섹터 분류용, 선택) |

## 사용 방법

### 1. CSV 파일 준비

증권사에서 다운로드한 CSV 파일을 `input/{증권사명}/` 디렉토리에 배치합니다. CP949/UTF-8 모두 그대로 사용할 수 있습니다(자동 디코딩).

```
input/
├── 미래에셋증권/
│   ├── 주식1.csv
│   ├── 해외주식1.csv
│   ├── ISA.csv
│   └── 연금저축1.csv
└── 한국투자증권/
    └── 국내계좌.csv
```

### 2. 실행

```bash
make run                              # 기본 실행
make dry                              # 드라이런 (시트 미반영)
go run ./cmd/atj --log-level DEBUG    # 디버그 로깅
```

CLI 플래그: `--dry-run`, `--log-level DEBUG|INFO|WARNING|ERROR`.

### 3. 결과 확인

구글 시트에 자동으로 생성되는 탭:

```
[미래에셋증권_주식1] [미래에셋증권_해외주식1] [미래에셋증권_ISA] ... [대시보드]
```

- 시트 이름 = `{증권사 폴더명}_{CSV 파일명(확장자 제외)}`
- **대시보드**: 포트폴리오 요약, 월별 성과, 종목별 현황, 투자 지표, 매매 인사이트, 차트

## 프로젝트 구조

```
auto-trading-journal/
├── cmd/atj/main.go        # 메인 진입점 + 오케스트레이션
├── internal/
│   ├── config/            # config.yaml + env 로드
│   ├── model/             # Trade 데이터 모델
│   ├── parser/            # 파서 인터페이스 + 레지스트리 + 미래에셋/한국투자
│   ├── symbol/            # KRX 종목 마스터 → 종목코드 보강
│   ├── sheets/            # Google Sheets API v4 래퍼
│   ├── writer/            # 시트 생성/중복필터/삽입/읽기
│   ├── summary/           # 대시보드 생성 (요약/지표/인사이트/추이/차트)
│   └── sector/            # OpenAI 섹터 분류
├── config/                # config.yaml, sector_cache.json
├── testdata/              # 테스트 픽스처
├── input/                 # CSV 입력 (gitignore)
├── Makefile
└── docs/
```

### 데이터 처리 파이프라인

1. **CSV 스캔**: `input/{증권사명}/` 하위 CSV 파일 탐색
2. **파서 감지**: CSV 헤더를 읽어 파서 자동 선택 (CP949/UTF-8 자동 디코딩)
3. **파싱**: 증권사 형식에 맞춰 Trade 리스트 생성
4. **종목코드 보강**: 국내 거래 중 코드가 빈 항목을 KRX 마스터에서 조회
5. **시트 확인**: 시트가 없으면 자동 생성 + 헤더 삽입
6. **중복 필터**: 기존 시트 데이터와 비교하여 중복 제거
7. **데이터 삽입**: 신규 거래 일괄 삽입
8. **대시보드 갱신**: 단일 "대시보드" 시트 재작성

### 대시보드 구성

| 섹션 | 내용 |
|------|------|
| 포트폴리오 요약 | 총 매수/매도 금액, 실현손익, 수익률, 승률 |
| 월별 성과 | 연월별 매수/매도 건수, 금액, 실현손익 |
| 종목별 현황 | 종목별 매수/매도 수량, 금액, 손익, 투자비중 |
| 투자 지표 | 계좌별/통화별/섹터별 투자비중, 수익/손실 Top 10 |
| 매매 인사이트 | 연승/연패, 요일별 통계, 매매 빈도 |
| 차트 | 월별 손익 추이, 승률·수익률, 손익비, 매수/매도 추이, 계좌별 비중 |

## 새 증권사 파서 추가

1. `internal/parser/`에 새 파서 파일 생성
2. `Parser` 인터페이스(`Name`/`CanParse`/`Parse`) 구현
3. `internal/parser/registry.go`의 `registry` 슬라이스에 등록
4. 테스트 + `testdata/` 픽스처 추가

## 테스트

```bash
make test                          # 전체 테스트 (= go test ./...)
go test ./internal/parser/ -v      # 파서 테스트
go vet ./...
```

## 문제 해결

### "파서 감지 실패"
CSV 헤더가 지원되는 형식과 일치하지 않습니다. `--log-level DEBUG`로 헤더를 확인하세요.

### Google Sheets API 인증 오류
1. `config/config.yaml` 또는 env `SERVICE_ACCOUNT_PATH`의 JSON 키 파일 경로 확인
2. 서비스 계정 이메일에 편집자 권한 부여 확인
3. Google Cloud Console에서 Sheets API 활성화 확인

### Rate Limit 오류 (429)
Google Sheets API 쓰기 제한에 걸리면 지수 백오프로 자동 재시도합니다. 반복되면 1~2분 후 재실행하세요.

### 중복 거래가 계속 입력됨
`(date, trade_type, stock_name, quantity, price)` 5개 필드의 정확한 일치 여부를 확인하세요.

## 라이선스

MIT License

---

**주의**: 이 프로젝트는 개인 투자 기록 관리용으로 설계되었습니다. 실제 투자 결정이나 세무 신고에 사용하기 전에 데이터의 정확성을 반드시 확인하세요.
