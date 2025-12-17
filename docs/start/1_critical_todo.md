# Critical Security Fix: Todo 체크리스트

## Phase 1: 코드 수정

- [x] `main.py` 수정 - 환경변수 `GOOGLE_SPREADSHEET_ID` 지원
- [x] `modules/google_sheets_client.py` 수정 - 하드코딩된 기본 경로 제거
- [x] `config/config.yaml` 수정 - 민감한 값을 빈 문자열로 변경
- [x] `config/config.yaml.example` 생성 - 예제 설정 파일

## Phase 2: Git 설정

- [x] `.gitignore` 업데이트 - config/config.yaml 추가
- [x] 변경사항 커밋

## Phase 3: Git 히스토리 정리

- [ ] BFG Repo-Cleaner 설치 (`brew install bfg`)
- [ ] 미러 클론 생성
- [ ] BFG로 민감한 정보 대체
- [ ] 가비지 컬렉션 실행
- [ ] 강제 푸시
- [ ] 히스토리 정리 확인 (`git log -p --all -S '민감한텍스트'`)

## Phase 4: 문서화

- [x] README.md 업데이트 - 환경변수 설정 섹션 추가

## Phase 5: 검증

- [ ] 환경변수 설정 후 애플리케이션 실행 테스트
- [ ] `python main.py --dry-run` 정상 동작 확인
