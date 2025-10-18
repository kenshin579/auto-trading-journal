#!/usr/bin/env python3
"""
해외 주식 파일 파싱 테스트
"""

import asyncio
from pathlib import Path
from modules.file_parser import FileParser
from modules.trade_models import ForeignTrade


async def test_foreign_parsing():
    """해외 주식 파일 파싱 테스트"""
    parser = FileParser()
    
    # 해외 주식 파일 찾기
    foreign_file = Path("stocks/계좌1 해외.md")
    if not foreign_file.exists():
        print(f"파일을 찾을 수 없습니다: {foreign_file}")
        return
    
    print(f"파일 파싱 중: {foreign_file}")
    
    # 파일 파싱
    trading_log = parser.parse_trading_log(foreign_file)
    
    print(f"\n=== 파싱 결과 ===")
    print(f"파일 타입: {trading_log.file_type}")
    print(f"거래 수: {len(trading_log.trades)}")
    
    # 처음 5개 거래 출력
    print(f"\n=== 처음 5개 거래 ===")
    for i, trade in enumerate(trading_log.trades[:5]):
        if isinstance(trade, ForeignTrade):
            print(f"\n거래 {i+1}:")
            print(f"  날짜: {trade.date}")
            print(f"  종목: {trade.stock_name}")
            print(f"  티커: {trade.ticker}")
            print(f"  통화: {trade.currency}")
            print(f"  타입: {trade.trade_type}")
            print(f"  환율: {trade.trade_exchange_rate}")
            
            if trade.trade_type == "매수":
                print(f"  수량: {trade.buy_quantity}")
                print(f"  단가: ${trade.buy_price}")
                print(f"  금액: ${trade.buy_amount}")
                print(f"  원화: ₩{trade.buy_amount_krw:,.0f}")
            else:
                print(f"  수량: {trade.sell_quantity}")
                print(f"  단가: ${trade.sell_price}")
                print(f"  금액: ${trade.sell_amount}")
                print(f"  원화: ₩{trade.sell_amount_krw:,.0f}")
                if trade.profit_rate != 0:
                    print(f"  손익률: {trade.profit_rate:.2f}%")
    
    # 통계
    print(f"\n=== 통계 ===")
    buy_trades = [t for t in trading_log.trades if isinstance(t, ForeignTrade) and t.trade_type == "매수"]
    sell_trades = [t for t in trading_log.trades if isinstance(t, ForeignTrade) and t.trade_type == "매도"]
    
    print(f"매수 거래: {len(buy_trades)}건")
    print(f"매도 거래: {len(sell_trades)}건")
    
    # 통화별 통계
    currencies = set(t.currency for t in trading_log.trades if isinstance(t, ForeignTrade))
    print(f"\n통화: {', '.join(currencies)}")
    
    # 종목별 통계
    stocks = set(t.stock_name for t in trading_log.trades if isinstance(t, ForeignTrade))
    print(f"\n종목 수: {len(stocks)}개")
    print("주요 종목:")
    for stock in list(stocks)[:10]:
        print(f"  - {stock}")


if __name__ == "__main__":
    asyncio.run(test_foreign_parsing()) 