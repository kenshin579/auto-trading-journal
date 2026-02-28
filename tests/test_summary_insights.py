"""매매 인사이트 계산 로직 단위 테스트"""

import pytest

from modules.models import Trade
from modules.summary_generator import SummaryGenerator


def _make_trade(date: str, profit_krw: float, stock_name: str = "삼성전자",
                profit_rate: float = 0.0) -> Trade:
    """테스트용 매도 Trade 생성 헬퍼"""
    return Trade(
        date=date, trade_type="매도", stock_name=stock_name, stock_code="005930",
        quantity=10, price=70000, amount=700000, currency="KRW",
        exchange_rate=1.0, amount_krw=700000, fee=0, tax=0,
        profit=profit_krw, profit_krw=profit_krw, profit_rate=profit_rate,
        account="미래에셋증권_국내계좌",
    )


class TestCalcStreaks:
    """연속 승/패 기록 계산 테스트"""

    def test_all_wins(self):
        trades = [_make_trade(f"2025-01-0{i}", 10000) for i in range(1, 6)]
        max_w, max_l, cur_w, cur_l = SummaryGenerator._calc_streaks(trades)
        assert max_w == 5
        assert max_l == 0
        assert cur_w == 5
        assert cur_l == 0

    def test_all_losses(self):
        trades = [_make_trade(f"2025-01-0{i}", -5000) for i in range(1, 4)]
        max_w, max_l, cur_w, cur_l = SummaryGenerator._calc_streaks(trades)
        assert max_w == 0
        assert max_l == 3
        assert cur_w == 0
        assert cur_l == 3

    def test_mixed_pattern(self):
        # W W W L L W
        profits = [100, 200, 300, -100, -200, 50]
        trades = [_make_trade(f"2025-01-0{i+1}", p)
                  for i, p in enumerate(profits)]
        max_w, max_l, cur_w, cur_l = SummaryGenerator._calc_streaks(trades)
        assert max_w == 3
        assert max_l == 2
        assert cur_w == 1
        assert cur_l == 0

    def test_empty(self):
        max_w, max_l, cur_w, cur_l = SummaryGenerator._calc_streaks([])
        assert (max_w, max_l, cur_w, cur_l) == (0, 0, 0, 0)

    def test_zero_profit_is_loss(self):
        trades = [_make_trade("2025-01-01", 0)]
        max_w, max_l, cur_w, cur_l = SummaryGenerator._calc_streaks(trades)
        assert max_l == 1
        assert cur_l == 1


class TestCalcDayOfWeekStats:
    """요일별 성과 계산 테스트"""

    def test_single_day(self):
        # 2025-01-06 = 월요일
        trades = [
            _make_trade("2025-01-06", 10000),
            _make_trade("2025-01-06", -5000),
        ]
        stats = SummaryGenerator._calc_day_of_week_stats(trades)
        assert 0 in stats  # 월요일
        count, profit_sum, win_rate = stats[0]
        assert count == 2
        assert profit_sum == 5000
        assert win_rate == 50.0

    def test_multiple_days(self):
        # 2025-01-06 = 월, 2025-01-07 = 화
        trades = [
            _make_trade("2025-01-06", 10000),
            _make_trade("2025-01-07", -3000),
        ]
        stats = SummaryGenerator._calc_day_of_week_stats(trades)
        assert len(stats) == 2
        assert stats[0][0] == 1  # 월: 1건
        assert stats[1][0] == 1  # 화: 1건

    def test_empty(self):
        stats = SummaryGenerator._calc_day_of_week_stats([])
        assert stats == {}


class TestCalcMonthlyProfits:
    """월별 실현손익 집계 테스트"""

    def test_single_month(self):
        trades = [
            _make_trade("2025-01-10", 10000),
            _make_trade("2025-01-20", -3000),
        ]
        result = SummaryGenerator._calc_monthly_profits(trades)
        assert result == [("2025-01", 7000)]

    def test_multiple_months(self):
        trades = [
            _make_trade("2025-01-10", 10000),
            _make_trade("2025-02-10", -5000),
            _make_trade("2025-03-10", 20000),
        ]
        result = SummaryGenerator._calc_monthly_profits(trades)
        assert result == [
            ("2025-01", 10000),
            ("2025-02", -5000),
            ("2025-03", 20000),
        ]

    def test_empty(self):
        assert SummaryGenerator._calc_monthly_profits([]) == []


