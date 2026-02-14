"""파서 레지스트리 - CSV 헤더 기반 파서 자동 선택"""

import csv
import logging
from pathlib import Path

from .parsers.base_parser import BaseParser
from .parsers.mirae_parser import MiraeDomesticParser, MiraeForeignParser
from .parsers.hankook_parser import HankookDomesticParser

logger = logging.getLogger(__name__)

# 등록된 파서 목록 (순서대로 매칭 시도)
PARSERS = [
    MiraeDomesticParser,
    MiraeForeignParser,
    HankookDomesticParser,
]


def detect_parser(file_path: Path) -> BaseParser:
    """CSV 첫 번째 행을 읽어 적합한 파서를 반환

    Args:
        file_path: CSV 파일 경로

    Returns:
        매칭된 파서 인스턴스

    Raises:
        ValueError: 지원되지 않는 CSV 포맷일 때
    """
    with open(file_path, "r", encoding="utf-8") as f:
        reader = csv.reader(f)
        header = next(reader)

    header_clean = [h.strip().strip('"') for h in header]
    logger.debug(f"CSV 헤더 감지: {header_clean[:5]}... ({file_path.name})")

    for parser_cls in PARSERS:
        if parser_cls.can_parse(header_clean):
            logger.info(f"파서 선택: {parser_cls.__name__} ({file_path.name})")
            return parser_cls()

    raise ValueError(f"지원되지 않는 CSV 포맷: {file_path}")
