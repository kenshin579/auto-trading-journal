"""시트 읽기 (read_all_trades) 단위 테스트

Google Sheets API를 직접 호출하지 않고
get_raw_grid_data() / get_sheet_data() 반환값을 모킹하여 테스트.
"""

from unittest.mock import AsyncMock

import pytest

from modules.google_sheets_client import GoogleSheetsClient
from modules.sheet_writer import (
    DOMESTIC_HEADERS,
    FOREIGN_HEADERS,
    SheetWriter,
    _extract_header_row,
    _get_num,
    _get_str,
    _row_to_trade,
)


# ── 헬퍼: 모킹용 셀/행 데이터 빌더 ──────────────────────────


def _cell(value):
    """값으로부터 Google Sheets effectiveValue + formattedValue 셀 생성."""
    cell = {"effectiveValue": {}}
    if isinstance(value, (int, float)):
        cell["effectiveValue"]["numberValue"] = value
        cell["formattedValue"] = str(value)
    elif isinstance(value, str):
        cell["effectiveValue"]["stringValue"] = value
        cell["formattedValue"] = value
    return cell


def _build_grid_data(rows):
    """행 리스트 → get_raw_grid_data() 반환 형식."""
    row_data = []
    for row in rows:
        row_data.append({"values": [_cell(v) for v in row]})
    return {"sheets": [{"data": [{"rowData": row_data}]}]}


def _build_header_grid_data(headers):
    """헤더 리스트 → get_sheet_data() 반환 형식."""
    return _build_grid_data([headers])


# ── Fixtures ──────────────────────────────────────────────────


@pytest.fixture
def mock_client():
    client = AsyncMock(spec=GoogleSheetsClient)
    return client


@pytest.fixture
def writer(mock_client):
    return SheetWriter(mock_client)


# ── 국내 샘플 데이터 ─────────────────────────────────────────

DOMESTIC_ROW = [
    "2026-02-13",  # A: 일자
    "매수",         # B: 구분
    "TIGER 조선TOP10",  # C: 종목명
    2.0,            # D: 수량
    28230.0,        # E: 단가
    56460.0,        # F: 금액
    0.0,            # G: 수수료
    0.0,            # H: 손익금액
    0.0,            # I: 수익률(%) — 시트에 0.00% 포맷으로 저장된 소수값
]

DOMESTIC_SELL_ROW = [
    "2026-02-13",
    "매도",
    "KODEX 미국배당다우존스",
    9.0,
    12452.0,
    112068.0,
    2794.0,
    16128.0,
    0.1681,  # 16.81%
]

# ── 해외 샘플 데이터 ─────────────────────────────────────────

FOREIGN_ROW = [
    "2026-01-02",   # A: 일자
    "매도",          # B: 구분
    "USD",           # C: 통화
    "AIG",           # D: 종목코드
    "아메리칸 인터내셔널 그룹",  # E: 종목명
    1.0,             # F: 수량
    84.81,           # G: 단가
    84.81,           # H: 금액(외화)
    1434.90,         # I: 환율
    121693.0,        # J: 금액(원화)
    0.21,            # K: 수수료
    0.0,             # L: 세금
    6.4512,          # M: 손익(외화)
    9165.0,          # N: 손익(원화)
    0.0825,          # O: 수익률 (8.25%)
]


# ── 헬퍼 함수 테스트 ─────────────────────────────────────────


class TestGetNum:
    def test_number_value(self):
        cell = {"effectiveValue": {"numberValue": 42.5}}
        assert _get_num(cell) == 42.5

    def test_missing_value(self):
        cell = {"effectiveValue": {}}
        assert _get_num(cell) == 0.0

    def test_default_value(self):
        cell = {"effectiveValue": {}}
        assert _get_num(cell, default=-1.0) == -1.0

    def test_empty_cell(self):
        assert _get_num({}) == 0.0


