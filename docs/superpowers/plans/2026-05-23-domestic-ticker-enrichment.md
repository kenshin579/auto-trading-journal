# 국내 종목 티커(종목코드) 추가 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 국내 매매일지 시트에 종목코드(티커) 별도 컬럼을 추가하고, 종목코드가 비어있는 국내 거래를 KRX 공개 종목 마스터에서 조회해 채운다.

**Architecture:** KRX 공개 마스터 파일(`kospi/kosdaq_code.mst.zip`)을 다운로드·캐시·파싱하는 신규 `modules/symbol_master.py`(이름→코드 dict)를 만들고, `StockDataProcessor`가 파싱 직후 빈 종목코드의 국내 거래만 보강한다. 국내 시트 레이아웃을 9→10컬럼으로 바꾸고 관련 인덱스를 시프트한다.

**Tech Stack:** Python 3.12, 표준 라이브러리만 사용(`urllib.request`, `zipfile`, `codecs` cp949 — 신규 의존성 없음), pytest.

**참조 스펙:** `docs/superpowers/specs/2026-05-23-domestic-ticker-enrichment-design.md`

---

## File Structure

- **Create** `modules/symbol_master.py` — KRX 마스터 다운로드/캐시/파싱 + `SymbolResolver` (이름→코드)
- **Create** `tests/test_symbol_master.py` — 마스터 파싱·캐시·리졸브 단위 테스트
- **Modify** `modules/models.py` — `to_domestic_row()`에 종목코드 삽입
- **Modify** `modules/sheet_writer.py` — `DOMESTIC_HEADERS`, `DOMESTIC_FORMATS`, `get_existing_keys()`, `_read_trades_from_sheet()`, `_row_to_trade()`, `read_all_trades()` 옛 헤더 경고
- **Modify** `main.py` — `StockDataProcessor`에 enrichment 단계 추가
- **Modify** `tests/test_parsers.py` 외 시트 관련 테스트 — 10컬럼 기준 갱신
- **Modify** `CLAUDE.md` — 컬럼 구조·마이그레이션 안내

---

## Task 1: KRX 마스터 라인 파서 (순수 함수, 네트워크 없음)

**Files:**
- Create: `modules/symbol_master.py`
- Test: `tests/test_symbol_master.py`

- [ ] **Step 1: 실패하는 테스트 작성**

`tests/test_symbol_master.py`:

```python
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
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `pytest tests/test_symbol_master.py -v`
Expected: FAIL — `ModuleNotFoundError: No module named 'modules.symbol_master'`

- [ ] **Step 3: 최소 구현**

`modules/symbol_master.py`:

```python
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
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `pytest tests/test_symbol_master.py -v`
Expected: PASS (3 passed)

- [ ] **Step 5: 커밋**

```bash
git add modules/symbol_master.py tests/test_symbol_master.py
git commit -m "feat: KRX 마스터 라인 파서 (이름→코드 추출)"
```

---

## Task 2: 다운로드 + 디스크 캐시 + ZIP/cp949 디코드

**Files:**
- Modify: `modules/symbol_master.py`
- Test: `tests/test_symbol_master.py`

- [ ] **Step 1: 실패하는 테스트 작성** (`tests/test_symbol_master.py` 상단 import에 추가)

기존 import 블록을 다음으로 교체:

```python
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
```

파일 끝에 테스트 추가:

```python
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
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `pytest tests/test_symbol_master.py -v`
Expected: FAIL — `ImportError: cannot import name '_extract_mst_text'`

- [ ] **Step 3: 최소 구현** (`modules/symbol_master.py` 상단 import 및 함수 추가)

`import logging` 아래에 추가:

```python
import io
import os
import time
import urllib.request
import zipfile
from pathlib import Path
```

`_parse_mst_lines` 위(또는 아래)에 추가:

```python
KOSPI_URL = "https://new.real.download.dws.co.kr/common/master/kospi_code.mst.zip"
KOSDAQ_URL = "https://new.real.download.dws.co.kr/common/master/kosdaq_code.mst.zip"
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
        mst_name = next(n for n in z.namelist() if n.endswith(".mst"))
        raw = z.read(mst_name)
    return raw.decode("cp949")
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `pytest tests/test_symbol_master.py -v`
Expected: PASS (7 passed)