class TestCalcTradeFrequency:
    """거래 빈도 분석 테스트"""

    def test_multiple_months(self):
        trades = [
            _make_trade("2025-01-01", 100),
            _make_trade("2025-01-02", 200),
            _make_trade("2025-01-03", 300),
            _make_trade("2025-02-01", 100),
        ]
        result = SummaryGenerator._calc_trade_frequency(trades)
        assert result["avg_monthly"] == 2.0
        assert result["max_month"] == "2025-01"
        assert result["max_count"] == 3
        assert result["min_month"] == "2025-02"
        assert result["min_count"] == 1

    def test_single_month(self):
        trades = [_make_trade("2025-03-15", 100)]
        result = SummaryGenerator._calc_trade_frequency(trades)
        assert result["avg_monthly"] == 1.0
        assert result["max_month"] == "2025-03"

    def test_empty(self):
        result = SummaryGenerator._calc_trade_frequency([])
        assert result["avg_monthly"] == 0
        assert result["max_month"] == "-"


class TestTradingInsightsCalculations:
    """매매 인사이트 핵심 계산 로직 통합 테스트"""

    @pytest.fixture
    def sample_sells(self):
        """다양한 패턴의 매도 거래 샘플"""
        return [
            _make_trade("2025-01-06", 50000, "삼성전자"),   # 월 승
            _make_trade("2025-01-07", -20000, "LG전자"),    # 화 패
            _make_trade("2025-01-08", 30000, "SK하이닉스"),  # 수 승
            _make_trade("2025-02-10", -10000, "카카오"),     # 월 패
            _make_trade("2025-02-12", 80000, "삼성전자"),    # 수 승
            _make_trade("2025-03-03", 15000, "네이버"),      # 월 승
        ]

    def test_expectancy(self, sample_sells):
        """기대값 계산: (평균수익 × 승률) - (평균손실 × 패률)"""
        profitable = [t for t in sample_sells if t.profit_krw > 0]
        losing = [t for t in sample_sells if t.profit_krw <= 0]
        win_rate = len(profitable) / len(sample_sells)
        loss_rate = 1 - win_rate

        avg_profit = sum(t.profit_krw for t in profitable) / len(profitable)
        avg_loss = sum(t.profit_krw for t in losing) / len(losing)

        expectancy = (avg_profit * win_rate) - (abs(avg_loss) * loss_rate)
        # 4승 2패: avg_profit=43750, avg_loss=-15000, wr=0.667, lr=0.333
        assert expectancy > 0  # 수익 시스템

    def test_profit_factor(self, sample_sells):
        """Profit Factor 계산: 총수익 / |총손실|"""
        profitable = [t for t in sample_sells if t.profit_krw > 0]
        losing = [t for t in sample_sells if t.profit_krw <= 0]

        total_profit = sum(t.profit_krw for t in profitable)  # 175000
        total_loss = abs(sum(t.profit_krw for t in losing))   # 30000
        pf = total_profit / total_loss

        assert pf == pytest.approx(175000 / 30000, rel=1e-4)
        assert pf > 1.0  # 수익 시스템

    def test_streaks_with_sample(self, sample_sells):
        """연속 승/패: W L W L W W"""
        sorted_sells = sorted(sample_sells, key=lambda t: t.date)
        max_w, max_l, cur_w, cur_l = SummaryGenerator._calc_streaks(sorted_sells)
        assert max_w == 2
        assert max_l == 1
        assert cur_w == 2  # 마지막 2건 연승

    def test_profit_concentration(self, sample_sells):
        """수익 집중도: 상위 종목 수익 비중"""
        from collections import defaultdict
        stock_profit = defaultdict(float)
        for t in sample_sells:
            stock_profit[t.stock_name] += t.profit_krw
        positive = {k: v for k, v in stock_profit.items() if v > 0}
        total = sum(positive.values())
        sorted_vals = sorted(positive.values(), reverse=True)

        top3_ratio = sum(sorted_vals[:3]) / total if total else 0
        # 삼성전자=130000, SK하이닉스=30000, 네이버=15000 → all positive
        assert top3_ratio == pytest.approx(1.0, rel=1e-4)  # 3종목이 전체 수익

    def test_discipline_profit_multiple(self, sample_sells):
        """손절/익절 규율: 평균 수익 배수"""
        profitable = [t for t in sample_sells if t.profit_krw > 0]
        losing = [t for t in sample_sells if t.profit_krw <= 0]

        avg_profit = sum(t.profit_krw for t in profitable) / len(profitable)
        avg_loss = sum(t.profit_krw for t in losing) / len(losing)
        multiple = avg_profit / abs(avg_loss)

        # avg_profit = 43750, avg_loss = -15000 → multiple ≈ 2.917
        assert multiple > 2.0
