#!/usr/bin/env python3
"""
파일 파싱 모듈
stocks 폴더의 매매일지 파일을 읽고 파싱합니다.
"""

import os
import re
from pathlib import Path
from typing import List, Dict, Optional, Tuple
from datetime import datetime
from dataclasses import dataclass
import logging

# Trade 클래스들을 새로운 모듈에서 import
from .trade_models import Trade, DomesticTrade, ForeignTrade, BaseTrade

# 로거 설정
logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(levelname)s - %(message)s')
logger = logging.getLogger(__name__)


@dataclass
class TradingLog:
    """하나의 매매일지 파일 정보"""
    file_path: str
    prefix: str  # 파일명에서 추출한 prefix (ex: "계좌1 국내")
    trades: List[BaseTrade]
    file_type: str  # "domestic" or "foreign"


class FileParser:
    """파일 파싱 클래스"""
    
    def __init__(self, stocks_dir: str = "stocks"):
        self.stocks_dir = Path(stocks_dir)
    
    def scan_files(self) -> List[Path]:
        """stocks 폴더에서 .md 파일 목록을 가져옵니다"""
        md_files = []
        
        # stocks 디렉토리가 존재하는지 확인
        if self.stocks_dir.exists() and self.stocks_dir.is_dir():
            # stocks 디렉토리에서만 .md 파일 찾기
            md_files = list(self.stocks_dir.glob("*.md"))
            logger.info(f"'{self.stocks_dir}' 폴더에서 {len(md_files)}개의 매매일지 파일을 찾았습니다")
        else:
            logger.warning(f"'{self.stocks_dir}' 폴더가 존재하지 않습니다")
        
        return md_files
    
    def detect_file_type(self, file_path: Path) -> str:
        """파일 타입 감지 (domestic/foreign)"""
        # 1. 파일명으로 감지
        if "해외" in file_path.name:
            return "foreign"
        elif "국내" in file_path.name:
            return "domestic"
        
        # 2. 헤더로 감지
        try:
            with open(file_path, 'r', encoding='utf-8') as f:
                header = f.readline().strip()
                if "통화" in header or "환율" in header or "ticker" in header.lower():
                    return "foreign"
                return "domestic"
        except Exception as e:
            logger.warning(f"파일 타입 감지 실패, 기본값 사용: {e}")
            return "domestic"
    
    def extract_prefix(self, file_path: Path) -> str:
        """파일명에서 prefix를 추출합니다"""
        # 파일명에서 .md 확장자 제거
        prefix = file_path.stem
        
        # 해외 파일의 경우 prefix에서 "해외" 제거
        # 예: "계좌1 해외" -> "계좌1"
        if "해외" in prefix:
            prefix = prefix.replace(" 해외", "").replace("해외", "")
            logger.debug(f"해외 파일 '{file_path.name}'에서 prefix '{prefix}' 추출 (해외 제거)")
        else:
            logger.debug(f"파일 '{file_path.name}'에서 prefix '{prefix}' 추출")
        
        return prefix
    
    def parse_trading_log(self, file_path: Path) -> TradingLog:
        """매매일지 파일을 파싱합니다"""
        file_type = self.detect_file_type(file_path)
        logger.info(f"파일 '{file_path.name}' 타입: {file_type}")
        
        if file_type == "domestic":
            return self._parse_domestic_log(file_path)
        else:
            return self._parse_foreign_log(file_path)
    
    def _parse_domestic_log(self, file_path: Path) -> TradingLog:
        """국내 주식 매매일지 파일을 파싱합니다"""
        prefix = self.extract_prefix(file_path)
        trades = []
        
        with open(file_path, 'r', encoding='utf-8') as f:
            lines = f.readlines()
        
        # 헤더를 건너뛰고 데이터 행부터 처리
        data_start_idx = 2
        
        for line_idx, line in enumerate(lines[data_start_idx:], start=data_start_idx):
            line = line.strip()
            if not line:
                continue
            
            # 탭으로 분리
            parts = line.split('\t')
            if len(parts) < 11:  # 최소 필드 개수 확인
                logger.warning(f"라인 {line_idx + 1}: 필드 부족 ({len(parts)}개)")
                continue
            
            try:
                # 날짜 포맷 변환 (YYYY/MM/DD -> YYYY-MM-DD)
                date_str = parts[0].strip()
                date_formatted = date_str.replace('/', '-')
                
                # 종목명
                stock_name = parts[1].strip()
                
                # 매수 정보
                buy_qty = int(parts[2]) if parts[2].strip() and parts[2].strip() != '0' else 0
                buy_price = float(parts[3]) if parts[3].strip() and parts[3].strip() != '0' else 0
                buy_amount = float(parts[4]) if parts[4].strip() and parts[4].strip() != '0' else 0
                
                # 매도 정보
                sell_qty = int(parts[5]) if parts[5].strip() and parts[5].strip() != '0' else 0
                sell_price = float(parts[6]) if parts[6].strip() and parts[6].strip() != '0' else 0
                sell_amount = float(parts[7]) if parts[7].strip() and parts[7].strip() != '0' else 0
                
                # 매수 거래가 있으면 추가
                if buy_qty > 0:
                    trades.append(DomesticTrade(
                        stock_name=stock_name,
                        date=date_formatted,
                        trade_type="매수",
                        quantity=buy_qty,
                        price=buy_price,
                        total_amount=buy_amount
                    ))
                
                # 매도 거래가 있으면 추가
                if sell_qty > 0:
                    trades.append(DomesticTrade(
                        stock_name=stock_name,
                        date=date_formatted,
                        trade_type="매도",
                        quantity=sell_qty,
                        price=sell_price,
                        total_amount=sell_amount
                    ))
                    
            except (ValueError, IndexError) as e:
                logger.warning(f"라인 {line_idx + 1} 파싱 실패: {line[:50]}... - {e}")
                continue
        
        logger.info(f"파일 '{file_path.name}'에서 {len(trades)}개의 거래를 파싱했습니다")
        
        return TradingLog(
            file_path=str(file_path),
            prefix=prefix,
            trades=trades,
            file_type="domestic"
        )
    
    def _parse_foreign_log(self, file_path: Path) -> TradingLog:
        """해외 주식 매매일지 파일을 파싱합니다"""
        prefix = self.extract_prefix(file_path)
        trades = []
        
        with open(file_path, 'r', encoding='utf-8') as f:
            lines = f.readlines()
        
        # 헤더를 건너뛰고 데이터 행부터 처리
        data_start_idx = 1  # 해외 주식은 헤더가 한 줄
        
        for line_idx, line in enumerate(lines[data_start_idx:], start=data_start_idx):
            line = line.strip()
            if not line:
                continue
            
            # 탭으로 분리
            parts = line.split('\t')
            if len(parts) < 22:  # 해외 주식은 더 많은 필드
                logger.warning(f"라인 {line_idx + 1}: 필드 부족 ({len(parts)}개)")
                continue
            
            try:
                # 필드 파싱
                date_str = parts[0].strip().replace('/', '-')
                currency = parts[1].strip()
                ticker = parts[2].strip()
                stock_name = parts[3].strip()
                
                # 수량 정보
                balance_qty = int(parts[4]) if parts[4].strip() else 0
                
                # 환율 정보
                avg_buy_rate = float(parts[5]) if parts[5].strip() else 0
                trade_rate = float(parts[6]) if parts[6].strip() else 0
                
                # 매수 정보
                buy_qty = int(parts[7]) if parts[7].strip() and parts[7].strip() != '0' else 0
                buy_price = float(parts[8]) if parts[8].strip() and parts[8].strip() != '0' else 0
                buy_amount = float(parts[9]) if parts[9].strip() and parts[9].strip() != '0' else 0
                buy_amount_krw = float(parts[10]) if parts[10].strip() and parts[10].strip() != '0' else 0
                
                # 매도 정보
                sell_qty = int(parts[11]) if parts[11].strip() and parts[11].strip() != '0' else 0
                sell_price = float(parts[12]) if parts[12].strip() and parts[12].strip() != '0' else 0
                sell_amount = float(parts[13]) if parts[13].strip() and parts[13].strip() != '0' else 0
                sell_amount_krw = float(parts[14]) if parts[14].strip() and parts[14].strip() != '0' else 0
                
                # 비용 정보
                commission = float(parts[15]) if parts[15].strip() else 0
                tax = float(parts[16]) if parts[16].strip() else 0
                total_cost_krw = float(parts[17]) if parts[17].strip() else 0
                
                # 손익 정보
                avg_buy_price_krw = float(parts[18]) if parts[18].strip() else 0
                trading_profit = float(parts[19]) if parts[19].strip() else 0
                trading_profit_krw = float(parts[20]) if parts[20].strip() else 0
                fx_gain_loss = float(parts[21]) if parts[21].strip() else 0
                total_profit = float(parts[22]) if parts[22].strip() else 0
                profit_rate = float(parts[23]) if parts[23].strip() else 0
                fx_profit_rate = float(parts[24]) if parts[24].strip() else 0
                
                # 매수 거래가 있으면 추가
                if buy_qty > 0:
                    trades.append(ForeignTrade(
                        stock_name=stock_name,
                        date=date_str,
                        currency=currency,
                        ticker=ticker,
                        trade_type="매수",
                        balance_quantity=balance_qty,
                        avg_buy_exchange_rate=avg_buy_rate,
                        trade_exchange_rate=trade_rate,
                        buy_quantity=buy_qty,
                        buy_price=buy_price,
                        buy_amount=buy_amount,
                        buy_amount_krw=buy_amount_krw,
                        sell_quantity=0,
                        sell_price=0,
                        sell_amount=0,
                        sell_amount_krw=0,
                        commission=commission,
                        tax=tax,
                        total_cost_krw=total_cost_krw,
                        avg_buy_price_krw=avg_buy_price_krw,
                        trading_profit=0,
                        trading_profit_krw=0,
                        fx_gain_loss=0,
                        total_profit=0,
                        profit_rate=0,
                        fx_profit_rate=0
                    ))
                
                # 매도 거래가 있으면 추가
                if sell_qty > 0:
                    trades.append(ForeignTrade(
                        stock_name=stock_name,
                        date=date_str,
                        currency=currency,
                        ticker=ticker,
                        trade_type="매도",
                        balance_quantity=balance_qty,
                        avg_buy_exchange_rate=avg_buy_rate,
                        trade_exchange_rate=trade_rate,
                        buy_quantity=0,
                        buy_price=0,
                        buy_amount=0,
                        buy_amount_krw=0,
                        sell_quantity=sell_qty,
                        sell_price=sell_price,
                        sell_amount=sell_amount,
                        sell_amount_krw=sell_amount_krw,
                        commission=commission,
                        tax=tax,
                        total_cost_krw=total_cost_krw,
                        avg_buy_price_krw=avg_buy_price_krw,
                        trading_profit=trading_profit,
                        trading_profit_krw=trading_profit_krw,
                        fx_gain_loss=fx_gain_loss,
                        total_profit=total_profit,
                        profit_rate=profit_rate,
                        fx_profit_rate=fx_profit_rate
                    ))
                    
            except (ValueError, IndexError) as e:
                logger.warning(f"라인 {line_idx + 1} 파싱 실패: {line[:50]}... - {e}")
                continue
        
        logger.info(f"파일 '{file_path.name}'에서 {len(trades)}개의 해외 거래를 파싱했습니다")
        
        return TradingLog(
            file_path=str(file_path),
            prefix=prefix,
            trades=trades,
            file_type="foreign"
        )


