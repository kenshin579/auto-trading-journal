"""Trade 데이터 모델"""

from dataclasses import dataclass


@dataclass
class Trade:
    """매매 거래 통합 모델 (국내/해외 공용)"""

    date: str  # YYYY-MM-DD
    trade_type: str  # 매수 / 매도
    stock_name: str  # 종목명
    stock_code: str  # 종목코드/티커 (없으면 "")
    quantity: float  # 수량
    price: float  # 단가 (외화 기준)
    amount: float  # 금액 (외화 기준)
    currency: str  # KRW / USD / JPY
    exchange_rate: float  # 환율 (국내는 1.0)
    amount_krw: float  # 원화 환산 금액
    fee: float  # 수수료
    tax: float  # 세금
    profit: float  # 실현손익 (외화)
    profit_krw: float  # 원화 실현손익
    profit_rate: float  # 수익률(%)
    account: str  # 증권사_계좌유형 (예: "미래에셋증권_국내계좌")

    def is_domestic(self) -> bool:
        return "국내" in self.account

    def is_foreign(self) -> bool:
        return "해외" in self.account

    def to_domestic_row(self) -> list:
        """국내계좌 시트 행 변환 (9컬럼)
        수익률은 퍼센트 소수로 변환 (14.68 → 0.1468)
        """
        return [
            self.date, self.trade_type, self.stock_name,
            self.quantity, self.price, self.amount,
            self.fee, self.profit,
            self.profit_rate / 100 if self.profit_rate else 0,
        ]

    def to_foreign_row(self) -> list:
        """해외계좌 시트 행 변환 (15컬럼)
        수익률은 퍼센트 소수로 변환 (14.68 → 0.1468)
        """
        return [
            self.date, self.trade_type, self.currency, self.stock_code,
            self.stock_name, self.quantity, self.price, self.amount,
            self.exchange_rate, self.amount_krw,
            self.fee, self.tax, self.profit, self.profit_krw,
            self.profit_rate / 100 if self.profit_rate else 0,
        ]

    def to_sheet_row(self) -> list:
        """계좌 유형에 따라 적절한 행 반환"""
        if self.is_foreign():
            return self.to_foreign_row()
        return self.to_domestic_row()

    @staticmethod
    def _num_str(v: float) -> str:
        """숫자를 시트 표현과 일치하는 문자열로 변환 (정수면 소수점 제거)"""
        return str(int(v)) if v == int(v) else str(v)

    def duplicate_key(self) -> tuple:
        """중복 체크 키 (시트에서 읽은 값과 비교 가능하도록 문자열 통일)"""
        return (self.date, self.trade_type, self.stock_name,
                self._num_str(self.quantity), self._num_str(self.price))
