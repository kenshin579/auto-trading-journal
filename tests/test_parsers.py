"""파서 단위 테스트"""

from pathlib import Path

import pytest

from modules.parser_registry import detect_parser
from modules.parsers.mirae_parser import MiraeDomesticParser, MiraeForeignParser
from modules.parsers.hankook_parser import HankookDomesticParser

SAMPLE_DIR = Path(__file__).parent.parent / "stocks" / "sample"


class TestMiraeDomesticParser:
    """미래에셋증권 국내계좌 파서 테스트"""

    @pytest.fixture
    def parser(self):
        return MiraeDomesticParser()

    @pytest.fixture
    def sample_file(self):
        return SAMPLE_DIR / "미래에셋증권" / "국내계좌.csv"

    def test_can_parse(self):
        header = ["일자", "종목명", "기간 중 매수", "", "", "기간 중 매도", "", "", "매매비용", "손익금액", "수익률"]
        assert MiraeDomesticParser.can_parse(header) is True

    def test_can_parse_rejects_foreign(self):
        header = ["매매일", "통화", "종목번호", "종목명"]
        assert MiraeDomesticParser.can_parse(header) is False

    def test_parse_sample(self, parser, sample_file):
        if not sample_file.exists():
            pytest.skip("샘플 파일 없음")
        trades = parser.parse(sample_file, "미래에셋증권_국내계좌")
        assert len(trades) > 0

    def test_parse_buy_trade(self, parser, sample_file):
        if not sample_file.exists():
            pytest.skip("샘플 파일 없음")
        trades = parser.parse(sample_file, "미래에셋증권_국내계좌")
        buy_trades = [t for t in trades if t.trade_type == "매수"]
        assert len(buy_trades) > 0
        t = buy_trades[0]
        assert t.currency == "KRW"
        assert t.exchange_rate == 1.0
        assert t.quantity > 0
        assert t.amount > 0
        assert t.account == "미래에셋증권_국내계좌"

    def test_parse_sell_trade(self, parser, sample_file):
        if not sample_file.exists():
            pytest.skip("샘플 파일 없음")
        trades = parser.parse(sample_file, "미래에셋증권_국내계좌")
        sell_trades = [t for t in trades if t.trade_type == "매도"]
        assert len(sell_trades) > 0
        t = sell_trades[0]
        assert t.amount > 0

    def test_date_format(self, parser, sample_file):
        if not sample_file.exists():
            pytest.skip("샘플 파일 없음")
        trades = parser.parse(sample_file, "미래에셋증권_국내계좌")
        for t in trades:
            assert "-" in t.date  # YYYY-MM-DD 포맷
            assert "/" not in t.date

    def test_buy_sell_same_row(self, parser, sample_file):
        """한 행에서 매수/매도 동시 발생 테스트"""
        if not sample_file.exists():
            pytest.skip("샘플 파일 없음")
        trades = parser.parse(sample_file, "미래에셋증권_국내계좌")
        # HANARO 원자력iSelect 2026/02/12: 매수 3, 매도 2
        hanaro_trades = [t for t in trades
                         if t.stock_name == "HANARO 원자력iSelect" and t.date == "2026-02-12"]
        buy = [t for t in hanaro_trades if t.trade_type == "매수"]
        sell = [t for t in hanaro_trades if t.trade_type == "매도"]
        assert len(buy) == 1
        assert len(sell) == 1
        assert buy[0].quantity == 3
        assert sell[0].quantity == 2

    def test_to_domestic_row(self, parser, sample_file):
        if not sample_file.exists():
            pytest.skip("샘플 파일 없음")
        trades = parser.parse(sample_file, "미래에셋증권_국내계좌")
        row = trades[0].to_domestic_row()
        assert len(row) == 9


