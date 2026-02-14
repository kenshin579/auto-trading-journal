"""Google Sheets 시트 쓰기 모듈"""

import logging
from collections import defaultdict
from typing import Any, Dict, List, Optional, Set

from .google_sheets_client import GoogleSheetsClient
from .models import Trade

logger = logging.getLogger(__name__)

# 시트 헤더 상수
DOMESTIC_HEADERS = [
    "일자", "구분", "종목명", "수량", "단가", "금액",
    "수수료", "손익금액", "수익률(%)",
]

FOREIGN_HEADERS = [
    "일자", "구분", "통화", "종목코드", "종목명", "수량", "단가",
    "금액(외화)", "환율", "금액(원화)", "수수료", "세금",
    "손익(외화)", "손익(원화)", "수익률(%)",
]

# 8색 팔레트 (날짜별 색상 구분)
COLOR_PALETTE = [
    {"red": 1.0, "green": 0.9, "blue": 0.9},
    {"red": 0.9, "green": 1.0, "blue": 0.9},
    {"red": 0.9, "green": 0.9, "blue": 1.0},
    {"red": 1.0, "green": 1.0, "blue": 0.9},
    {"red": 1.0, "green": 0.9, "blue": 1.0},
    {"red": 0.9, "green": 1.0, "blue": 1.0},
    {"red": 0.95, "green": 0.95, "blue": 0.85},
    {"red": 0.85, "green": 0.95, "blue": 0.95},
]

# 국내계좌 컬럼별 숫자 포맷 (1-based index)
DOMESTIC_FORMATS = [
    {'col': 4, 'pattern': '#,##0'},       # D: 수량
    {'col': 5, 'pattern': '#,##0'},       # E: 단가
    {'col': 6, 'pattern': '#,##0'},       # F: 금액
    {'col': 7, 'pattern': '#,##0'},       # G: 수수료
    {'col': 8, 'pattern': '#,##0'},       # H: 손익금액
    {'col': 9, 'pattern': '0.00%', 'type': 'PERCENT'},  # I: 수익률
]

# 해외계좌 컬럼별 숫자 포맷 (1-based index)
FOREIGN_FORMATS = [
    {'col': 6, 'pattern': '#,##0'},        # F: 수량
    {'col': 7, 'pattern': '#,##0.00'},     # G: 단가
    {'col': 8, 'pattern': '#,##0.00'},     # H: 금액(외화)
    {'col': 9, 'pattern': '#,##0.00'},     # I: 환율
    {'col': 10, 'pattern': '#,##0'},       # J: 금액(원화)
    {'col': 11, 'pattern': '#,##0.00'},    # K: 수수료
    {'col': 12, 'pattern': '#,##0.00'},    # L: 세금
    {'col': 13, 'pattern': '#,##0.00'},    # M: 손익(외화)
    {'col': 14, 'pattern': '#,##0'},       # N: 손익(원화)
    {'col': 15, 'pattern': '0.00%', 'type': 'PERCENT'},  # O: 수익률
]


