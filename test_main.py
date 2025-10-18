#!/usr/bin/env python3
"""
main.py 테스트 스크립트
리팩토링된 모듈들의 기능을 테스트합니다.
"""

import asyncio
import logging
from pathlib import Path

from modules.file_parser import FileParser, Trade
from modules.stock_classifier import StockClassifier
from modules.data_validator import DataValidator
from modules.sheet_manager import SheetManager
from modules.report_generator import ReportGenerator
from main import StockDataProcessor


# 로거 설정
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)


def test_stock_classifier():
    """StockClassifier 테스트"""
    logger.info("=== StockClassifier 테스트 시작 ===")
    
    classifier = StockClassifier()
    
    # 테스트 종목들
    test_stocks = [
        "삼성전자",
        "SK하이닉스",
        "KODEX 200",
        "TIGER 차이나전기차SOLACTIVE",
        "ACE 미국빅테크TOP7 PLUS",
        "대한전선",
        "SOL 미국S&P500"
    ]
    
    # 분류 실행
    results = classifier.classify(test_stocks)
    
    # 결과 출력
    for stock, stock_type in results.items():
        logger.info(f"{stock}: {stock_type}")
    
    logger.info("=== StockClassifier 테스트 완료 ===\n")


def test_data_validator():
    """DataValidator 테스트"""
    logger.info("=== DataValidator 테스트 시작 ===")
    
    validator = DataValidator()
    
    # 테스트 거래 데이터
    test_trades = [
        Trade("삼성전자", "2024-12-01", "매수", 10, 50000, 500000),
        Trade("SK하이닉스", "2024-12-02", "매도", 5, 100000, 500000),
        Trade("KODEX 200", "2024-12-03", "매수", 20, 25000, 500001),  # 총액 오류
        Trade("대한전선", "2024/12/04", "매수", 10, 10000, 100000),  # 날짜 형식 오류
        Trade("LG전자", "2024-12-05", "구매", 5, 80000, 400000),  # 거래 타입 오류
    ]
    
    # 검증 실행
    valid_trades, invalid_trades = validator.validate_all(test_trades)
    
    # 결과 출력
    logger.info(f"유효한 거래: {len(valid_trades)}개")
    logger.info(f"무효한 거래: {len(invalid_trades)}개")
    
    for trade, errors in invalid_trades:
        logger.warning(f"{trade.stock_name}: {', '.join(errors)}")
    
    logger.info("=== DataValidator 테스트 완료 ===\n")


async def test_sheet_manager():
    """SheetManager 테스트"""
    logger.info("=== SheetManager 테스트 시작 ===")
    
    # 설정 파일에서 스프레드시트 ID 가져오기
    import yaml
    with open("config/config.yaml", 'r', encoding='utf-8') as f:
        config = yaml.safe_load(f)
    
    spreadsheet_id = config.get('google_sheets', {}).get('spreadsheet_id', '')
    service_account_path = config.get('google_sheets', {}).get('service_account_path')
    
    if not spreadsheet_id:
        logger.error("스프레드시트 ID가 설정되지 않았습니다")
        return
    
    manager = SheetManager(spreadsheet_id, service_account_path)
    
    async with manager:
        # 시트 목록 조회
        sheets = await manager.list_sheets()
        logger.info(f"시트 목록: {sheets}")
        
        # 대상 시트 찾기
        target_sheets = await manager.find_target_sheets("계좌1 국내")
        for key, sheet in target_sheets.items():
            logger.info(f"{key}: {sheet}")
    
    logger.info("=== SheetManager 테스트 완료 ===\n")


def test_report_generator():
    """ReportGenerator 테스트"""
    logger.info("=== ReportGenerator 테스트 시작 ===")
    
    generator = ReportGenerator()
    
    # 테스트 데이터
    test_files = [Path("stocks/계좌1 국내.md")]
    test_results = {
        "주식_매수": 10,
        "주식_매도": 5,
        "ETF_매수": 3,
        "ETF_매도": 2
    }
    
    # 요약 리포트 생성
    report_path = generator.generate_summary_report(test_files, test_results, dry_run=True)
    logger.info(f"요약 리포트 생성: {report_path}")
    
    logger.info("=== ReportGenerator 테스트 완료 ===\n")


async def test_full_process():
    """전체 프로세스 테스트 (드라이런)"""
    logger.info("=== 전체 프로세스 테스트 시작 (드라이런) ===")
    
    processor = StockDataProcessor(dry_run=True)
    await processor.run()
    
    logger.info("=== 전체 프로세스 테스트 완료 ===\n")


async def main():
    """메인 테스트 함수"""
    logger.info("리팩토링된 모듈 테스트 시작\n")
    
    # 각 모듈 개별 테스트
    test_stock_classifier()
    test_data_validator()
    await test_sheet_manager()
    test_report_generator()
    
    # 전체 프로세스 테스트
    await test_full_process()
    
    logger.info("모든 테스트 완료!")


if __name__ == "__main__":
    asyncio.run(main()) 
