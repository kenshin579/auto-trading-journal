# Issue #3: Google Sheet 달러 입력 시 $ 자동 추가 문제

## 📋 문제 현상

Google Sheets에 해외 주식 거래 데이터 입력 후, 사용자가 해당 셀을 수정하려고 할 때 자동으로 달러 기호($)가 추가되는 현상이 발생합니다.

**예시**:
- 프로그램이 입력한 값: `$150.00`
- 사용자가 셀 클릭 후 값 수정 시: `$` 기호가 자동으로 붙음
- 원하는 동작: 문자열로 저장되어 자동 포맷팅이 적용되지 않아야 함

## 🔍 근본 원인 분석

### 1. 문제 발생 위치

**파일**: `modules/google_sheets_client.py`

**Line 166 (`update_cells` 메서드)**:
```python
result = self.service.spreadsheets().values().update(
    spreadsheetId=self.spreadsheet_id,
    range=range_name,
    valueInputOption='USER_ENTERED',  # ← 문제의 원인
    body=body
).execute()
```

**Line 190 (`batch_update_cells` 메서드)**:
```python
body = {
    'valueInputOption': 'USER_ENTERED',  # ← 문제의 원인
    'data': data
}
```

### 2. 근본 원인: `USER_ENTERED` 모드의 동작 방식

Google Sheets API의 `valueInputOption='USER_ENTERED'` 설정은 입력값을 "사용자가 UI에서 직접 입력한 것처럼" 해석합니다.

**동작 과정**:
1. 프로그램이 `"$150.00"` 문자열을 전송
2. Google Sheets가 이를 분석:
   - `$` 기호 감지 → 통화 포맷으로 인식
   - 숫자값 `150.00` 추출
   - 셀에 자동으로 통화 포맷 (`$#,##0.00`) 적용
3. 결과:
   - 표시값: `$150.00` (의도한 대로 보임)
   - 실제 셀 값: `150.00` (숫자)
   - 셀 포맷: 통화 (`$#,##0.00`)
4. 부작용: 사용자가 셀을 수정하려 하면 통화 포맷이 유지되어 자동으로 `$` 기호가 붙음

### 3. 데이터 소스 위치

**파일**: `modules/trade_models.py`

**Line 122-125 (`ForeignTrade.to_sheet_row` 메서드)**:
```python
return [
    self.stock_name,                       # 종목
    "미래에셋증권",                        # 증권사
    self.date,                             # 일자
    self.trade_type,                       # 종류 (매수/매도)
    f"${price:.2f}",                       # 주문가격 (달러 표시) ← $ 포함
    str(quantity),                         # 수량
    "",                                    # 수수료 (빈 값)
    f"${total_amount:.2f}"                 # 총액 (달러 표시) ← $ 포함
]
```

### 4. 영향 범위

- **영향 받는 데이터**: 해외 주식 거래 내역 (`ForeignTrade`)
- **영향 받는 컬럼**:
  - 주문가격 (5번째 컬럼)
  - 총액 (8번째 컬럼)
- **영향 받지 않는 데이터**: 국내 주식 거래 내역 (`DomesticTrade` - 원화 기호 ₩ 사용)

## 💡 해결 방안

### Option 1: `RAW` 입력 모드 사용 (권장)

**변경 내용**: `valueInputOption`을 `'USER_ENTERED'`에서 `'RAW'`로 변경

**장점**:
- 가장 간단한 수정 (한 줄 변경)
- 모든 값을 문자열 그대로 저장
- 자동 포맷팅 완전 차단
- 의도한 대로 `"$150.00"` 문자열로 저장됨

**단점**:
- 수식(formula)이 평가되지 않음 (현재 코드에서는 사용하지 않으므로 문제 없음)
- 기존 동작 변경에 대한 검증 필요

**구현 예시**:
```python
# google_sheets_client.py Line 166
result = self.service.spreadsheets().values().update(
    spreadsheetId=self.spreadsheet_id,
    range=range_name,
    valueInputOption='RAW',  # USER_ENTERED → RAW
    body=body
).execute()

# google_sheets_client.py Line 190
body = {
    'valueInputOption': 'RAW',  # USER_ENTERED → RAW
    'data': data
}
```

### Option 2: 순수 숫자 저장 + 포맷 별도 적용

**변경 내용**:
1. `trade_models.py`에서 `$` 기호 제거, 순수 숫자만 저장
2. 필요시 Google Sheets API로 숫자 포맷 별도 적용

**장점**:
- 데이터 구조적으로 더 올바름 (숫자는 숫자로 저장)
- 계산 및 분석에 유리
- 포맷 제어 가능

**단점**:
- 여러 파일 수정 필요
- 포맷 적용 로직 추가 필요
- 구현 복잡도 증가

**구현 예시**:
```python
# trade_models.py
def to_sheet_row(self) -> List[str]:
    return [
        self.stock_name,
        "미래에셋증권",
        self.date,
        self.trade_type,
        price,                    # $ 제거
        str(quantity),
        "",
        total_amount              # $ 제거
    ]

# sheet_manager.py or google_sheets_client.py
# 추가로 숫자 포맷 설정 로직 필요
```

### Option 3: TEXT 포맷 명시적 설정

**변경 내용**:
1. `USER_ENTERED` 유지
2. 데이터 입력 전 해당 컬럼에 TEXT 포맷 설정

**장점**:
- 텍스트 처리 명확히 보장
- 세밀한 제어 가능

**단점**:
- 구현 복잡도 가장 높음
- 추가 API 호출 필요 (성능 영향)
- 컬럼별 포맷 관리 필요

**구현 예시**:
```python
# google_sheets_client.py
# 데이터 입력 전 TEXT 포맷 설정
requests = [{
    'repeatCell': {
        'range': {
            'sheetId': sheet_id,
            'startRowIndex': start_row - 1,
            'endRowIndex': end_row,
            'startColumnIndex': 4,  # 5번째 컬럼 (주문가격)
            'endColumnIndex': 5
        },
        'cell': {
            'userEnteredFormat': {
                'numberFormat': {
                    'type': 'TEXT'
                }
            }
        },
        'fields': 'userEnteredFormat.numberFormat'
    }
}]
```

## 🎯 권장 해결 방안

**Option 1 (`RAW` 모드)**을 권장합니다.

**이유**:
1. 가장 간단하고 명확한 해결책
2. 현재 코드에 미치는 영향 최소
3. 의도한 동작(문자열 그대로 저장)과 정확히 일치
4. 추가 API 호출 불필요 (성능 유지)

**주의사항**:
- 변경 후 전체 기능 테스트 필요
- 특히 중복 체크 로직 (`check_duplicates`) 검증 필요
- 기존 데이터와의 호환성 확인

## 📚 참고 자료

- [Google Sheets API - ValueInputOption](https://developers.google.com/sheets/api/reference/rest/v4/ValueInputOption)
  - `USER_ENTERED`: Values will be parsed as if the user typed them into the UI
  - `RAW`: Values will not be parsed and will be stored as-is
- [Google Sheets API - Number Formats](https://developers.google.com/sheets/api/guides/formats)

## ✅ 다음 단계

1. 해결 방안 선택 (Option 1 권장)
2. 선택된 방안에 따른 코드 수정
3. 단위 테스트 작성/수정
4. 전체 기능 테스트 (특히 중복 체크)
5. 배포 및 모니터링
