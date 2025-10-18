#!/usr/bin/env python3
"""
거래 데이터 모델
국내/해외 주식 거래를 위한 데이터 클래스들
"""

from abc import ABC, abstractmethod
from dataclasses import dataclass
from typing import List, Optional
import logging

logger = logging.getLogger(__name__)


@dataclass
class BaseTrade(ABC):
    """거래 정보 기본 클래스"""
    stock_name: str  # 종목명
    date: str  # 거래 날짜 (YYYY-MM-DD)
    
    @abstractmethod
    def to_sheet_row(self) -> List[str]:
        """구글 시트 포맷으로 변환"""
        pass
    
    @abstractmethod
    def validate(self) -> bool:
        """데이터 유효성 검증"""
        pass


@dataclass
class DomesticTrade(BaseTrade):
    """국내 주식 거래 정보"""
    trade_type: str  # 매수/매도
    quantity: int  # 수량
    price: float  # 평균단가
    total_amount: float  # 총액
    
    def to_sheet_row(self) -> List[str]:
        """구글 시트 포맷으로 변환 (8개 컬럼)"""
        return [
            self.stock_name,
            "미래에셋증권",
            self.date,
            self.trade_type,
            f"₩{self.price:,.0f}",
            str(self.quantity),
            "",  # 수수료 (빈 값)
            f"₩{self.total_amount:,.0f}"
        ]
    
    def validate(self) -> bool:
        """데이터 유효성 검증"""
        if self.quantity <= 0:
            return False
        if self.price <= 0:
            return False
        if self.total_amount <= 0:
            return False
        if self.trade_type not in ["매수", "매도"]:
            return False
        return True


@dataclass
class ForeignTrade(BaseTrade):
    """해외 주식 거래 정보"""
    # 기본 정보
    currency: str  # 통화 (USD, EUR 등)
    ticker: str  # 종목 코드
    trade_type: str  # 매수/매도
    
    # 잔고 및 환율 정보
    balance_quantity: int  # 잔고 수량
    avg_buy_exchange_rate: float  # 매입평균환율
    trade_exchange_rate: float  # 매매일환율
    
    # 매수 정보
    buy_quantity: int  # 매수 수량
    buy_price: float  # 매수 단가
    buy_amount: float  # 매수 금액 (외화)
    buy_amount_krw: float  # 원화 매수 금액
    
    # 매도 정보
    sell_quantity: int  # 매도 수량
    sell_price: float  # 매도 단가
    sell_amount: float  # 매도 금액 (외화)
    sell_amount_krw: float  # 원화 매도 금액
    
    # 비용 정보
    commission: float  # 수수료
    tax: float  # 세금
    total_cost_krw: float  # 원화 총비용
    
    # 손익 정보
    avg_buy_price_krw: float  # 원매수평균가
    trading_profit: float  # 매매손익 (외화)
    trading_profit_krw: float  # 원화매매손익
    fx_gain_loss: float  # 환차손익
    total_profit: float  # 총평가손익
    profit_rate: float  # 손익률 (%)
    fx_profit_rate: float  # 환산손익률 (%)
    
    def to_sheet_row(self) -> List[str]:
        """구글 시트 포맷으로 변환 (8개 컬럼 - 국내 주식과 동일한 형식)"""
        # 매수/매도에 따라 다른 값 설정
        if self.trade_type == "매수":
            quantity = self.buy_quantity
            price = self.buy_price
            total_amount = self.buy_amount
        else:  # 매도
            quantity = self.sell_quantity
            price = self.sell_price
            total_amount = self.sell_amount
        
        return [
            self.stock_name,                       # 종목
            "미래에셋증권",                        # 증권사
            self.date,                             # 일자
            self.trade_type,                       # 종류 (매수/매도)
            f"${price:.2f}",                       # 주문가격 (달러 표시)
            str(quantity),                         # 수량
            "",                                    # 수수료 (빈 값)
            f"${total_amount:.2f}"                 # 총액 (달러 표시)
        ]
    
    def validate(self) -> bool:
        """데이터 유효성 검증"""
        # 통화 검증
        if self.currency not in ['USD', 'EUR', 'JPY', 'CNY', 'HKD', 'GBP']:
            logger.warning(f"지원하지 않는 통화: {self.currency}")
            return False
        
        # 티커 검증 (기본적인 형식만)
        if not self.ticker or len(self.ticker) > 10:
            logger.warning(f"잘못된 티커 형식: {self.ticker}")
            return False
        
        # 환율 검증
        if self.trade_exchange_rate <= 0:
            logger.warning(f"잘못된 환율: {self.trade_exchange_rate}")
            return False
        
        # 거래 타입 검증
        if self.trade_type not in ["매수", "매도"]:
            logger.warning(f"잘못된 거래 타입: {self.trade_type}")
            return False
        
        # 매수/매도 수량 검증
        if self.trade_type == "매수" and self.buy_quantity <= 0:
            logger.warning("매수 거래인데 매수 수량이 0입니다")
            return False
        if self.trade_type == "매도" and self.sell_quantity <= 0:
            logger.warning("매도 거래인데 매도 수량이 0입니다")
            return False
        
        return True


# 하위 호환성을 위한 별칭
Trade = DomesticTrade 