"""대시보드 시트 생성 모듈

기존 요약_월별 + 요약_종목별 → 대시보드 단일 시트로 통합.
4개 섹션: 포트폴리오 요약, 월별 성과, 종목별 현황, 투자 지표.
"""

import logging
from collections import defaultdict
from typing import Dict, List, Tuple

from .google_sheets_client import GoogleSheetsClient
from .sheet_writer import SheetWriter

from .models import Trade

logger = logging.getLogger(__name__)

DASHBOARD_SHEET = "대시보드"


class SummaryGenerator:
    """대시보드 시트 생성기"""

    def __init__(self, client: GoogleSheetsClient, sheet_writer: SheetWriter):
        self.client = client
        self.sheet_writer = sheet_writer

    async def generate_all(self, all_trades: List[Trade]):
        """대시보드 시트 생성 (초기화 후 재작성)"""
        await self._ensure_dashboard_sheet()

        current_row = 1
        current_row = await self._write_portfolio_summary(all_trades, current_row)
        current_row += 1  # 빈 행
        monthly_start = current_row
        current_row = await self._write_monthly_summary(all_trades, current_row)
        current_row += 1  # 빈 행
        stock_start = current_row
        current_row = await self._write_stock_summary(all_trades, current_row)
        current_row += 1  # 빈 행
        metrics_start = current_row
        current_row = await self._write_investment_metrics(all_trades, current_row)

        # 포맷 적용
        await self._apply_dashboard_formats(monthly_start, stock_start, metrics_start, current_row)

        logger.info("대시보드 시트 갱신 완료")

    async def _ensure_dashboard_sheet(self):
        """대시보드 시트 확보 (없으면 생성, 있으면 초기화)"""
        sheets = await self.client.list_sheets()
        if DASHBOARD_SHEET not in sheets:
            await self.client.create_sheet(DASHBOARD_SHEET)
        else:
            await self.client.clear_sheet(DASHBOARD_SHEET, start_row=1)

        await self.client.freeze_rows(DASHBOARD_SHEET, 1)

    async def _write_portfolio_summary(self, trades: List[Trade], start_row: int) -> int:
        """섹션 1: 포트폴리오 요약 (2행)"""
        buy_trades = [t for t in trades if t.trade_type == '매수']
        sell_trades = [t for t in trades if t.trade_type == '매도']

        total_buy = sum(t.amount_krw for t in buy_trades)
        total_sell = sum(t.amount_krw for t in sell_trades)
        total_profit = sum(t.profit_krw for t in sell_trades)
        total_return = total_profit / total_sell if total_sell else 0
        total_count = len(trades)

        # 승률: profit_krw > 0인 매도 건수 / 전체 매도 건수
        profitable = len([t for t in sell_trades if t.profit_krw > 0])
        win_rate = profitable / len(sell_trades) if sell_trades else 0

        headers = [
            "지표", "총 매수금액(원)", "총 매도금액(원)", "총 실현손익(원)",
            "총 수익률(%)", "총 거래건수", "승률(%)",
        ]
        values = [
            "값", total_buy, total_sell, total_profit,
            total_return, total_count, win_rate,
        ]

        await self.client.batch_update_cells(
            DASHBOARD_SHEET, {
                f"A{start_row}:G{start_row}": [headers],
                f"A{start_row + 1}:G{start_row + 1}": [values],
            }
        )

        return start_row + 2

    async def _write_monthly_summary(self, trades: List[Trade], start_row: int) -> int:
        """섹션 2: 월별 성과"""
        headers = [
            "연월", "계좌", "매수건수", "매수금액(원)",
            "매도건수", "매도금액(원)", "실현손익(원)", "수익률(%)",
        ]

        # (연월, 계좌) 기준 집계
        groups: Dict[Tuple[str, str], Dict] = defaultdict(
            lambda: {"buy_count": 0, "buy_amount": 0.0,
                     "sell_count": 0, "sell_amount": 0.0, "profit": 0.0}
        )

        for t in trades:
            month = t.date[:7]
            key = (month, t.account)
            g = groups[key]
            if t.trade_type == "매수":
                g["buy_count"] += 1
                g["buy_amount"] += t.amount_krw
            elif t.trade_type == "매도":
                g["sell_count"] += 1
                g["sell_amount"] += t.amount_krw
                g["profit"] += t.profit_krw

        rows = []
        for (month, account), g in sorted(groups.items()):
            profit_rate = g["profit"] / g["sell_amount"] if g["sell_amount"] else 0
            rows.append([
                month, account,
                g["buy_count"], g["buy_amount"],
                g["sell_count"], g["sell_amount"],
                g["profit"], profit_rate,
            ])

        # 헤더 + 데이터 작성
        await self.client.update_cells(DASHBOARD_SHEET, f"A{start_row}", [headers])
        if rows:
            end_row = start_row + len(rows)
            await self.client.batch_update_cells(
                DASHBOARD_SHEET, {f"A{start_row + 1}:H{end_row}": rows}
            )
            logger.info(f"대시보드 월별 성과: {len(rows)}행 작성")
            return end_row + 1

        return start_row + 1

    async def _write_stock_summary(self, trades: List[Trade], start_row: int) -> int:
        """섹션 3: 종목별 현황"""
        headers = [
            "종목명", "종목코드", "계좌", "통화",
            "총매수수량", "총매수금액(원)", "총매도수량", "총매도금액(원)",
            "실현손익(원)", "수익률(%)", "투자비중(%)",
        ]

        # (종목명, 종목코드, 계좌, 통화) 기준 집계
        groups: Dict[Tuple[str, str, str, str], Dict] = defaultdict(
            lambda: {"buy_qty": 0.0, "buy_amount": 0.0,
                     "sell_qty": 0.0, "sell_amount": 0.0, "profit": 0.0}
        )

        for t in trades:
            key = (t.stock_name, t.stock_code, t.account, t.currency)
            g = groups[key]
            if t.trade_type == "매수":
                g["buy_qty"] += t.quantity
                g["buy_amount"] += t.amount_krw
            elif t.trade_type == "매도":
                g["sell_qty"] += t.quantity
                g["sell_amount"] += t.amount_krw
                g["profit"] += t.profit_krw

        total_buy_amount = sum(g["buy_amount"] for g in groups.values())

        rows = []
        for (name, code, account, currency), g in sorted(groups.items()):
            profit_rate = g["profit"] / g["sell_amount"] if g["sell_amount"] else 0
            weight = g["buy_amount"] / total_buy_amount if total_buy_amount else 0
            rows.append([
                name, code, account, currency,
                g["buy_qty"], g["buy_amount"],
                g["sell_qty"], g["sell_amount"],
                g["profit"], profit_rate, weight,
            ])

        # 헤더 + 데이터 작성
        await self.client.update_cells(DASHBOARD_SHEET, f"A{start_row}", [headers])
        if rows:
            end_row = start_row + len(rows)
            await self.client.batch_update_cells(
                DASHBOARD_SHEET, {f"A{start_row + 1}:K{end_row}": rows}
            )
            logger.info(f"대시보드 종목별 현황: {len(rows)}행 작성")
            return end_row + 1

        return start_row + 1

    async def _write_investment_metrics(self, trades: List[Trade], start_row: int) -> int:
        """섹션 4: 투자 지표"""
        buy_trades = [t for t in trades if t.trade_type == '매수']
        sell_trades = [t for t in trades if t.trade_type == '매도']
        total_buy = sum(t.amount_krw for t in buy_trades)

        rows = [["[투자 지표]", ""]]

        # 계좌별 투자비중
        rows.append(["계좌별 투자비중", ""])
        account_buy: Dict[str, float] = defaultdict(float)
        for t in buy_trades:
            account_buy[t.account] += t.amount_krw
        for account, amount in sorted(account_buy.items()):
            rows.append([f"  {account}", amount / total_buy if total_buy else 0])

        # 통화별 투자비중
        rows.append(["통화별 투자비중", ""])
        currency_buy: Dict[str, float] = defaultdict(float)
        for t in buy_trades:
            currency_buy[t.currency] += t.amount_krw
        for currency, amount in sorted(currency_buy.items()):
            rows.append([f"  {currency}", amount / total_buy if total_buy else 0])

        # 상위 5종목 집중도
        stock_buy: Dict[str, float] = defaultdict(float)
        for t in buy_trades:
            stock_buy[t.stock_name] += t.amount_krw
        top5 = sorted(stock_buy.values(), reverse=True)[:5]
        top5_ratio = sum(top5) / total_buy if total_buy else 0
        rows.append(["상위 5종목 집중도", top5_ratio])

        # 평균 수익률 / 평균 손실률
        profit_rates = [t.profit_rate for t in sell_trades if t.profit_rate > 0]
        loss_rates = [t.profit_rate for t in sell_trades if t.profit_rate < 0]
        avg_profit = (sum(profit_rates) / len(profit_rates) / 100) if profit_rates else 0
        avg_loss = (sum(loss_rates) / len(loss_rates) / 100) if loss_rates else 0
        rows.append(["평균 수익률", avg_profit])
        rows.append(["평균 손실률", avg_loss])

        # 손익비
        pl_ratio = abs(avg_profit / avg_loss) if avg_loss else 0
        rows.append(["손익비", round(pl_ratio, 2)])

        # 최대 수익/손실 종목
        if sell_trades:
            best = max(sell_trades, key=lambda t: t.profit_krw)
            worst = min(sell_trades, key=lambda t: t.profit_krw)
            rows.append(["최대 수익 종목", f"{best.stock_name} (+{best.profit_krw:,.0f}원)"])
            rows.append(["최대 손실 종목", f"{worst.stock_name} ({worst.profit_krw:,.0f}원)"])

        # 데이터 작성
        end_row = start_row + len(rows) - 1
        await self.client.batch_update_cells(
            DASHBOARD_SHEET, {f"A{start_row}:B{end_row}": rows}
        )

        logger.info(f"대시보드 투자 지표: {len(rows)}행 작성")
        return start_row + len(rows)

    async def _apply_dashboard_formats(self, monthly_start: int,
                                       stock_start: int, metrics_start: int,
                                       total_rows: int):
        """대시보드 시트에 숫자 포맷 적용"""
        # 섹션 1: 포트폴리오 요약 (행 2)
        portfolio_formats = [
            {'col': 2, 'pattern': '#,##0'},       # B: 총 매수금액
            {'col': 3, 'pattern': '#,##0'},       # C: 총 매도금액
            {'col': 4, 'pattern': '#,##0'},       # D: 총 실현손익
            {'col': 5, 'pattern': '0.00%', 'type': 'PERCENT'},  # E: 총 수익률
            {'col': 6, 'pattern': '#,##0'},       # F: 총 거래건수
            {'col': 7, 'pattern': '0.00%', 'type': 'PERCENT'},  # G: 승률
        ]
        await self.client.apply_number_format_to_columns(
            DASHBOARD_SHEET, portfolio_formats, 2, 2
        )

        # 섹션 2: 월별 성과 (monthly_start+1 ~ stock_start-2)
        monthly_data_end = stock_start - 2
        if monthly_data_end > monthly_start:
            monthly_formats = [
                {'col': 3, 'pattern': '#,##0'},       # C: 매수건수
                {'col': 4, 'pattern': '#,##0'},       # D: 매수금액
                {'col': 5, 'pattern': '#,##0'},       # E: 매도건수
                {'col': 6, 'pattern': '#,##0'},       # F: 매도금액
                {'col': 7, 'pattern': '#,##0'},       # G: 실현손익
                {'col': 8, 'pattern': '0.00%', 'type': 'PERCENT'},  # H: 수익률
            ]
            await self.client.apply_number_format_to_columns(
                DASHBOARD_SHEET, monthly_formats, monthly_start + 1, monthly_data_end
            )

        # 섹션 3: 종목별 현황 (stock_start+1 ~ metrics_start-2)
        stock_data_end = metrics_start - 2
        if stock_data_end > stock_start:
            stock_formats = [
                {'col': 5, 'pattern': '#,##0'},       # E: 총매수수량
                {'col': 6, 'pattern': '#,##0'},       # F: 총매수금액
                {'col': 7, 'pattern': '#,##0'},       # G: 총매도수량
                {'col': 8, 'pattern': '#,##0'},       # H: 총매도금액
                {'col': 9, 'pattern': '#,##0'},       # I: 실현손익
                {'col': 10, 'pattern': '0.00%', 'type': 'PERCENT'},  # J: 수익률
                {'col': 11, 'pattern': '0.00%', 'type': 'PERCENT'},  # K: 투자비중
            ]
            await self.client.apply_number_format_to_columns(
                DASHBOARD_SHEET, stock_formats, stock_start + 1, stock_data_end
            )

        # 섹션 4: 투자 지표 - 비율 값에 퍼센트 포맷
        if total_rows > metrics_start:
            metrics_formats = [
                {'col': 2, 'pattern': '0.00%', 'type': 'PERCENT'},
            ]
            await self.client.apply_number_format_to_columns(
                DASHBOARD_SHEET, metrics_formats, metrics_start + 1, total_rows
            )