- [ ] **Step 5: 커밋**

```bash
git add modules/symbol_master.py tests/test_symbol_master.py
git commit -m "feat: KRX 마스터 다운로드/캐시/cp949 디코드"
```

---

## Task 3: SymbolResolver (이름→코드 조회)

**Files:**
- Modify: `modules/symbol_master.py`
- Test: `tests/test_symbol_master.py`

- [ ] **Step 1: 실패하는 테스트 작성**

`tests/test_symbol_master.py` import 에 `SymbolResolver` 추가:

```python
from modules.symbol_master import (
    _parse_mst_lines,
    _extract_mst_text,
    _fetch_zip,
    SymbolResolver,
    KOSPI_FWF_LEN,
    KOSDAQ_FWF_LEN,
)
```

파일 끝에 추가:

```python
class TestSymbolResolver:
    def test_resolve_returns_code_for_known_name(self):
        resolver = SymbolResolver(name_to_code={"TIGER 조선TOP10": "494670"})
        assert resolver.resolve("TIGER 조선TOP10") == "494670"

    def test_resolve_strips_whitespace(self):
        resolver = SymbolResolver(name_to_code={"삼성전자": "005930"})
        assert resolver.resolve("  삼성전자  ") == "005930"

    def test_resolve_returns_empty_for_unknown(self, caplog):
        resolver = SymbolResolver(name_to_code={})
        assert resolver.resolve("없는종목") == ""

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
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `pytest tests/test_symbol_master.py::TestSymbolResolver -v`
Expected: FAIL — `ImportError: cannot import name 'SymbolResolver'`

- [ ] **Step 3: 최소 구현** (`modules/symbol_master.py` 끝에 추가)

```python
def _load_all() -> dict:
    """KOSPI + KOSDAQ 마스터를 모두 로드해 {한글명: 단축코드} 통합 dict 반환."""
    merged: dict = {}
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

    def __init__(self, name_to_code: dict = None):
        self._map = name_to_code

    def _ensure_loaded(self):
        if self._map is None:
            self._map = _load_all()

    def resolve(self, stock_name: str) -> str:
        self._ensure_loaded()
        code = self._map.get(stock_name.strip(), "")
        if not code:
            logger.warning(f"종목코드 미발견 (KRX 마스터): {stock_name}")
        return code
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `pytest tests/test_symbol_master.py -v`
Expected: PASS (11 passed)

- [ ] **Step 5: 커밋**

```bash
git add modules/symbol_master.py tests/test_symbol_master.py
git commit -m "feat: SymbolResolver 종목명→코드 lazy 조회"
```

---

## Task 4: 국내 행/헤더에 종목코드 컬럼 추가

**Files:**
- Modify: `modules/models.py:33-42`
- Modify: `modules/sheet_writer.py:14-17` (DOMESTIC_HEADERS)
- Test: `tests/test_parsers.py` (또는 신규 모델 테스트)

- [ ] **Step 1: 실패하는 테스트 작성**

`tests/test_parsers.py` 끝에 추가:

```python
from modules.models import Trade


def _domestic_trade(**kw):
    base = dict(
        date="2026-02-13", trade_type="매수", stock_name="TIGER 조선TOP10",
        stock_code="494670", quantity=2, price=28230, amount=56460,
        currency="KRW", exchange_rate=1.0, amount_krw=56460, fee=0.0, tax=0.0,
        profit=0.0, profit_krw=0.0, profit_rate=0.0, account="미래에셋증권_국내계좌",
    )
    base.update(kw)
    return Trade(**base)


def test_to_domestic_row_includes_stock_code():
    row = _domestic_trade().to_domestic_row()
    assert len(row) == 10
    # 일자, 구분, 종목명, 종목코드, 수량, 단가, 금액, 수수료, 손익금액, 수익률
    assert row[2] == "TIGER 조선TOP10"
    assert row[3] == "494670"
    assert row[4] == 2
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `pytest tests/test_parsers.py::test_to_domestic_row_includes_stock_code -v`
Expected: FAIL — `assert 9 == 10` (현재 9컬럼, row[3]은 수량)

- [ ] **Step 3: 구현** — `modules/models.py` `to_domestic_row()` 교체

기존:

```python
    def to_domestic_row(self) -> list:
        """국내계좌 시트 행 변환 (9컬럼)
        수익률은 퍼센트 소수로 변환 (14.68 → 0.1468)
        """
        return [
            self.date, self.trade_type, self.stock_name,
            self.quantity, self.price, self.amount,
            self.fee, self.profit,
            self.profit_rate / 100 if self.profit_rate else 0,
        ]
