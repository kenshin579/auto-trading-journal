"""미래에셋증권 CSV 파서"""

import csv
import logging
from pathlib import Path
from typing import List

from .base_parser import BaseParser
from ..models import Trade

logger = logging.getLogger(__name__)


def _parse_float(value: str) -> float:
    """문자열을 float로 변환 (빈 값은 0.0)"""
    if not value or value.strip() == "":
        return 0.0
    return float(value.strip().replace(",", ""))


def _convert_date(date_str: str) -> str:
    """날짜 포맷 변환: 2026/02/13 → 2026-02-13"""
    return date_str.strip().replace("/", "-")


class MiraeDomesticParser(BaseParser):
    """미래에셋증권 국내계좌 파서

    CSV 구조 (11컬럼):
        col 0: 일자
        col 1: 종목명
        col 2: 매수 수량
        col 3: 매수 평균단가
        col 4: 매수금액
        col 5: 매도 수량
        col 6: 매도 평균단가
        col 7: 매도금액
        col 8: 매매비용
        col 9: 손익금액
        col 10: 수익률
    """

    @staticmethod
    def can_parse(header_row: List[str]) -> bool:
        keywords = {"일자", "종목명", "기간 중 매수"}
        header_set = set(h.strip() for h in header_row)
        return keywords.issubset(header_set)

    def parse(self, file_path: Path, account: str) -> List[Trade]:
        trades: List[Trade] = []

        with open(file_path, "r", encoding="utf-8") as f:
            reader = csv.reader(f)
            next(reader)  # 헤더 행 건너뜀
            next(reader)  # 서브헤더 행 건너뜀

            for line_num, row in enumerate(reader, start=3):
                if len(row) < 11:
                    continue
                date_raw = row[0].strip()
                stock_name = row[1].strip()
                if not date_raw and not stock_name:
                    continue  # 빈 행 건너뜀
                if not date_raw:
                    raise ValueError(f"날짜가 비어있습니다: {file_path.name}, {line_num}행")
                if not stock_name:
                    continue

                date = _convert_date(date_raw)
                buy_qty = _parse_float(row[2])
                buy_price = _parse_float(row[3])
                buy_amount = _parse_float(row[4])
                sell_qty = _parse_float(row[5])
                sell_price = _parse_float(row[6])
                sell_amount = _parse_float(row[7])
                fee = _parse_float(row[8])
                profit_amount = _parse_float(row[9])
                profit_rate = _parse_float(row[10])

                if buy_qty > 0:
                    trades.append(Trade(
                        date=date,
                        trade_type="매수",
                        stock_name=stock_name,
                        stock_code="",
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

                if sell_qty > 0:
                    trades.append(Trade(
                        date=date,
                        trade_type="매도",
                        stock_name=stock_name,
                        stock_code="",
                        quantity=sell_qty,
                        price=sell_price,
                        amount=sell_amount,
                        currency="KRW",
                        exchange_rate=1.0,
                        amount_krw=sell_amount,
                        fee=fee,
                        tax=0.0,
                        profit=profit_amount,
                        profit_krw=profit_amount,
                        profit_rate=profit_rate,
                        account=account,
                    ))

        logger.info(f"미래에셋 국내 파싱 완료: {len(trades)}건 ({file_path.name})")
        return trades


class MiraeForeignParser(BaseParser):
    """미래에셋증권 해외계좌 파서

    CSV 구조 (25컬럼):
        col 0: 매매일
        col 1: 통화
        col 2: 종목번호 (티커)
        col 3: 종목명
        col 4: 잔고 수량
        col 5: 매입평균환율
        col 6: 매매일환율
        col 7: 매수 수량
        col 8: 매수단가
        col 9: 매수금액
        col 10: 원화매수금액
        col 11: 매도 수량
        col 12: 매도단가
        col 13: 매도금액
        col 14: 원화매도금액
        col 15: 수수료
        col 16: 세금
        col 17: 원화총비용
        col 18: 원매수평균가
        col 19: 매매손익
        col 20: 원화매매손익
        col 21: 환차손익
        col 22: 총평가손익
        col 23: 손익률
        col 24: 환산손익률
    """

    @staticmethod
    def can_parse(header_row: List[str]) -> bool:
        keywords = {"매매일", "통화", "종목번호"}
        header_set = set(h.strip() for h in header_row)
        return keywords.issubset(header_set)

    def parse(self, file_path: Path, account: str) -> List[Trade]:
        trades: List[Trade] = []

        with open(file_path, "r", encoding="utf-8") as f:
            reader = csv.reader(f)
            next(reader)  # 헤더 행 건너뜀

            for line_num, row in enumerate(reader, start=2):
                if len(row) < 25:
                    continue
                date_raw = row[0].strip()
                currency = row[1].strip()
                stock_code = row[2].strip()
                stock_name = row[3].strip()
                if not date_raw and not stock_name:
                    continue  # 빈 행 건너뜀
                if not date_raw:
                    raise ValueError(f"날짜가 비어있습니다: {file_path.name}, {line_num}행")
                if not stock_name:
                    continue

                date = _convert_date(date_raw)
                exchange_rate = _parse_float(row[6])
                buy_qty = _parse_float(row[7])
                buy_price = _parse_float(row[8])
                buy_amount = _parse_float(row[9])
                buy_amount_krw = _parse_float(row[10])
                sell_qty = _parse_float(row[11])
                sell_price = _parse_float(row[12])
                sell_amount = _parse_float(row[13])
                sell_amount_krw = _parse_float(row[14])
                fee = _parse_float(row[15])
                tax = _parse_float(row[16])
                profit = _parse_float(row[19])
                profit_krw = _parse_float(row[22])  # 총평가손익
                profit_rate = _parse_float(row[23])

                if buy_qty > 0:
                    trades.append(Trade(
                        date=date,
                        trade_type="매수",
                        stock_name=stock_name,
                        stock_code=stock_code,
                        quantity=buy_qty,
                        price=buy_price,
                        amount=buy_amount,
                        currency=currency,
                        exchange_rate=exchange_rate,
                        amount_krw=buy_amount_krw,
                        fee=0.0,
                        tax=0.0,
                        profit=0.0,
                        profit_krw=0.0,
                        profit_rate=0.0,
                        account=account,
                    ))

                if sell_qty > 0:
                    trades.append(Trade(
                        date=date,
                        trade_type="매도",
                        stock_name=stock_name,
                        stock_code=stock_code,
                        quantity=sell_qty,
                        price=sell_price,
                        amount=sell_amount,
                        currency=currency,
                        exchange_rate=exchange_rate,
                        amount_krw=sell_amount_krw,
                        fee=fee,
                        tax=tax,
                        profit=profit,
                        profit_krw=profit_krw,
                        profit_rate=profit_rate,
                        account=account,
                    ))

        logger.info(f"미래에셋 해외 파싱 완료: {len(trades)}건 ({file_path.name})")
        return trades
