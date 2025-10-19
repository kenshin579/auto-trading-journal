# Issue #5: 마지막 행이 잘못된 색상으로 하이라이트 되는 버그

## 문제 설명
Google Sheets에 거래 데이터를 입력할 때, 날짜별 데이터가 올바른 색상으로 하이라이트 되어야 하는데 마지막 행이 잘못된 색상으로 표시되는 버그가 있습니다.

## 현상
- 3개의 같은 날짜 데이터를 입력할 때, 4번째 행까지 잘못된 색상으로 하이라이트됨
- 데이터가 없는 빈 행까지 색상이 적용되는 현상 발생

## 원인 분석

### 코드 위치
`modules/sheet_manager.py` 파일의 `apply_date_colors` 함수 (581번 줄)

### 버그 발생 코드
```python
# 580-582번 줄
'end_row': start_row + end_idx + 1,  # end_row는 exclusive
```

### 문제점 분석

1. **현재 계산 방식**
   - `end_idx`는 trades 리스트의 마지막 인덱스 (0-based)
   - 예: 3개 거래의 경우 인덱스는 0, 1, 2이므로 `end_idx = 2`
   - 현재 계산: `end_row = start_row + end_idx + 1`
   - 예: `start_row=10`, `end_idx=2` 일 때 `end_row=13`

2. **Google Sheets API 동작**
   - `endRowIndex`는 exclusive (해당 행을 포함하지 않음)
   - `endRowIndex=13`은 "13번 행 전까지" = 실제로 10, 11, 12, 13번 행을 하이라이트
   - 원래는 데이터가 10, 11, 12번 행에만 있음

3. **올바른 계산 방식**
   - 데이터가 있는 마지막 행: `start_row + end_idx`
   - Google API를 위한 exclusive 값: `start_row + end_idx + 1`
   - 하지만 현재 코드는 잘못되어 +1을 추가로 하고 있음

## 예제

### 현재 동작 (버그 있음)
```
거래 데이터: 3개 거래
- start_row = 10
- trades 인덱스 = [0, 1, 2]
- end_idx = 2

계산:
- end_row = 10 + 2 + 1 = 13

Google Sheets API:
- startRowIndex = 9 (0-based, 실제 10번 행)
- endRowIndex = 13 (exclusive, 실제 14번 행 전까지)

결과:
- 행 10: 데이터 O, 색상 O ✓
- 행 11: 데이터 O, 색상 O ✓
- 행 12: 데이터 O, 색상 O ✓
- 행 13: 데이터 X, 색상 O ✗ (버그)
```

### 수정 후 예상 동작
```
계산:
- end_row = 10 + 2 = 12

Google Sheets API:
- startRowIndex = 9
- endRowIndex = 12

결과:
- 행 10: 데이터 O, 색상 O ✓
- 행 11: 데이터 O, 색상 O ✓
- 행 12: 데이터 O, 색상 O ✓
- 행 13: 데이터 X, 색상 X ✓
```

## 해결 방안

### 수정이 필요한 코드
```python
# modules/sheet_manager.py 581번 줄
# 기존 코드:
'end_row': start_row + end_idx + 1,  # end_row는 exclusive

# 수정 코드:
'end_row': start_row + end_idx,  # end_row는 exclusive (API에서 +1 처리)
```

더 명확한 구현:
```python
# 데이터가 있는 마지막 행의 실제 계산
actual_last_row = start_row + end_idx
# Google API의 endRowIndex를 위해 exclusive하게 만들기 위해 실제 마지막 행의 다음 행을 지정
'end_row': actual_last_row,
```

## 관련 코드 위치

1. **문제 발생 계산**: `modules/sheet_manager.py:580-582`
2. **Google API 호출**: `modules/google_sheets_client.py:371-374`
3. **색상 적용 로직**: `modules/sheet_manager.py:535`

## 테스트 시나리오

### 버그 재현 테스트
1. 3개의 같은 날짜 데이터를 입력
2. 행 색상이 데이터 행만 적용되는지 확인
3. Google Sheets에서 4번째 행이 색상이 적용되는지 확인

### 수정 후 검증 테스트
1. 코드 수정 적용
2. 동일한 3개 같은 날짜 데이터로 테스트
3. 데이터가 있는 마지막 행까지만 색상이 적용되는지 확인
4. 다른 개수의 데이터로도 테스트 (1개, 5개, 10개)

## 영향 범위
- 모든 거래 데이터 색상 표시
- 날짜별/티커별 그룹화 표시
- 시각적 구분만 영향받고 데이터 무결성은 영향 없음

## 우선 순위
- **심각도**: 낮음 (시각적 버그)
- **영향도**: 높음 (모든 데이터 표시 시 발생)
- **수정 난이도**: 매우 낮음 (1줄 수정)