#!/usr/bin/env python3
"""
Google Sheets 관리 모듈
Google Sheets와의 상호작용을 담당합니다.
"""

import asyncio
import logging
from collections import defaultdict
from typing import Dict, List, Optional, Set, Tuple

from .file_parser import Trade
from .google_sheets_client import GoogleSheetsClient


logger = logging.getLogger(__name__)


class SheetManager:
    """Google Sheets 관리 클래스"""
    
    def __init__(self, spreadsheet_id: str, service_account_path: Optional[str] = None,
                 empty_row_threshold: int = 100):
        """초기화
        
        Args:
            spreadsheet_id: Google Sheets ID
            service_account_path: 서비스 계정 파일 경로
            empty_row_threshold: 빈 행 판단 임계값
        """
        self.spreadsheet_id = spreadsheet_id
        self.empty_row_threshold = empty_row_threshold
        self.client = GoogleSheetsClient(spreadsheet_id, service_account_path)
        
        # 색상 팔레트 (light colors)
        self.color_palette = [
            {"red": 1.0, "green": 0.9, "blue": 0.9},
            {"red": 0.9, "green": 1.0, "blue": 0.9},
            {"red": 0.9, "green": 0.9, "blue": 1.0},
            {"red": 1.0, "green": 1.0, "blue": 0.9},
            {"red": 1.0, "green": 0.9, "blue": 1.0},
            {"red": 0.9, "green": 1.0, "blue": 1.0},
            {"red": 0.95, "green": 0.95, "blue": 0.85},
            {"red": 0.85, "green": 0.95, "blue": 0.95}
        ]
    
    async def __aenter__(self):
        """비동기 컨텍스트 매니저 진입"""
        await self.client.__aenter__()
        return self
    
    async def __aexit__(self, exc_type, exc_val, exc_tb):
        """비동기 컨텍스트 매니저 종료"""
        await self.client.__aexit__(exc_type, exc_val, exc_tb)
    
    async def list_sheets(self) -> List[str]:
        """시트 목록 조회"""
        return await self.client.list_sheets()
    
    async def find_target_sheets(self, prefix: str, file_type: str = "domestic") -> Dict[str, Optional[str]]:
        """prefix와 파일 타입에 해당하는 시트들을 찾습니다
        
        Args:
            prefix: 파일명 prefix (예: "계좌1 국내")
            file_type: "domestic" 또는 "foreign"
        """
        logger.info(f"'{prefix}'로 시작하는 {file_type} 시트들을 찾는 중...")
        
        try:
            sheet_list = await self.list_sheets()
            logger.debug(f"전체 시트 목록: {sheet_list}")
            
            target_sheets = {
                "주식_매수": None,
                "주식_매도": None,
                "ETF_매수": None,
                "ETF_매도": None
            }
            
            # 파일 타입에 따른 시트 이름 패턴
            if file_type == "foreign":
                # 해외 주식용 시트 패턴
                patterns = {
                    "주식_매수": [
                        f"{prefix} 해외 주식 매수내역",
                        f"{prefix} 해외 주식 매수",
                        f"{prefix} 해외주식 매수내역",
                        f"{prefix} 해외주식 매수"
                    ],
                    "주식_매도": [
                        f"{prefix} 해외 주식 매도내역",
                        f"{prefix} 해외 주식 매도",
                        f"{prefix} 해외주식 매도내역",
                        f"{prefix} 해외주식 매도"
                    ],
                    "ETF_매수": [
                        f"{prefix} 해외 ETF 매수내역",
                        f"{prefix} 해외 ETF 매수",
                        f"{prefix} 해외ETF 매수내역",
                        f"{prefix} 해외ETF 매수"
                    ],
                    "ETF_매도": [
                        f"{prefix} 해외 ETF 매도내역",
                        f"{prefix} 해외 ETF 매도",
                        f"{prefix} 해외ETF 매도내역",
                        f"{prefix} 해외ETF 매도"
                    ]
                }
            else:
                # 국내 주식용 시트 패턴 (기존)
                patterns = {
                    "주식_매수": [f"{prefix} 주식 매수내역"],
                    "주식_매도": [f"{prefix} 주식 매도내역"],
                    "ETF_매수": [f"{prefix} ETF 매수내역"],
                    "ETF_매도": [f"{prefix} ETF 매도내역"]
                }
            
            # 시트 이름 매칭
            for sheet in sheet_list:
                for key, pattern_list in patterns.items():
                    for pattern in pattern_list:
                        if sheet == pattern:
                            target_sheets[key] = sheet
                            break
            
            logger.info(f"'{prefix}'에 매칭된 {file_type} 시트들:")
            for key, value in target_sheets.items():
                if value:
                    logger.info(f"  {key}: {value}")
                else:
                    logger.warning(f"  {key}: 시트를 찾을 수 없음")
            
            return target_sheets
            
        except Exception as e:
            logger.error(f"시트 목록 조회 실패: {e}")
            return {
                "주식_매수": None,
                "주식_매도": None,
                "ETF_매수": None,
                "ETF_매도": None
            }
    
    async def find_last_row(self, sheet_name: str) -> int:
        """시트의 마지막 데이터 행을 찾습니다"""
        logger.info(f"시트 '{sheet_name}'의 마지막 행을 찾는 중...")
        
        try:
            # 시트의 메타데이터를 가져와서 실제 행 수 확인
            metadata = await self.client.get_spreadsheet_metadata()
            
            if not metadata or 'sheets' not in metadata:
                logger.warning(f"시트 메타데이터를 가져올 수 없습니다")
                # 기본값으로 폴백
                sheet_data = await self.client.get_sheet_data(sheet_name, "A1:Z10000")
            else:
                # 해당 시트의 속성 찾기
                sheet_properties = None
                for sheet in metadata['sheets']:
                    if sheet['properties']['title'] == sheet_name:
                        sheet_properties = sheet['properties']
                        break
                
                if not sheet_properties:
                    logger.warning(f"시트 '{sheet_name}'의 속성을 찾을 수 없습니다")
                    sheet_data = await self.client.get_sheet_data(sheet_name, "A1:Z10000")
                else:
                    # 시트의 실제 행 수 가져오기
                    row_count = sheet_properties['gridProperties']['rowCount']
                    logger.debug(f"시트 '{sheet_name}'의 전체 행 수: {row_count}")
                    
                    # 실제 행 수만큼의 범위로 데이터 가져오기
                    sheet_data = await self.client.get_sheet_data(sheet_name, f"A1:Z{row_count}")
            
            # GridData에서 행 데이터 추출
            if not sheet_data or 'sheets' not in sheet_data:
                logger.warning(f"시트 '{sheet_name}'의 데이터를 읽을 수 없습니다")
                return 2  # 헤더 다음 행
            
            rows = sheet_data['sheets'][0]['data'][0].get('rowData', [])
            
            # 마지막 데이터 행 찾기
            last_data_row = 1  # 최소 헤더 행
            empty_row_count = 0
            max_column_in_last_row = 0
            
            for i, row in enumerate(rows):
                row_number = i + 1
                values = row.get('values', [])
                is_empty = True
                row_max_column = 0
                
                for col_idx, cell in enumerate(values):
                    cell_value = cell.get('effectiveValue', {})
                    if any(key in cell_value for key in ['numberValue', 'stringValue', 'boolValue']):
                        is_empty = False
                        row_max_column = col_idx + 1  # 1-based column index
                
                if is_empty:
                    empty_row_count += 1
                    if empty_row_count >= self.empty_row_threshold:
                        break
                else:
                    last_data_row = row_number
                    empty_row_count = 0
                    max_column_in_last_row = row_max_column
            
            # 마지막 데이터 행의 컬럼 범위 확인
            if last_data_row > 1 and max_column_in_last_row > 0:
                logger.info(f"시트 '{sheet_name}'의 마지막 데이터 행: {last_data_row}, 마지막 컬럼: {self._column_number_to_letter(max_column_in_last_row)}")
            else:
                logger.info(f"시트 '{sheet_name}'의 마지막 데이터 행: {last_data_row}")
            
            return last_data_row + 1  # 다음 빈 행 반환
            
        except Exception as e:
            logger.error(f"마지막 행 찾기 실패: {e}")
            return 2  # 오류 시 헤더 다음 행
    
    async def find_last_row_with_column_info(self, sheet_name: str) -> Tuple[int, int, int]:
        """시트의 마지막 데이터 행과 해당 행의 컬럼 범위를 찾습니다
        
        Returns:
            (마지막 데이터 행 + 1, 데이터 시작 컬럼, 데이터 끝 컬럼)
        """
        logger.info(f"시트 '{sheet_name}'의 마지막 행과 컬럼 정보를 찾는 중...")
        
        try:
            # 시트의 메타데이터를 가져와서 실제 행 수 확인
            metadata = await self.client.get_spreadsheet_metadata()
            
            if not metadata or 'sheets' not in metadata:
                logger.warning(f"시트 메타데이터를 가져올 수 없습니다")
                # 기본값으로 폴백
                sheet_data = await self.client.get_sheet_data(sheet_name, "A1:Z10000")
            else:
                # 해당 시트의 속성 찾기
                sheet_properties = None
                for sheet in metadata['sheets']:
                    if sheet['properties']['title'] == sheet_name:
                        sheet_properties = sheet['properties']
                        break
                
                if not sheet_properties:
                    logger.warning(f"시트 '{sheet_name}'의 속성을 찾을 수 없습니다")
                    sheet_data = await self.client.get_sheet_data(sheet_name, "A1:Z10000")
                else:
                    # 시트의 실제 행 수 가져오기
                    row_count = sheet_properties['gridProperties']['rowCount']
                    logger.debug(f"시트 '{sheet_name}'의 전체 행 수: {row_count}")
                    
                    # 실제 행 수만큼의 범위로 데이터 가져오기
                    sheet_data = await self.client.get_sheet_data(sheet_name, f"A1:Z{row_count}")
            
            # GridData에서 행 데이터 추출
            if not sheet_data or 'sheets' not in sheet_data:
                logger.warning(f"시트 '{sheet_name}'의 데이터를 읽을 수 없습니다")
                return (2, 2, 9)  # 헤더 다음 행, B열부터 I열까지 (기본값)
            
            rows = sheet_data['sheets'][0]['data'][0].get('rowData', [])
            
            # 1단계: 마지막 데이터 행 찾기
            last_data_row = 1  # 최소 헤더 행
            empty_row_count = 0
            
            for i, row in enumerate(rows):
                row_number = i + 1
                values = row.get('values', [])
                is_empty = True
                
                # 행에 데이터가 있는지 확인
                for cell in values:
                    cell_value = cell.get('effectiveValue', {})
                    if any(key in cell_value for key in ['numberValue', 'stringValue', 'boolValue']):
                        is_empty = False
                        break
                
                if is_empty:
                    empty_row_count += 1
                    if empty_row_count >= self.empty_row_threshold:
                        break
                else:
                    last_data_row = row_number
                    empty_row_count = 0
            
            # 2단계: 마지막 데이터 행에서 컬럼 범위 찾기
            last_row_start_col = None
            last_row_end_col = 0
            
            if last_data_row > 0 and last_data_row <= len(rows):
                last_row_data = rows[last_data_row - 1]  # 0-based index
                values = last_row_data.get('values', [])
                
                logger.debug(f"마지막 데이터 행 {last_data_row}의 셀 개수: {len(values)}")
                
                # 각 셀의 값 출력
                for col_idx, cell in enumerate(values):
                    cell_value = cell.get('effectiveValue', {})
                    formatted_value = cell.get('formattedValue', '')
                    col_letter = self._column_number_to_letter(col_idx + 1)
                    
                    # effectiveValue의 실제 값 추출
                    actual_value = None
                    if 'stringValue' in cell_value:
                        actual_value = cell_value['stringValue']
                    elif 'numberValue' in cell_value:
                        actual_value = cell_value['numberValue']
                    elif 'boolValue' in cell_value:
                        actual_value = cell_value['boolValue']
                    
                    logger.debug(f"  {col_letter}열: {actual_value} (표시값: {formatted_value})")
                
                for col_idx, cell in enumerate(values):
                    cell_value = cell.get('effectiveValue', {})
                    if any(key in cell_value for key in ['numberValue', 'stringValue', 'boolValue']):
                        # 실제 값 확인
                        actual_value = None
                        if 'stringValue' in cell_value:
                            actual_value = cell_value['stringValue']
                        elif 'numberValue' in cell_value:
                            actual_value = cell_value['numberValue']
                        elif 'boolValue' in cell_value:
                            actual_value = cell_value['boolValue']
                        
                        # 빈 문자열이 아닌 경우에만 데이터로 간주
                        if actual_value is not None and str(actual_value).strip() != '':
                            if last_row_start_col is None:
                                last_row_start_col = col_idx + 1  # 1-based column index
                                logger.debug(f"마지막 데이터 행의 첫 번째 데이터 컬럼: {col_idx + 1} ({self._column_number_to_letter(col_idx + 1)})")
                            last_row_end_col = col_idx + 1  # 1-based column index
            
            # 기본값 설정
            if last_row_start_col is None:
                last_row_start_col = 2  # B열
            if last_row_end_col < last_row_start_col:
                last_row_end_col = last_row_start_col + 7  # 8개 컬럼
            
            logger.info(f"시트 '{sheet_name}'의 마지막 데이터 행: {last_data_row}, "
                       f"해당 행의 컬럼 범위: {self._column_number_to_letter(last_row_start_col)}-"
                       f"{self._column_number_to_letter(last_row_end_col)} "
                       f"(다음 입력 위치: 행 {last_data_row + 1})")
            
            return (last_data_row + 1, last_row_start_col, last_row_end_col)
            
        except Exception as e:
            logger.error(f"마지막 행과 컬럼 정보 찾기 실패: {e}")
            return (2, 2, 9)  # 오류 시 헤더 다음 행, B열부터 I열까지
    
    def _column_number_to_letter(self, column_number: int) -> str:
        """컬럼 번호를 문자로 변환 (1 -> A, 26 -> Z, 27 -> AA)"""
        result = ""
        while column_number > 0:
            column_number -= 1
            result = chr(65 + column_number % 26) + result
            column_number //= 26
        return result
    
    async def check_duplicates(self, sheet_name: str, trades: List[Trade]) -> Set[int]:
        """중복 거래를 확인합니다"""
        logger.info(f"시트 '{sheet_name}'에서 중복 확인 중...")
        
        try:
            # 시트의 메타데이터를 가져와서 실제 행 수 확인
            metadata = await self.client.get_spreadsheet_metadata()
            
            # 범위 결정
            range_str = "A2:C10000"  # 기본값
            if metadata and 'sheets' in metadata:
                for sheet in metadata['sheets']:
                    if sheet['properties']['title'] == sheet_name:
                        row_count = sheet['properties']['gridProperties']['rowCount']
                        logger.debug(f"시트 '{sheet_name}'의 전체 행 수: {row_count}")
                        range_str = f"A2:C{row_count}"
                        break
            
            # A열: 종목, C열: 일자
            sheet_data = await self.client.get_sheet_data(sheet_name, range_str)
            
            # GridData에서 기존 거래 정보 추출
            existing_trades = set()
            if sheet_data and 'sheets' in sheet_data:
                rows = sheet_data['sheets'][0]['data'][0].get('rowData', [])
                
                for row in rows:
                    values = row.get('values', [])
                    if len(values) >= 3:  # 종목, 증권, 일자
                        stock_name = None
                        date_str = None
                        
                        # 종목명 (A열)
                        if 'effectiveValue' in values[0]:
                            stock_name = values[0]['effectiveValue'].get('stringValue', '')
                        
                        # 일자 (C열)
                        if len(values) > 2 and 'effectiveValue' in values[2]:
                            date_str = values[2]['effectiveValue'].get('stringValue', '')
                        
                        if stock_name and date_str:
                            existing_trades.add((stock_name, date_str))
            
            # 중복 거래 인덱스 찾기
            duplicate_indices = set()
            for i, trade in enumerate(trades):
                trade_key = (trade.stock_name, trade.date)
                if trade_key in existing_trades:
                    logger.warning(f"중복 거래 발견: {trade.stock_name} - {trade.date}")
                    duplicate_indices.add(i)
            
            if duplicate_indices:
                logger.warning(f"총 {len(duplicate_indices)}개의 중복 거래가 발견되어 건너뜁니다")
            
            return duplicate_indices
            
        except Exception as e:
            logger.error(f"중복 확인 실패: {e}")
            return set()
    
    async def batch_insert_trades(self, sheet_trades: Dict[str, List[Tuple[List[Trade], int]]]) -> Dict[str, bool]:
        """여러 시트에 거래 데이터를 배치로 입력합니다
        
        Args:
            sheet_trades: {시트명: [(거래리스트, 시작행), ...]}
            
        Returns:
            {시트명: 성공여부}
        """
        results = {}
        batch_updates = {}
        color_info = {}
        
        # 각 시트의 컬럼 정보를 미리 가져옴
        sheet_column_info = {}
        for sheet_name in sheet_trades.keys():
            _, start_col, end_col = await self.find_last_row_with_column_info(sheet_name)
            logger.debug(f"시트 '{sheet_name}'의 컬럼 범위: {start_col} - {end_col}")
            sheet_column_info[sheet_name] = (start_col, end_col)
        
        # 배치 업데이트 데이터 준비
        for sheet_name, trade_groups in sheet_trades.items():
            if sheet_name not in batch_updates:
                batch_updates[sheet_name] = {}
                color_info[sheet_name] = []
            
            start_col, end_col = sheet_column_info.get(sheet_name, (2, 9))
            num_cols = end_col - start_col + 1
            
            for trades, start_row in trade_groups:
                if not trades:
                    continue
                
                # 범위 계산 - 시트의 데이터 컬럼 범위에 맞춰 조정
                end_row = start_row + len(trades) - 1
                start_col_letter = self._column_number_to_letter(start_col)
                end_col_letter = self._column_number_to_letter(end_col)
                range_str = f"{start_col_letter}{start_row}:{end_col_letter}{end_row}"
                
                # 구글 시트 포맷으로 변환
                rows_data = []
                for trade in trades:
                    row = trade.to_sheet_row()
                    # 데이터 크기 확인 로그 추가
                    if len(row) != 8:
                        logger.warning(f"예상과 다른 데이터 크기: {len(row)}개 (예상: 8개), 데이터: {row}")
                    # 필요한 컬럼 수에 맞춰 조정
                    if len(row) > num_cols:
                        row = row[:num_cols]
                    elif len(row) < num_cols:
                        row.extend([''] * (num_cols - len(row)))
                    rows_data.append(row)
                
                batch_updates[sheet_name][range_str] = rows_data
                color_info[sheet_name].append((trades, start_row, start_col, end_col))
                
                logger.info(f"시트 '{sheet_name}' 범위 {range_str}에 {len(trades)}개 거래 준비")
                
                # 각 거래의 위치 정보 로그
                for i, trade in enumerate(trades):
                    row_num = start_row + i
                    logger.debug(f"  - 행 {row_num}: {trade.stock_name} ({trade.date} {trade.trade_type})")
                
                # 다음 그룹을 위해 현재 행 업데이트
                current_row = end_row + 1
        
        # 배치 업데이트 실행
        for sheet_name, ranges in batch_updates.items():
            try:
                logger.info(f"시트 '{sheet_name}'에 배치 업데이트 실행 중...")
                success = await self._retry_with_backoff(
                    self.client.batch_update_cells,
                    sheet_name,
                    ranges,
                    max_retries=3
                )
                
                results[sheet_name] = success
                
                if success:
                    logger.info(f"시트 '{sheet_name}' 배치 업데이트 성공")
                    
                    # 입력된 데이터 위치 상세 로그
                    for range_str, data in ranges.items():
                        logger.info(f"  ✓ {range_str} 범위에 {len(data)}개 행 입력 완료")
                        # 첫 번째와 마지막 데이터 표시
                        if data:
                            logger.info(f"    첫 번째: {data[0][0]} ({data[0][3]} {data[0][2]})")
                            if len(data) > 1:
                                logger.info(f"    마지막: {data[-1][0]} ({data[-1][3]} {data[-1][2]})")
                    
                    # 색상 적용
                    if sheet_name in color_info:
                        for trades, start_row, start_col, end_col in color_info[sheet_name]:
                            self.apply_date_colors(sheet_name, trades, start_row, start_col, end_col)
                else:
                    logger.error(f"시트 '{sheet_name}' 배치 업데이트 실패")
                    
            except Exception as e:
                logger.error(f"시트 '{sheet_name}' 배치 업데이트 중 오류: {e}")
                results[sheet_name] = False
        
        return results
    
    def apply_date_colors(self, sheet_name: str, trades: List[Trade], start_row: int, start_col: int, end_col: int):
        """날짜별로 색상을 적용합니다"""
        if not trades:
            return
        
        # 날짜별로 거래 그룹화
        date_groups = defaultdict(list)
        for i, trade in enumerate(trades):
            date_groups[trade.date].append(i)
        
        logger.info(f"{len(date_groups)}개 날짜 그룹에 색상 적용 중...")
        
        # 모든 색상 적용 요청을 준비
        color_ranges = []
        
        # 각 날짜 그룹에 색상 할당
        for date_idx, (date, trade_indices) in enumerate(date_groups.items()):
            color = self.color_palette[date_idx % len(self.color_palette)]
            
            # 연속된 행을 그룹화하여 처리
            trade_indices.sort()
            
            # 연속된 인덱스를 찾아서 범위로 그룹화
            i = 0
            while i < len(trade_indices):
                start_idx = trade_indices[i]
                end_idx = start_idx
                
                # 연속된 인덱스 찾기
                while i + 1 < len(trade_indices) and trade_indices[i + 1] == trade_indices[i] + 1:
                    i += 1
                    end_idx = trade_indices[i]
                
                # 색상 적용 범위 추가
                color_ranges.append({
                    'start_row': start_row + start_idx,
                    'end_row': start_row + end_idx + 1,  # end_row는 exclusive
                    'start_col': start_col,
                    'end_col': end_col,
                    'color': color
                })
                
                i += 1
            
            logger.debug(f"날짜 {date}: {len(trade_indices)}개 거래")
        
        # 배치로 색상 적용
        if color_ranges:
            success = self.client.batch_apply_colors(sheet_name, color_ranges)
            if success:
                logger.info(f"배치 색상 적용 성공: {len(color_ranges)}개 범위")
            else:
                logger.warning(f"배치 색상 적용 실패")
    
    async def _retry_with_backoff(self, func, *args, max_retries=3, initial_delay=1, **kwargs):
        """지수 백오프를 사용한 재시도 로직"""
        for attempt in range(max_retries):
            try:
                return await func(*args, **kwargs) if asyncio.iscoroutinefunction(func) else func(*args, **kwargs)
            except Exception as e:
                if attempt == max_retries - 1:
                    raise
                delay = initial_delay * (2 ** attempt)
                logger.warning(f"시도 {attempt + 1}/{max_retries} 실패: {e}. {delay}초 후 재시도...")
                await asyncio.sleep(delay) 