```

교체 후:

```python
    def to_domestic_row(self) -> list:
        """국내계좌 시트 행 변환 (10컬럼)
        수익률은 퍼센트 소수로 변환 (14.68 → 0.1468)
        """
        return [
            self.date, self.trade_type, self.stock_name, self.stock_code,
            self.quantity, self.price, self.amount,
            self.fee, self.profit,
            self.profit_rate / 100 if self.profit_rate else 0,
        ]
```

`modules/sheet_writer.py` DOMESTIC_HEADERS 교체:

```python
DOMESTIC_HEADERS = [
    "일자", "구분", "종목명", "종목코드", "수량", "단가", "금액",
    "수수료", "손익금액", "수익률(%)",
]
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `pytest tests/test_parsers.py::test_to_domestic_row_includes_stock_code -v`
Expected: PASS

- [ ] **Step 5: 커밋**

```bash
git add modules/models.py modules/sheet_writer.py tests/test_parsers.py
git commit -m "feat: 국내 행/헤더에 종목코드 컬럼 추가 (9→10)"
```

---

## Task 5: sheet_writer 컬럼 인덱스 시프트 (포맷·중복키·read-back)

**Files:**
- Modify: `modules/sheet_writer.py:26-33` (DOMESTIC_FORMATS)
- Modify: `modules/sheet_writer.py` `get_existing_keys()` 국내 분기
- Modify: `modules/sheet_writer.py` `_read_trades_from_sheet()` min_cols
- Modify: `modules/sheet_writer.py` `_row_to_trade()` 국내 분기
- Test: `tests/test_sheet_reader.py`

- [ ] **Step 1: 실패하는 테스트 작성**

`tests/test_sheet_reader.py` 끝에 추가 (read-back이 10컬럼에서 종목코드를 복원하는지):

```python
from modules.sheet_writer import _row_to_trade


def _cell(value):
    if isinstance(value, (int, float)):
        return {"formattedValue": str(value), "effectiveValue": {"numberValue": value}}
    return {"formattedValue": value, "effectiveValue": {"stringValue": value}}


def test_row_to_trade_domestic_reads_stock_code():
    # 일자, 구분, 종목명, 종목코드, 수량, 단가, 금액, 수수료, 손익, 수익률
    values = [
        _cell("2026-02-13"), _cell("매수"), _cell("TIGER 조선TOP10"),
        _cell("494670"), _cell(2), _cell(28230), _cell(56460),
        _cell(0), _cell(0), _cell(0.0),
    ]
    trade = _row_to_trade(values, "미래에셋증권_국내계좌", is_foreign=False,
                          date_val="2026-02-13")
    assert trade is not None
    assert trade.stock_name == "TIGER 조선TOP10"
    assert trade.stock_code == "494670"
    assert trade.quantity == 2
    assert trade.price == 28230
```

> 참고: `_get_str`/`_get_num`은 `effectiveValue.{stringValue,numberValue}`를 읽으며, 위 `_cell` 헬퍼가 그 형식과 일치한다(확인 완료).

- [ ] **Step 2: 테스트 실패 확인**

Run: `pytest tests/test_sheet_reader.py::test_row_to_trade_domestic_reads_stock_code -v`
Expected: FAIL — `stock_code == ""` (현재 하드코딩) 또는 인덱스 불일치

- [ ] **Step 3: 구현**

`modules/sheet_writer.py` DOMESTIC_FORMATS 교체 (1-based, 종목코드(4) 추가로 +1 시프트):