class TestGetStr:
    def test_string_value(self):
        cell = {"effectiveValue": {"stringValue": "매수"}}
        assert _get_str(cell) == "매수"

    def test_missing_value(self):
        cell = {"effectiveValue": {}}
        assert _get_str(cell) == ""

    def test_default_value(self):
        cell = {"effectiveValue": {}}
        assert _get_str(cell, default="N/A") == "N/A"


class TestExtractHeaderRow:
    def test_domestic_headers(self):
        data = _build_header_grid_data(DOMESTIC_HEADERS)
        assert _extract_header_row(data) == DOMESTIC_HEADERS

    def test_foreign_headers(self):
        data = _build_header_grid_data(FOREIGN_HEADERS)
        assert _extract_header_row(data) == FOREIGN_HEADERS

    def test_empty_data(self):
        assert _extract_header_row({}) == []
        assert _extract_header_row(None) == []

    def test_no_sheets(self):
        assert _extract_header_row({"sheets": []}) == []


# ── _row_to_trade 테스트 ─────────────────────────────────────


class TestRowToTrade:
    def test_domestic_row_to_trade(self):
        values = [_cell(v) for v in DOMESTIC_ROW]
        trade = _row_to_trade(values, "미래에셋증권_주식1", False, "2026-02-13")

        assert trade is not None
        assert trade.date == "2026-02-13"
        assert trade.trade_type == "매수"
        assert trade.stock_name == "TIGER 조선TOP10"
        assert trade.stock_code == ""
        assert trade.quantity == 2.0
        assert trade.price == 28230.0
        assert trade.amount == 56460.0
        assert trade.currency == "KRW"
        assert trade.exchange_rate == 1.0
        assert trade.amount_krw == 56460.0
        assert trade.fee == 0.0
        assert trade.tax == 0.0
        assert trade.profit == 0.0
        assert trade.profit_krw == 0.0
        assert trade.profit_rate == 0.0  # 0.0 * 100 = 0.0
        assert trade.account == "미래에셋증권_주식1"

    def test_foreign_row_to_trade(self):
        values = [_cell(v) for v in FOREIGN_ROW]
        trade = _row_to_trade(values, "미래에셋증권_해외주식1", True, "2026-01-02")

        assert trade is not None
        assert trade.date == "2026-01-02"
        assert trade.trade_type == "매도"
        assert trade.stock_name == "아메리칸 인터내셔널 그룹"
        assert trade.stock_code == "AIG"
        assert trade.quantity == 1.0
        assert trade.price == 84.81
        assert trade.amount == 84.81
        assert trade.currency == "USD"
        assert trade.exchange_rate == 1434.90
        assert trade.amount_krw == 121693.0
        assert trade.fee == 0.21
        assert trade.tax == 0.0
        assert trade.profit == 6.4512
        assert trade.profit_krw == 9165.0
        assert trade.account == "미래에셋증권_해외주식1"

    def test_profit_rate_conversion(self):
        """수익률 역변환: 시트의 0.1681 → Trade의 16.81"""
        values = [_cell(v) for v in DOMESTIC_SELL_ROW]
        trade = _row_to_trade(values, "미래에셋증권_주식3", False, "2026-02-13")

        assert trade is not None
        assert trade.profit_rate == pytest.approx(16.81, abs=0.01)

    def test_foreign_profit_rate_conversion(self):
        """해외 수익률 역변환: 시트의 0.0825 → Trade의 8.25"""
        values = [_cell(v) for v in FOREIGN_ROW]
        trade = _row_to_trade(values, "미래에셋증권_해외주식1", True, "2026-01-02")

        assert trade is not None
        assert trade.profit_rate == pytest.approx(8.25, abs=0.01)


# ── _read_trades_from_sheet 테스트 ───────────────────────────