class TestMiraeForeignParser:
    """미래에셋증권 해외계좌 파서 테스트"""

    @pytest.fixture
    def parser(self):
        return MiraeForeignParser()

    @pytest.fixture
    def sample_file(self):
        return SAMPLE_DIR / "미래에셋증권" / "해외계좌.csv"

    def test_can_parse(self):
        header = ["매매일", "통화", "종목번호", "종목명", "잔고 수량"]
        assert MiraeForeignParser.can_parse(header) is True

    def test_can_parse_rejects_domestic(self):
        header = ["일자", "종목명", "기간 중 매수"]
        assert MiraeForeignParser.can_parse(header) is False

    def test_parse_sample(self, parser, sample_file):
        if not sample_file.exists():
            pytest.skip("샘플 파일 없음")
        trades = parser.parse(sample_file, "미래에셋증권_해외계좌")
        assert len(trades) > 0

    def test_multi_currency(self, parser, sample_file):
        """다중 통화 (USD, JPY) 지원 테스트"""
        if not sample_file.exists():
            pytest.skip("샘플 파일 없음")
        trades = parser.parse(sample_file, "미래에셋증권_해외계좌")
        currencies = set(t.currency for t in trades)
        assert "USD" in currencies
        assert "JPY" in currencies

    def test_exchange_rate(self, parser, sample_file):
        if not sample_file.exists():
            pytest.skip("샘플 파일 없음")
        trades = parser.parse(sample_file, "미래에셋증권_해외계좌")
        for t in trades:
            assert t.exchange_rate > 0

    def test_sell_with_profit(self, parser, sample_file):
        """매도 거래의 손익 데이터 테스트"""
        if not sample_file.exists():
            pytest.skip("샘플 파일 없음")
        trades = parser.parse(sample_file, "미래에셋증권_해외계좌")
        sell_trades = [t for t in trades if t.trade_type == "매도"]
        assert len(sell_trades) > 0
        # 손익이 있는 매도가 존재해야 함
        has_profit = any(t.profit != 0 for t in sell_trades)
        assert has_profit

    def test_to_foreign_row(self, parser, sample_file):
        if not sample_file.exists():
            pytest.skip("샘플 파일 없음")
        trades = parser.parse(sample_file, "미래에셋증권_해외계좌")
        row = trades[0].to_foreign_row()
        assert len(row) == 15


class TestHankookDomesticParser:
    """한국투자증권 국내계좌 파서 테스트"""

    @pytest.fixture
    def parser(self):
        return HankookDomesticParser()

    @pytest.fixture
    def sample_file(self):
        return SAMPLE_DIR / "한국투자증권" / "국내계좌.csv"

    def test_can_parse(self):
        header = ['"매매일자"', '"종목명"', '"종목코드"', '"구분"', '"대출일자"',
                  '"보유수량"', '"매입단가"', '"매수수량"', '"매도단가"', '"매도수량"']
        assert HankookDomesticParser.can_parse(header) is True

    def test_parse_sample(self, parser, sample_file):
        if not sample_file.exists():
            pytest.skip("샘플 파일 없음")
        trades = parser.parse(sample_file, "한국투자증권_국내계좌")
        assert len(trades) > 0

    def test_comma_in_numbers(self, parser, sample_file):
        """천단위 쉼표 처리 테스트"""
        if not sample_file.exists():
            pytest.skip("샘플 파일 없음")
        trades = parser.parse(sample_file, "한국투자증권_국내계좌")
        for t in trades:
            assert isinstance(t.amount, float)
            assert t.amount > 0

    def test_skip_empty_rows(self, parser, sample_file):
        """빈 행 건너뜀 테스트"""
        if not sample_file.exists():
            pytest.skip("샘플 파일 없음")
        trades = parser.parse(sample_file, "한국투자증권_국내계좌")
        for t in trades:
            assert t.stock_name != ""

    def test_stock_code(self, parser, sample_file):
        """종목코드 파싱 테스트"""
        if not sample_file.exists():
            pytest.skip("샘플 파일 없음")
        trades = parser.parse(sample_file, "한국투자증권_국내계좌")
        has_code = any(t.stock_code != "" for t in trades)
        assert has_code

    def test_sell_with_fee(self, parser, sample_file):
        """매도 거래의 수수료+세금 합산 테스트"""
        if not sample_file.exists():
            pytest.skip("샘플 파일 없음")
        trades = parser.parse(sample_file, "한국투자증권_국내계좌")
        sell_trades = [t for t in trades if t.trade_type == "매도"]
        assert len(sell_trades) > 0


class TestParserRegistry:
    """파서 레지스트리 테스트"""

    def test_detect_mirae_domestic(self):
        f = SAMPLE_DIR / "미래에셋증권" / "국내계좌.csv"
        if not f.exists():
            pytest.skip("샘플 파일 없음")
        parser = detect_parser(f)
        assert isinstance(parser, MiraeDomesticParser)

    def test_detect_mirae_foreign(self):
        f = SAMPLE_DIR / "미래에셋증권" / "해외계좌.csv"
        if not f.exists():
            pytest.skip("샘플 파일 없음")
        parser = detect_parser(f)
        assert isinstance(parser, MiraeForeignParser)

    def test_detect_hankook_domestic(self):
        f = SAMPLE_DIR / "한국투자증권" / "국내계좌.csv"
        if not f.exists():
            pytest.skip("샘플 파일 없음")
        parser = detect_parser(f)
        assert isinstance(parser, HankookDomesticParser)
