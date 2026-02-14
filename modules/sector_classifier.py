"""OpenAI 기반 종목 섹터 분류 모듈

종목명/종목코드를 기반으로 GICS 기반 한국어 섹터로 분류.
JSON 파일 캐시로 반복 API 호출 방지.
"""

import json
import logging
from pathlib import Path
from typing import Dict, List, Tuple

from openai import AsyncOpenAI

logger = logging.getLogger(__name__)

SECTORS = [
    "에너지", "소재", "산업재", "경기소비재", "필수소비재",
    "헬스케어", "금융", "IT", "통신서비스", "유틸리티", "부동산", "기타",
]

SYSTEM_PROMPT = """당신은 주식 종목 섹터 분류 전문가입니다.
주어진 종목명과 종목코드를 보고 GICS 기반 한국어 섹터로 분류하세요.

사용 가능한 섹터: 에너지, 소재, 산업재, 경기소비재, 필수소비재, 헬스케어, 금융, IT, 통신서비스, 유틸리티, 부동산, 기타

규칙:
- ETF는 주요 투자 대상 섹터로 분류
- 분류 불가 시 "기타"
- 반드시 JSON 형식으로만 응답: {"종목명": "섹터명", ...}
- 다른 텍스트 없이 JSON만 출력"""


class SectorClassifier:
    """OpenAI API를 사용한 종목 섹터 분류기"""

    def __init__(self, api_key: str, model: str = "gpt-4o-mini",
                 cache_path: str = "config/sector_cache.json"):
        self.openai_client = AsyncOpenAI(api_key=api_key)
        self.model = model
        self.cache_path = Path(cache_path)
        self.cache: Dict[str, str] = self._load_cache()

    def _load_cache(self) -> Dict[str, str]:
        if not self.cache_path.exists():
            return {}
        try:
            with open(self.cache_path, "r", encoding="utf-8") as f:
                data = json.load(f)
            if not isinstance(data, dict):
                logger.warning(f"캐시 파일 형식 오류, 초기화: {self.cache_path}")
                return {}
            return {k: v for k, v in data.items()
                    if isinstance(k, str) and isinstance(v, str)}
        except (json.JSONDecodeError, OSError) as e:
            logger.warning(f"캐시 파일 로드 실패, 초기화: {e}")
            return {}

    def _save_cache(self):
        self.cache_path.parent.mkdir(parents=True, exist_ok=True)
        with open(self.cache_path, "w", encoding="utf-8") as f:
            json.dump(self.cache, f, ensure_ascii=False, indent=2)

    async def classify(self, stocks: List[Tuple[str, str, str]]) -> Dict[str, str]:
        """종목 리스트를 섹터로 분류

        Args:
            stocks: (stock_name, stock_code, currency) 튜플 리스트

        Returns:
            {stock_name: 섹터명} 딕셔너리
        """
        result: Dict[str, str] = {}
        uncached: List[Tuple[str, str, str]] = []

        for name, code, currency in stocks:
            if name in self.cache:
                result[name] = self.cache[name]
            else:
                uncached.append((name, code, currency))

        if not uncached:
            logger.info(f"섹터 분류: 전체 {len(stocks)}건 캐시 사용")
            return result

        logger.info(f"섹터 분류: {len(uncached)}건 OpenAI 호출 (캐시 {len(result)}건)")

        # 국내/해외 분리하여 배치 처리
        domestic = [(n, c, cur) for n, c, cur in uncached if cur == "KRW"]
        foreign = [(n, c, cur) for n, c, cur in uncached if cur != "KRW"]

        if domestic:
            classified = await self._call_openai(domestic, is_domestic=True)
            result.update(classified)
            self.cache.update(classified)

        if foreign:
            classified = await self._call_openai(foreign, is_domestic=False)
            result.update(classified)
            self.cache.update(classified)

        self._save_cache()
        return result

    async def _call_openai(self, stocks: List[Tuple[str, str, str]],
                           is_domestic: bool) -> Dict[str, str]:
        """OpenAI API 호출로 섹터 분류"""
        market = "한국" if is_domestic else "해외"
        stock_list = "\n".join(
            f"- {name} ({code})" for name, code, _ in stocks
        )
        user_prompt = f"다음 {market} 주식 종목들의 섹터를 분류해주세요:\n{stock_list}"

        try:
            response = await self.openai_client.chat.completions.create(
                model=self.model,
                messages=[
                    {"role": "system", "content": SYSTEM_PROMPT},
                    {"role": "user", "content": user_prompt},
                ],
                temperature=0,
                response_format={"type": "json_object"},
                timeout=30,
            )
            content = response.choices[0].message.content
            classified = json.loads(content)

            # 유효한 섹터만 필터링
            valid = {}
            for name, sector in classified.items():
                if sector in SECTORS:
                    valid[name] = sector
                else:
                    logger.warning(f"알 수 없는 섹터 '{sector}' → '기타' 처리: {name}")
                    valid[name] = "기타"

            # 응답에서 누락된 종목 처리
            for name, _, _ in stocks:
                if name not in valid:
                    logger.warning(f"OpenAI 응답에서 누락된 종목 → '기타' 처리: {name}")
                    valid[name] = "기타"

            return valid

        except Exception as e:
            logger.error(f"OpenAI 섹터 분류 실패: {e}")
            return {name: "기타" for name, _, _ in stocks}