class TestReadTradesFromSheet:
    async def test_skip_empty_row(self, writer, mock_client):
        """빈 행이 포함된 데이터에서 유효한 행만 파싱."""
        grid_data = _build_grid_data([
            DOMESTIC_ROW,
            ["", "", "", 0, 0, 0, 0, 0, 0],  # 빈 날짜 → 스킵
            DOMESTIC_SELL_ROW,
        ])
        mock_client.get_raw_grid_data.return_value = grid_data

        trades = await writer._read_trades_from_sheet("test_sheet", False)
        assert len(trades) == 2

    async def test_skip_incomplete_row(self, writer, mock_client):
        """컬럼 부족 행 스킵."""
        grid_data = _build_grid_data([
            DOMESTIC_ROW,
            ["2026-02-13", "매수", "종목"],  # 3컬럼만 → 스킵
        ])
        mock_client.get_raw_grid_data.return_value = grid_data

        trades = await writer._read_trades_from_sheet("test_sheet", False)
        assert len(trades) == 1

    async def test_empty_sheet(self, writer, mock_client):
        """빈 시트."""
        mock_client.get_raw_grid_data.return_value = {"sheets": [{"data": [{"rowData": []}]}]}

        trades = await writer._read_trades_from_sheet("empty", False)
        assert trades == []

    async def test_api_error(self, writer, mock_client):
        """API 에러 시 빈 리스트 반환."""
        mock_client.get_raw_grid_data.side_effect = Exception("API error")

        trades = await writer._read_trades_from_sheet("error_sheet", False)
        assert trades == []


# ── read_all_trades 헤더 검증 테스트 ─────────────────────────


class TestReadAllTrades:
    async def test_header_validation_domestic(self, writer, mock_client):
        """DOMESTIC_HEADERS 일치 시 국내로 인식."""
        mock_client.list_sheets.return_value = ["미래에셋증권_주식1"]
        mock_client.get_sheet_data.return_value = _build_header_grid_data(DOMESTIC_HEADERS)
        mock_client.get_raw_grid_data.return_value = _build_grid_data([DOMESTIC_ROW])

        trades = await writer.read_all_trades()
        assert len(trades) == 1
        assert trades[0].currency == "KRW"

    async def test_header_validation_foreign(self, writer, mock_client):
        """FOREIGN_HEADERS 일치 시 해외로 인식."""
        mock_client.list_sheets.return_value = ["미래에셋증권_해외주식1"]
        mock_client.get_sheet_data.return_value = _build_header_grid_data(FOREIGN_HEADERS)
        mock_client.get_raw_grid_data.return_value = _build_grid_data([FOREIGN_ROW])

        trades = await writer.read_all_trades()
        assert len(trades) == 1
        assert trades[0].currency == "USD"

    async def test_header_validation_skip(self, writer, mock_client):
        """헤더 불일치 시 스킵 (대시보드 등)."""
        mock_client.list_sheets.return_value = ["대시보드", "미래에셋증권_주식1"]
        mock_client.get_sheet_data.side_effect = [
            _build_header_grid_data(["지표", "총 매수금액(원)", "총 매도금액(원)"]),
            _build_header_grid_data(DOMESTIC_HEADERS),
        ]
        mock_client.get_raw_grid_data.return_value = _build_grid_data([DOMESTIC_ROW])

        trades = await writer.read_all_trades()
        assert len(trades) == 1  # 대시보드 스킵, 주식1만 읽음

    async def test_multiple_sheets(self, writer, mock_client):
        """여러 시트에서 데이터 합산."""
        mock_client.list_sheets.return_value = [
            "미래에셋증권_주식1",
            "미래에셋증권_해외주식1",
            "대시보드",
        ]
        mock_client.get_sheet_data.side_effect = [
            _build_header_grid_data(DOMESTIC_HEADERS),
            _build_header_grid_data(FOREIGN_HEADERS),
            _build_header_grid_data(["지표", "값"]),
        ]
        mock_client.get_raw_grid_data.side_effect = [
            _build_grid_data([DOMESTIC_ROW, DOMESTIC_SELL_ROW]),
            _build_grid_data([FOREIGN_ROW]),
        ]

        trades = await writer.read_all_trades()
        assert len(trades) == 3  # 국내 2건 + 해외 1건
