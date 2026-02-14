# Critical Security Fix: 민감한 정보 제거 및 환경변수 전환

## 개요

`config/config.yaml`에 하드코딩된 민감한 정보를 환경변수로 전환하고, Git 히스토리에서 민감한 정보를 완전히 제거합니다.

## 문제점

### 현재 상태

```yaml
# config/config.yaml (현재)
google_sheets:
  spreadsheet_id: <REDACTED_SPREADSHEET_ID>  # 실제 ID 노출
  service_account_path: <REDACTED_SERVICE_ACCOUNT_PATH>  # 경로 노출
```

### 위험 요소

| 항목 | 위험 수준 | 설명 |
|------|----------|------|
| `spreadsheet_id` | HIGH | Google Sheets ID가 Git에 커밋됨 |
| `service_account_path` | HIGH | 사용자 홈 디렉토리 경로 노출 |
| Git 히스토리 | CRITICAL | 초기 커밋(15d6c64)부터 민감한 정보 포함 |

---

## 요구사항

### Task 1: 환경변수 전환

#### 1.1 main.py 수정

**현재 코드** (`main.py:58-60`):
```python
self.spreadsheet_id = config.get('google_sheets', {}).get('spreadsheet_id', '')
if not self.spreadsheet_id:
    raise ValueError("설정 파일에 spreadsheet_id가 없습니다")
```

**변경 후**:
```python
self.spreadsheet_id = os.getenv('GOOGLE_SPREADSHEET_ID') or config.get('google_sheets', {}).get('spreadsheet_id', '')
if not self.spreadsheet_id:
    raise ValueError("환경변수 GOOGLE_SPREADSHEET_ID 또는 설정 파일에 spreadsheet_id가 없습니다")
```

#### 1.2 google_sheets_client.py 수정

**현재 코드** (`modules/google_sheets_client.py:36-37`):
```python
self.service_account_path = os.getenv('SERVICE_ACCOUNT_PATH',
                                     '<REDACTED_SERVICE_ACCOUNT_PATH>')
```

**변경 후**:
```python
self.service_account_path = os.getenv('SERVICE_ACCOUNT_PATH')
if not self.service_account_path:
    raise ValueError("환경변수 SERVICE_ACCOUNT_PATH가 설정되지 않았습니다")
```

#### 1.3 config/config.yaml 수정

**변경 후**:
```yaml
# Google Sheets 설정
# 환경변수 우선: GOOGLE_SPREADSHEET_ID, SERVICE_ACCOUNT_PATH
google_sheets:
  spreadsheet_id: ""  # 환경변수 GOOGLE_SPREADSHEET_ID 사용 권장
  service_account_path: ""  # 환경변수 SERVICE_ACCOUNT_PATH 사용 권장

# 로깅 설정
logging:
  level: INFO

# 처리 설정
batch_size: 10
empty_row_threshold: 100
stock_type_cache_file: stock_type_cache.json
```

#### 1.4 config/config.yaml.example 생성

```yaml
# Google Sheets 설정
# 환경변수로 설정하거나 아래 값을 직접 입력
google_sheets:
  spreadsheet_id: "YOUR_SPREADSHEET_ID"  # 또는 환경변수 GOOGLE_SPREADSHEET_ID
  service_account_path: "/path/to/service_account_key.json"  # 또는 환경변수 SERVICE_ACCOUNT_PATH

# 로깅 설정
logging:
  level: INFO

# 처리 설정
batch_size: 10
empty_row_threshold: 100
stock_type_cache_file: stock_type_cache.json
```

#### 1.5 .gitignore 업데이트

추가할 항목:
```gitignore
# 민감한 설정 파일
config/config.yaml
config/config.local.yaml
*.json  # service account key 파일 방지

# 환경변수 파일
.env
.env.local
.env.*.local
```

---

### Task 2: Git 히스토리에서 민감한 정보 제거

#### 2.1 BFG Repo-Cleaner 사용 (권장)

```bash
# 1. BFG 설치 (macOS)
brew install bfg

# 2. 레포지토리 미러 클론
cd /tmp
git clone --mirror https://github.com/USERNAME/auto-trading-journal.git

# 3. 민감한 텍스트가 포함된 파일 생성
cat > /tmp/replacements.txt << 'EOF'
<YOUR_SPREADSHEET_ID>==>REDACTED_SPREADSHEET_ID
<YOUR_SERVICE_ACCOUNT_PATH>==>REDACTED_SERVICE_ACCOUNT_PATH
EOF

# 4. BFG로 민감한 정보 대체
bfg --replace-text /tmp/replacements.txt auto-trading-journal.git

# 5. 정리 및 가비지 컬렉션
cd auto-trading-journal.git
git reflog expire --expire=now --all && git gc --prune=now --aggressive

# 6. 강제 푸시 (주의: 협업자에게 사전 고지 필요)
git push --force
```

#### 2.2 git filter-repo 사용 (대안)

```bash
# 1. git-filter-repo 설치
pip install git-filter-repo

# 2. 민감한 정보 대체
git filter-repo --replace-text <(cat << 'EOF'
<YOUR_SPREADSHEET_ID>==>REDACTED_SPREADSHEET_ID
<YOUR_SERVICE_ACCOUNT_PATH>==>REDACTED_SERVICE_ACCOUNT_PATH
EOF
) --force

# 3. 강제 푸시
git push --force --all
git push --force --tags
```

#### 2.3 작업 후 확인

```bash
# 히스토리에서 민감한 정보가 제거되었는지 확인
git log -p --all -S '<YOUR_SPREADSHEET_ID>'
# 결과가 없어야 함

git log -p --all -S '<YOUR_SERVICE_ACCOUNT_PATH>'
# 결과가 없어야 함
```

---

### Task 3: 환경변수 설정 문서화

#### 3.1 README.md 업데이트

환경변수 섹션 추가:

```markdown
## 환경변수 설정

### 필수 환경변수

| 변수명 | 설명 | 예시 |
|--------|------|------|
| `GOOGLE_SPREADSHEET_ID` | Google Sheets 문서 ID | `1lIZ...18RQ` |
| `SERVICE_ACCOUNT_PATH` | 서비스 계정 키 파일 경로 | `/path/to/key.json` |

### 선택 환경변수

| 변수명 | 설명 | 기본값 |
|--------|------|--------|
| `OPENAI_API_KEY` | OpenAI API 키 (주식/ETF 분류용) | - |

### 설정 방법

```bash
# ~/.zshrc 또는 ~/.bashrc에 추가
export GOOGLE_SPREADSHEET_ID="your-spreadsheet-id"
export SERVICE_ACCOUNT_PATH="/path/to/service_account_key.json"
export OPENAI_API_KEY="your-openai-api-key"  # 선택
```
```

---

## 참고 자료

- [BFG Repo-Cleaner](https://rtyley.github.io/bfg-repo-cleaner/)
- [git-filter-repo](https://github.com/newren/git-filter-repo)
- [GitHub: Removing sensitive data](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/removing-sensitive-data-from-a-repository)
