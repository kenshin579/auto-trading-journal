# 대시보드 "나라별 섹터 비중" 추가

- 작성일: 2026-06-14
- 범위: `internal/writer/reader.go`(선행), `internal/summary`(신규 섹션 + 차트)

## 배경 / 목표

매매일지 거래 행에는 이제 per-row 섹터/산업이 채워져 있다(국내=KIS 한글, 미국·일본=FMP 영문).
대시보드에 **국가별로 쪼갠 섹터 투자비중**을 추가한다.

기존 "섹터별 투자비중"(투자지표 섹션, `internal/sector` OpenAI 통합 12 GICS)은 **그대로 유지**하고,
**나라별 native 섹터 비중 섹션을 새로 추가**한다(성격이 다름 — 기존=전체 통합 GICS, 신규=국가별 원본 섹터).

## 선행 작업: ReadAllTrades 가 섹터/산업을 읽도록

현재 `rowToTrade`(reader.go:143)는 섹터/산업 열을 건너뛴다(수량부터 읽음) → `Trade.Sector/Industry` 가 항상 빈 값.
per-row 섹터로 비중을 내려면 먼저 읽어야 한다.

- 국내 12컬럼: idx 4=섹터, 5=산업 → `Sector=getStr(row,4)`, `Industry=getStr(row,5)`
- 해외 17컬럼: idx 5=섹터, 6=산업 → `Sector=getStr(row,5)`, `Industry=getStr(row,6)`

## 신규 섹션: 나라별 섹터 비중

### 국가 구분
`Trade.Currency` → 국가 라벨: `KRW→국내`, `USD→미국`, `JPY→일본`. 그 외 통화는 통화코드 라벨(현재 미보유).
국가별로 거래를 그룹화하고, 데이터 있는 국가만 출력.

### 집계 (국가 안에서)
국가 내 `Trade.Sector`(빈 값 제외)별로:
- `매수금액` = Σ AmountKRW (TradeType=="매수")
- `매도금액` = Σ AmountKRW (TradeType=="매도")
- `순매수`  = 매수금액 − 매도금액
- `비중(%)` = 매수금액 / 국가 총매수금액  (국가 안에서 100%, Q2-A)

정렬: 매수금액 내림차순, 동률 시 섹터명 사전식.

### 표 레이아웃 (국가별 블록 3개 세로 스택)
국가별로 헤더 + 컬럼헤더 + 섹터행들:
```
[국내]
섹터 | 매수금액 | 매도금액 | 순매수 | 비중(%)
전기·전자 | ... | ... | ... | 32.1%
금융      | ... | ... | ... | 18.4%
...
[미국]
섹터 | 매수금액 | 매도금액 | 순매수 | 비중(%)
Technology | ... | ... | ... | 41.0%
...
[일본]
...
```
- 비중·금액은 기존 포맷(원화 통화/백분율) 재사용.

### 비중 기준
- 표는 매수/매도/순매수 모두 표시.
- **비중(%)·차트는 매수금액 기준**(항상 양수 → pie 안전, 기존 섹터비중과 일관). 순매수는 음수 가능해 비중 미사용.

## 차트: 국가별 섹터 비중 pie

- 국가마다 pie 1개(국내/미국/일본): label=섹터, value=매수금액.
- `buildPieChartSpec(sheetID, title, labelCol, valueCol, dataStart, dataEnd, anchorRow, anchorCol, w, h)` 재사용.
- 각 국가 섹터 블록의 (섹터, 매수금액) 행 범위를 차트 SourceRange 로 지정. 섹션 작성 시 국가별 데이터 시작/끝 행을 기록해 차트 anchor/range 계산.

## 구현 배치
- `internal/summary`: 신규 `writeCountrySectorWeight()`(sections.go 또는 신규 파일) — `GenerateAll()`(summary.go:68) 순서에 추가(기존 6섹션 뒤).
- 집계 순수 함수 분리(테스트 용이): `countrySectorRows(trades) → map[국가][]sectorRow`.
- 차트: `createCharts()`(charts.go:122)에 국가별 pie 추가.

## 테스트
- `rowToTrade`: 국내/해외 행에서 섹터/산업 파싱(라운드트립).
- `countrySectorRows`: 국가 그룹화 + 섹터별 매수/매도/순매수/비중 + 정렬 + 빈 섹터 제외 + 데이터 없는 국가 제외.
- 통화→국가 라벨 매핑.
- `go test ./...` 전체 통과. (시트 기록/차트는 mock 부재로 순수 집계 위주 단위 테스트)

## 범위 밖 (YAGNI)
- 산업별 비중(Q3 제외). 순매수 기준 비중. US/JP 외 국가. 기존 GICS 섹션 변경.
