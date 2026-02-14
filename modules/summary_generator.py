"""대시보드 시트 생성 모듈

기존 요약_월별 + 요약_종목별 → 대시보드 단일 시트로 통합.
4개 섹션: 포트폴리오 요약, 월별 성과, 종목별 현황, 투자 지표.
"""

import logging
from collections import defaultdict
from typing import TYPE_CHECKING, Dict, List, Optional, Tuple

from .google_sheets_client import GoogleSheetsClient
from .sheet_writer import SheetWriter

from .models import Trade

if TYPE_CHECKING:
    from .sector_classifier import SectorClassifier

logger = logging.getLogger(__name__)

DASHBOARD_SHEET = "대시보드"


class SummaryGenerator:
    """대시보드 시트 생성기"""

    def __init__(self, client: GoogleSheetsClient, sheet_writer: SheetWriter,
                 sector_classifier: Optional["SectorClassifier"] = None):
        self.client = client
        self.sheet_writer = sheet_writer
        self.sector_classifier = sector_classifier

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
        await self._apply_header_colors(monthly_start, stock_start, metrics_start)
        await self._apply_dashboard_formats(monthly_start, stock_start, metrics_start, current_row)

        logger.info("대시보드 시트 갱신 완료")

    async def _ensure_dashboard_sheet(self):
        """대시보드 시트 확보 (없으면 생성, 있으면 초기화)"""
        sheets = await self.client.list_sheets()
        if DASHBOARD_SHEET not in sheets:
            await self.client.create_sheet(DASHBOARD_SHEET)
        else:
            await self.client.clear_sheet(DASHBOARD_SHEET, start_row=1)
            await self.client.clear_background_colors(DASHBOARD_SHEET)

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
        pct_rows = []   # % 포맷 행 (0-based offset)
        krw_rows = []   # 원화 포맷 행

        # 계좌별 투자비중
        rows.append(["계좌별 투자비중", ""])
        account_buy: Dict[str, float] = defaultdict(float)
        for t in buy_trades:
            account_buy[t.account] += t.amount_krw
        for account, amount in sorted(account_buy.items()):
            pct_rows.append(len(rows))
            rows.append([f"  {account}", amount / total_buy if total_buy else 0])

        # 통화별 투자비중
        rows.append(["통화별 투자비중", ""])
        currency_buy: Dict[str, float] = defaultdict(float)
        for t in buy_trades:
            currency_buy[t.currency] += t.amount_krw
        for currency, amount in sorted(currency_buy.items()):
            pct_rows.append(len(rows))
            rows.append([f"  {currency}", amount / total_buy if total_buy else 0])

        # 섹터별 투자비중
        if self.sector_classifier:
            sector_map = await self._get_sector_map(buy_trades)
            if sector_map:
                rows.append(["섹터별 투자비중", ""])
                sector_buy: Dict[str, float] = defaultdict(float)
                for t in buy_trades:
                    sector = sector_map.get(t.stock_name, "기타")
                    sector_buy[sector] += t.amount_krw
                for sector, amount in sorted(sector_buy.items(),
                                             key=lambda x: x[1], reverse=True):
                    pct_rows.append(len(rows))
                    rows.append([f"  {sector}", amount / total_buy if total_buy else 0])

        # 상위 5종목 집중도
        stock_buy: Dict[str, float] = defaultdict(float)
        for t in buy_trades:
            stock_buy[t.stock_name] += t.amount_krw
        top5 = sorted(stock_buy.values(), reverse=True)[:5]
        top5_ratio = sum(top5) / total_buy if total_buy else 0
        pct_rows.append(len(rows))
        rows.append(["상위 5종목 집중도", top5_ratio])

        # 평균 수익률 / 평균 손실률
        profit_rates = [t.profit_rate for t in sell_trades if t.profit_rate > 0]
        loss_rates = [t.profit_rate for t in sell_trades if t.profit_rate < 0]
        avg_profit = (sum(profit_rates) / len(profit_rates) / 100) if profit_rates else 0
        avg_loss = (sum(loss_rates) / len(loss_rates) / 100) if loss_rates else 0
        pct_rows.append(len(rows))
        rows.append(["평균 수익률", avg_profit])
        pct_rows.append(len(rows))
        rows.append(["평균 손실률", avg_loss])

        # 손익비
        pl_ratio = abs(avg_profit / avg_loss) if avg_loss else 0
        rows.append(["손익비", round(pl_ratio, 2)])

        # 수익 Top 5 / 손실 Top 5 (종목별 합산)
        if sell_trades:
            stock_profit: Dict[str, float] = defaultdict(float)
            for t in sell_trades:
                stock_profit[t.stock_name] += t.profit_krw
            sorted_stocks = sorted(stock_profit.items(), key=lambda x: x[1], reverse=True)

            rows.append(["수익 Top 5", ""])
            for name, profit in sorted_stocks[:5]:
                krw_rows.append(len(rows))
                rows.append([f"  {name}", profit])

            rows.append(["손실 Top 5", ""])
            for name, profit in sorted_stocks[-5:][::-1]:
                if profit < 0:
                    krw_rows.append(len(rows))
                    rows.append([f"  {name}", profit])

        # 데이터 작성
        end_row = start_row + len(rows) - 1
        await self.client.batch_update_cells(
            DASHBOARD_SHEET, {f"A{start_row}:B{end_row}": rows}
        )

        # 행별 포맷 적용
        await self._apply_metrics_formats(start_row, pct_rows, krw_rows)

        logger.info(f"대시보드 투자 지표: {len(rows)}행 작성")
        return start_row + len(rows)

    async def _apply_header_colors(self, monthly_start: int,
                                     stock_start: int, metrics_start: int):
        """대시보드 헤더 행에 배경색 적용"""
        header_color = {'red': 0.24, 'green': 0.52, 'blue': 0.78}  # 파란색
        header_rows = [
            {'row': 1, 'end_col': 7},             # 포트폴리오 요약 (A~G)
            {'row': monthly_start, 'end_col': 8},  # 월별 성과 (A~H)
            {'row': stock_start, 'end_col': 11},   # 종목별 현황 (A~K)
        ]

        color_ranges = []
        for h in header_rows:
            color_ranges.append({
                'start_row': h['row'],
                'end_row': h['row'],
                'start_col': 1,
                'end_col': h['end_col'],
                'color': header_color,
            })

        await self.client.batch_apply_colors(DASHBOARD_SHEET, color_ranges)
        logger.info("대시보드 헤더 배경색 적용 완료")

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

        # 섹션 4: 투자 지표 → _apply_metrics_formats()에서 행별 처리

    async def _apply_metrics_formats(self, start_row: int,
                                     pct_offsets: List[int],
                                     krw_offsets: List[int]):
        """투자 지표 섹션 행별 포맷 적용 (연속 행 그룹핑)"""
        pct_fmt = [{'col': 2, 'pattern': '0.00%', 'type': 'PERCENT'}]
        krw_fmt = [{'col': 2, 'pattern': '₩#,##0'}]

        for offsets, fmt in [(pct_offsets, pct_fmt), (krw_offsets, krw_fmt)]:
            if not offsets:
                continue
            abs_rows = sorted(start_row + o for o in offsets)
            for r_start, r_end in self._group_consecutive_rows(abs_rows):
                await self.client.apply_number_format_to_columns(
                    DASHBOARD_SHEET, fmt, r_start, r_end
                )

    @staticmethod
    def _group_consecutive_rows(rows: List[int]) -> List[Tuple[int, int]]:
        """연속된 행 번호를 (start, end) 그룹으로 묶기"""
        if not rows:
            return []
        groups = []
        start = end = rows[0]
        for r in rows[1:]:
            if r == end + 1:
                end = r
            else:
                groups.append((start, end))
                start = end = r
        groups.append((start, end))
        return groups

    async def _get_sector_map(self, buy_trades: List[Trade]) -> Dict[str, str]:
        """매수 종목 리스트에서 섹터 매핑 조회"""
        stocks = list({
            (t.stock_name, t.stock_code, t.currency)
            for t in buy_trades
        })
        try:
            return await self.sector_classifier.classify(stocks)
        except Exception as e:
            logger.error(f"섹터 분류 실패, 건너뜀: {e}")
            return {}
