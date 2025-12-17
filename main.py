#!/usr/bin/env python3
"""
주식 매매일지 구글 시트 자동 입력 스크립트
stocks 폴더의 매매일지를 파싱하여 구글 시트에 자동으로 입력합니다.
"""

import argparse
import asyncio
import logging
import os
from collections import defaultdict
from pathlib import Path
from typing import Dict, List

import yaml

from modules.file_parser import FileParser, TradingLog
from modules.stock_classifier import StockClassifier
from modules.data_validator import DataValidator
from modules.sheet_manager import SheetManager
from modules.report_generator import ReportGenerator


# 설정 파일 로드
def load_config():
    config_path = Path("config/config.yaml")
    if not config_path.exists():
        raise FileNotFoundError(f"설정 파일을 찾을 수 없습니다: {config_path}")
    
    with open(config_path, 'r', encoding='utf-8') as f:
        return yaml.safe_load(f)


# 설정 로드
config = load_config()

# 로거 설정
logging.basicConfig(
    level=config.get('logging', {}).get('level', 'INFO'),
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)


class StockDataProcessor:
    """주식 데이터 처리 클래스 - 전체 프로세스 조율"""
    
    def __init__(self, dry_run: bool = False):
        """초기화
        
        Args:
            dry_run: True면 실제 데이터 입력 없이 시뮬레이션만 수행
        """
        self.dry_run = dry_run
        if dry_run:
            logger.info("=== 드라이런 모드로 실행 중 (실제 데이터 입력 없음) ===")
        
        # 스프레드시트 ID 가져오기 (환경변수 우선)
        self.spreadsheet_id = os.getenv('GOOGLE_SPREADSHEET_ID') or config.get('google_sheets', {}).get('spreadsheet_id', '')
        if not self.spreadsheet_id:
            raise ValueError("환경변수 GOOGLE_SPREADSHEET_ID 또는 설정 파일에 spreadsheet_id가 없습니다")
        
        # 서비스 계정 경로 가져오기
        service_account_path = config.get('google_sheets', {}).get('service_account_path')
        
        # 각 모듈 초기화
        self.file_parser = FileParser()
        self.stock_classifier = StockClassifier(
            cache_file=config.get('stock_type_cache_file', 'stock_type_cache.json'),
            batch_size=config.get('batch_size', 10)
        )
        self.data_validator = DataValidator()
        self.sheet_manager = SheetManager(
            spreadsheet_id=self.spreadsheet_id,
            service_account_path=service_account_path,
            empty_row_threshold=config.get('empty_row_threshold', 100)
        )
        self.report_generator = ReportGenerator()
    
    async def process_trading_log(self, trading_log: TradingLog) -> Dict[str, int]:
        """하나의 매매일지 파일을 처리합니다"""
        logger.info(f"파일 '{trading_log.prefix}' 처리 시작... (타입: {trading_log.file_type})")
        
        # 1. 데이터 검증
        valid_trades, invalid_trades = self.data_validator.validate_all(trading_log.trades)
        if invalid_trades:
            logger.warning(f"{len(invalid_trades)}개의 무효한 거래가 제외됩니다")
        
        if not valid_trades:
            logger.warning(f"파일 '{trading_log.prefix}'에 유효한 거래가 없습니다")
            return {}
        
        # 2. 종목 타입 분류
        if trading_log.file_type == "foreign":
            # 해외 주식은 티커 기반 분류
            stock_types = self.stock_classifier.classify_foreign(valid_trades)
        else:
            # 국내 주식은 기존 방식
            unique_stocks = list(set(trade.stock_name for trade in valid_trades))
            stock_types = self.stock_classifier.classify(unique_stocks)
        
        # 3. 대상 시트 찾기
        target_sheets = await self.sheet_manager.find_target_sheets(
            trading_log.prefix, 
            file_type=trading_log.file_type
        )
        
        # 4. 거래를 타입별로 분류
        classified_trades = {
            "주식_매수": [],
            "주식_매도": [],
            "ETF_매수": [],
            "ETF_매도": []
        }
        
        for trade in valid_trades:
            stock_type = stock_types.get(trade.stock_name, "주식")
            trade_category = f"{stock_type}_{trade.trade_type}"
            classified_trades[trade_category].append(trade)
        
        # 날짜순 정렬
        for trades in classified_trades.values():
            trades.sort(key=lambda t: t.date)
        
        # 5. 각 시트에 대한 데이터 준비
        sheet_trades = {}
        results = {}
        column_info = {}  # 컬럼 정보 저장용
        
        for category, sheet_name in target_sheets.items():
            trades = classified_trades[category]
            if not trades or not sheet_name:
                continue
            
            # 마지막 행 찾기
            last_row, data_start_col, data_end_col = await self.sheet_manager.find_last_row_with_column_info(sheet_name)
            column_info[sheet_name] = (data_start_col, data_end_col)  # 컬럼 정보 저장
            
            # 중복 체크
            duplicate_indices = await self.sheet_manager.check_duplicates(sheet_name, trades)
            non_duplicate_trades = [t for i, t in enumerate(trades) if i not in duplicate_indices]
            
            if duplicate_indices:
                logger.warning(f"{len(duplicate_indices)}개 중복 거래 발견, 건너뜀")
            
            if non_duplicate_trades:
                if sheet_name not in sheet_trades:
                    sheet_trades[sheet_name] = []
                sheet_trades[sheet_name].append((non_duplicate_trades, last_row))
                results[category] = len(non_duplicate_trades)
        
        # 6. 배치 업데이트 실행
        if not self.dry_run and sheet_trades:
            insert_results = await self.sheet_manager.batch_insert_trades(sheet_trades)
            
            # 실패한 시트의 결과를 0으로 업데이트
            for sheet_name, success in insert_results.items():
                if not success:
                    for category, target_sheet in target_sheets.items():
                        if target_sheet == sheet_name:
                            results[category] = 0
        elif self.dry_run:
            # 드라이런 모드 로그
            logger.info("[DRY-RUN] 다음 데이터를 입력할 예정:")
            for sheet_name, trade_groups in sheet_trades.items():
                for trades, start_row in trade_groups:
                    start_col, end_col = column_info.get(sheet_name, (None, None))
                    logger.info(f"  시트: {sheet_name}, 시작행: {start_row}, 시작컬럼: {start_col}, 종료컬럼: {end_col}, 거래수: {len(trades)}")
        
        # 7. 결과 출력
        logger.info("=== 처리 결과 ===")
        for category, count in results.items():
            logger.info(f"{category}: {count}건")
        
        total_count = sum(results.values())
        logger.info(f"총 {total_count}건 처리 완료")
        
        return results
    
    async def run(self):
        """메인 실행 함수"""
        logger.info("=== 주식 데이터 구글 시트 입력 시작 ===")
        
        # SheetManager를 context manager로 사용
        async with self.sheet_manager:
            try:
                # 파일 스캔
                files = self.file_parser.scan_files()
                if not files:
                    logger.warning("처리할 파일이 없습니다")
                    return
                
                # 각 파일 처리
                total_results = defaultdict(int)
                processed_logs = []
                validation_results = {}
                
                for i, file_path in enumerate(files, 1):
                    try:
                        logger.info(f"[{i}/{len(files)}] 파일 '{file_path.name}' 처리 중...")
                        trading_log = self.file_parser.parse_trading_log(file_path)
                        processed_logs.append(trading_log)
                        
                        # 검증 결과 저장 (상세 리포트용)
                        valid_trades, invalid_trades = self.data_validator.validate_all(trading_log.trades)
                        validation_results[str(file_path)] = {
                            'valid': len(valid_trades),
                            'invalid': len(invalid_trades),
                            'invalid_details': invalid_trades
                        }
                        
                        # 파일 처리
                        file_results = await self.process_trading_log(trading_log)
                        
                        # 결과 집계
                        for category, count in file_results.items():
                            total_results[category] += count
                        
                        logger.info(f"[{i}/{len(files)}] 파일 '{file_path.name}' 처리 완료")
                        
                    except Exception as e:
                        logger.error(f"[{i}/{len(files)}] 파일 '{file_path.name}' 처리 실패: {e}")
                        continue
                
                # 최종 결과 출력
                logger.info("=== 전체 처리 결과 ===")
                for category, count in total_results.items():
                    logger.info(f"{category}: {count}건")
                
                total_count = sum(total_results.values())
                logger.info(f"총 {total_count}건 처리 완료")
                
                # 리포트 생성
                self.report_generator.generate_summary_report(files, total_results, self.dry_run)
                if processed_logs:
                    self.report_generator.generate_detailed_report(
                        processed_logs, validation_results, self.dry_run
                    )
                
            finally:
                logger.info("스크립트 실행 완료")


def main():
    """메인 함수"""
    # 커맨드라인 인자 파싱
    parser = argparse.ArgumentParser(description='주식 매매일지를 구글 시트에 자동 입력합니다.')
    parser.add_argument('--dry-run', action='store_true', 
                        help='실제 데이터 입력 없이 시뮬레이션만 수행합니다.')
    parser.add_argument('--log-level', default='INFO',
                        choices=['DEBUG', 'INFO', 'WARNING', 'ERROR'],
                        help='로그 레벨을 설정합니다 (기본값: INFO)')
    
    args = parser.parse_args()
    
    # 로그 레벨 설정
    logging.getLogger().setLevel(getattr(logging, args.log_level))
    
    # 프로세서 생성 및 실행
    processor = StockDataProcessor(dry_run=args.dry_run)
    asyncio.run(processor.run())


if __name__ == "__main__":
    main() 