class SheetWriter:
    """Google Sheets 시트 생성/삽입/색상 적용"""

    def __init__(self, client: GoogleSheetsClient):
        self.client = client
        self._sheet_cache: Optional[List[str]] = None

    async def _get_sheets(self) -> List[str]:
        """시트 목록 (캐시)"""
        if self._sheet_cache is None:
            self._sheet_cache = await self.client.list_sheets()
        return self._sheet_cache

    def _invalidate_cache(self):
        self._sheet_cache = None

    async def ensure_sheet_exists(self, sheet_name: str, is_foreign: bool = False) -> bool:
        """시트가 없으면 생성하고 헤더를 삽입. freeze + filter 적용.

        Args:
            sheet_name: 시트 이름
            is_foreign: 해외계좌 여부

        Returns:
            새로 생성했으면 True, 이미 존재하면 False
        """
        sheets = await self._get_sheets()
        if sheet_name in sheets:
            await self.apply_sheet_formatting(sheet_name, is_foreign)
            return False

        headers = FOREIGN_HEADERS if is_foreign else DOMESTIC_HEADERS
        await self.client.create_sheet(sheet_name)
        await self.client.update_cells(sheet_name, "A1", [headers])
        self._invalidate_cache()
        logger.info(f"시트 '{sheet_name}' 생성 및 헤더 삽입 완료")

        await self.apply_sheet_formatting(sheet_name, is_foreign)
        return True

    async def apply_sheet_formatting(self, sheet_name: str, is_foreign: bool = False):
        """시트에 freeze + filter 적용"""
        num_cols = len(FOREIGN_HEADERS) if is_foreign else len(DOMESTIC_HEADERS)
        await self.client.freeze_rows(sheet_name, 1)
        await self.client.set_auto_filter(sheet_name, 1, 1, num_cols)

    async def get_existing_keys(self, sheet_name: str, is_foreign: bool = False) -> Set[tuple]:
        """기존 데이터에서 중복 체크용 키 셋 반환

        키: (일자, 구분, 종목명, 수량, 단가)
        spreadsheets.get()으로 effectiveValue + formattedValue를 함께 조회하여
        날짜는 formattedValue(시리얼 넘버 방지), 숫자는 effectiveValue(원본 정밀도)를 사용.
        """
        try:
            data = await self.client.get_raw_grid_data(sheet_name, "A2:O10000")
            if not data or "sheets" not in data:
                return set()

            rows = data["sheets"][0]["data"][0].get("rowData", [])
            keys = set()
            for row in rows:
                values = row.get("values", [])
                if len(values) < 5:
                    continue

                # 날짜(col 0): formattedValue 사용 (시리얼 넘버 → "YYYY-MM-DD")
                date_cell = values[0]
                date_val = date_cell.get("formattedValue", "")
                if not date_val:
                    continue

                # 나머지 컬럼: effectiveValue 사용 (원본 정밀도)
                def _get_cell_value(cell):
                    ev = cell.get("effectiveValue", {})
                    return ev.get("stringValue") or ev.get("numberValue")

                # 국내: (일자, 구분, 종목명, 수량, 단가) = cols 0,1,2,3,4
                # 해외: (일자, 구분, 종목명, 수량, 단가) = cols 0,1,4,5,6
                trade_type = str(_get_cell_value(values[1]) or "")
                if is_foreign and len(values) >= 7:
                    stock_name = str(_get_cell_value(values[4]) or "")
                    key = (date_val, trade_type, stock_name,
                           _normalize_num(_get_cell_value(values[5])),
                           _normalize_num(_get_cell_value(values[6])))
                else:
                    stock_name = str(_get_cell_value(values[2]) or "")
                    key = (date_val, trade_type, stock_name,
                           _normalize_num(_get_cell_value(values[3])),
                           _normalize_num(_get_cell_value(values[4])))
                keys.add(key)

            logger.info(f"시트 '{sheet_name}'에서 {len(keys)}개 기존 키 로드")
            return keys

        except Exception as e:
            logger.error(f"기존 키 로드 실패 ({sheet_name}): {e}")
            return set()

    async def find_last_row(self, sheet_name: str) -> int:
        """시트의 마지막 데이터 행 + 1 (다음 삽입 위치) 반환"""
        try:
            metadata = await self.client.get_spreadsheet_metadata()
            row_count = 10000
            if metadata and "sheets" in metadata:
                for sheet in metadata["sheets"]:
                    if sheet["properties"]["title"] == sheet_name:
                        row_count = sheet["properties"]["gridProperties"]["rowCount"]
                        break

            data = await self.client.get_sheet_data(sheet_name, f"A1:A{row_count}")
            if not data or "sheets" not in data:
                return 2

            rows = data["sheets"][0]["data"][0].get("rowData", [])
            last_row = 1
            empty_count = 0
            for i, row in enumerate(rows):
                values = row.get("values", [])
                is_empty = True
                for cell in values:
                    ev = cell.get("effectiveValue", {})
                    if any(k in ev for k in ("numberValue", "stringValue", "boolValue")):
                        is_empty = False
                        break
                if is_empty:
                    empty_count += 1
                    if empty_count >= 100:
                        break
                else:
                    last_row = i + 1
                    empty_count = 0

            return last_row + 1

        except Exception as e:
            logger.error(f"마지막 행 탐색 실패 ({sheet_name}): {e}")
            return 2

    async def insert_trades(self, sheet_name: str, trades: List[Trade],
                            is_foreign: bool = False) -> int:
        """거래 데이터를 시트에 삽입하고 색상/숫자 포맷을 적용

        Args:
            sheet_name: 시트 이름
            trades: 삽입할 Trade 리스트
            is_foreign: 해외계좌 여부

        Returns:
            삽입된 거래 수
        """
        if not trades:
            return 0

        start_row = await self.find_last_row(sheet_name)
        num_cols = len(FOREIGN_HEADERS) if is_foreign else len(DOMESTIC_HEADERS)

        # 데이터 행 준비
        rows_data = []
        for trade in trades:
            row = trade.to_foreign_row() if is_foreign else trade.to_domestic_row()
            # 컬럼 수 맞춤
            if len(row) > num_cols:
                row = row[:num_cols]
            elif len(row) < num_cols:
                row.extend([""] * (num_cols - len(row)))
            rows_data.append(row)

        # 범위 계산
        end_row = start_row + len(trades) - 1
        end_col_letter = _col_letter(num_cols)
        range_str = f"A{start_row}:{end_col_letter}{end_row}"

        # 데이터 삽입
        success = await self.client.batch_update_cells(
            sheet_name, {range_str: rows_data}
        )

        if not success:
            logger.error(f"시트 '{sheet_name}' 데이터 삽입 실패")
            return 0

        logger.info(f"시트 '{sheet_name}'에 {len(trades)}건 삽입 (행 {start_row}-{end_row})")

        # 숫자 포맷 적용
        await self._apply_number_formats(sheet_name, start_row, end_row, is_foreign)

        # 날짜별 색상 적용
        color_ranges = self._build_date_color_ranges(trades, start_row, 1, num_cols)
        if color_ranges:
            await self.client.batch_apply_colors(sheet_name, color_ranges)
            logger.info(f"날짜 색상 적용: {len(color_ranges)}개 범위")

        return len(trades)

    async def _apply_number_formats(self, sheet_name: str, start_row: int,
                                    end_row: int, is_foreign: bool):
        """컬럼별 숫자 포맷 적용"""
        formats = FOREIGN_FORMATS if is_foreign else DOMESTIC_FORMATS
        await self.client.apply_number_format_to_columns(
            sheet_name, formats, start_row, end_row
        )

    def _build_date_color_ranges(self, trades: List[Trade],
                                 start_row: int, start_col: int,
                                 end_col: int) -> List[Dict[str, Any]]:
        """날짜별 색상 범위 생성"""
        if not trades:
            return []

        date_groups: Dict[str, List[int]] = defaultdict(list)
        for i, trade in enumerate(trades):
            date_groups[trade.date].append(i)

        color_ranges = []
        for date_idx, (date, indices) in enumerate(date_groups.items()):
            color = COLOR_PALETTE[date_idx % len(COLOR_PALETTE)]
            indices.sort()

            i = 0
            while i < len(indices):
                s = indices[i]
                e = s
                while i + 1 < len(indices) and indices[i + 1] == indices[i] + 1:
                    i += 1
                    e = indices[i]
                color_ranges.append({
                    "start_row": start_row + s,
                    "end_row": start_row + e,
                    "start_col": start_col,
                    "end_col": end_col,
                    "color": color,
                })
                i += 1

        return color_ranges


def _normalize_num(v) -> str:
    """셀 값을 duplicate_key()의 _num_str() 형식과 일치하도록 정규화.

    - 숫자(int/float): 정수면 소수점 제거 (2.0 → "2", 28230.0 → "28230")
    - 문자열: 천단위 쉼표 제거 ("28,230" → "28230")
    """
    if v is None:
        return ""
    if isinstance(v, (int, float)):
        return str(int(v)) if v == int(v) else str(v)
    return str(v).replace(',', '')


def _col_letter(col_num: int) -> str:
    """컬럼 번호 → 문자 (1=A, 26=Z, 27=AA)"""
    result = ""
    while col_num > 0:
        col_num -= 1
        result = chr(65 + col_num % 26) + result
        col_num //= 26
    return result
