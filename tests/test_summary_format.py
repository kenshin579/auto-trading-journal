"""대시보드 숫자 포맷 관련 단위 테스트"""

from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from modules.models import Trade
from modules.summary_generator import SummaryGenerator


def _make_buy_trade(date: str, stock_name: str, amount_krw: float,
                    currency: str = "KRW", account: str = "미래에셋증권_주식1") -> Trade:
    """테스트용 매수 Trade 생성 헬퍼"""
    return Trade(
        date=date, trade_type="매수", stock_name=stock_name, stock_code="005930",
        quantity=10, price=amount_krw / 10, amount=amount_krw, currency=currency,
        exchange_rate=1.0, amount_krw=amount_krw, fee=0, tax=0,
        profit=0, profit_krw=0, profit_rate=0.0,
        account=account,
    )


def _make_sell_trade(date: str, stock_name: str, amount_krw: float,
                     profit_krw: float, profit_rate: float = 5.0,
                     currency: str = "KRW",
                     account: str = "미래에셋증권_주식1") -> Trade:
    """테스트용 매도 Trade 생성 헬퍼"""
    return Trade(
        date=date, trade_type="매도", stock_name=stock_name, stock_code="005930",
        quantity=10, price=amount_krw / 10, amount=amount_krw, currency=currency,
        exchange_rate=1.0, amount_krw=amount_krw, fee=0, tax=0,
        profit=profit_krw, profit_krw=profit_krw, profit_rate=profit_rate,
        account=account,
    )


class TestGroupConsecutiveRows:
    """연속 행 그룹핑 테스트"""

    def test_single_row(self):
        assert SummaryGenerator._group_consecutive_rows([5]) == [(5, 5)]

    def test_consecutive(self):
        assert SummaryGenerator._group_consecutive_rows([3, 4, 5]) == [(3, 5)]

    def test_gaps(self):
        result = SummaryGenerator._group_consecutive_rows([3, 4, 7, 8, 9, 12])
        assert result == [(3, 4), (7, 9), (12, 12)]

    def test_empty(self):
        assert SummaryGenerator._group_consecutive_rows([]) == []

    def test_two_separate(self):
        assert SummaryGenerator._group_consecutive_rows([1, 5]) == [(1, 1), (5, 5)]


class TestMetricsOffsetMapping:
    """투자 지표 섹션의 pct_rows/krw_rows offset이 정확한지 검증"""

    @pytest.fixture
    def sample_trades(self):
        """매수 + 매도 거래 샘플"""
        return [
            _make_buy_trade("2025-01-01", "삼성전자", 1000000),
            _make_buy_trade("2025-01-02", "LG전자", 500000, account="미래에셋증권_ISA"),
            _make_sell_trade("2025-01-10", "삼성전자", 1100000, 100000, 10.0),
            _make_sell_trade("2025-01-15", "LG전자", 450000, -50000, -10.0,
                             account="미래에셋증권_ISA"),
        ]

    @pytest.mark.asyncio
    async def test_investment_metrics_offset_consistency(self, sample_trades):
        """pct_rows offset이 실제 비율 값 행을, krw_rows offset이 금액 행을 가리키는지 확인"""
        mock_client = AsyncMock()
        mock_client.batch_update_cells = AsyncMock(return_value=True)
        mock_client.apply_number_format_to_columns = AsyncMock(return_value=True)

        mock_writer = MagicMock()
        gen = SummaryGenerator(mock_client, mock_writer, sector_classifier=None)

        # rows, pct_rows, krw_rows를 캡처하기 위해 메서드 내부 로직을 재현
        # _write_investment_metrics 호출
        start_row = 100
        await gen._write_investment_metrics(sample_trades, start_row)

        # _apply_metrics_formats 호출 시 전달된 인자 검증
        format_call = mock_client.apply_number_format_to_columns
        assert format_call.call_count >= 1

        # batch_update_cells로 전달된 rows 데이터 확인
        batch_call = mock_client.batch_update_cells
        assert batch_call.called
        call_args = batch_call.call_args
        ranges_dict = call_args[0][1]  # 두 번째 positional arg
        rows = list(ranges_dict.values())[0]

        # pct_rows의 각 offset이 실제로 0~1 범위 비율 값을 가리키는지 검증
        # apply_number_format_to_columns 호출에서 PERCENT 타입 호출 추출
        pct_calls = [
            c for c in format_call.call_args_list
            if any(f.get('type') == 'PERCENT' for f in c[0][1])
        ]
        krw_calls = [
            c for c in format_call.call_args_list
            if any('₩' in f.get('pattern', '') for f in c[0][1])
        ]

        # PERCENT 포맷이 적용된 행들의 값이 0~1 범위인지 확인
        for call in pct_calls:
            fmt_start = call[0][2]  # start_row arg
            fmt_end = call[0][3]    # end_row arg
            for abs_row in range(fmt_start, fmt_end + 1):
                row_idx = abs_row - start_row
                if 0 <= row_idx < len(rows):
                    val = rows[row_idx][1]
                    if isinstance(val, (int, float)):
                        assert -1.0 <= val <= 1.0, \
                            f"Row {row_idx} ({rows[row_idx][0]}): PERCENT 포맷인데 값이 {val}"

        # KRW 포맷이 적용된 행들의 값이 금액인지 확인
        for call in krw_calls:
            fmt_start = call[0][2]
            fmt_end = call[0][3]
            for abs_row in range(fmt_start, fmt_end + 1):
                row_idx = abs_row - start_row
                if 0 <= row_idx < len(rows):
                    val = rows[row_idx][1]
                    if isinstance(val, (int, float)) and val != 0:
                        assert abs(val) > 1.0, \
                            f"Row {row_idx} ({rows[row_idx][0]}): KRW 포맷인데 값이 {val}"

    @pytest.mark.asyncio
    async def test_trading_insights_offset_consistency(self, sample_trades):
        """매매 인사이트 섹션의 pct/krw offset 일관성 검증"""
        mock_client = AsyncMock()
        mock_client.batch_update_cells = AsyncMock(return_value=True)
        mock_client.apply_number_format_to_columns = AsyncMock(return_value=True)

        mock_writer = MagicMock()
        gen = SummaryGenerator(mock_client, mock_writer, sector_classifier=None)

        start_row = 200
        await gen._write_trading_insights(sample_trades, start_row)

        batch_call = mock_client.batch_update_cells
        assert batch_call.called
        call_args = batch_call.call_args
        ranges_dict = call_args[0][1]
        rows = list(ranges_dict.values())[0]

        format_call = mock_client.apply_number_format_to_columns

        # KRW 포맷 적용된 행의 값이 금액인지 확인
        krw_calls = [
            c for c in format_call.call_args_list
            if any('₩' in f.get('pattern', '') for f in c[0][1])
        ]
        for call in krw_calls:
            fmt_start = call[0][2]
            fmt_end = call[0][3]
            for abs_row in range(fmt_start, fmt_end + 1):
                row_idx = abs_row - start_row
                if 0 <= row_idx < len(rows):
                    val = rows[row_idx][1]
                    if isinstance(val, (int, float)) and val != 0:
                        assert abs(val) > 1.0, \
                            f"Row {row_idx} ({rows[row_idx][0]}): KRW 포맷인데 값이 {val}"
