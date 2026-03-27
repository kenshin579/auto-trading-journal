# 월별 성과 추이 구현 문서

## 구현 방향

**방안 B 채택**: 섹션 6을 신규로 추가하여 전체 포트폴리오 합산 월별 추이를 제공한다.

- 섹션 2 (월별 성과)는 기존 (연월, 계좌) 기준 계좌별 상세로 유지
- 섹션 6 (월별 성과 추이)는 연월 단독 기준으로 전체 합산 핵심 지표 제공
- 최소 데이터 요건 없음 (매도 1건이어도 표시, 데이터가 쌓이면 자연스럽게 의미 생김)
- 이동 평균/차트는 구현하지 않음 (추후 필요 시 확장)

## 변경 파일

### 1. `modules/summary_generator.py`

#### 1-1. 정적 메서드 추가: `_calc_monthly_trend()`

매도 거래를 월별로 그룹핑하여 핵심 지표를 산출하는 정적 메서드.

```python
@staticmethod
def _calc_monthly_trend(
    sorted_sells: List[Trade],
) -> List[Dict]:
    """월별 성과 추이 계산

    Returns:
        List of dicts, 각 dict는 한 달의 지표:
        {
            "month": "2025-01",
            "sell_count": 15,
            "profit_krw": 150000,
            "return_rate": 0.0468,      # 소수 (시트에서 % 포맷 적용)
            "win_rate": 0.60,           # 소수
            "avg_profit_rate": 0.0835,  # 소수
            "avg_loss_rate": -0.0312,   # 소수 (음수)
            "pl_ratio": 2.68,           # |avg_profit_rate / avg_loss_rate|
            "profit_factor": 1.85,      # gross_profit / |gross_loss|
            "expectancy": 8500,         # 원화
            "mom_change": 0.153,        # 전월대비 변화 소수 (15.3%)
        }
    """
```

**계산 로직**:

```python
# 월별 그룹핑
month_groups: Dict[str, List[Trade]] = defaultdict(list)
for t in sorted_sells:
    month_groups[t.date[:7]].append(t)

results = []
prev_profit = None

for month in sorted(month_groups.keys()):
    sells = month_groups[month]
    profitable = [t for t in sells if t.profit_krw > 0]
    losing = [t for t in sells if t.profit_krw <= 0]

    sell_count = len(sells)
    total_sell_amount = sum(t.amount_krw for t in sells)
    profit_krw = sum(t.profit_krw for t in sells)

    # 수익률
    return_rate = profit_krw / total_sell_amount if total_sell_amount else 0

    # 승률
    win_rate = len(profitable) / sell_count

    # 평균 수익률 / 손실률
    avg_profit_rate = (
        sum(t.profit_rate for t in profitable) / len(profitable) / 100
        if profitable else 0
    )
    avg_loss_rate = (
        sum(t.profit_rate for t in losing) / len(losing) / 100
        if losing else 0
    )

    # 손익비
    pl_ratio = abs(avg_profit_rate / avg_loss_rate) if avg_loss_rate else 0

    # Profit Factor
    gross_profit = sum(t.profit_krw for t in profitable)
    gross_loss = abs(sum(t.profit_krw for t in losing))
    profit_factor = gross_profit / gross_loss if gross_loss else 0

    # 기대값
    avg_profit_amount = gross_profit / len(profitable) if profitable else 0
    avg_loss_amount = abs(sum(t.profit_krw for t in losing)) / len(losing) if losing else 0
    expectancy = (avg_profit_amount * win_rate) - (avg_loss_amount * (1 - win_rate))

    # 전월대비 변화
    mom_change = None
    if prev_profit is not None and prev_profit != 0:
        mom_change = (profit_krw - prev_profit) / abs(prev_profit)
    prev_profit = profit_krw

    results.append({
        "month": month,
        "sell_count": sell_count,
        "profit_krw": profit_krw,
        "return_rate": return_rate,
        "win_rate": win_rate,
        "avg_profit_rate": avg_profit_rate,
        "avg_loss_rate": avg_loss_rate,
        "pl_ratio": round(pl_ratio, 2),
        "profit_factor": round(profit_factor, 2),
        "expectancy": expectancy,
        "mom_change": mom_change,
    })

return results
```

#### 1-2. 섹션 작성 메서드: `_write_monthly_trend()`

```python
async def _write_monthly_trend(self, trades: List[Trade], start_row: int) -> int:
    """섹션 6: 월별 성과 추이"""
    headers = [
        "연월", "매도건수", "실현손익(원)", "수익률(%)",
        "승률(%)", "평균수익률(%)", "평균손실률(%)",
        "손익비", "Profit Factor", "기대값(원)", "전월대비(%)",
    ]

    sell_trades = [t for t in trades if t.trade_type == '매도']
    sorted_sells = sorted(sell_trades, key=lambda t: t.date)
    trend_data = self._calc_monthly_trend(sorted_sells)

    rows = []
    for d in trend_data:
        rows.append([
            d["month"],
            d["sell_count"],
            d["profit_krw"],
            d["return_rate"],
            d["win_rate"],
            d["avg_profit_rate"],
            d["avg_loss_rate"],
            d["pl_ratio"],
            d["profit_factor"],
            d["expectancy"],
            d["mom_change"] if d["mom_change"] is not None else "",
        ])

    # 헤더 + 데이터 작성
    await self.client.update_cells(DASHBOARD_SHEET, f"A{start_row}", [headers])
    if rows:
        end_row = start_row + len(rows)
        await self.client.batch_update_cells(
            DASHBOARD_SHEET, {f"A{start_row + 1}:K{end_row}": rows}
        )
        logger.info(f"대시보드 월별 성과 추이: {len(rows)}행 작성")
        return end_row + 1

    return start_row + 1
```

