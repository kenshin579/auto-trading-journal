"""한국투자증권 CSV 파서"""

import csv
import logging
from pathlib import Path
from typing import List

from .base_parser import BaseParser
from ..models import Trade

logger = logging.getLogger(__name__)


def _parse_float(value: str) -> float:
    """쌍따옴표, 천단위 쉼표 제거 후 float 변환"""
    cleaned = value.strip().strip('"').replace(",", "")
    if not cleaned:
        return 0.0
    return float(cleaned)


def _convert_date(date_str: str) -> str:
    """날짜 포맷 변환: 2026/02/13 → 2026-02-13"""
    return date_str.strip().strip('"').replace("/", "-")


class HankookDomesticParser(BaseParser):
    """한국투자증권 국내계좌 파서

    CSV 구조 (17컬럼, 쌍따옴표):
        col 0: 매매일자
        col 1: 종목명
        col 2: 종목코드
        col 3: 구분
        col 4: 대출일자
        col 5: 보유수량
        col 6: 매입단가
        col 7: 매수수량
        col 8: 매도단가
        col 9: 매도수량
        col 10: 매수금액
        col 11: 매도금액
        col 12: 실현손익
        col 13: 손익률
        col 14: 수수료
        col 15: 이자
        col 16: 제세금
    """

    @staticmethod
    def can_parse(header_row: List[str]) -> bool:
        keywords = {"매매일자", "종목코드", "매입단가"}
        header_set = set(h.strip().strip('"') for h in header_row)
        return keywords.issubset(header_set)

    def parse(self, file_path: Path, account: str) -> List[Trade]:
        trades: List[Trade] = []

        with open(file_path, "r", encoding="utf-8") as f:
            reader = csv.reader(f)
            next(reader)  # 헤더 행 건너뜀

            for line_num, row in enumerate(reader, start=2):
                if len(row) < 17:
                    continue

                stock_name = row[1].strip().strip('"')
                if not stock_name:
                    continue

                date_raw = row[0].strip().strip('"')
                if not date_raw:
                    continue

                date = _convert_date(date_raw)
                stock_code = row[2].strip().strip('"')
                buy_price = _parse_float(row[6])  # 매입단가
                buy_qty = _parse_float(row[7])  # 매수수량
                sell_price = _parse_float(row[8])  # 매도단가
                sell_qty = _parse_float(row[9])  # 매도수량
                buy_amount = _parse_float(row[10])
                sell_amount = _parse_float(row[11])
                realized_profit = _parse_float(row[12])
                profit_rate = _parse_float(row[13])
                commission = _parse_float(row[14])
                tax = _parse_float(row[16])

                if buy_qty > 0 and buy_amount > 0:
                    trades.append(Trade(
                        date=date,
                        trade_type="매수",
                        stock_name=stock_name,
                        stock_code=stock_code,
                        quantity=buy_qty,
                        price=buy_price,
                        amount=buy_amount,
                        currency="KRW",
                        exchange_rate=1.0,
                        amount_krw=buy_amount,
                        fee=0.0,
                        tax=0.0,
                        profit=0.0,
                        profit_krw=0.0,
                        profit_rate=0.0,
                        account=account,
                    ))

                if sell_qty > 0 and sell_amount > 0:
                    total_fee = commission + tax
                    trades.append(Trade(
                        date=date,
                        trade_type="매도",
                        stock_name=stock_name,
                        stock_code=stock_code,
                        quantity=sell_qty,
                        price=sell_price,
                        amount=sell_amount,
                        currency="KRW",
                        exchange_rate=1.0,
                        amount_krw=sell_amount,
                        fee=total_fee,
                        tax=0.0,
                        profit=realized_profit,
                        profit_krw=realized_profit,
                        profit_rate=profit_rate,
                        account=account,
                    ))

        logger.info(f"한국투자증권 국내 파싱 완료: {len(trades)}건 ({file_path.name})")
        return trades
