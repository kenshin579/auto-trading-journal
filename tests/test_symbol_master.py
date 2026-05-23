"""KRX 종목 마스터 파서/리졸버 테스트"""

import io
import logging
import os
import time
import zipfile

import pytest

from modules.symbol_master import (
    _parse_mst_lines,
    _extract_mst_text,
    _fetch_zip,
    KOSPI_FWF_LEN,
    KOSDAQ_FWF_LEN,
    SymbolResolver,
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
    mtime_before = cache_file.stat().st_mtime
    assert _fetch_zip("http://x", "kospi_code.mst.zip") == b"cached-bytes"
    assert cache_file.stat().st_mtime == mtime_before  # 캐시 히트는 mtime을 갱신하지 않아야 함


def test_fetch_zip_downloads_when_cache_missing(tmp_path, monkeypatch):
    zip_bytes = _make_zip("kospi_code.mst", b"data")
    monkeypatch.setattr("modules.symbol_master._cache_dir", lambda: tmp_path)
    monkeypatch.setattr("modules.symbol_master._download", lambda url: zip_bytes)
    result = _fetch_zip("http://x", "kospi_code.mst.zip")
    assert result == zip_bytes
    assert (tmp_path / "kospi_code.mst.zip").read_bytes() == zip_bytes


def test_fetch_zip_falls_back_to_stale_cache_on_download_error(tmp_path, monkeypatch):
    cache_file = tmp_path / "kospi_code.mst.zip"
    cache_file.write_bytes(b"stale")
    old = time.time() - 100 * 24 * 3600
    os.utime(cache_file, (old, old))
    monkeypatch.setattr("modules.symbol_master._cache_dir", lambda: tmp_path)

    def _boom(url):
        raise OSError("network down")

    monkeypatch.setattr("modules.symbol_master._download", _boom)
    assert _fetch_zip("http://x", "kospi_code.mst.zip") == b"stale"


def test_extract_mst_text_raises_on_missing_mst():
    zip_bytes = _make_zip("kospi_code.dat", b"wrong")  # .mst 없음
    with pytest.raises(ValueError, match=".mst"):
        _extract_mst_text(zip_bytes)


def test_fetch_zip_raises_when_no_cache_and_download_fails(tmp_path, monkeypatch):
    monkeypatch.setattr("modules.symbol_master._cache_dir", lambda: tmp_path)

    def _boom(url):
        raise OSError("network down")

    monkeypatch.setattr("modules.symbol_master._download", _boom)
    with pytest.raises(OSError, match="network down"):
        _fetch_zip("http://x", "kospi_code.mst.zip")


class TestSymbolResolver:
    def test_resolve_returns_code_for_known_name(self):
        resolver = SymbolResolver(name_to_code={"TIGER 조선TOP10": "494670"})
        assert resolver.resolve("TIGER 조선TOP10") == "494670"

    def test_resolve_strips_whitespace(self):
        resolver = SymbolResolver(name_to_code={"삼성전자": "005930"})
        assert resolver.resolve("  삼성전자  ") == "005930"

    def test_resolve_returns_empty_for_unknown(self, caplog):
        resolver = SymbolResolver(name_to_code={})
        with caplog.at_level(logging.WARNING, logger="modules.symbol_master"):
            assert resolver.resolve("없는종목") == ""
        assert "없는종목" in caplog.text

    def test_resolve_lazy_loads_only_once(self, monkeypatch):
        calls = {"n": 0}

        def _fake_load():
            calls["n"] += 1
            return {"삼성전자": "005930"}

        monkeypatch.setattr("modules.symbol_master._load_all", _fake_load)
        resolver = SymbolResolver()
        resolver.resolve("삼성전자")
        resolver.resolve("삼성전자")
        assert calls["n"] == 1
