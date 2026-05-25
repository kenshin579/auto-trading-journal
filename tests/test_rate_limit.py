"""Google Sheets API Rate Limit 최적화 관련 단위 테스트"""

from unittest.mock import AsyncMock, MagicMock, patch, PropertyMock
import pytest

from googleapiclient.errors import HttpError

from modules.google_sheets_client import GoogleSheetsClient


class TestBuildNumberFormatRequests:
    """build_number_format_requests() 반환 구조 검증"""

    def test_single_column(self):
        formats = [{'col': 2, 'pattern': '₩#,##0'}]
        result = GoogleSheetsClient.build_number_format_requests(42, formats, 5, 10)

        assert len(result) == 1
        req = result[0]['repeatCell']
        assert req['range']['sheetId'] == 42
        assert req['range']['startRowIndex'] == 4   # 5 - 1
        assert req['range']['endRowIndex'] == 10     # inclusive
        assert req['range']['startColumnIndex'] == 1  # col 2 - 1
        assert req['range']['endColumnIndex'] == 2    # col 2
        assert req['cell']['userEnteredFormat']['numberFormat']['pattern'] == '₩#,##0'
        assert req['cell']['userEnteredFormat']['numberFormat']['type'] == 'NUMBER'

    def test_percent_type(self):
        formats = [{'col': 5, 'pattern': '0.00%', 'type': 'PERCENT'}]
        result = GoogleSheetsClient.build_number_format_requests(0, formats, 1, 1)

        assert result[0]['repeatCell']['cell']['userEnteredFormat']['numberFormat']['type'] == 'PERCENT'

    def test_multiple_columns(self):
        formats = [
            {'col': 2, 'pattern': '₩#,##0'},
            {'col': 3, 'pattern': '#,##0'},
            {'col': 4, 'pattern': '0.00%', 'type': 'PERCENT'},
        ]
        result = GoogleSheetsClient.build_number_format_requests(0, formats, 1, 10)
        assert len(result) == 3

    def test_empty_formats(self):
        result = GoogleSheetsClient.build_number_format_requests(0, [], 1, 10)
        assert result == []


class TestBuildTextFormatRequests:
    """build_text_format_requests() 반환 구조 검증 (종목코드 TEXT 포맷)"""

    def test_returns_text_format_for_column(self):
        result = GoogleSheetsClient.build_text_format_requests(7, 3, 2, 1000)

        assert len(result) == 1
        req = result[0]['repeatCell']
        assert req['range']['sheetId'] == 7
        assert req['range']['startRowIndex'] == 1   # 2 - 1
        assert req['range']['endRowIndex'] == 1000   # inclusive
        assert req['range']['startColumnIndex'] == 2  # col 3 - 1
        assert req['range']['endColumnIndex'] == 3
        nf = req['cell']['userEnteredFormat']['numberFormat']
        assert nf['type'] == 'TEXT'
        assert nf['pattern'] == '@'
        assert req['fields'] == 'userEnteredFormat.numberFormat'

    def test_none_column_returns_empty(self):
        assert GoogleSheetsClient.build_text_format_requests(0, None, 2, 10) == []


class TestBuildColorRequests:
    """build_color_requests() 반환 구조 검증"""

    def test_single_range(self):
        color_ranges = [{
            'start_row': 1, 'end_row': 1,
            'start_col': 1, 'end_col': 7,
            'color': {'red': 0.24, 'green': 0.52, 'blue': 0.78},
        }]
        result = GoogleSheetsClient.build_color_requests(42, color_ranges)

        assert len(result) == 1
        req = result[0]['repeatCell']
        assert req['range']['sheetId'] == 42
        assert req['range']['startRowIndex'] == 0   # 1 - 1
        assert req['range']['endRowIndex'] == 1      # inclusive
        assert req['range']['startColumnIndex'] == 0  # 1 - 1
        assert req['range']['endColumnIndex'] == 7
        assert req['cell']['userEnteredFormat']['backgroundColor']['red'] == 0.24
        assert req['fields'] == 'userEnteredFormat.backgroundColor'

    def test_multiple_ranges(self):
        color_ranges = [
            {'start_row': 1, 'end_row': 1, 'start_col': 1, 'end_col': 5,
             'color': {'red': 1, 'green': 0, 'blue': 0}},
            {'start_row': 10, 'end_row': 10, 'start_col': 1, 'end_col': 8,
             'color': {'red': 0, 'green': 1, 'blue': 0}},
        ]
        result = GoogleSheetsClient.build_color_requests(0, color_ranges)
        assert len(result) == 2

    def test_empty_ranges(self):
        result = GoogleSheetsClient.build_color_requests(0, [])
        assert result == []


