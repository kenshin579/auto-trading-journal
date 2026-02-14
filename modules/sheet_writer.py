"""Google Sheets 시트 쓰기/읽기 모듈"""

import logging
import unicodedata
from collections import defaultdict
from typing import Any, Dict, List, Optional, Set, Tuple

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

# 국내계좌 컬럼별 숫자 포맷 (1-based index)
DOMESTIC_FORMATS = [
    {'col': 4, 'pattern': '#,##0'},              # D: 수량
    {'col': 5, 'pattern': '₩#,##0'},             # E: 단가
    {'col': 6, 'pattern': '₩#,##0'},             # F: 금액
    {'col': 7, 'pattern': '₩#,##0'},             # G: 수수료
    {'col': 8, 'pattern': '₩#,##0'},             # H: 손익금액
    {'col': 9, 'pattern': '0.00%', 'type': 'PERCENT'},  # I: 수익률
]

# 해외계좌 - 통화 무관 컬럼 포맷 (1-based index)
FOREIGN_FORMATS_COMMON = [
    {'col': 6, 'pattern': '#,##0'},              # F: 수량
    {'col': 9, 'pattern': '#,##0.00'},           # I: 환율
    {'col': 10, 'pattern': '₩#,##0'},            # J: 금액(원화)
    {'col': 14, 'pattern': '₩#,##0'},            # N: 손익(원화)
    {'col': 15, 'pattern': '0.00%', 'type': 'PERCENT'},  # O: 수익률
]

# 해외계좌 - 통화별 외화 컬럼 (1-based index)
FOREIGN_CURRENCY_COLS = [7, 8, 11, 12, 13]  # G:단가, H:금액외화, K:수수료, L:세금, M:손익외화

# 통화별 포맷 패턴
CURRENCY_PATTERNS = {
    'USD': '$#,##0.00',
    'JPY': '¥#,##0',
    'EUR': '€#,##0.00',
    'GBP': '£#,##0.00',
    'CNY': '¥#,##0.00',
}
CURRENCY_PATTERN_DEFAULT = '#,##0.00'


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
            await self.client.clear_background_colors(sheet_name)
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
        await self._apply_number_formats(sheet_name, start_row, end_row, is_foreign, trades)

        return len(trades)

    async def _apply_number_formats(self, sheet_name: str, start_row: int,
                                    end_row: int, is_foreign: bool,
                                    trades: Optional[List[Trade]] = None):
        """컬럼별 숫자 포맷 적용"""
        if not is_foreign:
            await self.client.apply_number_format_to_columns(
                sheet_name, DOMESTIC_FORMATS, start_row, end_row
            )
            return

        # 해외: 통화 무관 컬럼 일괄 적용
        await self.client.apply_number_format_to_columns(
            sheet_name, FOREIGN_FORMATS_COMMON, start_row, end_row
        )

        # 해외: 통화별 외화 컬럼 행 단위 적용
        if not trades:
            return
        currency_rows: Dict[str, List[int]] = defaultdict(list)
        for i, trade in enumerate(trades):
            currency_rows[trade.currency].append(start_row + i)

        for currency, rows in currency_rows.items():
            pattern = CURRENCY_PATTERNS.get(currency, CURRENCY_PATTERN_DEFAULT)
            formats = [{'col': c, 'pattern': pattern} for c in FOREIGN_CURRENCY_COLS]
            # 연속 행 구간을 묶어서 API 호출 최소화
            for range_start, range_end in _group_consecutive_rows(rows):
                await self.client.apply_number_format_to_columns(
                    sheet_name, formats, range_start, range_end
                )

    # ── 시트 읽기 ──────────────────────────────────────────────

    async def read_all_trades(self) -> List[Trade]:
        """모든 매매일지 시트에서 Trade 리스트를 읽어 반환.

        헤더 행(1행)을 검증하여 매매일지 시트만 식별.
        DOMESTIC_HEADERS 일치 → 국내, FOREIGN_HEADERS 일치 → 해외.
        NFD/NFC 유니코드 중복 시트는 하나만 읽음.
        """
        sheets = await self.client.list_sheets()
        all_trades: List[Trade] = []
        seen_names: Set[str] = set()

        for sheet_name in sheets:
            # NFD/NFC 유니코드 중복 시트 방지
            normalized_name = unicodedata.normalize("NFC", sheet_name)
            if normalized_name in seen_names:
                logger.info(f"시트 '{sheet_name}' 스킵 (유니코드 중복: {normalized_name})")
                continue
            seen_names.add(normalized_name)

            header_data = await self.client.get_sheet_data(sheet_name, "A1:O1")
            header_row = _extract_header_row(header_data)
            if not header_row:
                continue

            if header_row == DOMESTIC_HEADERS:
                is_foreign = False
            elif header_row == FOREIGN_HEADERS:
                is_foreign = True
            else:
                logger.debug(f"시트 '{sheet_name}' 스킵 (매매일지 헤더 불일치)")
                continue

            trades = await self._read_trades_from_sheet(
                sheet_name, is_foreign, account_name=normalized_name
            )
            all_trades.extend(trades)
            logger.info(
                f"시트 '{normalized_name}'에서 {len(trades)}건 읽음 "
                f"({'해외' if is_foreign else '국내'})"
            )

        logger.info(f"전체 매매일지 시트에서 총 {len(all_trades)}건 읽음")
        return all_trades

    async def _read_trades_from_sheet(
        self, sheet_name: str, is_foreign: bool,
        account_name: Optional[str] = None,
    ) -> List[Trade]:
        """개별 매매일지 시트에서 Trade 리스트 반환.

        Args:
            sheet_name: API 호출에 사용할 원본 시트 이름
            is_foreign: 해외계좌 여부
            account_name: Trade.account에 설정할 이름 (None이면 sheet_name 사용)
        """
        account = account_name or sheet_name
        try:
            data = await self.client.get_raw_grid_data(sheet_name, "A2:O10000")
            if not data or "sheets" not in data:
                return []

            rows = data["sheets"][0]["data"][0].get("rowData", [])
            trades: List[Trade] = []
            min_cols = 15 if is_foreign else 9

            for row in rows:
                values = row.get("values", [])
                if len(values) < min_cols:
                    continue

                date_val = values[0].get("formattedValue", "")
                if not date_val:
                    continue

                trade = _row_to_trade(values, account, is_foreign, date_val)
                if trade:
                    trades.append(trade)

            return trades

        except Exception as e:
            logger.error(f"시트 데이터 읽기 실패 ({sheet_name}): {e}")
            return []


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


