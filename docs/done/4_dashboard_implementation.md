# 대시보드 차트 시각화 구현 문서

## 1. 구현 범위

PRD의 6개 차트 중 **P0/P1 4개 차트**를 우선 구현한다. P2(요일별 성과, 수익 Top 10)는 추후 확장.

| 차트 | 유형 | 데이터 소스 |
|------|------|------------|
| 월별 실현손익 추이 | Column | 섹션 5 (월별 성과 추이) |
| 월별 승률·수익률 추이 | Line (2시리즈) | 섹션 5 (월별 성과 추이) |
| 계좌별 투자비중 | Pie | 섹션 3 (투자 지표) |
| 손익비·Profit Factor 추이 | Line (2시리즈) | 섹션 5 (월별 성과 추이) |

## 2. 설계 결정

### 2.1 차트 데이터 참조 방식

**옵션 A 채택: 기존 테이블 데이터 직접 참조**

섹션 5(월별 성과 추이)는 고정 컬럼 구조(A~K열)이므로 `sourceRange`로 직접 참조 가능하다. `generate_all()`에서 이미 `trend_start` 변수로 섹션 5의 시작 행을 추적하고 있어 차트 생성 시 그대로 전달하면 된다.

### 2.2 섹션 3 (계좌별 투자비중) 파이 차트 데이터

섹션 3은 지표명-값 2컬럼의 동적 구조라 차트가 직접 참조하기 어렵다. **차트 전용 데이터 영역**을 사용한다.

- 위치: `N열~O열` (차트 영역과 같은 우측)
- `_write_investment_metrics()`에서 계좌별 투자비중 데이터를 계산할 때, 동일 데이터를 N~O열에도 작성
- 파이 차트는 이 N~O열 데이터를 `sourceRange`로 참조

```
N열           O열
계좌명         비중(소수)
미래에셋_국내   0.45
미래에셋_해외   0.30
한투_국내      0.25
```

### 2.3 차트 배치

N열(columnIndex=13) 기준, 각 차트를 세로로 20행 간격 배치:

| 차트 | anchorCell rowIndex | 크기 |
|------|---------------------|------|
| 월별 실현손익 | 0 | 600×370 |
| 승률·수익률 추이 | 20 | 600×370 |
| 계좌별 투자비중 | 40 | 450×370 |
| 손익비·PF 추이 | 40 (columnIndex=20) | 450×370 |

> 투자비중과 손익비·PF 차트는 같은 행에 나란히 배치 (450px 폭이므로 가능)

## 3. google_sheets_client.py 변경사항

### 3.1 신규 메서드: `get_charts()`

```python
async def get_charts(self, sheet_name: str) -> List[Dict[str, Any]]:
    """시트에 포함된 차트 목록 반환 (chartId 포함)"""
    metadata = await self.get_spreadsheet_metadata()
    sheet_id = await self._get_sheet_id(sheet_name)
    for sheet in metadata.get('sheets', []):
        if sheet['properties']['sheetId'] == sheet_id:
            return sheet.get('charts', [])
    return []
```

### 3.2 신규 메서드: `delete_all_charts()`

```python
async def delete_all_charts(self, sheet_name: str) -> bool:
    """시트의 모든 차트 삭제"""
    charts = await self.get_charts(sheet_name)
    if not charts:
        return True

    requests = [
        {"deleteEmbeddedObject": {"objectId": c["chartId"]}}
        for c in charts
    ]
    self.service.spreadsheets().batchUpdate(
        spreadsheetId=self.spreadsheet_id,
        body={'requests': requests}
    ).execute()
    return True
```

### 3.3 신규 메서드: `add_charts()`

```python
async def add_charts(self, chart_requests: List[Dict]) -> bool:
    """여러 차트를 배치로 추가

    Args:
        chart_requests: addChart 요청 리스트
    """
    if not chart_requests:
        return True

    requests = [{"addChart": {"chart": spec}} for spec in chart_requests]
    self.service.spreadsheets().batchUpdate(
        spreadsheetId=self.spreadsheet_id,
        body={'requests': requests}
    ).execute()
    return True
```

## 4. summary_generator.py 변경사항

### 4.1 generate_all() 수정

기존 포맷 적용 후, 차트 생성 단계를 추가한다.

