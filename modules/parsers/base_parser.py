"""파서 추상 클래스"""

from abc import ABC, abstractmethod
from pathlib import Path
from typing import List

from ..models import Trade


class BaseParser(ABC):
    """증권사별 CSV 파서의 기본 클래스"""

    @staticmethod
    @abstractmethod
    def can_parse(header_row: List[str]) -> bool:
        """헤더를 보고 이 파서가 처리 가능한지 판단"""

    @abstractmethod
    def parse(self, file_path: Path, account: str) -> List[Trade]:
        """CSV 파일을 파싱하여 Trade 리스트 반환

        Args:
            file_path: CSV 파일 경로
            account: 계좌 식별자 (예: "미래에셋증권_국내계좌")

        Returns:
            Trade 리스트
        """