def _group_consecutive_rows(rows: List[int]) -> List[tuple]:
    """행 번호 리스트를 연속 구간으로 묶기. [(start, end), ...]"""
    if not rows:
        return []
    sorted_rows = sorted(rows)
    ranges = []
    start = end = sorted_rows[0]
    for r in sorted_rows[1:]:
        if r == end + 1:
            end = r
        else:
            ranges.append((start, end))
            start = end = r
    ranges.append((start, end))
    return ranges


def _col_letter(col_num: int) -> str:
    """컬럼 번호 → 문자 (1=A, 26=Z, 27=AA)"""
    result = ""
    while col_num > 0:
        col_num -= 1
        result = chr(65 + col_num % 26) + result
        col_num //= 26
    return result


# ── 시트 읽기 헬퍼 ──────────────────────────────────────────


def _get_num(cell: Dict, default: float = 0.0) -> float:
    """effectiveValue에서 숫자 추출."""
    ev = cell.get("effectiveValue", {})
    v = ev.get("numberValue")
    return float(v) if v is not None else default


def _get_str(cell: Dict, default: str = "") -> str:
    """effectiveValue에서 문자열 추출."""
    ev = cell.get("effectiveValue", {})
    v = ev.get("stringValue")
    return str(v) if v is not None else default


def _extract_header_row(data: Dict) -> List[str]:
    """get_sheet_data() 결과에서 1행 헤더를 문자열 리스트로 추출."""
    if not data or "sheets" not in data:
        return []
    sheets_data = data.get("sheets", [])
    if not sheets_data:
        return []
    row_data = sheets_data[0].get("data", [{}])[0].get("rowData", [])
    if not row_data:
        return []
    values = row_data[0].get("values", [])
    return [
        (cell.get("effectiveValue", {}).get("stringValue") or "")
        for cell in values
    ]


def _row_to_trade(
    values: List[Dict], sheet_name: str, is_foreign: bool, date_val: str
) -> Optional[Trade]:
    """시트 행 데이터 → Trade 객체 변환.

    Args:
        values: get_raw_grid_data()의 rowData.values 리스트
        sheet_name: 시트 이름 (account 필드로 사용)
        is_foreign: 해외계좌 여부
        date_val: formattedValue에서 추출한 날짜 문자열
    """
    try:
        if is_foreign:
            # 해외: A~O (15컬럼)
            return Trade(
                date=date_val,
                trade_type=_get_str(values[1]),
                stock_name=_get_str(values[4]),
                stock_code=_get_str(values[3]),
                quantity=_get_num(values[5]),
                price=_get_num(values[6]),
                amount=_get_num(values[7]),
                currency=_get_str(values[2]),
                exchange_rate=_get_num(values[8]),
                amount_krw=_get_num(values[9]),
                fee=_get_num(values[10]),
                tax=_get_num(values[11]),
                profit=_get_num(values[12]),
                profit_krw=_get_num(values[13]),
                profit_rate=_get_num(values[14]) * 100,
                account=sheet_name,
            )
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
    except (IndexError, KeyError, TypeError) as e:
        logger.warning(f"행 변환 실패 ({sheet_name}, date={date_val}): {e}")
        return None
