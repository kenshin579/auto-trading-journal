"""KRX 종목 마스터 파서/리졸버 테스트"""

from modules.symbol_master import (
    _parse_mst_lines,
    KOSPI_FWF_LEN,
    KOSDAQ_FWF_LEN,
)


def _make_line(short_code: str, std_code: str, name: str, fwf_len: int) -> str:
    """mst 한 행 모사: [0:9]=단축코드, [9:21]=표준코드, [21:len-fwf]=한글명, 끝=fwf 꼬리"""
    return short_code.ljust(9) + std_code.ljust(12) + name + ("X" * fwf_len)


def test_parse_mst_lines_extracts_name_to_code():
    text = "\n".join([
        _make_line("494670", "KR7494670001", "TIGER 조선TOP10", KOSPI_FWF_LEN),
        _make_line("005930", "KR7005930003", "삼성전자", KOSPI_FWF_LEN),
    ])
    result = _parse_mst_lines(text, KOSPI_FWF_LEN)
    assert result["TIGER 조선TOP10"] == "494670"
    assert result["삼성전자"] == "005930"


def test_parse_mst_lines_skips_short_lines():
    result = _parse_mst_lines("too-short\n", KOSPI_FWF_LEN)
    assert result == {}


def test_parse_mst_lines_first_occurrence_wins():
    text = "\n".join([
        _make_line("111111", "KR7111111111", "중복명", KOSDAQ_FWF_LEN),
        _make_line("222222", "KR7222222222", "중복명", KOSDAQ_FWF_LEN),
    ])
    result = _parse_mst_lines(text, KOSDAQ_FWF_LEN)
    assert result["중복명"] == "111111"
