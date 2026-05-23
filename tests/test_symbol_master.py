"""KRX 종목 마스터 파서/리졸버 테스트"""

import io
import time
import zipfile

import pytest

from modules.symbol_master import (
    _parse_mst_lines,
    _extract_mst_text,
    _fetch_zip,
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


def _make_zip(mst_name: str, content: bytes) -> bytes:
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w") as z:
        z.writestr(mst_name, content)
    return buf.getvalue()


def test_extract_mst_text_decodes_cp949():
    zip_bytes = _make_zip("kospi_code.mst", "삼성전자".encode("cp949"))
    assert _extract_mst_text(zip_bytes) == "삼성전자"


def test_fetch_zip_uses_fresh_cache_without_download(tmp_path, monkeypatch):
    cache_file = tmp_path / "kospi_code.mst.zip"
    cache_file.write_bytes(b"cached-bytes")
    monkeypatch.setattr("modules.symbol_master._cache_dir", lambda: tmp_path)

    def _boom(url):
        raise AssertionError("신선한 캐시가 있으면 다운로드하면 안 됨")

    monkeypatch.setattr("modules.symbol_master._download", _boom)
    assert _fetch_zip("http://x", "kospi_code.mst.zip") == b"cached-bytes"


def test_fetch_zip_downloads_when_cache_missing(tmp_path, monkeypatch):
    monkeypatch.setattr("modules.symbol_master._cache_dir", lambda: tmp_path)
    monkeypatch.setattr("modules.symbol_master._download", lambda url: b"downloaded")
    result = _fetch_zip("http://x", "kospi_code.mst.zip")
    assert result == b"downloaded"
    assert (tmp_path / "kospi_code.mst.zip").read_bytes() == b"downloaded"


def test_fetch_zip_falls_back_to_stale_cache_on_download_error(tmp_path, monkeypatch):
    cache_file = tmp_path / "kospi_code.mst.zip"
    cache_file.write_bytes(b"stale")
    old = time.time() - 100 * 24 * 3600
    import os
    os.utime(cache_file, (old, old))
    monkeypatch.setattr("modules.symbol_master._cache_dir", lambda: tmp_path)

    def _boom(url):
        raise OSError("network down")

    monkeypatch.setattr("modules.symbol_master._download", _boom)
    assert _fetch_zip("http://x", "kospi_code.mst.zip") == b"stale"