def main():
    """테스트 함수"""
    parser = FileParser()
    
    # 파일 스캔
    files = parser.scan_files()
    
    # 각 파일 파싱
    for file_path in files:
        trading_log = parser.parse_trading_log(file_path)
        
        print(f"\n파일: {trading_log.prefix} (타입: {trading_log.file_type})")
        print(f"거래 수: {len(trading_log.trades)}")
        
        # 처음 5개 거래 출력
        for i, trade in enumerate(trading_log.trades[:5]):
            print(f"\n거래 {i+1}:")
            print(f"  종목: {trade.stock_name}")
            print(f"  날짜: {trade.date}")
            
            if isinstance(trade, DomesticTrade):
                print(f"  구분: {trade.trade_type}")
                print(f"  수량: {trade.quantity}")
                print(f"  단가: ₩{trade.price:,.0f}")
                print(f"  총액: ₩{trade.total_amount:,.0f}")
            elif isinstance(trade, ForeignTrade):
                print(f"  통화: {trade.currency}")
                print(f"  티커: {trade.ticker}")
                print(f"  구분: {trade.trade_type}")
                if trade.trade_type == "매수":
                    print(f"  수량: {trade.buy_quantity}")
                    print(f"  단가: ${trade.buy_price:.2f}")
                    print(f"  금액: ${trade.buy_amount:.2f}")
                else:
                    print(f"  수량: {trade.sell_quantity}")
                    print(f"  단가: ${trade.sell_price:.2f}")
                    print(f"  금액: ${trade.sell_amount:.2f}")
                    print(f"  손익률: {trade.profit_rate:.2f}%")


if __name__ == "__main__":
    main() 