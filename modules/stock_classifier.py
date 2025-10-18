#!/usr/bin/env python3
"""
종목 분류 모듈
주식과 ETF를 구분하는 기능을 제공합니다.
"""

import json
import logging
import os
from typing import Dict, List
from pathlib import Path

from openai import OpenAI


logger = logging.getLogger(__name__)


class StockClassifier:
    """종목 분류 클래스"""
    
    def __init__(self, cache_file: str = 'stock_type_cache.json', batch_size: int = 10):
        """초기화
        
        Args:
            cache_file: 캐시 파일 경로
            batch_size: 배치 처리 크기
        """
        self.cache_file = cache_file
        self.batch_size = batch_size
        self.cache = self._load_cache()
        self.openai_api_key = os.getenv("OPENAI_API_KEY")
        
        # ETF 키워드
        self.etf_keywords = [
            'KODEX', 'TIGER', 'KBSTAR', 'ARIRANG', 'KIWOOM', 
            'ACE', 'RISE', 'SOL', 'HANARO', 'KOSEF', 
            'KINDEX', 'FOCUS', 'PLUS', 'TIMEFOLIO', 'SAMSUNG',
            'SMART', 'MIRAE', 'TRUE', 'QV', 'TREX',
            'ETF', 'ETN', '액티브', '채권', 'TOP'
        ]
        
        # 해외 ETF 티커 목록
        self.foreign_etf_tickers = {
            # 주요 지수 ETF
            'SPY', 'QQQ', 'IWM', 'DIA', 'VTI', 'VOO', 'IVV', 'MDY', 'IJH', 'IJR',
            'VTV', 'VUG', 'VB', 'VBR', 'VBK', 'VO', 'VOT', 'VOE',
            
            # 섹터 ETF  
            'XLE', 'XLF', 'XLK', 'XLV', 'XLI', 'XLU', 'XLY', 'XLP', 'XLB', 'XLRE', 'XLC',
            'VGT', 'VFH', 'VHT', 'VIS', 'VAW', 'VDE', 'VCR', 'VDC', 'VPU', 'VNQI',
            
            # 채권 ETF
            'TLT', 'IEF', 'SHY', 'AGG', 'BND', 'LQD', 'HYG', 'JNK', 'VCIT', 'VCSH',
            'BSV', 'BIV', 'BLV', 'VMBS', 'VTEB', 'MUB', 'TIP', 'STIP', 'VTIP',
            
            # 상품 ETF
            'GLD', 'SLV', 'USO', 'DBA', 'UNG', 'IAU', 'GLDM', 'SLV', 'PALL', 'PPLT',
            'PDBC', 'DJP', 'GSG', 'RJI', 'DBC', 'BNO', 'UCO', 'SCO',
            
            # 배당/인컴 ETF
            'JEPI', 'JEPQ', 'SCHD', 'VIG', 'VYM', 'DVY', 'SDY', 'DGRO', 'DIVO', 'QYLD',
            'XYLD', 'RYLD', 'PFF', 'PGX', 'PFFD', 'HDV', 'SPYD', 'SPHD', 'FDL', 'IDV',
            
            # 테마/혁신 ETF
            'ARKK', 'ARKG', 'ARKQ', 'ARKW', 'ARKF', 'ARKX', 'IZRL', 'PRNT',
            'ICLN', 'TAN', 'LIT', 'HACK', 'SOXX', 'SMH', 'SKYY', 'CLOU', 'IGV',
            'ROBO', 'BOTZ', 'AIQ', 'QTUM', 'BLOK', 'BETZ', 'NERD', 'HERO', 'ESPO',
            
            # 국제/지역 ETF
            'EEM', 'EFA', 'VEA', 'VWO', 'IEMG', 'IEFA', 'VXUS', 'IXUS', 'ACWI', 'ACWX',
            'EWJ', 'EWT', 'EWZ', 'EWY', 'EWA', 'EWU', 'EWG', 'EWQ', 'EWI', 'EWP',
            'FXI', 'MCHI', 'ASHR', 'INDA', 'EPI', 'RSX', 'ERUS', 'VNM', 'FM',
            
            # 스타일/팩터 ETF
            'MTUM', 'VLUE', 'QUAL', 'SIZE', 'USMV', 'SPLV', 'EFAV', 'ACWV',
            'VBR', 'VBK', 'IWF', 'IWD', 'IWN', 'IWO', 'IWS', 'IWP', 'IWV',
            
            # 기타 ETF
            'TQQQ', 'SQQQ', 'UPRO', 'SPXU', 'UVXY', 'SVXY', 'VXX', 'VIXY',
            'PSQ', 'SH', 'SDS', 'DOG', 'DXD', 'QID', 'SDOW', 'SOXL', 'SOXS'
        }
    
    def _load_cache(self) -> Dict[str, str]:
        """캐시 로드"""
        if os.path.exists(self.cache_file):
            try:
                with open(self.cache_file, 'r', encoding='utf-8') as f:
                    return json.load(f)
            except Exception as e:
                logger.warning(f"캐시 파일 로드 실패: {e}")
        return {}
    
    def _save_cache(self):
        """캐시 저장"""
        try:
            # 주식과 ETF를 분리하여 정렬
            stocks = [(k, v) for k, v in self.cache.items() if v == "주식"]
            etfs = [(k, v) for k, v in self.cache.items() if v == "ETF"]
            
            # 각각 이름순으로 정렬
            stocks.sort(key=lambda x: x[0])
            etfs.sort(key=lambda x: x[0])
            
            # 주식 먼저, ETF 나중에 오도록 병합
            from collections import OrderedDict
            sorted_cache = OrderedDict(stocks + etfs)
            
            with open(self.cache_file, 'w', encoding='utf-8') as f:
                json.dump(sorted_cache, f, ensure_ascii=False, indent=2)
                
            logger.info(f"종목 타입 캐시 저장 완료: 주식 {len(stocks)}개, ETF {len(etfs)}개")
        except Exception as e:
            logger.error(f"캐시 파일 저장 실패: {e}")
    
    def classify(self, stock_names: List[str]) -> Dict[str, str]:
        """종목 분류
        
        Args:
            stock_names: 분류할 종목명 리스트
            
        Returns:
            종목명과 타입(주식/ETF) 매핑
        """
        # 캐시에서 조회
        uncached_stocks = []
        results = {}
        
        for stock in stock_names:
            if stock in self.cache:
                results[stock] = self.cache[stock]
                logger.debug(f"캐시에서 종목 타입 조회: {stock} -> {self.cache[stock]}")
            else:
                uncached_stocks.append(stock)
        
        # 캐시에 없는 종목들 처리
        if uncached_stocks:
            logger.info(f"{len(uncached_stocks)}개 종목에 대해 타입 판별 필요")
            
            # 배치 처리
            for i in range(0, len(uncached_stocks), self.batch_size):
                batch = uncached_stocks[i:i + self.batch_size]
                batch_results = self._classify_batch(batch)
                results.update(batch_results)
                
                # 캐시 업데이트
                self.cache.update(batch_results)
            
            # 캐시 저장
            self._save_cache()
        
        return results
    
    def _classify_batch(self, stocks: List[str]) -> Dict[str, str]:
        """배치 분류"""
        # OpenAI API 사용 가능한 경우
        if self.openai_api_key and self.openai_api_key != "your_openai_api_key_here":
            try:
                return self._classify_with_openai(stocks)
            except Exception as e:
                logger.error(f"ChatGPT API 호출 실패: {e}")
                logger.warning("키워드 기반 판별로 폴백")
        
        # 폴백: 키워드 기반 분류
        return self._classify_with_keywords(stocks)
    
    def _classify_with_openai(self, stocks: List[str]) -> Dict[str, str]:
        """OpenAI API를 사용한 분류"""
        stock_list = "\n".join([f"- {stock}" for stock in stocks])
        
        prompt = f"""다음 한국 증권 종목들이 주식인지 ETF인지 분류해주세요.
각 종목에 대해 "종목명: 주식" 또는 "종목명: ETF" 형식으로만 답해주세요.

종목 목록:
{stock_list}

주의사항:
1. ETF는 Exchange Traded Fund로, KODEX, TIGER, ACE, SOL, RISE, HANARO, WON 등의 브랜드명이 포함됩니다.
2. 채권형 ETF, 액티브 ETF도 모두 ETF입니다.
3. 개별 기업명(예: 삼성전자, SK텔레콤, 대한전선 등)은 주식입니다.
4. 불확실한 경우 ETF 특성(인덱스 추종, 섹터별 묶음, 채권 등)을 고려하세요."""

        client = OpenAI(api_key=self.openai_api_key)
        response = client.chat.completions.create(
            model="gpt-4-1106-preview",
            messages=[
                {"role": "system", "content": "당신은 한국 증권시장 전문가입니다."},
                {"role": "user", "content": prompt}
            ],
            temperature=0.1,
            max_tokens=500
        )
        
        # 응답 파싱
        results = {}
        response_text = response.choices[0].message.content.strip()
        logger.debug(f"ChatGPT 응답:\n{response_text}")
        
        for line in response_text.split('\n'):
            line = line.strip()
            if ':' in line:
                parts = line.split(':', 1)
                if len(parts) == 2:
                    stock_name = parts[0].strip()
                    stock_type = parts[1].strip()
                    
                    # 종목명 매칭
                    for original_stock in stocks:
                        if stock_name in original_stock or original_stock in stock_name:
                            if "불확실" not in stock_type and "?" not in stock_type:
                                results[original_stock] = stock_type
                                logger.debug(f"종목 타입 판별 (ChatGPT): {original_stock} -> {stock_type}")
                            break
        
        # 누락된 종목은 키워드 기반으로 처리
        missing_stocks = [s for s in stocks if s not in results]
        if missing_stocks:
            logger.info(f"{len(missing_stocks)}개 종목을 키워드 기반으로 분류")
            keyword_results = self._classify_with_keywords(missing_stocks)
            results.update(keyword_results)
        
        return results
    
    def _classify_with_keywords(self, stocks: List[str]) -> Dict[str, str]:
        """키워드 기반 분류"""
        results = {}
        
        for stock in stocks:
            stock_upper = stock.upper()
            if any(etf_keyword in stock_upper for etf_keyword in self.etf_keywords):
                results[stock] = 'ETF'
            else:
                results[stock] = '주식'
            logger.debug(f"종목 타입 판별 (키워드): {stock} -> {results[stock]}")
        
        return results
    
    def _classify_foreign_with_openai(self, trades: List) -> Dict[str, str]:
        """OpenAI API를 사용한 해외 종목 분류"""
        from .trade_models import ForeignTrade
        
        # 티커와 종목명 목록 생성
        ticker_info = []
        for trade in trades:
            if isinstance(trade, ForeignTrade):
                ticker_info.append(f"- {trade.ticker}: {trade.stock_name}")
        
        ticker_list = "\n".join(ticker_info)
        
        prompt = f"""다음 미국 증권 종목들이 주식인지 ETF인지 분류해주세요.
각 종목에 대해 "종목명: 주식" 또는 "종목명: ETF" 형식으로만 답해주세요.

종목 목록 (티커: 종목명):
{ticker_list}

주의사항:
1. ETF는 Exchange Traded Fund로, SPY, QQQ, IWM, JEPI, SCHD, TLT, GLD 등이 대표적입니다.
2. SPDR, iShares, Vanguard, Invesco 등의 운용사가 발행한 것들은 대부분 ETF입니다.
3. 개별 기업(예: Apple, Microsoft, Tesla, Coca-Cola 등)은 주식입니다.
4. 우선주(Preferred Stock)나 REIT도 개별 종목이므로 주식으로 분류합니다.
5. 티커가 3-4자리이고 특정 지수나 섹터를 추종하면 ETF일 가능성이 높습니다."""

        client = OpenAI(api_key=self.openai_api_key)
        response = client.chat.completions.create(
            model="gpt-4-1106-preview",
            messages=[
                {"role": "system", "content": "당신은 미국 증권시장 전문가입니다."},
                {"role": "user", "content": prompt}
            ],
            temperature=0.1,
            max_tokens=1000
        )
        
        # 응답 파싱
        results = {}
        response_text = response.choices[0].message.content.strip()
        logger.debug(f"ChatGPT 해외 종목 응답:\n{response_text}")
        
        for line in response_text.split('\n'):
            line = line.strip()
            if ':' in line:
                parts = line.split(':', 1)
                if len(parts) == 2:
                    stock_name_part = parts[0].strip()
                    stock_type = parts[1].strip()
                    
                    # 종목명 매칭
                    for trade in trades:
                        if isinstance(trade, ForeignTrade):
                            if (stock_name_part in trade.stock_name or 
                                trade.stock_name in stock_name_part or
                                trade.ticker in stock_name_part):
                                if "불확실" not in stock_type and "?" not in stock_type:
                                    results[trade.stock_name] = stock_type
                                    # 캐시 키 업데이트
                                    cache_key = f"{trade.ticker}_{trade.stock_name}"
                                    self.cache[cache_key] = stock_type
                                    logger.debug(f"해외 종목 타입 판별 (ChatGPT): {trade.ticker} {trade.stock_name} -> {stock_type}")
                                break
        
        return results
    
    def _classify_foreign_with_keywords(self, trades: List) -> Dict[str, str]:
        """키워드 기반 해외 종목 분류"""
        from .trade_models import ForeignTrade
        
        results = {}
        
        for trade in trades:
            if isinstance(trade, ForeignTrade):
                ticker = trade.ticker.upper()
                stock_name = trade.stock_name
                
                # 티커로 ETF 판별
                if ticker in self.foreign_etf_tickers:
                    stock_type = "ETF"
                # 이름에 ETF 관련 키워드가 있는지 확인
                elif any(keyword in stock_name.upper() for keyword in ['ETF', 'FUND', 'SHARES', 'TRUST', 'SPDR', 'ISHARES']):
                    stock_type = "ETF"
                else:
                    stock_type = "주식"
                
                results[stock_name] = stock_type
                cache_key = f"{ticker}_{stock_name}"
                self.cache[cache_key] = stock_type
                logger.debug(f"해외 종목 타입 판별 (키워드): {ticker} {stock_name} -> {stock_type}")
        
        return results
    
    def classify_foreign(self, trades: List) -> Dict[str, str]:
        """해외 종목 분류 (티커 기반 + OpenAI)
        
        Args:
            trades: ForeignTrade 객체 리스트
            
        Returns:
            종목명과 타입(주식/ETF) 매핑
        """
        from .trade_models import ForeignTrade
        
        results = {}
        uncached_trades = []
        
        # 1. 캐시 확인
        for trade in trades:
            if isinstance(trade, ForeignTrade):
                ticker = trade.ticker.upper()
                stock_name = trade.stock_name
                
                # 캐시 확인
                cache_key = f"{ticker}_{stock_name}"
                if cache_key in self.cache:
                    results[stock_name] = self.cache[cache_key]
                    logger.debug(f"캐시에서 해외 종목 타입 조회: {cache_key} -> {self.cache[cache_key]}")
                else:
                    uncached_trades.append(trade)
        
        # 2. 캐시에 없는 종목들 처리
        if uncached_trades:
            logger.info(f"{len(uncached_trades)}개 해외 종목에 대해 타입 판별 필요")
            
            # OpenAI API 사용 가능한 경우
            if self.openai_api_key and self.openai_api_key != "your_openai_api_key_here":
                try:
                    # 배치 처리
                    for i in range(0, len(uncached_trades), self.batch_size):
                        batch = uncached_trades[i:i + self.batch_size]
                        batch_results = self._classify_foreign_with_openai(batch)
                        results.update(batch_results)
                except Exception as e:
                    logger.error(f"ChatGPT API 호출 실패: {e}")
                    logger.warning("티커/키워드 기반 판별로 폴백")
            
            # 3. OpenAI로 분류되지 않은 종목은 키워드 기반으로 처리
            remaining_trades = [t for t in uncached_trades if t.stock_name not in results]
            if remaining_trades:
                keyword_results = self._classify_foreign_with_keywords(remaining_trades)
                results.update(keyword_results)
        
        # 캐시 저장
        if uncached_trades:
            self._save_cache()
        
        return results 