#### 1-3. `generate_all()` 수정

섹션 6 추가 + 종목별 현황(섹션 3)을 맨 아래로 이동:

```python
async def generate_all(self, all_trades: List[Trade]):
    await self._ensure_dashboard_sheet()

    current_row = 1
    current_row = await self._write_portfolio_summary(all_trades, current_row)
    current_row += 1
    monthly_start = current_row
    current_row = await self._write_monthly_summary(all_trades, current_row)
    current_row += 1
    metrics_start = current_row
    current_row = await self._write_investment_metrics(all_trades, current_row)
    current_row += 1
    insights_start = current_row
    current_row = await self._write_trading_insights(all_trades, current_row)
    current_row += 1
    trend_start = current_row  # 신규
    current_row = await self._write_monthly_trend(all_trades, current_row)
    current_row += 1
    stock_start = current_row  # 종목별 현황을 맨 아래로 이동
    current_row = await self._write_stock_summary(all_trades, current_row)

    # 포맷 적용
    await self._apply_header_colors(monthly_start, trend_start, stock_start)
    await self._apply_dashboard_formats(monthly_start, metrics_start,
                                        insights_start, trend_start,
                                        stock_start, current_row)
```

#### 1-4. `_apply_header_colors()` 수정

섹션 순서 변경에 맞춰 헤더 배경색 적용:

```python
async def _apply_header_colors(self, monthly_start, trend_start, stock_start):
    header_color = {'red': 0.24, 'green': 0.52, 'blue': 0.78}
    header_rows = [
        {'row': 1, 'end_col': 7},              # 포트폴리오 요약 (A~G)
        {'row': monthly_start, 'end_col': 8},   # 월별 성과 (A~H)
        {'row': trend_start, 'end_col': 11},    # 월별 성과 추이 (A~K)
        {'row': stock_start, 'end_col': 11},    # 종목별 현황 (A~K)
    ]
    # ... 기존 로직 동일
```

#### 1-5. `_apply_dashboard_formats()` 수정

섹션 6 컬럼별 포맷 추가:

```python
# 섹션 6: 월별 성과 추이
trend_data_end = total_rows  # 또는 다음 섹션 시작 - 2
if trend_data_end > trend_start:
    trend_formats = [
        {'col': 2, 'pattern': '#,##0'},                           # B: 매도건수
        {'col': 3, 'pattern': '₩#,##0'},                         # C: 실현손익
        {'col': 4, 'pattern': '0.00%', 'type': 'PERCENT'},       # D: 수익률
        {'col': 5, 'pattern': '0.00%', 'type': 'PERCENT'},       # E: 승률
        {'col': 6, 'pattern': '0.00%', 'type': 'PERCENT'},       # F: 평균수익률
        {'col': 7, 'pattern': '0.00%', 'type': 'PERCENT'},       # G: 평균손실률
        {'col': 8, 'pattern': '0.00'},                            # H: 손익비
        {'col': 9, 'pattern': '0.00'},                            # I: Profit Factor
        {'col': 10, 'pattern': '₩#,##0'},                        # J: 기대값
        {'col': 11, 'pattern': '0.0%', 'type': 'PERCENT'},       # K: 전월대비
    ]
    await self.client.apply_number_format_to_columns(
        DASHBOARD_SHEET, trend_formats, trend_start + 1, trend_data_end
    )
```

### 2. `tests/test_summary_monthly_trend.py` (신규)

`_calc_monthly_trend()` 정적 메서드의 단위 테스트.

```python
"""월별 성과 추이 계산 로직 단위 테스트"""

import pytest
from modules.models import Trade
from modules.summary_generator import SummaryGenerator


def _make_sell(date, profit_krw, amount_krw=700000, profit_rate=0.0):
    """테스트용 매도 Trade 생성 헬퍼"""
    return Trade(
        date=date, trade_type="매도", stock_name="삼성전자", stock_code="005930",
        quantity=10, price=70000, amount=amount_krw, currency="KRW",
        exchange_rate=1.0, amount_krw=amount_krw, fee=0, tax=0,
        profit=profit_krw, profit_krw=profit_krw, profit_rate=profit_rate,
        account="테스트_국내계좌",
    )
```

**테스트 케이스**:

| 테스트 | 검증 |
|--------|------|
| `test_single_month` | 1개월 데이터 → 기본 지표 산출, mom_change는 None |
| `test_two_months_mom_change` | 2개월 → 전월대비 변화율 정확성 |
| `test_all_wins_month` | 손실 없는 월 → avg_loss_rate=0, pl_ratio=0, profit_factor=0 |
| `test_all_losses_month` | 수익 없는 월 → avg_profit_rate=0, win_rate=0 |
| `test_mixed_months` | 3개월 혼합 → 각 월별 독립 계산 확인 |
| `test_empty_sells` | 빈 리스트 → 빈 결과 반환 |
| `test_win_rate_calculation` | 승률 정확성 (수익3건/전체5건 = 0.6) |
| `test_expectancy_calculation` | 기대값 공식 검증 |

## 대시보드 시트 레이아웃 (변경 후)

```
[섹션 1: 포트폴리오 요약] (행 1-2)
(빈 행)
[섹션 2: 월별 성과] - 계좌별 상세 (기존 유지)
(빈 행)
[섹션 4: 투자 지표] (기존 유지)
(빈 행)
[섹션 5: 매매 인사이트] (기존 유지)
(빈 행)
[섹션 6: 월별 성과 추이] ← 신규
  연월 | 매도건수 | 실현손익 | 수익률 | 승률 | 평균수익률 | 평균손실률 | 손익비 | PF | 기대값 | 전월대비
(빈 행)
[섹션 3: 종목별 현황] ← 맨 아래로 이동
```
