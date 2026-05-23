"""KRX 공개 종목 마스터 (kospi/kosdaq_code.mst) 다운로드·캐시·파싱.

한투 API 가 아니라 KRX 가 공개 다운로드로 제공하는 .mst.zip (cp949 + fixed-width).
종목명 → 단축코드(티커) 매핑만 필요하므로 fwf 전체 컬럼 파싱은 생략하고
앞부분(단축코드/표준코드/한글명)만 추출한다.
"""

import logging

logger = logging.getLogger(__name__)

# mst 한 행의 마지막 fixed-width 영역 길이 (KIS SDK krxmaster.go 와 동일)
KOSPI_FWF_LEN = 227
KOSDAQ_FWF_LEN = 221


def _parse_mst_lines(text: str, fwf_len: int) -> dict:
    """디코드된 mst 텍스트에서 {한글명: 단축코드} dict 추출.

    한 행: [0:9]=단축코드, [9:21]=표준코드(ISIN), [21:len-fwf_len]=한글명.
    동일 종목명이 여러 번 나오면 첫 항목 우선.
    """
    out: dict = {}
    for line in text.split("\n"):
        line = line.rstrip("\r")
        if len(line) < fwf_len + 21:
            continue
        code = line[0:9].strip()
        name = line[21:len(line) - fwf_len].strip()
        if code and name:
            out.setdefault(name, code)
    return out
