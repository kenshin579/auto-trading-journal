#!/usr/bin/env python3
"""
주식 매매일지 구글 시트 자동 입력 스크립트 (v2)
input/ 폴더의 증권사별 CSV를 파싱하여 구글 시트에 자동으로 입력합니다.
"""

import argparse
import asyncio
import logging
import os
from collections import defaultdict
from pathlib import Path
from typing import Dict, List, Tuple

import yaml

from modules.models import Trade
from modules.parser_registry import detect_parser
from modules.google_sheets_client import GoogleSheetsClient
from modules.sheet_writer import SheetWriter
from modules.summary_generator import SummaryGenerator


def load_config():
    config_path = Path("config/config.yaml")
    if not config_path.exists():
        raise FileNotFoundError(f"설정 파일을 찾을 수 없습니다: {config_path}")
    with open(config_path, "r", encoding="utf-8") as f:
        return yaml.safe_load(f)


config = load_config()

logging.basicConfig(
    level=config.get("logging", {}).get("level", "INFO"),
    format="%(asctime)s - %(name)s - %(levelname)s - %(message)s",
)
logger = logging.getLogger(__name__)


class StockDataProcessor:
    """주식 데이터 처리 클래스 (v2)"""

    def __init__(self, dry_run: bool = False):
        self.dry_run = dry_run
        if dry_run:
            logger.info("=== 드라이런 모드로 실행 중 (실제 데이터 입력 없음) ===")

        self.spreadsheet_id = (
            os.getenv("GOOGLE_SPREADSHEET_ID")
            or config.get("google_sheets", {}).get("spreadsheet_id", "")
        )
        if not self.spreadsheet_id:
            raise ValueError(
                "환경변수 GOOGLE_SPREADSHEET_ID 또는 설정 파일에 spreadsheet_id가 없습니다"
            )

        service_account_path = config.get("google_sheets", {}).get("service_account_path")
        self.client = GoogleSheetsClient(self.spreadsheet_id, service_account_path)
        self.sheet_writer = SheetWriter(self.client)
        self.summary_generator = SummaryGenerator(self.client, self.sheet_writer)

    def scan_csv_files(self) -> List[Tuple[str, str, Path]]:
        """input/ 하위 CSV 파일 스캔

        Returns:
            [(증권사명, 계좌유형, 파일경로), ...]
        """
        input_dir = Path("input")
        if not input_dir.exists():
            logger.warning("input/ 디렉토리가 존재하지 않습니다")
            return []

        results = []
        for broker_dir in sorted(input_dir.iterdir()):
            if not broker_dir.is_dir():
                continue
            for csv_file in sorted(broker_dir.glob("*.csv")):
                account_type = csv_file.stem  # "국내계좌" or "해외계좌"
                results.append((broker_dir.name, account_type, csv_file))

        logger.info(f"CSV 파일 {len(results)}개 발견")
        return results

    async def process_file(
        self, broker: str, account_type: str, file_path: Path
    ) -> List[Trade]:
        """CSV 파일 하나를 처리하여 Trade 리스트 반환 및 시트에 삽입"""
        sheet_name = f"{broker}_{account_type}"
        account = sheet_name
        is_foreign = "해외" in account_type

        # 1. 파서 감지 및 파싱
        try:
            parser = detect_parser(file_path)
        except ValueError as e:
            logger.error(f"파서 감지 실패: {e}")
            return []

        trades = parser.parse(file_path, account)
        if not trades:
            logger.warning(f"파싱 결과 없음: {file_path.name}")
            return []

        # 날짜순 정렬
        trades.sort(key=lambda t: t.date)

        # 2. 시트 존재 확인 → 없으면 생성
        created = await self.sheet_writer.ensure_sheet_exists(sheet_name, is_foreign)
        if created:
            logger.info(f"새 시트 생성됨: {sheet_name}")

        # 3. 중복 필터링
        existing_keys = await self.sheet_writer.get_existing_keys(sheet_name, is_foreign)
        new_trades = [t for t in trades if t.duplicate_key() not in existing_keys]
        skipped = len(trades) - len(new_trades)
        if skipped > 0:
            logger.info(f"{skipped}건 중복 건너뜀 ({sheet_name})")

        if not new_trades:
            logger.info(f"신규 거래 없음: {sheet_name}")
            return trades  # 요약용으로 전체 반환

        # 4. 시트에 삽입
        if not self.dry_run:
            inserted = await self.sheet_writer.insert_trades(
                sheet_name, new_trades, is_foreign
            )
            logger.info(f"{sheet_name}: {inserted}건 삽입 완료")
        else:
            logger.info(f"[DRY-RUN] {sheet_name}: {len(new_trades)}건 삽입 예정")

        return trades  # 요약용으로 전체 반환

    async def run(self):
        """메인 실행"""
        logger.info("=== 매매일지 구글 시트 입력 시작 (v2) ===")

        async with self.client:
            try:
                # 1. CSV 파일 스캔
                csv_files = self.scan_csv_files()
                if not csv_files:
                    logger.warning("처리할 CSV 파일이 없습니다")
                    return

                # 2. 각 파일 처리
                all_trades: List[Trade] = []
                results: Dict[str, int] = defaultdict(int)

                for i, (broker, account_type, file_path) in enumerate(csv_files, 1):
                    sheet_name = f"{broker}_{account_type}"
                    logger.info(
                        f"[{i}/{len(csv_files)}] {file_path.name} 처리 중..."
                    )
                    try:
                        trades = await self.process_file(broker, account_type, file_path)
                        all_trades.extend(trades)
                        results[sheet_name] += len(
                            [t for t in trades if t.duplicate_key()]
                        )
                        logger.info(
                            f"[{i}/{len(csv_files)}] {file_path.name} 완료 ({len(trades)}건)"
                        )
                    except Exception as e:
                        logger.error(
                            f"[{i}/{len(csv_files)}] {file_path.name} 처리 실패: {e}"
                        )

                # 3. 요약 시트 갱신
                if all_trades and not self.dry_run:
                    logger.info("=== 요약 시트 갱신 중 ===")
                    await self.summary_generator.generate_all(all_trades)
                elif self.dry_run and all_trades:
                    logger.info(f"[DRY-RUN] 요약 시트 갱신 예정 (총 {len(all_trades)}건)")

                # 4. 결과 출력
                logger.info("=== 전체 처리 결과 ===")
                total = len(all_trades)
                logger.info(f"총 {total}건 처리 완료")

            finally:
                logger.info("스크립트 실행 완료")


def main():
    parser = argparse.ArgumentParser(
        description="주식 매매일지를 구글 시트에 자동 입력합니다 (v2)."
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="실제 데이터 입력 없이 시뮬레이션만 수행합니다.",
    )
    parser.add_argument(
        "--log-level",
        default="INFO",
        choices=["DEBUG", "INFO", "WARNING", "ERROR"],
        help="로그 레벨을 설정합니다 (기본값: INFO)",
    )
    args = parser.parse_args()
    logging.getLogger().setLevel(getattr(logging, args.log_level))

    processor = StockDataProcessor(dry_run=args.dry_run)
    asyncio.run(processor.run())


if __name__ == "__main__":
    main()
