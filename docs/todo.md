# TODO - Google Sheet 주식 데이터 입력 스크립트

## ⚠️ 중요 변경사항 (2024-12-21)

### MCP 사용 중단 결정
- **문제점**: 
  - fast-agent를 통한 MCP 연동이 제대로 동작하지 않음
  - Google Sheets MCP 서버와의 통신이 불안정함
  - 복잡한 설정에 비해 실제 동작이 제한적임

- **변경 방향**:
  - MCP 관련 코드 모두 제거
  - Google Sheets API를 직접 사용하도록 변경
  - Sequential Thinking MCP도 제거하고 일반 Python 로직으로 대체

- **제거 대상**:
  - `mcp_client.py` - MCP 클라이언트 코드
  - `GoogleSheetsClient`, `GoogleSheetsMCPWrapper` 클래스
  - fast-agent 관련 의존성
  - MCP 설정 파일들 (`mcp.json`, `fastagent.config.yaml`)

- **대체 방안**:
  - Google Sheets API v4 직접 사용
  - `google-api-python-client` 라이브러리 활용
  - 서비스 계정 인증 방식 유지

### Cleanup 작업 완료 (2024-12-21)
- **삭제된 파일**:
  - `mcp_client.py` ✅
  - `config/mcp.json` ✅
  - `config/fastagent.config.yaml` ✅
  - `test_mcp_client.py` ✅
  - `google_sheets_mcp.py` ✅
  - `example_usage.py` ✅
  - `TEST_GUIDE.md` ✅

- **생성된 파일**:
  - `google_sheets_client.py` - Google Sheets API v4 직접 사용하는 클라이언트

- **수정된 파일**:
  - `main.py` - MCP 대신 GoogleSheetsClient 사용, MCP 관련 주석 모두 제거
  - `requirements.txt` - MCP 의존성 제거, Google API 클라이언트 추가
  - `config/config.yaml` - 서비스 계정 경로 추가
  - `test_main.py` - MCP 관련 TODO 주석 제거

- **주요 변경사항**:
  - 색상 적용 기능 구현 (`apply_color_to_range` 메서드)
  - 배치 업데이트 지원
  - 서비스 계정 인증 방식 사용

---

## 1. 환경 설정 및 초기화
- [x] Python 프로젝트 초기 설정
  - [x] requirements.txt 파일 생성 (필요한 패키지 정의)
  - [x] .env 파일 설정 (OPENAI_API_KEY 등)
- [x] MCP 연결 설정
  - [x] Google Sheets MCP 연결 코드 작성
  - [x] Sequential Thinking MCP 연결 코드 작성
  - [x] MCP 연결 테스트 코드 작성

## 2. 파일 파싱 모듈 개발
- [x] stocks 폴더 스캔 기능
  - [x] .md 파일 목록 가져오기
  - [x] 파일명에서 prefix 추출 (ex: "계좌1 국내")
- [x] 매매일지 파싱 기능
  - [x] 날짜 추출 (# 매매일지 - YYYY/MM/DD)
  - [x] 탭으로 구분된 데이터 파싱
  - [x] 매수/매도 구분 로직
    - [x] 금일매수 수량 > 0 → 매수
    - [x] 금일매도 수량 > 0 → 매도
  - [x] 거래 데이터 구조화 (종목명, 수량, 가격, 날짜 등)

## 3. 종목 타입 판별 모듈
- [x] ChatGPT API 연동
  - [x] API 클라이언트 설정
  - [x] 종목 리스트 일괄 처리 함수
  - [x] 응답 캐싱 (동일 종목 중복 호출 방지)
- [x] 종목 분류 로직
  - [x] 주식/ETF 구분
  - [x] 에러 처리 (API 실패 시)

## 4. Google Sheets 작업 모듈
- [x] 시트 검색 기능
  - [x] prefix로 시트 목록 필터링
  - [x] 매수/매도, 주식/ETF별 시트 매칭
- [x] 마지막 행 찾기 로직
  - [x] 시트 데이터 읽기
  - [x] 빈 행 카운트 (100개 기준)
  - [x] 실제 마지막 데이터 행 위치 확인
- [x] 중복 체크 기능
  - [x] 기존 데이터 읽기
  - [x] 날짜+종목명 기준 중복 확인
  - [x] 중복 시 로그 출력
- [x] 데이터 입력 기능
  - [x] 구글 시트 포맷에 맞춰 데이터 변환
    - [x] 종목, 증권(미래에셋증권), 일자, 종류, 주문가격, 수량, 총액
    - [x] 수수료, 메모는 빈 값으로
  - [x] 날짜순 정렬
  - [x] 마지막 행에 데이터 추가
- [x] 색상 적용 기능
  - [x] 날짜별 색상 그룹핑
  - [x] Light 색상 팔레트 정의
  - [ ] 입력된 셀에만 색상 적용 (MCP 제한으로 스킵)

## 5. Sequential Thinking 통합
- [ ] MCP 선택 로직 구현
  - [ ] 작업별 적절한 MCP 선택
  - [ ] Sequential Thinking으로 의사결정

## 6. 메인 실행 로직
- [x] 전체 워크플로우 구현
  1. [x] stocks 폴더 파일 스캔
  2. [x] 각 파일별 처리
     - [x] 파일 파싱
     - [x] 종목 타입 판별 (일괄)
     - [x] 시트 매칭
     - [x] 중복 체크
     - [x] 데이터 입력
     - [x] 색상 적용 (로그만)
  3. [x] 결과 리포트 출력

## 7. 에러 처리 및 로깅
- [x] 로깅 시스템 구축
  - [x] 파일별 처리 상태
  - [x] 중복 데이터 로그
  - [x] API 호출 로그
  - [x] 에러 로그
- [x] 예외 처리
  - [x] 파일 파싱 에러
  - [x] API 연결 실패
  - [x] 구글 시트 접근 에러
  - [x] 데이터 포맷 불일치

## 8. 테스트 및 검증
- [x] 단위 테스트 작성
  - [x] 파일 파싱 테스트
  - [ ] 종목 분류 테스트
  - [ ] 중복 체크 테스트
- [ ] 통합 테스트
  - [ ] 샘플 데이터로 전체 플로우 테스트
  - [ ] 실제 구글 시트 연동 테스트

## 9. 문서화
- [ ] README.md 작성
  - [ ] 설치 방법
  - [ ] 환경 변수 설정
  - [ ] 실행 방법
  - [ ] 주의사항
- [ ] 코드 주석 추가

## 10. 최적화
- [x] ChatGPT API 호출 최소화
  - [x] 종목 캐싱
  - [x] 배치 처리
- [ ] 구글 시트 작업 최적화
  - [ ] 배치 업데이트
  - [ ] 불필요한 읽기 최소화

## 우선순위
1. 환경 설정 및 MCP 연결 (필수) ✓
2. 파일 파싱 모듈 (핵심) ✓
3. Google Sheets 기본 기능 (핵심) ✓
4. 종목 타입 판별 (핵심) ✓
5. 중복 체크 및 색상 적용 (중요) ✓
6. 에러 처리 및 로깅 (중요) ✓
7. 테스트 및 문서화 (권장) △