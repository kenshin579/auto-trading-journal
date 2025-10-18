#!/usr/bin/env python3
"""
데이터 검증 모듈
거래 데이터의 유효성을 검증하는 기능을 제공합니다.
"""

import logging
import re
from typing import List, Tuple

from .trade_models import Trade, BaseTrade, DomesticTrade, ForeignTrade


logger = logging.getLogger(__name__)


class DataValidator:
    """데이터 검증 클래스"""
    
    def __init__(self, tolerance_percent: float = 0.001, tolerance_amount: float = 10):
        """초기화
        
        Args:
            tolerance_percent: 총액 계산 오차 허용 비율 (기본값: 0.1%)
            tolerance_amount: 총액 계산 오차 허용 금액 (기본값: 10원)
        """
        self.tolerance_percent = tolerance_percent
        self.tolerance_amount = tolerance_amount
        
        # 지원하는 통화 목록
        self.supported_currencies = ['USD', 'EUR', 'JPY', 'CNY', 'HKD', 'GBP', 'CAD', 'AUD']
    
    def validate_trade(self, trade: BaseTrade) -> List[str]:
        """개별 거래 데이터 검증
        
        Args:
            trade: 검증할 거래 데이터
            
        Returns:
            검증 실패 메시지 리스트 (비어있으면 유효함)
        """
        errors = []
        
        # 날짜 형식 검증 (공통)
        try:
            # YYYY-MM-DD 형식인지 확인
            parts = trade.date.split('-')
            if len(parts) != 3:
                errors.append(f"날짜 형식 오류: {trade.date} (예상: YYYY-MM-DD)")
            else:
                year, month, day = parts
                if len(year) != 4 or not year.isdigit():
                    errors.append(f"연도 형식 오류: {year}")
                if not (1 <= int(month) <= 12):
                    errors.append(f"월 범위 오류: {month}")
                if not (1 <= int(day) <= 31):
                    errors.append(f"일 범위 오류: {day}")
        except Exception as e:
            errors.append(f"날짜 파싱 오류: {trade.date} - {e}")
        
        # 타입별 검증
        if isinstance(trade, DomesticTrade):
            errors.extend(self._validate_domestic_trade(trade))
        elif isinstance(trade, ForeignTrade):
            errors.extend(self._validate_foreign_trade(trade))
        
        return errors
    
    def _validate_domestic_trade(self, trade: DomesticTrade) -> List[str]:
        """국내 주식 거래 검증"""
        errors = []
        
        # 수량 검증
        if trade.quantity <= 0:
            errors.append(f"수량이 0 이하입니다: {trade.quantity}")
        
        # 가격 검증
        if trade.price <= 0:
            errors.append(f"가격이 0 이하입니다: {trade.price}")
        
        # 총액 계산 검증
        expected_total = trade.quantity * trade.price
        tolerance = max(expected_total * self.tolerance_percent, self.tolerance_amount)
        if abs(trade.total_amount - expected_total) > tolerance:
            errors.append(
                f"총액 계산 오류: {trade.total_amount} != "
                f"{trade.quantity} × {trade.price} = {expected_total}"
            )
        
        # 거래 타입 검증
        if trade.trade_type not in ["매수", "매도"]:
            errors.append(f"잘못된 거래 타입: {trade.trade_type}")
        
        return errors
    
    def _validate_foreign_trade(self, trade: ForeignTrade) -> List[str]:
        """해외 주식 거래 검증"""
        errors = []
        
        # 통화 검증
        if trade.currency not in self.supported_currencies:
            errors.append(f"지원하지 않는 통화: {trade.currency}")
        
        # 티커 형식 검증
        if not trade.ticker or len(trade.ticker) > 10:
            errors.append(f"잘못된 티커 형식: {trade.ticker}")
        elif not re.match(r'^[A-Z0-9\-\.]{1,10}$', trade.ticker.upper()):
            errors.append(f"티커에 허용되지 않는 문자 포함: {trade.ticker}")
        
        # 환율 검증
        if trade.trade_exchange_rate <= 0:
            errors.append(f"거래일 환율이 0 이하입니다: {trade.trade_exchange_rate}")
        
        # 거래 타입 검증
        if trade.trade_type not in ["매수", "매도"]:
            errors.append(f"잘못된 거래 타입: {trade.trade_type}")
        
        # 매수/매도 수량 검증
        if trade.trade_type == "매수":
            if trade.buy_quantity <= 0:
                errors.append(f"매수 거래인데 매수 수량이 0입니다")
            if trade.buy_price <= 0:
                errors.append(f"매수 가격이 0 이하입니다: {trade.buy_price}")
        elif trade.trade_type == "매도":
            if trade.sell_quantity <= 0:
                errors.append(f"매도 거래인데 매도 수량이 0입니다")
            if trade.sell_price <= 0:
                errors.append(f"매도 가격이 0 이하입니다: {trade.sell_price}")
        
        return errors
    
    def validate_all(self, trades: List[BaseTrade]) -> Tuple[List[BaseTrade], List[Tuple[BaseTrade, List[str]]]]:
        """모든 거래 데이터 검증
        
        Args:
            trades: 검증할 거래 데이터 리스트
            
        Returns:
            (유효한 거래 리스트, 무효한 거래와 오류 메시지 리스트)
        """
        valid_trades = []
        invalid_trades = []
        
        for trade in trades:
            errors = self.validate_trade(trade)
            if errors:
                invalid_trades.append((trade, errors))
                logger.warning(
                    f"거래 데이터 검증 실패 - {trade.stock_name} {trade.date}: "
                    f"{', '.join(errors)}"
                )
            else:
                valid_trades.append(trade)
        
        if invalid_trades:
            logger.warning(f"총 {len(invalid_trades)}개의 거래 데이터가 검증 실패하여 제외됩니다")
        
        return valid_trades, invalid_trades 