```python
DOMESTIC_FORMATS = [
    {'col': 5, 'pattern': '#,##0'},              # E: 수량
    {'col': 6, 'pattern': '₩#,##0'},             # F: 단가
    {'col': 7, 'pattern': '₩#,##0'},             # G: 금액
    {'col': 8, 'pattern': '₩#,##0'},             # H: 수수료
    {'col': 9, 'pattern': '₩#,##0'},             # I: 손익금액
    {'col': 10, 'pattern': '0.00%', 'type': 'PERCENT'},  # J: 수익률
]
```

`get_existing_keys()` 국내 분기 (else 절) 교체 — 수량/단가 인덱스 3,4 → 4,5:

기존:

```python
                else:
                    stock_name = str(_get_cell_value(values[2]) or "")
                    key = (date_val, trade_type, stock_name,
                           _normalize_num(_get_cell_value(values[3])),
                           _normalize_num(_get_cell_value(values[4])))
```

교체 후:

```python
                else:
                    # 국내(10컬럼): 종목명=2, 종목코드=3, 수량=4, 단가=5
                    stock_name = str(_get_cell_value(values[2]) or "")
                    key = (date_val, trade_type, stock_name,
                           _normalize_num(_get_cell_value(values[4])),
                           _normalize_num(_get_cell_value(values[5])))
```

`_read_trades_from_sheet()` min_cols 교체:

기존: `min_cols = 15 if is_foreign else 9`
교체 후: `min_cols = 15 if is_foreign else 10`

`_row_to_trade()` 국내 분기(else) 교체 — 인덱스 시프트 + 종목코드 복원:

기존:

```python
        else:
            # 국내: A~I (9컬럼)
            amount = _get_num(values[5])
            profit = _get_num(values[7])
            return Trade(
                date=date_val,
                trade_type=_get_str(values[1]),
                stock_name=_get_str(values[2]),
                stock_code="",
                quantity=_get_num(values[3]),
                price=_get_num(values[4]),
                amount=amount,
                currency="KRW",
                exchange_rate=1.0,
                amount_krw=amount,
                fee=_get_num(values[6]),
                tax=0.0,
                profit=profit,
                profit_krw=profit,
                profit_rate=_get_num(values[8]) * 100,
                account=sheet_name,
            )
```

교체 후:

```python
        else:
            # 국내: A~J (10컬럼) — 일자,구분,종목명,종목코드,수량,단가,금액,수수료,손익,수익률
            amount = _get_num(values[6])
            profit = _get_num(values[8])
            return Trade(
                date=date_val,
                trade_type=_get_str(values[1]),
                stock_name=_get_str(values[2]),
                stock_code=_get_str(values[3]),
                quantity=_get_num(values[4]),
                price=_get_num(values[5]),
                amount=amount,
                currency="KRW",
                exchange_rate=1.0,
                amount_krw=amount,
                fee=_get_num(values[7]),
                tax=0.0,
                profit=profit,
                profit_krw=profit,
                profit_rate=_get_num(values[9]) * 100,
                account=sheet_name,
            )
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `pytest tests/test_sheet_reader.py -v`
Expected: PASS

- [ ] **Step 5: 커밋**

```bash
git add modules/sheet_writer.py tests/test_sheet_reader.py
git commit -m "feat: 국내 시트 컬럼 인덱스 10컬럼으로 시프트 (포맷/중복키/read-back)"
```

---

## Task 6: 옛 9컬럼 헤더 감지 시 경고 (마이그레이션 안내)

**Files:**
- Modify: `modules/sheet_writer.py` 헤더 상수부 + `read_all_trades()` 헤더 분기
- Test: 수동 검증 (로그)

- [ ] **Step 1: OLD 헤더 상수 추가** — `modules/sheet_writer.py` `DOMESTIC_HEADERS` 정의 아래에 추가

```python
# 마이그레이션 감지용: 종목코드 컬럼 추가 이전(9컬럼) 헤더
OLD_DOMESTIC_HEADERS_V1 = [
    "일자", "구분", "종목명", "수량", "단가", "금액",
    "수수료", "손익금액", "수익률(%)",
]
```

- [ ] **Step 2: `read_all_trades()` 헤더 분기에 경고 추가** — 기존 else 절 교체

기존:

```python
            if header_row == DOMESTIC_HEADERS:
                is_foreign = False
            elif header_row == FOREIGN_HEADERS:
                is_foreign = True
            else:
                logger.debug(f"시트 '{sheet_name}' 스킵 (매매일지 헤더 불일치)")
                continue
