#!/usr/bin/env python3
"""
SheetManager의 find_last_row_with_column_info 함수에 대한 단위 테스트
"""

import asyncio
import pytest
from unittest.mock import Mock, patch, AsyncMock
from .sheet_manager import SheetManager


class TestFindLastRowWithColumnInfo:
    """find_last_row_with_column_info 함수 테스트"""
    
    @pytest.fixture
    def sheet_manager(self):
        """SheetManager 인스턴스 생성"""
        return SheetManager("test-spreadsheet-id")
    
    @pytest.mark.asyncio
    async def test_empty_sheet(self, sheet_manager):
        """빈 시트 테스트"""
        # 빈 응답 모킹
        sheet_manager.client.get_sheet_data = AsyncMock(return_value={
            'sheets': [{
                'data': [{
                    'rowData': []
                }]
            }]
        })
        
        last_row, start_col, end_col = await sheet_manager.find_last_row_with_column_info("test_sheet")
        
        assert last_row == 2  # 헤더 다음 행
        assert start_col == 2  # B열 (기본값)
        assert end_col == 9    # I열 (기본값)
    
    @pytest.mark.asyncio
    async def test_single_row_with_data(self, sheet_manager):
        """단일 행 데이터 테스트"""
        # 한 행의 데이터 (C열부터 E열까지)
        sheet_manager.client.get_sheet_data = AsyncMock(return_value={
            'sheets': [{
                'data': [{
                    'rowData': [
                        {  # 1행 (헤더)
                            'values': [
                                {'effectiveValue': {'stringValue': '종목'}},
                                {'effectiveValue': {'stringValue': '증권사'}},
                                {'effectiveValue': {'stringValue': '일자'}},
                                {'effectiveValue': {'stringValue': '유형'}},
                                {'effectiveValue': {'stringValue': '수량'}}
                            ]
                        },
                        {  # 2행 (데이터)
                            'values': [
                                {},  # A열 (비어있음)
                                {},  # B열 (비어있음)
                                {'effectiveValue': {'stringValue': '삼성전자'}},  # C열
                                {'effectiveValue': {'stringValue': '키움증권'}},  # D열
                                {'effectiveValue': {'stringValue': '2024-12-01'}}  # E열
                            ]
                        }
                    ]
                }]
            }]
        })
        
        last_row, start_col, end_col = await sheet_manager.find_last_row_with_column_info("test_sheet")
        
        assert last_row == 3      # 마지막 데이터 행(2) + 1
        assert start_col == 3     # C열 (3번째 컬럼)
        assert end_col == 5       # E열 (5번째 컬럼)
    
    @pytest.mark.asyncio
    async def test_multiple_rows_different_columns(self, sheet_manager):
        """여러 행의 데이터가 서로 다른 컬럼 범위를 가질 때"""
        sheet_manager.client.get_sheet_data = AsyncMock(return_value={
            'sheets': [{
                'data': [{
                    'rowData': [
                        {  # 1행 (헤더) - B열부터 I열까지
                            'values': [
                                {},
                                {'effectiveValue': {'stringValue': '종목'}},
                                {'effectiveValue': {'stringValue': '증권사'}},
                                {'effectiveValue': {'stringValue': '일자'}},
                                {'effectiveValue': {'stringValue': '유형'}},
                                {'effectiveValue': {'stringValue': '수량'}},
                                {'effectiveValue': {'stringValue': '단가'}},
                                {'effectiveValue': {'stringValue': '금액'}},
                                {'effectiveValue': {'stringValue': '계좌'}}
                            ]
                        },
                        {  # 2행 - B열부터 E열까지
                            'values': [
                                {},
                                {'effectiveValue': {'stringValue': '삼성전자'}},
                                {'effectiveValue': {'stringValue': '키움'}},
                                {'effectiveValue': {'stringValue': '2024-12-01'}},
                                {'effectiveValue': {'stringValue': '매수'}}
                            ]
                        },
                        {  # 3행 - C열부터 F열까지
                            'values': [
                                {},
                                {},
                                {'effectiveValue': {'stringValue': 'SK하이닉스'}},
                                {'effectiveValue': {'stringValue': '미래에셋'}},
                                {'effectiveValue': {'stringValue': '2024-12-02'}},
                                {'effectiveValue': {'stringValue': '매도'}}
                            ]
                        },
                        {  # 4행 (마지막 데이터 행) - D열부터 G열까지
                            'values': [
                                {},
                                {},
                                {},
                                {'effectiveValue': {'stringValue': 'KODEX 200'}},
                                {'effectiveValue': {'stringValue': 'KB증권'}},
                                {'effectiveValue': {'stringValue': '2024-12-03'}},
                                {'effectiveValue': {'stringValue': '매수'}}
                            ]
                        }
                    ]
                }]
            }]
        })
        
        last_row, start_col, end_col = await sheet_manager.find_last_row_with_column_info("test_sheet")
        
        assert last_row == 5      # 마지막 데이터 행(4) + 1
        assert start_col == 4     # D열 (마지막 행의 시작 컬럼)
        assert end_col == 7       # G열 (마지막 행의 끝 컬럼)
    
    @pytest.mark.asyncio
    async def test_with_empty_rows_in_between(self, sheet_manager):
        """중간에 빈 행이 있는 경우"""
        sheet_manager.client.get_sheet_data = AsyncMock(return_value={
            'sheets': [{
                'data': [{
                    'rowData': [
                        {  # 1행 (헤더)
                            'values': [
                                {'effectiveValue': {'stringValue': '종목'}},
                                {'effectiveValue': {'stringValue': '증권사'}}
                            ]
                        },
                        {  # 2행 (데이터)
                            'values': [
                                {'effectiveValue': {'stringValue': '삼성전자'}},
                                {'effectiveValue': {'stringValue': '키움'}}
                            ]
                        },
                        {  # 3행 (빈 행)
                            'values': []
                        },
                        {  # 4행 (빈 행)
                            'values': []
                        },
                        {  # 5행 (마지막 데이터)
                            'values': [
                                {},
                                {},
                                {'effectiveValue': {'stringValue': 'SK하이닉스'}},
                                {'effectiveValue': {'stringValue': '미래에셋'}},
                                {'effectiveValue': {'stringValue': '2024-12-02'}}
                            ]
                        }
                    ]
                }]
            }]
        })
        
        last_row, start_col, end_col = await sheet_manager.find_last_row_with_column_info("test_sheet")
        
        assert last_row == 6      # 마지막 데이터 행(5) + 1
        assert start_col == 3     # C열 (마지막 행의 시작 컬럼)
        assert end_col == 5       # E열 (마지막 행의 끝 컬럼)
    
    @pytest.mark.asyncio
    async def test_exceeds_empty_row_threshold(self, sheet_manager):
        """빈 행이 임계값을 초과하는 경우"""
        # 100개 이상의 빈 행 생성
        row_data = [
            {  # 헤더
                'values': [
                    {'effectiveValue': {'stringValue': '종목'}},
                    {'effectiveValue': {'stringValue': '증권사'}}
                ]
            },
            {  # 데이터 행
                'values': [
                    {'effectiveValue': {'stringValue': '삼성전자'}},
                    {'effectiveValue': {'stringValue': '키움'}}
                ]
            }
        ]
        
        # 100개의 빈 행 추가
        for _ in range(100):
            row_data.append({'values': []})
        
        sheet_manager.client.get_sheet_data = AsyncMock(return_value={
            'sheets': [{
                'data': [{
                    'rowData': row_data
                }]
            }]
        })
        
        last_row, start_col, end_col = await sheet_manager.find_last_row_with_column_info("test_sheet")
        
        assert last_row == 3      # 마지막 데이터 행(2) + 1
        assert start_col == 1     # A열 (마지막 데이터 행의 시작)
        assert end_col == 2       # B열 (마지막 데이터 행의 끝)
    
    @pytest.mark.asyncio
    async def test_error_handling(self, sheet_manager):
        """API 호출 오류 처리 테스트"""
        # API 호출 시 예외 발생
        sheet_manager.client.get_sheet_data = AsyncMock(side_effect=Exception("API Error"))
        
        last_row, start_col, end_col = await sheet_manager.find_last_row_with_column_info("test_sheet")
        
        # 기본값 반환 확인
        assert last_row == 2      # 기본값
        assert start_col == 2     # B열 (기본값)
        assert end_col == 9       # I열 (기본값)
    
    @pytest.mark.asyncio
    async def test_only_number_values(self, sheet_manager):
        """숫자값만 있는 경우"""
        sheet_manager.client.get_sheet_data = AsyncMock(return_value={
            'sheets': [{
                'data': [{
                    'rowData': [
                        {  # 1행
                            'values': [
                                {'effectiveValue': {'numberValue': 100}},
                                {'effectiveValue': {'numberValue': 200}},
                                {'effectiveValue': {'numberValue': 300}}
                            ]
                        },
                        {  # 2행 (마지막)
                            'values': [
                                {},
                                {'effectiveValue': {'numberValue': 50}},
                                {'effectiveValue': {'numberValue': 100}},
                                {'effectiveValue': {'numberValue': 150}},
                                {'effectiveValue': {'numberValue': 200}}
                            ]
                        }
                    ]
                }]
            }]
        })
        
        last_row, start_col, end_col = await sheet_manager.find_last_row_with_column_info("test_sheet")
        
        assert last_row == 3      # 마지막 데이터 행(2) + 1
        assert start_col == 2     # B열
        assert end_col == 5       # E열
    
    @pytest.mark.asyncio
    async def test_mixed_value_types(self, sheet_manager):
        """다양한 타입의 값이 섞여 있는 경우"""
        sheet_manager.client.get_sheet_data = AsyncMock(return_value={
            'sheets': [{
                'data': [{
                    'rowData': [
                        {  # 1행 (마지막 데이터 행)
                            'values': [
                                {'effectiveValue': {'stringValue': 'Text'}},
                                {'effectiveValue': {'numberValue': 123}},
                                {'effectiveValue': {'boolValue': True}},
                                {},  # 빈 셀
                                {'effectiveValue': {'stringValue': 'End'}}
                            ]
                        }
                    ]
                }]
            }]
        })
        
        last_row, start_col, end_col = await sheet_manager.find_last_row_with_column_info("test_sheet")
        
        assert last_row == 2      # 마지막 데이터 행(1) + 1
        assert start_col == 1     # A열
        assert end_col == 5       # E열 (중간에 빈 셀이 있어도 마지막 데이터까지)


if __name__ == "__main__":
    pytest.main([__file__, "-v"]) 