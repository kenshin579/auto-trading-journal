#!/usr/bin/env python3
"""
리포트 생성 모듈
처리 결과에 대한 리포트를 생성합니다.
"""

import logging
from datetime import datetime
from pathlib import Path
from typing import Dict, List

from .file_parser import TradingLog


logger = logging.getLogger(__name__)


class ReportGenerator:
    """리포트 생성 클래스"""
    
    def __init__(self, report_dir: str = "reports"):
        """초기화
        
        Args:
            report_dir: 리포트 저장 디렉토리
        """
        self.report_dir = Path(report_dir)
        self.report_dir.mkdir(parents=True, exist_ok=True)
    
    def generate_summary_report(self, files: List[Path], results: Dict[str, int], 
                              dry_run: bool = False) -> Path:
        """처리 결과 요약 리포트 생성
        
        Args:
            files: 처리된 파일 목록
            results: 처리 결과 (카테고리별 건수)
            dry_run: 드라이런 모드 여부
            
        Returns:
            생성된 리포트 파일 경로
        """
        timestamp = datetime.now().strftime('%Y%m%d_%H%M%S')
        report_file = self.report_dir / f"summary_report_{timestamp}.txt"
        
        # 리포트 내용 생성
        report_content = self._create_summary_content(files, results, dry_run)
        
        # 리포트 파일 저장
        with open(report_file, 'w', encoding='utf-8') as f:
            f.write(report_content)
        
        logger.info(f"요약 리포트가 {report_file}에 저장되었습니다")
        return report_file
    
    def generate_detailed_report(self, trading_logs: List[TradingLog], 
                               validation_results: Dict, dry_run: bool = False) -> Path:
        """상세 처리 리포트 생성
        
        Args:
            trading_logs: 처리된 매매일지 목록
            validation_results: 검증 결과
            dry_run: 드라이런 모드 여부
            
        Returns:
            생성된 리포트 파일 경로
        """
        timestamp = datetime.now().strftime('%Y%m%d_%H%M%S')
        report_file = self.report_dir / f"detailed_report_{timestamp}.txt"
        
        # 리포트 내용 생성
        report_content = self._create_detailed_content(trading_logs, validation_results, dry_run)
        
        # 리포트 파일 저장
        with open(report_file, 'w', encoding='utf-8') as f:
            f.write(report_content)
        
        logger.info(f"상세 리포트가 {report_file}에 저장되었습니다")
        return report_file
    
    def _create_summary_content(self, files: List[Path], results: Dict[str, int], 
                               dry_run: bool) -> str:
        """요약 리포트 내용 생성"""
        mode_str = "[드라이런 모드] " if dry_run else ""
        
        content = f"{mode_str}주식 매매일지 처리 리포트\n"
        content += "=" * 50 + "\n\n"
        
        # 처리 시간
        content += f"처리 시간: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}\n"
        content += f"처리 모드: {'드라이런' if dry_run else '실제 실행'}\n\n"
        
        # 파일 정보
        content += f"처리된 파일 수: {len(files)}\n"
        content += "처리된 파일 목록:\n"
        for file in files:
            content += f"  - {file.name}\n"
        content += "\n"
        
        # 처리 결과
        total_count = sum(results.values())
        content += f"총 거래 수: {total_count:,}건\n\n"
        
        content += "카테고리별 처리 결과:\n"
        for category, count in sorted(results.items()):
            content += f"  {category}: {count:,}건\n"
        
        # 통계 정보
        if total_count > 0:
            content += "\n통계 정보:\n"
            stock_buy = results.get('주식_매수', 0)
            stock_sell = results.get('주식_매도', 0)
            etf_buy = results.get('ETF_매수', 0)
            etf_sell = results.get('ETF_매도', 0)
            
            content += f"  주식 거래: {stock_buy + stock_sell:,}건 "
            content += f"(매수: {stock_buy:,}, 매도: {stock_sell:,})\n"
            content += f"  ETF 거래: {etf_buy + etf_sell:,}건 "
            content += f"(매수: {etf_buy:,}, 매도: {etf_sell:,})\n"
        
        return content
    
    def _create_detailed_content(self, trading_logs: List[TradingLog], 
                               validation_results: Dict, dry_run: bool) -> str:
        """상세 리포트 내용 생성"""
        mode_str = "[드라이런 모드] " if dry_run else ""
        
        content = f"{mode_str}주식 매매일지 상세 처리 리포트\n"
        content += "=" * 50 + "\n\n"
        
        # 처리 시간
        content += f"처리 시간: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}\n"
        content += f"처리 모드: {'드라이런' if dry_run else '실제 실행'}\n\n"
        
        # 각 파일별 상세 정보
        for log in trading_logs:
            content += f"\n파일: {log.file_path}\n"
            content += "-" * 30 + "\n"
            content += f"  계좌: {log.prefix}\n"
            content += f"  거래 수: {len(log.trades)}건\n"
            
            # 검증 결과가 있으면 추가
            if log.file_path in validation_results:
                valid_count = validation_results[log.file_path].get('valid', 0)
                invalid_count = validation_results[log.file_path].get('invalid', 0)
                content += f"  유효한 거래: {valid_count}건\n"
                content += f"  무효한 거래: {invalid_count}건\n"
                
                # 무효한 거래 상세 정보
                invalid_trades = validation_results[log.file_path].get('invalid_details', [])
                if invalid_trades:
                    content += "  무효한 거래 상세:\n"
                    for trade, errors in invalid_trades[:5]:  # 최대 5개만 표시
                        content += f"    - {trade.stock_name} ({trade.date}): "
                        content += ", ".join(errors) + "\n"
                    if len(invalid_trades) > 5:
                        content += f"    ... 외 {len(invalid_trades) - 5}건\n"
        
        return content 