#!/usr/bin/env python3
"""
Google Sheets API v4 클라이언트
MCP 대신 Google Sheets API를 직접 사용합니다.
"""

import logging
from typing import Dict, List, Any, Optional
from pathlib import Path
import os

from google.oauth2 import service_account
from googleapiclient.discovery import build
from googleapiclient.errors import HttpError

logger = logging.getLogger(__name__)


class GoogleSheetsClient:
    """Google Sheets API v4 클라이언트"""
    
    def __init__(self, spreadsheet_id: str, service_account_path: Optional[str] = None):
        """Google Sheets 클라이언트 초기화
        
        Args:
            spreadsheet_id: Google 스프레드시트 ID
            service_account_path: 서비스 계정 키 파일 경로
        """
        self.spreadsheet_id = spreadsheet_id
        
        # 서비스 계정 경로 설정 (환경변수 우선)
        if service_account_path:
            self.service_account_path = service_account_path
        else:
            self.service_account_path = os.getenv('SERVICE_ACCOUNT_PATH')
            if not self.service_account_path:
                raise ValueError("환경변수 SERVICE_ACCOUNT_PATH가 설정되지 않았습니다")
        
        self.service = None
        self._connect()
    
    def _connect(self):
        """Google Sheets API에 연결"""
        try:
            # 서비스 계정 인증
            credentials = service_account.Credentials.from_service_account_file(
                self.service_account_path,
                scopes=['https://www.googleapis.com/auth/spreadsheets']
            )
            
            # API 서비스 빌드
            self.service = build('sheets', 'v4', credentials=credentials)
            logger.info("Google Sheets API에 연결됨")
            
        except Exception as e:
            logger.error(f"Google Sheets API 연결 실패: {e}")
            raise
    
    async def __aenter__(self):
        """비동기 컨텍스트 관리자 진입"""
        return self
    
    async def __aexit__(self, exc_type, exc_val, exc_tb):
        """비동기 컨텍스트 관리자 종료"""
        # Google Sheets API는 별도의 연결 종료가 필요 없음
        pass
    
    async def list_sheets(self) -> List[str]:
        """스프레드시트의 모든 시트 목록을 반환"""
        try:
            # 스프레드시트 메타데이터 가져오기
            spreadsheet = self.service.spreadsheets().get(
                spreadsheetId=self.spreadsheet_id
            ).execute()
            
            # 시트 이름 추출
            sheets = spreadsheet.get('sheets', [])
            sheet_names = [sheet['properties']['title'] for sheet in sheets]
            
            logger.info(f"{len(sheet_names)}개의 시트를 찾음")
            return sheet_names
            
        except HttpError as e:
            logger.error(f"시트 목록 조회 실패: {e}")
            return []
    
    async def get_spreadsheet_metadata(self) -> Dict[str, Any]:
        """스프레드시트의 전체 메타데이터를 반환"""
        try:
            # 스프레드시트 메타데이터 가져오기
            spreadsheet = self.service.spreadsheets().get(
                spreadsheetId=self.spreadsheet_id
            ).execute()
            
            logger.debug("스프레드시트 메타데이터를 성공적으로 가져옴")
            return spreadsheet
            
        except HttpError as e:
            logger.error(f"스프레드시트 메타데이터 조회 실패: {e}")
            return {}
    
    async def get_sheet_data(self, sheet_name: str, range_str: Optional[str] = None) -> Dict[str, Any]:
        """시트의 데이터를 가져옵니다"""
        try:
            # 범위 설정
            if range_str:
                range_name = f"{sheet_name}!{range_str}"
            else:
                range_name = sheet_name
            
            # 데이터 가져오기
            result = self.service.spreadsheets().values().get(
                spreadsheetId=self.spreadsheet_id,
                range=range_name,
                valueRenderOption='FORMATTED_VALUE'
            ).execute()
            
            # GridData 형식으로 변환 (기존 코드와의 호환성을 위해)
            values = result.get('values', [])
            
            # GridData 형식 모방
            grid_data = {
                'sheets': [{
                    'data': [{
                        'rowData': []
                    }]
                }]
            }
            
            # 각 행을 GridData 형식으로 변환
            for row in values:
                row_data = {'values': []}
                for cell_value in row:
                    cell = {'effectiveValue': {}}
                    
                    # 값 타입에 따라 적절한 필드 설정
                    if isinstance(cell_value, (int, float)):
                        cell['effectiveValue']['numberValue'] = cell_value
                    elif isinstance(cell_value, bool):
                        cell['effectiveValue']['boolValue'] = cell_value
                    elif cell_value is not None:
                        cell['effectiveValue']['stringValue'] = str(cell_value)
                    
                    row_data['values'].append(cell)
                
                grid_data['sheets'][0]['data'][0]['rowData'].append(row_data)
            
            return grid_data
            
        except HttpError as e:
            logger.error(f"시트 데이터 조회 실패: {e}")
            return {}
    
    async def update_cells(self, sheet_name: str, range_str: str, data: List[List[Any]]) -> bool:
        """셀 데이터를 업데이트합니다"""
        try:
            range_name = f"{sheet_name}!{range_str}"
            
            body = {
                'values': data
            }
            
            result = self.service.spreadsheets().values().update(
                spreadsheetId=self.spreadsheet_id,
                range=range_name,
                valueInputOption='USER_ENTERED',
                body=body
            ).execute()
            
            updated_cells = result.get('updatedCells', 0)
            logger.info(f"{updated_cells}개 셀 업데이트 완료")
            return True
            
        except HttpError as e:
            logger.error(f"셀 업데이트 실패: {e}")
            return False
    
    async def batch_update_cells(self, sheet_name: str, ranges: Dict[str, List[List[Any]]]) -> bool:
        """여러 범위의 셀을 한 번에 업데이트"""
        try:
            # 배치 업데이트 데이터 준비
            data = []
            for range_str, values in ranges.items():
                data.append({
                    'range': f"{sheet_name}!{range_str}",
                    'values': values
                })
            
            body = {
                'valueInputOption': 'USER_ENTERED',
                'data': data
            }
            
            result = self.service.spreadsheets().values().batchUpdate(
                spreadsheetId=self.spreadsheet_id,
                body=body
            ).execute()
            
            total_updated_cells = result.get('totalUpdatedCells', 0)
            logger.info(f"배치 업데이트: 총 {total_updated_cells}개 셀 업데이트 완료")
            return True
            
        except HttpError as e:
            logger.error(f"배치 업데이트 실패: {e}")
            return False
    
    def apply_color_to_range(self, sheet_name: str, start_row: int, end_row: int, 
                           start_col: int, end_col: int, color: Dict[str, float]) -> bool:
        """특정 범위에 색상을 적용합니다"""
        try:
            # 시트 ID 가져오기
            spreadsheet = self.service.spreadsheets().get(
                spreadsheetId=self.spreadsheet_id
            ).execute()
            
            sheet_id = None
            for sheet in spreadsheet.get('sheets', []):
                if sheet['properties']['title'] == sheet_name:
                    sheet_id = sheet['properties']['sheetId']
                    break
            
            if sheet_id is None:
                logger.error(f"시트 '{sheet_name}'를 찾을 수 없습니다")
                return False
            
            # 색상 적용 요청
            requests = [{
                'repeatCell': {
                    'range': {
                        'sheetId': sheet_id,
                        'startRowIndex': start_row - 1,  # 0-based index
                        'endRowIndex': end_row,
                        'startColumnIndex': start_col - 1,
                        'endColumnIndex': end_col
                    },
                    'cell': {
                        'userEnteredFormat': {
                            'backgroundColor': color
                        }
                    },
                    'fields': 'userEnteredFormat.backgroundColor'
                }
            }]
            
            body = {'requests': requests}
            
            self.service.spreadsheets().batchUpdate(
                spreadsheetId=self.spreadsheet_id,
                body=body
            ).execute()
            
            logger.info(f"색상 적용 완료: 행 {start_row}-{end_row}, 열 {start_col}-{end_col}")
            return True
            
        except HttpError as e:
            logger.error(f"색상 적용 실패: {e}")
            return False
    
    def apply_number_format(self, sheet_name: str, start_row: int, end_row: int,
                            columns: List[int], format_pattern: str) -> bool:
        """특정 범위에 숫자 포맷을 적용합니다

        Args:
            sheet_name: 시트 이름
            start_row: 시작 행
            end_row: 종료 행
            columns: 포맷 적용할 열 번호 리스트 (1-based)
            format_pattern: 포맷 패턴
                - "#,##0" : 천 단위 구분
                - "#,##0.00" : 소수점 2자리
                - "[$₩-412]#,##0" : 원화
                - "[$$-409]#,##0.00" : 달러

        Returns:
            성공 여부
        """
        try:
            # 시트 ID 가져오기
            spreadsheet = self.service.spreadsheets().get(
                spreadsheetId=self.spreadsheet_id
            ).execute()

            sheet_id = None
            for sheet in spreadsheet.get('sheets', []):
                if sheet['properties']['title'] == sheet_name:
                    sheet_id = sheet['properties']['sheetId']
                    break

            if sheet_id is None:
                logger.error(f"시트 '{sheet_name}'를 찾을 수 없습니다")
                return False

            # 각 열에 대해 포맷 적용 요청 생성
            requests = []
            for col in columns:
                requests.append({
                    'repeatCell': {
                        'range': {
                            'sheetId': sheet_id,
                            'startRowIndex': start_row - 1,  # 0-based index
                            'endRowIndex': end_row,
                            'startColumnIndex': col - 1,
                            'endColumnIndex': col
                        },
                        'cell': {
                            'userEnteredFormat': {
                                'numberFormat': {
                                    'type': 'NUMBER',
                                    'pattern': format_pattern
                                }
                            }
                        },
                        'fields': 'userEnteredFormat.numberFormat'
                    }
                })

            body = {'requests': requests}

            self.service.spreadsheets().batchUpdate(
                spreadsheetId=self.spreadsheet_id,
                body=body
            ).execute()

            logger.info(f"숫자 포맷 적용 완료: {sheet_name} 행 {start_row}-{end_row}, 열 {columns}")
            return True

        except HttpError as e:
            logger.error(f"숫자 포맷 적용 실패: {e}")
            return False

    def batch_apply_colors(self, sheet_name: str, color_ranges: List[Dict[str, Any]]) -> bool:
        """여러 범위에 한 번에 색상을 적용합니다
        
        Args:
            sheet_name: 시트 이름
            color_ranges: 색상 적용 정보 리스트
                [{
                    'start_row': int,
                    'end_row': int,
                    'start_col': int,
                    'end_col': int,
                    'color': Dict[str, float]
                }, ...]
        
        Returns:
            성공 여부
        """
        try:
            # 시트 ID 가져오기
            spreadsheet = self.service.spreadsheets().get(
                spreadsheetId=self.spreadsheet_id
            ).execute()
            
            sheet_id = None
            for sheet in spreadsheet.get('sheets', []):
                if sheet['properties']['title'] == sheet_name:
                    sheet_id = sheet['properties']['sheetId']
                    break
            
            if sheet_id is None:
                logger.error(f"시트 '{sheet_name}'를 찾을 수 없습니다")
                return False
            
            # 모든 색상 적용 요청을 한 번에 준비
            requests = []
            for color_range in color_ranges:
                requests.append({
                    'repeatCell': {
                        'range': {
                            'sheetId': sheet_id,
                            'startRowIndex': color_range['start_row'] - 1,  # 0-based index
                            'endRowIndex': color_range['end_row'],
                            'startColumnIndex': color_range['start_col'] - 1,
                            'endColumnIndex': color_range['end_col']
                        },
                        'cell': {
                            'userEnteredFormat': {
                                'backgroundColor': color_range['color']
                            }
                        },
                        'fields': 'userEnteredFormat.backgroundColor'
                    }
                })
            
            if not requests:
                logger.warning("색상 적용할 요청이 없습니다")
                return True
            
            body = {'requests': requests}
            
            self.service.spreadsheets().batchUpdate(
                spreadsheetId=self.spreadsheet_id,
                body=body
            ).execute()
            
            logger.info(f"배치 색상 적용 완료: {len(requests)}개 범위")
            return True

        except HttpError as e:
            logger.error(f"배치 색상 적용 실패: {e}")
            return False

    async def create_sheet(self, title: str) -> bool:
        """새 시트(탭)를 추가합니다

        Args:
            title: 시트 이름

        Returns:
            성공 여부
        """
        try:
            requests = [{"addSheet": {"properties": {"title": title}}}]
            body = {"requests": requests}
            self.service.spreadsheets().batchUpdate(
                spreadsheetId=self.spreadsheet_id,
                body=body
            ).execute()
            logger.info(f"시트 '{title}' 생성 완료")
            return True
        except HttpError as e:
            logger.error(f"시트 '{title}' 생성 실패: {e}")
            return False

    async def clear_sheet(self, sheet_name: str, start_row: int = 2) -> bool:
        """시트 데이터를 삭제합니다 (헤더 유지)

        Args:
            sheet_name: 시트 이름
            start_row: 삭제 시작 행 (기본: 2, 헤더 유지)

        Returns:
            성공 여부
        """
        try:
            self.service.spreadsheets().values().clear(
                spreadsheetId=self.spreadsheet_id,
                range=f"{sheet_name}!A{start_row}:Z",
                body={}
            ).execute()
            logger.info(f"시트 '{sheet_name}' 데이터 삭제 완료 (행 {start_row}부터)")
            return True
        except HttpError as e:
            logger.error(f"시트 '{sheet_name}' 데이터 삭제 실패: {e}")
            return False