```python
async def generate_all(self, all_trades: List[Trade]):
    await self._ensure_dashboard_sheet()

    # ... 기존 섹션 작성 (변경 없음) ...

    # 포맷 적용 (기존)
    await self._apply_header_colors(monthly_start, trend_start, stock_start)
    await self._apply_dashboard_formats(...)

    # 차트 생성 (신규)
    await self._create_charts(
        trend_start=trend_start,
        trend_end=stock_start - 1,  # 섹션 5 데이터 마지막 행
        pie_data_range=pie_data_range,  # 계좌별 투자비중 데이터 위치
    )
```

### 4.2 _ensure_dashboard_sheet() 수정

시트 초기화 시 기존 차트도 삭제한다.

```python
async def _ensure_dashboard_sheet(self):
    sheets = await self.client.list_sheets()
    if DASHBOARD_SHEET not in sheets:
        await self.client.create_sheet(DASHBOARD_SHEET)
    else:
        await self.client.clear_sheet(DASHBOARD_SHEET, start_row=1)
        await self.client.clear_background_colors(DASHBOARD_SHEET)
        await self.client.clear_number_formats(DASHBOARD_SHEET)
        await self.client.delete_all_charts(DASHBOARD_SHEET)  # 추가
```

### 4.3 _write_investment_metrics() 수정

계좌별 투자비중 데이터를 N~O열에도 작성하고, 해당 범위를 반환한다.

```python
# 계좌별 투자비중 계산 후 N~O열에 차트 데이터 작성
pie_data = []
for account, amount in sorted(account_buy.items()):
    pie_data.append([account, amount / total_buy if total_buy else 0])

if pie_data:
    pie_start_row = start_row
    pie_end_row = start_row + len(pie_data) - 1
    await self.client.batch_update_cells(
        DASHBOARD_SHEET,
        {f"N{pie_start_row}:O{pie_end_row}": pie_data}
    )
```

반환값에 `pie_data_range` 정보를 추가해야 하므로, 반환 타입을 `Tuple[int, Dict]`로 변경하거나 인스턴스 변수로 저장한다.

**방안: 인스턴스 변수 사용**

```python
self._pie_data_range = None  # (start_row, end_row) or None

# _write_investment_metrics() 내부에서 설정
self._pie_data_range = (pie_start_row, pie_end_row) if pie_data else None
```

### 4.4 신규 메서드: `_create_charts()`

```python
async def _create_charts(self, trend_start: int, trend_end: int,
                          pie_data_range: Optional[Tuple[int, int]]):
    """대시보드 차트 생성"""
    sheet_id = await self.client._get_sheet_id(DASHBOARD_SHEET)
    if sheet_id is None:
        return

    # 섹션 5 데이터 행 범위 (0-based, 헤더 포함)
    # trend_start = 헤더 행(1-based), 데이터는 trend_start+1부터
    data_start = trend_start  # 헤더 포함 (차트가 헤더를 시리즈명으로 사용)
    data_end = trend_end      # 1-based → 0-based 변환 시 -1

    chart_specs = []

    # 차트 1: 월별 실현손익 (Column)
    chart_specs.append(self._build_basic_chart_spec(
        sheet_id=sheet_id,
        title="월별 실현손익 추이",
        chart_type="COLUMN",
        domain_col=0,           # A열: 연월
        series_cols=[2],        # C열: 실현손익
        data_start=data_start - 1,   # 0-based
        data_end=data_end - 1,
        anchor_row=0, anchor_col=13,
        width=600, height=370,
    ))

    # 차트 2: 승률·수익률 추이 (Line)
    chart_specs.append(self._build_basic_chart_spec(
        sheet_id=sheet_id,
        title="월별 승률 & 수익률 추이",
        chart_type="LINE",
        domain_col=0,           # A열: 연월
        series_cols=[3, 4],     # D열: 수익률, E열: 승률
        data_start=data_start - 1,
        data_end=data_end - 1,
        anchor_row=20, anchor_col=13,
        width=600, height=370,
    ))

    # 차트 3: 계좌별 투자비중 (Pie)
    if pie_data_range:
        chart_specs.append(self._build_pie_chart_spec(
            sheet_id=sheet_id,
            title="계좌별 투자비중",
            label_col=13,       # N열
            value_col=14,       # O열
            data_start=pie_data_range[0] - 1,
            data_end=pie_data_range[1],
            anchor_row=40, anchor_col=13,
            width=450, height=370,
        ))

    # 차트 4: 손익비·PF 추이 (Line)
    chart_specs.append(self._build_basic_chart_spec(
        sheet_id=sheet_id,
        title="손익비 & Profit Factor 추이",
        chart_type="LINE",
        domain_col=0,           # A열: 연월
        series_cols=[7, 8],     # H열: 손익비, I열: Profit Factor
        data_start=data_start - 1,
        data_end=data_end - 1,
        anchor_row=40, anchor_col=20,
        width=450, height=370,
    ))

    if chart_specs:
        await self.client.add_charts(chart_specs)
        logger.info(f"대시보드 차트 {len(chart_specs)}개 생성 완료")
```

