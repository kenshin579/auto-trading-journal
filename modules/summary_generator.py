"""요약 시트 생성 모듈"""

import logging
from collections import defaultdict
from typing import Dict, List, Tuple

from .google_sheets_client import GoogleSheetsClient
from .sheet_writer import SheetWriter
from .models import Trade

logger = logging.getLogger(__name__)

MONTHLY_SHEET = "요약_월별"
STOCK_SHEET = "요약_종목별"

MONTHLY_HEADERS = [
    "연월", "계좌", "매수건수", "매수금액(원)", "매도건수", "매도금액(원)", "실현손익(원)",
]

STOCK_HEADERS = [
    "종목명", "종목코드", "계좌", "통화",
    "총매수수량", "총매수금액(원)", "총매도수량", "총매도금액(원)", "실현손익(원)",
]


class SummaryGenerator:
    """요약 시트 생성기"""

    def __init__(self, client: GoogleSheetsClient, sheet_writer: SheetWriter):
        self.client = client
        self.sheet_writer = sheet_writer

    async def generate_all(self, all_trades: List[Trade]):
        """월별 + 종목별 요약 시트 모두 갱신"""
        await self.generate_monthly_summary(all_trades)
        await self.generate_stock_summary(all_trades)
        logger.info("요약 시트 갱신 완료")

    async def generate_monthly_summary(self, all_trades: List[Trade]):
        """월별 요약 시트 생성 (초기화 후 재작성)"""
        # 시트 확보
        sheets = await self.client.list_sheets()
        if MONTHLY_SHEET not in sheets:
            await self.client.create_sheet(MONTHLY_SHEET)
            await self.client.update_cells(MONTHLY_SHEET, "A1", [MONTHLY_HEADERS])
        else:
            await self.client.clear_sheet(MONTHLY_SHEET, start_row=2)

        # (연월, 계좌) 기준 집계
        groups: Dict[Tuple[str, str], Dict] = defaultdict(
            lambda: {"buy_count": 0, "buy_amount": 0.0, "sell_count": 0, "sell_amount": 0.0, "profit": 0.0}
        )

        for t in all_trades:
            month = t.date[:7]  # YYYY-MM
            key = (month, t.account)
            g = groups[key]
            if t.trade_type == "매수":
                g["buy_count"] += 1
                g["buy_amount"] += t.amount_krw
            elif t.trade_type == "매도":
                g["sell_count"] += 1
                g["sell_amount"] += t.amount_krw
                g["profit"] += t.profit_krw

        # 정렬 후 데이터 작성
        rows = []
        for (month, account), g in sorted(groups.items()):
            rows.append([
                month, account,
                g["buy_count"], g["buy_amount"],
                g["sell_count"], g["sell_amount"],
                g["profit"],
            ])

        if rows:
            end_row = 1 + len(rows)
            await self.client.batch_update_cells(
                MONTHLY_SHEET, {f"A2:G{end_row}": rows}
            )
            logger.info(f"요약_월별: {len(rows)}행 작성")

    async def generate_stock_summary(self, all_trades: List[Trade]):
        """종목별 요약 시트 생성 (초기화 후 재작성)"""
        # 시트 확보
        sheets = await self.client.list_sheets()
        if STOCK_SHEET not in sheets:
            await self.client.create_sheet(STOCK_SHEET)
            await self.client.update_cells(STOCK_SHEET, "A1", [STOCK_HEADERS])
        else:
            await self.client.clear_sheet(STOCK_SHEET, start_row=2)

        # (종목명, 종목코드, 계좌, 통화) 기준 집계
        groups: Dict[Tuple[str, str, str, str], Dict] = defaultdict(
            lambda: {"buy_qty": 0.0, "buy_amount": 0.0, "sell_qty": 0.0, "sell_amount": 0.0, "profit": 0.0}
        )

        for t in all_trades:
            key = (t.stock_name, t.stock_code, t.account, t.currency)
            g = groups[key]
            if t.trade_type == "매수":
                g["buy_qty"] += t.quantity
                g["buy_amount"] += t.amount_krw
            elif t.trade_type == "매도":
                g["sell_qty"] += t.quantity
                g["sell_amount"] += t.amount_krw
                g["profit"] += t.profit_krw

        # 정렬 후 데이터 작성
        rows = []
        for (name, code, account, currency), g in sorted(groups.items()):
            rows.append([
                name, code, account, currency,
                g["buy_qty"], g["buy_amount"],
                g["sell_qty"], g["sell_amount"],
                g["profit"],
            ])

        if rows:
            end_row = 1 + len(rows)
            await self.client.batch_update_cells(
                STOCK_SHEET, {f"A2:I{end_row}": rows}
            )
            logger.info(f"요약_종목별: {len(rows)}행 작성")
