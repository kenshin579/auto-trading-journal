"""대시보드 시트 생성 모듈

기존 요약_월별 + 요약_종목별 → 대시보드 단일 시트로 통합.
6개 섹션: 포트폴리오 요약, 월별 성과, 투자 지표, 매매 인사이트, 월별 성과 추이, 종목별 현황.
"""

import logging
from collections import defaultdict
from datetime import datetime
from typing import TYPE_CHECKING, Dict, List, Optional, Tuple

from .google_sheets_client import GoogleSheetsClient
from .sheet_writer import SheetWriter

from .models import Trade

if TYPE_CHECKING:
    from .sector_classifier import SectorClassifier

logger = logging.getLogger(__name__)

DASHBOARD_SHEET = "대시보드"

# 차트 배치 상수 (0-based 열/행 인덱스)
CHART_COL_START = 13        # N열
CHART_COL_SECONDARY = 20    # U열
CHART_ROW_SPACING = 20      # 차트 간 행 간격


class SummaryGenerator:
    """대시보드 시트 생성기"""

    def __init__(self, client: GoogleSheetsClient, sheet_writer: SheetWriter,
                 sector_classifier: Optional["SectorClassifier"] = None):
        self.client = client
        self.sheet_writer = sheet_writer
        self.sector_classifier = sector_classifier
        self._pie_data_range: Optional[Tuple[int, int]] = None
        self._trade_count_data_range: Optional[Tuple[int, int]] = None

    async def generate_all(self, all_trades: List[Trade]):
        """대시보드 시트 생성 (초기화 후 재작성)"""
        await self._ensure_dashboard_sheet()

        current_row = 1
        current_row = await self._write_portfolio_summary(all_trades, current_row)
        current_row += 1  # 빈 행
        monthly_start = current_row
        current_row = await self._write_monthly_summary(all_trades, current_row)
        current_row += 1  # 빈 행
        metrics_start = current_row
        current_row = await self._write_investment_metrics(all_trades, current_row)
        current_row += 1  # 빈 행
        insights_start = current_row
        current_row = await self._write_trading_insights(all_trades, current_row)
        current_row += 1  # 빈 행
        trend_start = current_row
        current_row = await self._write_monthly_trend(all_trades, current_row)
        current_row += 1  # 빈 행
        stock_start = current_row
        current_row = await self._write_stock_summary(all_trades, current_row)

        # 포맷 적용
        await self._apply_header_colors(monthly_start, trend_start, stock_start)
        await self._apply_dashboard_formats(monthly_start, metrics_start,
                                            insights_start, trend_start,
                                            stock_start, current_row)

        # 차트용 데이터 작성 및 차트 생성
        await self._write_trade_count_data(all_trades)
        await self._create_charts(
            trend_start=trend_start,
            trend_end=stock_start - 1,
        )

        logger.info("대시보드 시트 갱신 완료")

    async def _ensure_dashboard_sheet(self):
        """대시보드 시트 확보 (없으면 생성, 있으면 초기화)"""
        sheets = await self.client.list_sheets()
        if DASHBOARD_SHEET not in sheets:
            await self.client.create_sheet(DASHBOARD_SHEET)
        else:
            await self.client.clear_sheet(DASHBOARD_SHEET, start_row=1)
            await self.client.clear_background_colors(DASHBOARD_SHEET)
            await self.client.clear_number_formats(DASHBOARD_SHEET)
            await self.client.delete_all_charts(DASHBOARD_SHEET)

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

        # 파이 차트용 데이터 (N~O열에 별도 작성)
        pie_data = []
        for account, amount in sorted(account_buy.items()):
            pct_rows.append(len(rows))
            ratio = amount / total_buy if total_buy else 0
            rows.append([f"  {account}", ratio])
            pie_data.append([account, ratio])

        if pie_data:
            pie_end_row = start_row + len(pie_data) - 1
            await self.client.batch_update_cells(
                DASHBOARD_SHEET,
                {f"N{start_row}:O{pie_end_row}": pie_data}
            )
            self._pie_data_range = (start_row, pie_end_row)
        else:
            self._pie_data_range = None

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

        # 수익 Top 10 / 손실 Top 10 (종목별 합산)
        if sell_trades:
            stock_profit: Dict[str, float] = defaultdict(float)
            for t in sell_trades:
                stock_profit[t.stock_name] += t.profit_krw
            sorted_stocks = sorted(stock_profit.items(), key=lambda x: x[1], reverse=True)

            rows.append(["수익 Top 10", ""])
            for name, profit in sorted_stocks[:10]:
                krw_rows.append(len(rows))
                rows.append([f"  {name}", profit])

            rows.append(["손실 Top 10", ""])
            for name, profit in sorted_stocks[-10:][::-1]:
                if profit < 0:
                    krw_rows.append(len(rows))
                    rows.append([f"  {name}", profit])

        # 데이터 작성
        end_row = start_row + len(rows) - 1
        await self.client.batch_update_cells(
            DASHBOARD_SHEET, {f"A{start_row}:B{end_row}": rows}
        )

        # 행별 포맷 적용
        await self._apply_metrics_formats(start_row, pct_rows, krw_rows, len(rows))

        logger.info(f"대시보드 투자 지표: {len(rows)}행 작성")
        return start_row + len(rows)

    async def _write_trading_insights(self, trades: List[Trade], start_row: int) -> int:
        """섹션 5: 매매 인사이트"""
        sell_trades = [t for t in trades if t.trade_type == '매도']

        rows = [["[매매 인사이트]", ""]]
        pct_rows = []   # % 포맷 행 (0-based offset)
        krw_rows = []   # 원화 포맷 행

        if not sell_trades:
            rows.append(["매도 거래 없음", ""])
            end_row = start_row + len(rows) - 1
            await self.client.batch_update_cells(
                DASHBOARD_SHEET, {f"A{start_row}:B{end_row}": rows}
            )
            return start_row + len(rows)

        # --- 공통 계산 ---
        profitable = [t for t in sell_trades if t.profit_krw > 0]
        losing = [t for t in sell_trades if t.profit_krw <= 0]
        win_rate = len(profitable) / len(sell_trades) if sell_trades else 0
        loss_rate = 1 - win_rate

        avg_profit_amount = (sum(t.profit_krw for t in profitable) / len(profitable)
                             if profitable else 0)
        avg_loss_amount = (sum(t.profit_krw for t in losing) / len(losing)
                           if losing else 0)

        # --- 5-1. 매매 기대값 (Expectancy) ---
        rows.append(["매매 기대값 (Expectancy)", ""])
        expectancy = (avg_profit_amount * win_rate) - (abs(avg_loss_amount) * loss_rate)
        krw_rows.append(len(rows))
        rows.append(["  1건당 기대 수익", expectancy])

        # --- 5-2. Profit Factor ---
        total_gross_profit = sum(t.profit_krw for t in profitable)
        total_gross_loss = abs(sum(t.profit_krw for t in losing))
        profit_factor = (total_gross_profit / total_gross_loss
                         if total_gross_loss else 0)
        rows.append(["Profit Factor", round(profit_factor, 2)])

        # --- 5-3. 연속 승/패 기록 ---
        rows.append(["연속 승/패 기록", ""])
        sorted_sells = sorted(sell_trades, key=lambda t: t.date)
        max_wins, max_losses, cur_wins, cur_losses = self._calc_streaks(sorted_sells)
        rows.append(["  최대 연승", max_wins])
        rows.append(["  최대 연패", max_losses])
        if cur_wins > 0:
            rows.append(["  현재 연승", cur_wins])
        else:
            rows.append(["  현재 연패", cur_losses])

        # --- 5-4. 요일별 성과 ---
        rows.append(["요일별 성과", ""])
        day_names = ["월요일", "화요일", "수요일", "목요일", "금요일", "토요일", "일요일"]
        day_stats = self._calc_day_of_week_stats(sorted_sells)
        for weekday_idx in range(7):
            stats = day_stats.get(weekday_idx)
            if stats:
                count, profit_sum, day_win_rate = stats
                rows.append([
                    f"  {day_names[weekday_idx]}",
                    f"{count}건 / ₩{profit_sum:,.0f} / 승률 {day_win_rate:.1f}%",
                ])

        # --- 5-5. 월별 수익 추세 (MoM 변화) ---
        rows.append(["월별 수익 추세", ""])
        monthly_profits = self._calc_monthly_profits(sorted_sells)
        prev_profit = None
        for month, profit_sum in monthly_profits:
            if prev_profit is not None and prev_profit != 0:
                change = profit_sum - prev_profit
                change_pct = change / abs(prev_profit) * 100
                label = f"  {month} (전월대비 {change_pct:+.1f}%)"
            else:
                label = f"  {month}"
            krw_rows.append(len(rows))
            rows.append([label, profit_sum])
            prev_profit = profit_sum

        # --- 5-6. 손절/익절 규율 분석 ---
        rows.append(["손절/익절 규율", ""])
        krw_rows.append(len(rows))
        rows.append(["  평균 수익 금액", avg_profit_amount])
        krw_rows.append(len(rows))
        rows.append(["  평균 손실 금액", avg_loss_amount])
        profit_multiple = (avg_profit_amount / abs(avg_loss_amount)
                           if avg_loss_amount else 0)
        rows.append(["  평균 수익 배수", round(profit_multiple, 2)])

        # --- 5-7. 거래 빈도 분석 ---
        rows.append(["거래 빈도 분석", ""])
        freq_stats = self._calc_trade_frequency(sorted_sells)
        rows.append(["  월 평균 거래건수", round(freq_stats["avg_monthly"], 1)])
        rows.append([f"  거래 가장 많은 달 ({freq_stats['max_month']})",
                     freq_stats["max_count"]])
        rows.append([f"  거래 가장 적은 달 ({freq_stats['min_month']})",
                     freq_stats["min_count"]])

        # --- 5-8. 수익 집중도 분석 ---
        rows.append(["수익 집중도", ""])
        stock_profit: Dict[str, float] = defaultdict(float)
        for t in sell_trades:
            stock_profit[t.stock_name] += t.profit_krw
        positive_stocks = {k: v for k, v in stock_profit.items() if v > 0}
        total_positive = sum(positive_stocks.values())
        sorted_positive = sorted(positive_stocks.values(), reverse=True)
        top3_ratio = sum(sorted_positive[:3]) / total_positive if total_positive else 0
        top5_ratio = sum(sorted_positive[:5]) / total_positive if total_positive else 0
        pct_rows.append(len(rows))
        rows.append(["  상위 3종목 수익 비중", top3_ratio])
        pct_rows.append(len(rows))
        rows.append(["  상위 5종목 수익 비중", top5_ratio])

        # 데이터 작성
        end_row = start_row + len(rows) - 1
        await self.client.batch_update_cells(
            DASHBOARD_SHEET, {f"A{start_row}:B{end_row}": rows}
        )

        # 행별 포맷 적용
        await self._apply_metrics_formats(start_row, pct_rows, krw_rows, len(rows))

        logger.info(f"대시보드 매매 인사이트: {len(rows)}행 작성")
        return start_row + len(rows)

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

        await self.client.update_cells(DASHBOARD_SHEET, f"A{start_row}", [headers])
        if rows:
            end_row = start_row + len(rows)
            await self.client.batch_update_cells(
                DASHBOARD_SHEET, {f"A{start_row + 1}:K{end_row}": rows}
            )
            logger.info(f"대시보드 월별 성과 추이: {len(rows)}행 작성")
            return end_row + 1

        return start_row + 1

    @staticmethod
    def _calc_monthly_trend(sorted_sells: List[Trade]) -> List[Dict]:
        """월별 성과 추이 계산

        Returns:
            List of dicts, 각 dict는 한 달의 지표:
            month, sell_count, profit_krw, return_rate, win_rate,
            avg_profit_rate, avg_loss_rate, pl_ratio, profit_factor,
            expectancy, mom_change
        """
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

            return_rate = profit_krw / total_sell_amount if total_sell_amount else 0
            win_rate = len(profitable) / sell_count

            avg_profit_rate = (
                sum(t.profit_rate for t in profitable) / len(profitable) / 100
                if profitable else 0
            )
            avg_loss_rate = (
                sum(t.profit_rate for t in losing) / len(losing) / 100
                if losing else 0
            )

            pl_ratio = abs(avg_profit_rate / avg_loss_rate) if avg_loss_rate else 0

            gross_profit = sum(t.profit_krw for t in profitable)
            gross_loss = abs(sum(t.profit_krw for t in losing))
            profit_factor = gross_profit / gross_loss if gross_loss else 0

            avg_profit_amount = gross_profit / len(profitable) if profitable else 0
            avg_loss_amount = gross_loss / len(losing) if losing else 0
            expectancy = (avg_profit_amount * win_rate) - (avg_loss_amount * (1 - win_rate))

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

    @staticmethod
    def _calc_streaks(sorted_sells: List[Trade]) -> Tuple[int, int, int, int]:
        """매도 거래의 연속 승/패 기록 계산

        Returns:
            (max_wins, max_losses, current_wins, current_losses)
        """
        max_wins = max_losses = 0
        cur_wins = cur_losses = 0

        for t in sorted_sells:
            if t.profit_krw > 0:
                cur_wins += 1
                cur_losses = 0
                max_wins = max(max_wins, cur_wins)
            else:
                cur_losses += 1
                cur_wins = 0
                max_losses = max(max_losses, cur_losses)

        return max_wins, max_losses, cur_wins, cur_losses

    @staticmethod
    def _calc_day_of_week_stats(
        sorted_sells: List[Trade],
    ) -> Dict[int, Tuple[int, float, float]]:
        """요일별 (거래건수, 실현손익합, 승률%) 계산"""
        day_groups: Dict[int, List[Trade]] = defaultdict(list)
        for t in sorted_sells:
            weekday = datetime.strptime(t.date, "%Y-%m-%d").weekday()
            day_groups[weekday].append(t)

        result = {}
        for weekday, group in sorted(day_groups.items()):
            count = len(group)
            profit_sum = sum(t.profit_krw for t in group)
            wins = len([t for t in group if t.profit_krw > 0])
            win_rate = wins / count * 100 if count else 0
            result[weekday] = (count, profit_sum, win_rate)
        return result

    @staticmethod
    def _calc_monthly_profits(sorted_sells: List[Trade]) -> List[Tuple[str, float]]:
        """월별 실현손익 집계 (정렬된 순서)"""
        monthly: Dict[str, float] = defaultdict(float)
        for t in sorted_sells:
            month = t.date[:7]
            monthly[month] += t.profit_krw
        return sorted(monthly.items())

    @staticmethod
    def _calc_trade_frequency(sorted_sells: List[Trade]) -> Dict:
        """월별 거래빈도 통계"""
        monthly_counts: Dict[str, int] = defaultdict(int)
        for t in sorted_sells:
            month = t.date[:7]
            monthly_counts[month] += 1

        if not monthly_counts:
            return {"avg_monthly": 0, "max_month": "-", "max_count": 0,
                    "min_month": "-", "min_count": 0}

        avg = sum(monthly_counts.values()) / len(monthly_counts)
        max_month = max(monthly_counts, key=monthly_counts.get)
        min_month = min(monthly_counts, key=monthly_counts.get)
        return {
            "avg_monthly": avg,
            "max_month": max_month,
            "max_count": monthly_counts[max_month],
            "min_month": min_month,
            "min_count": monthly_counts[min_month],
        }

    async def _apply_header_colors(self, monthly_start: int,
                                     trend_start: int, stock_start: int):
        """대시보드 헤더 행에 배경색 적용"""
        header_color = {'red': 0.24, 'green': 0.52, 'blue': 0.78}  # 파란색
        header_rows = [
            {'row': 1, 'end_col': 7},              # 포트폴리오 요약 (A~G)
            {'row': monthly_start, 'end_col': 8},   # 월별 성과 (A~H)
            {'row': trend_start, 'end_col': 11},    # 월별 성과 추이 (A~K)
            {'row': stock_start, 'end_col': 11},    # 종목별 현황 (A~K)
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
                                       metrics_start: int, insights_start: int,
                                       trend_start: int, stock_start: int,
                                       total_rows: int):
        """대시보드 시트에 숫자 포맷 적용"""
        # 섹션 1: 포트폴리오 요약 (행 2)
        portfolio_formats = [
            {'col': 2, 'pattern': '₩#,##0'},     # B: 총 매수금액
            {'col': 3, 'pattern': '₩#,##0'},     # C: 총 매도금액
            {'col': 4, 'pattern': '₩#,##0'},     # D: 총 실현손익
            {'col': 5, 'pattern': '0.00%', 'type': 'PERCENT'},  # E: 총 수익률
            {'col': 6, 'pattern': '#,##0'},       # F: 총 거래건수
            {'col': 7, 'pattern': '0.00%', 'type': 'PERCENT'},  # G: 승률
        ]
        await self.client.apply_number_format_to_columns(
            DASHBOARD_SHEET, portfolio_formats, 2, 2
        )

        # 섹션 2: 월별 성과 (monthly_start+1 ~ metrics_start-2)
        monthly_data_end = metrics_start - 2
        if monthly_data_end > monthly_start:
            monthly_formats = [
                {'col': 3, 'pattern': '#,##0'},       # C: 매수건수
                {'col': 4, 'pattern': '₩#,##0'},     # D: 매수금액
                {'col': 5, 'pattern': '#,##0'},       # E: 매도건수
                {'col': 6, 'pattern': '₩#,##0'},     # F: 매도금액
                {'col': 7, 'pattern': '₩#,##0'},     # G: 실현손익
                {'col': 8, 'pattern': '0.00%', 'type': 'PERCENT'},  # H: 수익률
            ]
            await self.client.apply_number_format_to_columns(
                DASHBOARD_SHEET, monthly_formats, monthly_start + 1, monthly_data_end
            )

        # 섹션 6: 월별 성과 추이 (trend_start+1 ~ stock_start-2)
        trend_data_end = stock_start - 2
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

        # 섹션 3: 종목별 현황 (stock_start+1 ~ total_rows)
        stock_data_end = total_rows
        if stock_data_end > stock_start:
            stock_formats = [
                {'col': 5, 'pattern': '#,##0'},       # E: 총매수수량
                {'col': 6, 'pattern': '₩#,##0'},     # F: 총매수금액
                {'col': 7, 'pattern': '#,##0'},       # G: 총매도수량
                {'col': 8, 'pattern': '₩#,##0'},     # H: 총매도금액
                {'col': 9, 'pattern': '₩#,##0'},     # I: 실현손익
                {'col': 10, 'pattern': '0.00%', 'type': 'PERCENT'},  # J: 수익률
                {'col': 11, 'pattern': '0.00%', 'type': 'PERCENT'},  # K: 투자비중
            ]
            await self.client.apply_number_format_to_columns(
                DASHBOARD_SHEET, stock_formats, stock_start + 1, stock_data_end
            )

        # 섹션 4/5: 투자 지표·매매 인사이트 → _apply_metrics_formats()에서 행별 처리

    async def _apply_metrics_formats(self, start_row: int,
                                     pct_offsets: List[int],
                                     krw_offsets: List[int],
                                     total_rows: int):
        """투자 지표 섹션 행별 포맷 적용 (연속 행 그룹핑)"""
        # 1단계: 섹션 전체 B열을 기본 NUMBER 포맷으로 초기화
        default_fmt = [{'col': 2, 'pattern': '#,##0.##'}]
        await self.client.apply_number_format_to_columns(
            DASHBOARD_SHEET, default_fmt, start_row, start_row + total_rows - 1
        )

        # 2단계: pct/krw 개별 포맷 적용
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

    async def _write_trade_count_data(self, trades: List[Trade]):
        """월별 매수/매도 건수·금액 차트용 데이터를 Q~U열에 작성"""
        month_stats: Dict[str, Dict[str, float]] = defaultdict(
            lambda: {"buy_count": 0, "sell_count": 0,
                     "buy_amount": 0.0, "sell_amount": 0.0}
        )
        for t in trades:
            month = t.date[:7]
            if t.trade_type == "매수":
                month_stats[month]["buy_count"] += 1
                month_stats[month]["buy_amount"] += t.amount_krw
            elif t.trade_type == "매도":
                month_stats[month]["sell_count"] += 1
                month_stats[month]["sell_amount"] += t.amount_krw

        if not month_stats:
            self._trade_count_data_range = None
            return

        rows = [["연월", "매수건수", "매도건수", "매수금액(원)", "매도금액(원)"]]
        for month in sorted(month_stats.keys()):
            s = month_stats[month]
            rows.append([month, s["buy_count"], s["sell_count"],
                         s["buy_amount"], s["sell_amount"]])

        start_row = 1
        end_row = start_row + len(rows) - 1
        await self.client.batch_update_cells(
            DASHBOARD_SHEET, {f"Q{start_row}:U{end_row}": rows}
        )
        self._trade_count_data_range = (start_row, end_row)
        logger.info(f"월별 매수/매도 차트 데이터: {len(rows) - 1}개월 작성")

    async def _create_charts(self, trend_start: int, trend_end: int):
        """대시보드 차트 생성

        Args:
            trend_start: 섹션 5(월별 성과 추이) 헤더 행 (1-based)
            trend_end: 섹션 5 마지막 데이터 행 (1-based)
        """
        sheet_id = await self.client.get_sheet_id(DASHBOARD_SHEET)
        if sheet_id is None:
            logger.error("대시보드 시트 ID를 찾을 수 없어 차트 생성 건너뜀")
            return

        # 섹션 5 데이터가 없으면 (헤더만 있으면) 차트 생성 스킵
        if trend_end <= trend_start:
            logger.info("월별 성과 추이 데이터 없음, 차트 생성 건너뜀")
            return

        # 0-based 인덱스 변환 (헤더 포함)
        data_start_0 = trend_start - 1
        data_end_0 = trend_end

        chart_specs = []

        # 차트 1: 월별 실현손익 추이 (Column)
        chart_specs.append(self._build_basic_chart_spec(
            sheet_id=sheet_id,
            title="월별 실현손익 추이",
            chart_type="COLUMN",
            domain_col=0,
            series_cols=[2],
            data_start=data_start_0,
            data_end=data_end_0,
            anchor_row=0,
            anchor_col=CHART_COL_START,
            width=600, height=370,
        ))

        # 차트 2: 월별 승률·수익률 추이 (Line)
        chart_specs.append(self._build_basic_chart_spec(
            sheet_id=sheet_id,
            title="월별 승률 & 수익률 추이",
            chart_type="LINE",
            domain_col=0,
            series_cols=[3, 4],
            data_start=data_start_0,
            data_end=data_end_0,
            anchor_row=CHART_ROW_SPACING,
            anchor_col=CHART_COL_START,
            width=600, height=370,
        ))

        # 차트 3: 계좌별 투자비중 (Pie)
        if self._pie_data_range:
            pie_start, pie_end = self._pie_data_range
            chart_specs.append(self._build_pie_chart_spec(
                sheet_id=sheet_id,
                title="계좌별 투자비중",
                label_col=CHART_COL_START,
                value_col=CHART_COL_START + 1,
                data_start=pie_start - 1,
                data_end=pie_end,
                anchor_row=CHART_ROW_SPACING * 2,
                anchor_col=CHART_COL_START,
                width=450, height=370,
            ))

        # 차트 4: 손익비·Profit Factor 추이 (Line)
        chart_specs.append(self._build_basic_chart_spec(
            sheet_id=sheet_id,
            title="손익비 & Profit Factor 추이",
            chart_type="LINE",
            domain_col=0,
            series_cols=[7, 8],
            data_start=data_start_0,
            data_end=data_end_0,
            anchor_row=CHART_ROW_SPACING * 2,
            anchor_col=CHART_COL_SECONDARY,
            width=450, height=370,
        ))

        # 차트 5, 6: 월별 매수/매도 건수·금액 추이
        if self._trade_count_data_range:
            tc_start, tc_end = self._trade_count_data_range

            # 차트 5: 건수 추이 (Column, 그룹)
            chart_specs.append(self._build_basic_chart_spec(
                sheet_id=sheet_id,
                title="월별 매수/매도 건수 추이",
                chart_type="COLUMN",
                domain_col=16,          # Q열 (연월)
                series_cols=[17, 18],   # R열 (매수건수), S열 (매도건수)
                data_start=tc_start - 1,
                data_end=tc_end,
                anchor_row=CHART_ROW_SPACING * 3,
                anchor_col=CHART_COL_START,
                width=600, height=370,
            ))

            # 차트 6: 금액 추이 (Column, 그룹)
            chart_specs.append(self._build_basic_chart_spec(
                sheet_id=sheet_id,
                title="월별 매수/매도 금액 추이",
                chart_type="COLUMN",
                domain_col=16,          # Q열 (연월)
                series_cols=[19, 20],   # T열 (매수금액), U열 (매도금액)
                data_start=tc_start - 1,
                data_end=tc_end,
                anchor_row=CHART_ROW_SPACING * 3,
                anchor_col=CHART_COL_SECONDARY,
                width=600, height=370,
            ))

        if chart_specs:
            await self.client.add_charts(chart_specs)
            logger.info(f"대시보드 차트 {len(chart_specs)}개 생성 완료")

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
            sheet_id: 시트 ID
            title: 차트 제목
            chart_type: COLUMN, LINE, BAR
            domain_col: X축 컬럼 (0-based)
            series_cols: Y축 컬럼 리스트 (0-based)
            data_start: 데이터 시작 행 (0-based, 헤더 포함)
            data_end: 데이터 끝 행 (0-based, exclusive)
            anchor_row: 차트 배치 행 (0-based)
            anchor_col: 차트 배치 열 (0-based)
            width: 차트 너비(px)
            height: 차트 높이(px)
        """
        def source_range(col: int) -> Dict:
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

    @staticmethod
    def _build_pie_chart_spec(
        sheet_id: int, title: str,
        label_col: int, value_col: int,
        data_start: int, data_end: int,
        anchor_row: int, anchor_col: int,
        width: int = 450, height: int = 370,
    ) -> Dict:
        """Pie 차트 스펙 생성

        Args:
            sheet_id: 시트 ID
            title: 차트 제목
            label_col: 레이블 컬럼 (0-based)
            value_col: 값 컬럼 (0-based)
            data_start: 데이터 시작 행 (0-based)
            data_end: 데이터 끝 행 (0-based, exclusive)
            anchor_row: 차트 배치 행 (0-based)
            anchor_col: 차트 배치 열 (0-based)
            width: 차트 너비(px)
            height: 차트 높이(px)
        """
        def source_range(col: int) -> Dict:
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