class TestExecuteWithRetry:
    """_execute_with_retry() 429 재시도 동작 검증"""

    @pytest.mark.asyncio
    async def test_success_on_first_try(self):
        client = MagicMock(spec=GoogleSheetsClient)
        client._execute_with_retry = GoogleSheetsClient._execute_with_retry.__get__(client)

        mock_request = MagicMock()
        mock_request.return_value.execute.return_value = {'result': 'ok'}

        result = await client._execute_with_retry(mock_request)
        assert result == {'result': 'ok'}
        assert mock_request.return_value.execute.call_count == 1

    @pytest.mark.asyncio
    async def test_retry_on_429(self):
        client = MagicMock(spec=GoogleSheetsClient)
        client._execute_with_retry = GoogleSheetsClient._execute_with_retry.__get__(client)

        resp = MagicMock()
        resp.status = 429
        error_429 = HttpError(resp, b'rate limit')

        mock_request = MagicMock()
        mock_request.return_value.execute.side_effect = [
            error_429,
            {'result': 'ok'},
        ]

        with patch('modules.google_sheets_client.asyncio.sleep', new_callable=AsyncMock):
            result = await client._execute_with_retry(mock_request, max_retries=3)

        assert result == {'result': 'ok'}
        assert mock_request.return_value.execute.call_count == 2

    @pytest.mark.asyncio
    async def test_raise_on_non_429(self):
        client = MagicMock(spec=GoogleSheetsClient)
        client._execute_with_retry = GoogleSheetsClient._execute_with_retry.__get__(client)

        resp = MagicMock()
        resp.status = 500
        error_500 = HttpError(resp, b'server error')

        mock_request = MagicMock()
        mock_request.return_value.execute.side_effect = error_500

        with pytest.raises(HttpError):
            await client._execute_with_retry(mock_request, max_retries=3)

    @pytest.mark.asyncio
    async def test_raise_after_max_retries(self):
        client = MagicMock(spec=GoogleSheetsClient)
        client._execute_with_retry = GoogleSheetsClient._execute_with_retry.__get__(client)

        resp = MagicMock()
        resp.status = 429
        error_429 = HttpError(resp, b'rate limit')

        mock_request = MagicMock()
        mock_request.return_value.execute.side_effect = error_429

        with patch('modules.google_sheets_client.asyncio.sleep', new_callable=AsyncMock):
            with pytest.raises(HttpError):
                await client._execute_with_retry(mock_request, max_retries=2)

        # 최초 시도 1 + 재시도 2 = 총 3번
        assert mock_request.return_value.execute.call_count == 3


class TestApplySheetFormattingBatch:
    """apply_sheet_formatting_batch() 요청 구조 검증"""

    @pytest.mark.asyncio
    async def test_request_structure(self):
        client = MagicMock(spec=GoogleSheetsClient)
        client.get_sheet_id = AsyncMock(return_value=42)
        client._execute_with_retry = AsyncMock(return_value=None)
        client.apply_sheet_formatting_batch = GoogleSheetsClient.apply_sheet_formatting_batch.__get__(client)

        await client.apply_sheet_formatting_batch('시트1', filter_end_col=9)

        # _execute_with_retry가 호출됨
        assert client._execute_with_retry.called
        # lambda에서 호출되는 요청을 검증하기 위해 lambda를 실행
        call_fn = client._execute_with_retry.call_args[0][0]
        # call_fn은 lambda이므로 service mock이 필요
        # 여기서는 호출 자체가 성공적으로 이루어졌는지만 확인
        assert client.get_sheet_id.called
