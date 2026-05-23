"""KRX 공개 종목 마스터 (kospi/kosdaq_code.mst) 다운로드·캐시·파싱.

한투 API 가 아니라 KRX 가 공개 다운로드로 제공하는 .mst.zip (cp949 + fixed-width).
종목명 → 단축코드(티커) 매핑만 필요하므로 fwf 전체 컬럼 파싱은 생략하고
앞부분(단축코드/표준코드/한글명)만 추출한다.
"""

import io
import logging
import os
import time
import urllib.request
import zipfile
from pathlib import Path

logger = logging.getLogger(__name__)

# mst 한 행의 마지막 fixed-width 영역 길이 (KIS SDK krxmaster.go 와 동일)
KOSPI_FWF_LEN = 227
KOSDAQ_FWF_LEN = 221

KOSPI_URL = "https://new.real.download.dws.co.kr/common/master/kospi_code.mst.zip"
KOSDAQ_URL = "https://new.real.download.dws.co.kr/common/master/kosdaq_code.mst.zip"
# KRX 마스터는 영업일마다 갱신되나 종목코드 변동은 드물어 7일 캐시로 충분
CACHE_TTL_SEC = 7 * 24 * 3600


def _cache_dir() -> Path:
    d = Path(os.path.expanduser("~/.cache/auto-trading-journal"))
    d.mkdir(parents=True, exist_ok=True)
    return d


def _download(url: str) -> bytes:
    with urllib.request.urlopen(url, timeout=30) as resp:
        return resp.read()


def _fetch_zip(url: str, cache_name: str) -> bytes:
    """캐시가 신선하면 캐시 사용, 만료/없음이면 다운로드. 다운로드 실패 시 만료 캐시라도 사용."""
    path = _cache_dir() / cache_name
    if path.exists() and (time.time() - path.stat().st_mtime) < CACHE_TTL_SEC:
        return path.read_bytes()
    try:
        data = _download(url)
        try:
            zipfile.ZipFile(io.BytesIO(data))
        except zipfile.BadZipFile as exc:
            raise ValueError(f"다운로드 데이터가 유효한 ZIP이 아닙니다 ({cache_name})") from exc
        path.write_bytes(data)
        return data
    except Exception as e:
        if path.exists():
            logger.warning(f"KRX 마스터 다운로드 실패, 만료 캐시 사용 ({cache_name}): {e}")
            return path.read_bytes()
        raise


def _extract_mst_text(zip_bytes: bytes) -> str:
    """ZIP byte 에서 .mst 파일을 찾아 cp949 디코드한 텍스트 반환."""
    with zipfile.ZipFile(io.BytesIO(zip_bytes)) as z:
        mst_name = next((n for n in z.namelist() if n.endswith(".mst")), None)
        if mst_name is None:
            raise ValueError(f"ZIP에 .mst 파일이 없습니다 (entries: {z.namelist()})")
        raw = z.read(mst_name)
    return raw.decode("cp949")


def _parse_mst_lines(text: str, fwf_len: int) -> dict[str, str]:
    """디코드된 mst 텍스트에서 {한글명: 단축코드} dict 추출.

    한 행: [0:9]=단축코드, [9:21]=표준코드(ISIN), [21:len-fwf_len]=한글명.
    동일 종목명이 여러 번 나오면 첫 항목 우선.
    """
    out: dict[str, str] = {}
    for line in text.splitlines():
        if len(line) < fwf_len + 21:
            continue
        code = line[0:9].strip()
        name = line[21:len(line) - fwf_len].strip()
        if code and name:
            out.setdefault(name, code)
    return out


def _load_all() -> dict[str, str]:
    """KOSPI + KOSDAQ 마스터를 모두 로드해 {한글명: 단축코드} 통합 dict 반환."""
    merged: dict[str, str] = {}
    sources = [
        (KOSPI_URL, "kospi_code.mst.zip", KOSPI_FWF_LEN),
        (KOSDAQ_URL, "kosdaq_code.mst.zip", KOSDAQ_FWF_LEN),
    ]
    for url, cache_name, fwf_len in sources:
        text = _extract_mst_text(_fetch_zip(url, cache_name))
        for name, code in _parse_mst_lines(text, fwf_len).items():
            merged.setdefault(name, code)
    logger.info(f"KRX 종목 마스터 로드 완료: {len(merged)}종목")
    return merged


class SymbolResolver:
    """종목명 → 단축코드(티커) 리졸버. 최초 호출 시 마스터를 1회 로드."""

    def __init__(self, name_to_code: dict[str, str] | None = None):
        self._map: dict[str, str] | None = name_to_code

    def _ensure_loaded(self) -> None:
        if self._map is None:
            self._map = _load_all()

    def resolve(self, stock_name: str) -> str:
        self._ensure_loaded()
        stripped = stock_name.strip()
        code = self._map.get(stripped, "")
        if not code:
            logger.warning(f"종목코드 미발견 (KRX 마스터): {stripped!r}")
        return code
