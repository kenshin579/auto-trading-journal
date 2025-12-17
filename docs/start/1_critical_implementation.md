# Critical Security Fix: 구현 문서

## 수정 대상 파일

| 파일 | 작업 |
|------|------|
| `main.py` | 환경변수 `GOOGLE_SPREADSHEET_ID` 지원 추가 |
| `modules/google_sheets_client.py` | 하드코딩된 기본 경로 제거 |
| `config/config.yaml` | 민감한 값 제거 |
| `config/config.yaml.example` | 새로 생성 |
| `.gitignore` | config.yaml 추가 |
| `README.md` | 환경변수 설정 섹션 추가 |

---

## 1. main.py 수정

**위치**: `main.py:58-60`

```python
# 변경 전
self.spreadsheet_id = config.get('google_sheets', {}).get('spreadsheet_id', '')
if not self.spreadsheet_id:
    raise ValueError("설정 파일에 spreadsheet_id가 없습니다")

# 변경 후
self.spreadsheet_id = os.getenv('GOOGLE_SPREADSHEET_ID') or config.get('google_sheets', {}).get('spreadsheet_id', '')
if not self.spreadsheet_id:
    raise ValueError("환경변수 GOOGLE_SPREADSHEET_ID 또는 설정 파일에 spreadsheet_id가 없습니다")
```

---

## 2. google_sheets_client.py 수정

**위치**: `modules/google_sheets_client.py:36-37`

```python
# 변경 전
self.service_account_path = os.getenv('SERVICE_ACCOUNT_PATH',
                                     '<REDACTED_SERVICE_ACCOUNT_PATH>')

# 변경 후
self.service_account_path = os.getenv('SERVICE_ACCOUNT_PATH')
if not self.service_account_path:
    raise ValueError("환경변수 SERVICE_ACCOUNT_PATH가 설정되지 않았습니다")
```

---

## 3. config/config.yaml 수정

```yaml
# Google Sheets 설정
# 환경변수 우선: GOOGLE_SPREADSHEET_ID, SERVICE_ACCOUNT_PATH
google_sheets:
  spreadsheet_id: ""
  service_account_path: ""

# 로깅 설정
logging:
  level: INFO

# 처리 설정
batch_size: 10
empty_row_threshold: 100
stock_type_cache_file: stock_type_cache.json
```

---

## 4. config/config.yaml.example 생성

```yaml
# Google Sheets 설정
# 환경변수로 설정하거나 아래 값을 직접 입력
google_sheets:
  spreadsheet_id: "YOUR_SPREADSHEET_ID"
  service_account_path: "/path/to/service_account_key.json"

# 로깅 설정
logging:
  level: INFO

# 처리 설정
batch_size: 10
empty_row_threshold: 100
stock_type_cache_file: stock_type_cache.json
```

---

## 5. .gitignore 추가 항목

```gitignore
# 민감한 설정 파일
config/config.yaml
config/config.local.yaml

# 환경변수 파일
.env
.env.local
.env.*.local
```

---

## 6. Git 히스토리 정리 (BFG 사용)

```bash
# BFG 설치
brew install bfg

# 미러 클론
cd /tmp
git clone --mirror git@github.com:kenshin579/auto-trading-journal.git

# 대체 텍스트 파일 생성
cat > /tmp/replacements.txt << 'EOF'
<YOUR_SPREADSHEET_ID>==>REDACTED_SPREADSHEET_ID
<YOUR_SERVICE_ACCOUNT_PATH>==>REDACTED_SERVICE_ACCOUNT_PATH
EOF

# BFG 실행
bfg --replace-text /tmp/replacements.txt auto-trading-journal.git

# 정리
cd auto-trading-journal.git
git reflog expire --expire=now --all && git gc --prune=now --aggressive

# 강제 푸시
git push --force
```

---

## 7. README.md 환경변수 섹션 추가

```markdown
## 환경변수 설정

### 필수 환경변수

| 변수명 | 설명 |
|--------|------|
| `GOOGLE_SPREADSHEET_ID` | Google Sheets 문서 ID |
| `SERVICE_ACCOUNT_PATH` | 서비스 계정 키 파일 경로 |

### 선택 환경변수

| 변수명 | 설명 |
|--------|------|
| `OPENAI_API_KEY` | OpenAI API 키 (주식/ETF 분류용) |

### 설정 방법

~/.zshrc에 추가:
```bash
export GOOGLE_SPREADSHEET_ID="your-spreadsheet-id"
export SERVICE_ACCOUNT_PATH="/path/to/service_account_key.json"
```
```