```

교체 후:

```python
            if header_row == DOMESTIC_HEADERS:
                is_foreign = False
            elif header_row == FOREIGN_HEADERS:
                is_foreign = True
            elif header_row == OLD_DOMESTIC_HEADERS_V1:
                logger.warning(
                    f"시트 '{sheet_name}'는 옛 9컬럼 포맷입니다. 종목코드 컬럼 추가를 위해 "
                    f"이 시트를 삭제 후 재실행하거나 D열에 '종목코드' 컬럼을 수동 삽입하세요. "
                    f"(이번 실행에서는 스킵)"
                )
                continue
            else:
                logger.debug(f"시트 '{sheet_name}' 스킵 (매매일지 헤더 불일치)")
                continue
```

- [ ] **Step 3: 전체 테스트 회귀 확인**

Run: `pytest -v`
Expected: PASS (Task 4/5에서 갱신한 테스트 포함 전부 통과)

- [ ] **Step 4: 커밋**

```bash
git add modules/sheet_writer.py
git commit -m "feat: 옛 9컬럼 국내 시트 감지 시 마이그레이션 경고"
```

---

## Task 7: 파이프라인 enrichment 훅 (빈 코드 국내 거래 보강)

**Files:**
- Modify: `main.py` (`StockDataProcessor.__init__`, `process_file`)
- Test: `tests/test_enrichment.py` (신규)

- [ ] **Step 1: 실패하는 테스트 작성**

`tests/test_enrichment.py`:

```python
"""국내 종목코드 enrichment 단위 테스트"""

from modules.models import Trade
from modules.symbol_master import SymbolResolver
from main import enrich_domestic_codes


def _trade(stock_name, stock_code, account="미래에셋증권_국내계좌"):
    return Trade(
        date="2026-02-13", trade_type="매수", stock_name=stock_name,
        stock_code=stock_code, quantity=1, price=100, amount=100,
        currency="KRW", exchange_rate=1.0, amount_krw=100, fee=0.0, tax=0.0,
        profit=0.0, profit_krw=0.0, profit_rate=0.0, account=account,
    )


def test_fills_empty_domestic_code():
    resolver = SymbolResolver(name_to_code={"TIGER 조선TOP10": "494670"})
    trades = [_trade("TIGER 조선TOP10", "")]
    enrich_domestic_codes(trades, resolver)
    assert trades[0].stock_code == "494670"


def test_keeps_existing_code():
    resolver = SymbolResolver(name_to_code={"삼성전자": "999999"})
    trades = [_trade("삼성전자", "005930", account="한국투자증권_국내계좌")]
    enrich_domestic_codes(trades, resolver)
    assert trades[0].stock_code == "005930"  # CSV 코드 유지, 덮어쓰지 않음


def test_skips_foreign_trades():
    resolver = SymbolResolver(name_to_code={"애플": "AAPL-WRONG"})
    trades = [_trade("애플", "", account="미래에셋증권_해외계좌")]
    enrich_domestic_codes(trades, resolver)
    assert trades[0].stock_code == ""  # 해외는 건드리지 않음
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `pytest tests/test_enrichment.py -v`
Expected: FAIL — `ImportError: cannot import name 'enrich_domestic_codes' from 'main'`

- [ ] **Step 3: 구현**

`main.py` import 블록에 추가:

```python
from modules.symbol_master import SymbolResolver
```

모듈 레벨 함수 추가 (`StockDataProcessor` 클래스 정의 위):

```python
def enrich_domestic_codes(trades: List[Trade], resolver: SymbolResolver) -> None:
    """국내 거래 중 종목코드가 비어있는 항목을 KRX 마스터로 보강 (in-place).

    해외 거래와 이미 코드가 있는 거래(예: 한국투자 국내)는 건드리지 않는다.
    """
    for t in trades:
        if t.is_domestic() and not t.stock_code:
            code = resolver.resolve(t.stock_name)
            if code:
                t.stock_code = code
```

`StockDataProcessor.__init__` 끝(`self.summary_generator = ...` 다음)에 추가:

```python
        # KRX 종목 마스터 리졸버 (최초 resolve 시 lazy 로드)
        self.symbol_resolver = SymbolResolver()
```

