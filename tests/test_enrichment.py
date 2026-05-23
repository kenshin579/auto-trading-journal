"""국내 종목코드 enrichment 단위 테스트"""

from modules.models import Trade
from modules.symbol_master import SymbolResolver
from main import enrich_domestic_codes


def _trade(stock_name, stock_code, account="미래에셋증권_국내계좌"):
    return Trade(
        date="2026-02-13", trade_type="매수", stock_name=stock_name,
        stock_code=stock_code, quantity=1, price=100, amount=100,
        currency="KRW", exchange_rate=1.0, amount_krw=100, fee=0.0, tax=0.0,
        profit=0.0, profit_krw=0.0, profit_rate=0.0, account=account,
    )


def test_fills_empty_domestic_code():
    resolver = SymbolResolver(name_to_code={"TIGER 조선TOP10": "494670"})
    trades = [_trade("TIGER 조선TOP10", "")]
    enrich_domestic_codes(trades, resolver)
    assert trades[0].stock_code == "494670"


def test_keeps_existing_code():
    resolver = SymbolResolver(name_to_code={"삼성전자": "999999"})
    trades = [_trade("삼성전자", "005930", account="한국투자증권_국내계좌")]
    enrich_domestic_codes(trades, resolver)
    assert trades[0].stock_code == "005930"  # CSV 코드 유지, 덮어쓰지 않음


def test_skips_foreign_trades():
    resolver = SymbolResolver(name_to_code={"애플": "AAPL-WRONG"})
    trades = [_trade("애플", "", account="미래에셋증권_해외계좌")]
    enrich_domestic_codes(trades, resolver)
    assert trades[0].stock_code == ""  # 해외는 건드리지 않음
