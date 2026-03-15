"""월별 성과 추이 계산 로직 단위 테스트"""

import pytest

from modules.models import Trade
from modules.summary_generator import SummaryGenerator


def _make_sell(date: str, profit_krw: float, amount_krw: float = 700000,
               profit_rate: float = 0.0) -> Trade:
    """테스트용 매도 Trade 생성 헬퍼"""
    return Trade(
        date=date, trade_type="매도", stock_name="삼성전자", stock_code="005930",
        quantity=10, price=70000, amount=amount_krw, currency="KRW",
        exchange_rate=1.0, amount_krw=amount_krw, fee=0, tax=0,
        profit=profit_krw, profit_krw=profit_krw, profit_rate=profit_rate,
        account="테스트_국내계좌",
    )


class TestCalcMonthlyTrend:
    """_calc_monthly_trend() 단위 테스트"""

    def test_single_month(self):
        """1개월 데이터 → 기본 지표 산출, mom_change는 None"""
        trades = [
            _make_sell("2025-01-05", 10000, profit_rate=5.0),
            _make_sell("2025-01-10", -5000, profit_rate=-2.5),
            _make_sell("2025-01-15", 8000, profit_rate=4.0),
        ]
        result = SummaryGenerator._calc_monthly_trend(trades)

        assert len(result) == 1
        r = result[0]
        assert r["month"] == "2025-01"
        assert r["sell_count"] == 3
        assert r["profit_krw"] == 13000  # 10000 - 5000 + 8000
        assert r["mom_change"] is None

    def test_two_months_mom_change(self):
        """2개월 → 전월대비 변화율 정확성"""
        trades = [
            _make_sell("2025-01-05", 10000, profit_rate=5.0),
            _make_sell("2025-02-05", 15000, profit_rate=7.0),
        ]
        result = SummaryGenerator._calc_monthly_trend(trades)

        assert len(result) == 2
        assert result[0]["mom_change"] is None
        # (15000 - 10000) / |10000| = 0.5
        assert result[1]["mom_change"] == pytest.approx(0.5)

    def test_all_wins_month(self):
        """손실 없는 월 → avg_loss_rate=0, pl_ratio=0, profit_factor=0"""
        trades = [
            _make_sell("2025-01-05", 10000, profit_rate=5.0),
            _make_sell("2025-01-10", 20000, profit_rate=10.0),
        ]
        result = SummaryGenerator._calc_monthly_trend(trades)

        r = result[0]
        assert r["win_rate"] == 1.0
        assert r["avg_loss_rate"] == 0
        assert r["pl_ratio"] == 0
        assert r["profit_factor"] == 0

    def test_all_losses_month(self):
        """수익 없는 월 → avg_profit_rate=0, win_rate=0"""
        trades = [
            _make_sell("2025-01-05", -5000, profit_rate=-2.5),
            _make_sell("2025-01-10", -3000, profit_rate=-1.5),
        ]
        result = SummaryGenerator._calc_monthly_trend(trades)

        r = result[0]
        assert r["win_rate"] == 0.0
        assert r["avg_profit_rate"] == 0
        assert r["profit_factor"] == 0

    def test_mixed_months(self):
        """3개월 혼합 → 각 월별 독립 계산 확인"""
        trades = [
            _make_sell("2025-01-05", 10000, profit_rate=5.0),
            _make_sell("2025-02-05", -5000, profit_rate=-2.5),
            _make_sell("2025-03-05", 20000, profit_rate=10.0),
        ]
        result = SummaryGenerator._calc_monthly_trend(trades)

        assert len(result) == 3
        assert result[0]["month"] == "2025-01"
        assert result[1]["month"] == "2025-02"
        assert result[2]["month"] == "2025-03"

        # 1월: 수익만
        assert result[0]["win_rate"] == 1.0
        # 2월: 손실만
        assert result[1]["win_rate"] == 0.0
        # 3월: 수익만
        assert result[2]["win_rate"] == 1.0

    def test_empty_sells(self):
        """빈 리스트 → 빈 결과 반환"""
        result = SummaryGenerator._calc_monthly_trend([])
        assert result == []

    def test_win_rate_calculation(self):
        """승률 정확성 (수익3건/전체5건 = 0.6)"""
        trades = [
            _make_sell("2025-01-01", 10000, profit_rate=5.0),
            _make_sell("2025-01-02", 5000, profit_rate=2.5),
            _make_sell("2025-01-03", -3000, profit_rate=-1.5),
            _make_sell("2025-01-04", 8000, profit_rate=4.0),
            _make_sell("2025-01-05", -2000, profit_rate=-1.0),
        ]
        result = SummaryGenerator._calc_monthly_trend(trades)

        assert result[0]["win_rate"] == pytest.approx(0.6)

    def test_expectancy_calculation(self):
        """기대값 공식 검증: (avg_profit * win_rate) - (avg_loss * loss_rate)"""
        trades = [
            _make_sell("2025-01-01", 10000, profit_rate=5.0),
            _make_sell("2025-01-02", 20000, profit_rate=10.0),
            _make_sell("2025-01-03", -5000, profit_rate=-2.5),
        ]
        result = SummaryGenerator._calc_monthly_trend(trades)

        r = result[0]
        # win_rate = 2/3, loss_rate = 1/3
        # avg_profit_amount = (10000 + 20000) / 2 = 15000
        # avg_loss_amount = 5000 / 1 = 5000
        # expectancy = 15000 * (2/3) - 5000 * (1/3) = 10000 - 1666.67 = 8333.33
        expected = 15000 * (2 / 3) - 5000 * (1 / 3)
        assert r["expectancy"] == pytest.approx(expected)

    def test_profit_factor_calculation(self):
        """Profit Factor = gross_profit / |gross_loss|"""
        trades = [
            _make_sell("2025-01-01", 30000, profit_rate=10.0),
            _make_sell("2025-01-02", -10000, profit_rate=-5.0),
        ]
        result = SummaryGenerator._calc_monthly_trend(trades)

        # 30000 / 10000 = 3.0
        assert result[0]["profit_factor"] == 3.0

    def test_pl_ratio_calculation(self):
        """손익비 = |avg_profit_rate / avg_loss_rate|"""
        trades = [
            _make_sell("2025-01-01", 10000, profit_rate=10.0),  # 10% → 0.10
            _make_sell("2025-01-02", -5000, profit_rate=-5.0),  # -5% → -0.05
        ]
        result = SummaryGenerator._calc_monthly_trend(trades)

        # |0.10 / -0.05| = 2.0
        assert result[0]["pl_ratio"] == 2.0

    def test_return_rate_calculation(self):
        """수익률 = profit_krw / total_sell_amount"""
        trades = [
            _make_sell("2025-01-01", 10000, amount_krw=100000, profit_rate=10.0),
            _make_sell("2025-01-02", -5000, amount_krw=200000, profit_rate=-2.5),
        ]
        result = SummaryGenerator._calc_monthly_trend(trades)

        # profit = 10000 - 5000 = 5000
        # total_sell_amount = 100000 + 200000 = 300000
        # return_rate = 5000 / 300000 = 0.01667
        expected = 5000 / 300000
        assert result[0]["return_rate"] == pytest.approx(expected)

    def test_mom_change_with_zero_prev_profit(self):
        """전월 손익이 0이면 mom_change는 None"""
        trades = [
            _make_sell("2025-01-01", 5000, profit_rate=5.0),
            _make_sell("2025-01-02", -5000, profit_rate=-5.0),  # 1월 합계 = 0
            _make_sell("2025-02-01", 10000, profit_rate=10.0),
        ]
        result = SummaryGenerator._calc_monthly_trend(trades)

        assert result[0]["profit_krw"] == 0
        assert result[1]["mom_change"] is None  # prev=0이므로 None