`process_file()`에서 파싱 직후·중복 필터 전에 보강. 기존:

```python
        trades = parser.parse(file_path, account)
        if not trades:
```

교체 후:

```python
        trades = parser.parse(file_path, account)
        enrich_domestic_codes(trades, self.symbol_resolver)
        if not trades:
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `pytest tests/test_enrichment.py -v`
Expected: PASS (3 passed)

- [ ] **Step 5: 커밋**

```bash
git add main.py tests/test_enrichment.py
git commit -m "feat: 국내 빈 종목코드 KRX 마스터로 enrichment"
```

---

## Task 8: 통합 검증 + 문서 갱신

**Files:**
- Modify: `CLAUDE.md`
- Test: 전체 회귀 + 드라이런

- [ ] **Step 1: 전체 테스트 회귀**

Run: `pytest -v`
Expected: 전부 PASS. 실패 시 해당 테스트의 컬럼 인덱스/컬럼 수 가정을 10컬럼 기준으로 수정.

- [ ] **Step 2: 실제 마스터 매칭 스모크 테스트 (네트워크)**

샘플 종목명이 KRX 마스터에서 실제로 매칭되는지 확인:

```bash
python -c "
from modules.symbol_master import SymbolResolver
r = SymbolResolver()
for n in ['TIGER 조선TOP10', 'KODEX AI전력핵심설비', 'ACE KRX금현물', '삼성전자']:
    print(n, '->', repr(r.resolve(n)))
"
```

Expected: 각 종목명에 대해 6자리 코드 출력 (빈 문자열이면 종목명 표기 불일치 → 로그 확인). 일부 미발견은 경고 로그로 남고 빈 코드로 둔다(설계상 허용).

- [ ] **Step 3: 드라이런으로 파이프라인 확인**

Run: `python main.py --dry-run --log-level INFO`
Expected: 에러 없이 완료. "종목코드 미발견" 경고가 있으면 해당 종목명만 확인.

- [ ] **Step 4: CLAUDE.md 갱신**

`### Key Data Model` 의 `to_domestic_row()` 설명을 `국내 10컬럼 행 반환`으로 수정하고, `## Input File Format` 또는 새 절에 마이그레이션 안내 추가:

```markdown
### 국내 시트 컬럼 (종목코드 추가)

국내계좌 시트는 종목명 뒤에 `종목코드` 컬럼을 포함한 10컬럼 구조입니다.
종목코드는 미래에셋 국내처럼 CSV에 코드가 없는 경우 KRX 공개 종목 마스터
(`modules/symbol_master.py`)에서 종목명으로 조회해 채웁니다.

**기존 9컬럼 시트 마이그레이션**: 종목코드 컬럼 도입 이전에 생성된 국내 시트는
삭제 후 재실행하거나 D열에 `종목코드` 컬럼을 수동 삽입해야 합니다. 옛 포맷 시트는
실행 시 경고 로그와 함께 스킵됩니다.
```

- [ ] **Step 5: 인코딩 확인 후 커밋**

```bash
file -I CLAUDE.md
git add CLAUDE.md
git commit -m "docs: 국내 종목코드 컬럼/마이그레이션 안내"
```

---

## Self-Review 결과

- **스펙 커버리지**: 데이터 출처(Task 1-3), 캐시/TTL(Task 2), enrichment 훅·국내 한정·기존 코드 유지(Task 7), 컬럼 분리 B(Task 4-5), 요약 시트(자동 개선, Task 5 검증), 마이그레이션 B + 경고(Task 6, 8) 모두 매핑됨.
- **타입 일관성**: `SymbolResolver(name_to_code=...)`, `resolve()`, `enrich_domestic_codes(trades, resolver)`, `_parse_mst_lines/_fetch_zip/_extract_mst_text/_load_all` 시그니처가 정의·사용처에서 일치.
- **플레이스홀더**: 없음. 모든 코드 단계에 실제 코드 포함.
- **주의**: Task 5 Step 1의 `_cell` 헬퍼는 `_get_str`/`_get_num`의 실제 셀 키 읽기 방식에 맞춰 조정 필요(Step 2 실패 메시지로 확인). 이는 의도된 검증 지점.