### 4.5 신규 메서드: `_build_basic_chart_spec()`

Column/Line 차트 공통 빌더.

```python
@staticmethod
def _build_basic_chart_spec(
    sheet_id: int, title: str, chart_type: str,
    domain_col: int, series_cols: List[int],
    data_start: int, data_end: int,
    anchor_row: int, anchor_col: int,
    width: int = 600, height: int = 370,
) -> Dict:
    """BasicChart 스펙 생성 (COLUMN, LINE, BAR 공용)

    Args:
        domain_col: X축 컬럼 (0-based)
        series_cols: Y축 컬럼 리스트 (0-based)
        data_start: 데이터 시작 행 (0-based, 헤더 포함)
        data_end: 데이터 끝 행 (0-based, exclusive)
    """
    source_range = lambda col: {
        "sourceRange": {
            "sources": [{
                "sheetId": sheet_id,
                "startRowIndex": data_start,
                "endRowIndex": data_end,
                "startColumnIndex": col,
                "endColumnIndex": col + 1,
            }]
        }
    }

    return {
        "spec": {
            "title": title,
            "basicChart": {
                "chartType": chart_type,
                "legendPosition": "BOTTOM_LEGEND",
                "headerCount": 1,
                "domains": [{"domain": source_range(domain_col)}],
                "series": [
                    {"series": source_range(col), "targetAxis": "LEFT_AXIS"}
                    for col in series_cols
                ],
            },
        },
        "position": {
            "overlayPosition": {
                "anchorCell": {
                    "sheetId": sheet_id,
                    "rowIndex": anchor_row,
                    "columnIndex": anchor_col,
                },
                "widthPixels": width,
                "heightPixels": height,
            }
        },
    }
```

### 4.6 신규 메서드: `_build_pie_chart_spec()`

```python
@staticmethod
def _build_pie_chart_spec(
    sheet_id: int, title: str,
    label_col: int, value_col: int,
    data_start: int, data_end: int,
    anchor_row: int, anchor_col: int,
    width: int = 450, height: int = 370,
) -> Dict:
    """Pie 차트 스펙 생성"""
    def source_range(col):
        return {
            "sourceRange": {
                "sources": [{
                    "sheetId": sheet_id,
                    "startRowIndex": data_start,
                    "endRowIndex": data_end,
                    "startColumnIndex": col,
                    "endColumnIndex": col + 1,
                }]
            }
        }

    return {
        "spec": {
            "title": title,
            "pieChart": {
                "legendPosition": "RIGHT_LEGEND",
                "domain": source_range(label_col),
                "series": source_range(value_col),
            },
        },
        "position": {
            "overlayPosition": {
                "anchorCell": {
                    "sheetId": sheet_id,
                    "rowIndex": anchor_row,
                    "columnIndex": anchor_col,
                },
                "widthPixels": width,
                "heightPixels": height,
            }
        },
    }
```

## 5. 테스트 전략

### 5.1 단위 테스트 (`tests/test_dashboard_charts.py`)

- `_build_basic_chart_spec()`: 반환 dict 구조 검증 (chartType, sourceRange 인덱스, position)
- `_build_pie_chart_spec()`: 반환 dict 구조 검증 (domain, series, position)
- 각 빌더의 행 인덱스 변환 정확성 확인

### 5.2 통합 테스트

- `python main.py --dry-run` 으로 에러 없이 완료되는지 확인
- 실제 시트에서 차트 4개가 생성되는지 수동 검증
- 대시보드 재실행 시 기존 차트가 삭제되고 새로 생성되는지 확인
