"""대시보드 차트 빌더 단위 테스트"""

import pytest

from modules.summary_generator import SummaryGenerator


class TestBuildBasicChartSpec:
    """_build_basic_chart_spec() 단위 테스트"""

    def test_column_chart_structure(self):
        """Column 차트 스펙의 기본 구조 검증"""
        spec = SummaryGenerator._build_basic_chart_spec(
            sheet_id=123, title="월별 실현손익 추이",
            chart_type="COLUMN",
            domain_col=0, series_cols=[2],
            data_start=10, data_end=20,
            anchor_row=0, anchor_col=13,
            width=600, height=370,
        )

        assert "spec" in spec
        assert "position" in spec

        basic = spec["spec"]["basicChart"]
        assert basic["chartType"] == "COLUMN"
        assert basic["headerCount"] == 1
        assert basic["legendPosition"] == "BOTTOM_LEGEND"

    def test_source_range_indices(self):
        """sourceRange의 행/열 인덱스가 정확한지 검증"""
        spec = SummaryGenerator._build_basic_chart_spec(
            sheet_id=42, title="test",
            chart_type="LINE",
            domain_col=0, series_cols=[3, 4],
            data_start=5, data_end=15,
            anchor_row=0, anchor_col=13,
        )

        basic = spec["spec"]["basicChart"]

        # domain (X축) 검증
        domain_src = basic["domains"][0]["domain"]["sourceRange"]["sources"][0]
        assert domain_src["sheetId"] == 42
        assert domain_src["startRowIndex"] == 5
        assert domain_src["endRowIndex"] == 15
        assert domain_src["startColumnIndex"] == 0
        assert domain_src["endColumnIndex"] == 1

        # series (Y축) 검증 - 2개 시리즈
        assert len(basic["series"]) == 2

        s1 = basic["series"][0]["series"]["sourceRange"]["sources"][0]
        assert s1["startColumnIndex"] == 3
        assert s1["endColumnIndex"] == 4

        s2 = basic["series"][1]["series"]["sourceRange"]["sources"][0]
        assert s2["startColumnIndex"] == 4
        assert s2["endColumnIndex"] == 5

    def test_anchor_position(self):
        """차트 배치 위치 검증"""
        spec = SummaryGenerator._build_basic_chart_spec(
            sheet_id=1, title="test",
            chart_type="COLUMN",
            domain_col=0, series_cols=[2],
            data_start=0, data_end=10,
            anchor_row=20, anchor_col=13,
            width=600, height=370,
        )

        pos = spec["position"]["overlayPosition"]
        assert pos["anchorCell"]["rowIndex"] == 20
        assert pos["anchorCell"]["columnIndex"] == 13
        assert pos["anchorCell"]["sheetId"] == 1
        assert pos["widthPixels"] == 600
        assert pos["heightPixels"] == 370

    def test_single_series(self):
        """시리즈 1개일 때 구조 검증"""
        spec = SummaryGenerator._build_basic_chart_spec(
            sheet_id=1, title="test",
            chart_type="COLUMN",
            domain_col=0, series_cols=[2],
            data_start=0, data_end=5,
            anchor_row=0, anchor_col=13,
        )

        series = spec["spec"]["basicChart"]["series"]
        assert len(series) == 1
        assert series[0]["targetAxis"] == "LEFT_AXIS"

    def test_line_chart_type(self):
        """LINE 차트 타입 설정 검증"""
        spec = SummaryGenerator._build_basic_chart_spec(
            sheet_id=1, title="추이",
            chart_type="LINE",
            domain_col=0, series_cols=[7, 8],
            data_start=0, data_end=10,
            anchor_row=40, anchor_col=20,
            width=450, height=370,
        )

        assert spec["spec"]["basicChart"]["chartType"] == "LINE"
        assert spec["position"]["overlayPosition"]["widthPixels"] == 450

    def test_title_set(self):
        """차트 제목이 spec에 포함되는지 검증"""
        spec = SummaryGenerator._build_basic_chart_spec(
            sheet_id=1, title="월별 승률 & 수익률 추이",
            chart_type="LINE",
            domain_col=0, series_cols=[3],
            data_start=0, data_end=5,
            anchor_row=0, anchor_col=13,
        )

        assert spec["spec"]["title"] == "월별 승률 & 수익률 추이"


class TestBuildPieChartSpec:
    """_build_pie_chart_spec() 단위 테스트"""

    def test_pie_chart_structure(self):
        """Pie 차트 스펙의 기본 구조 검증"""
        spec = SummaryGenerator._build_pie_chart_spec(
            sheet_id=123, title="계좌별 투자비중",
            label_col=13, value_col=14,
            data_start=5, data_end=8,
            anchor_row=40, anchor_col=13,
            width=450, height=370,
        )

        assert "spec" in spec
        assert "position" in spec
        assert "pieChart" in spec["spec"]
        assert spec["spec"]["title"] == "계좌별 투자비중"

        pie = spec["spec"]["pieChart"]
        assert pie["legendPosition"] == "RIGHT_LEGEND"

    def test_pie_source_ranges(self):
        """Pie 차트의 domain/series sourceRange 검증"""
        spec = SummaryGenerator._build_pie_chart_spec(
            sheet_id=42, title="test",
            label_col=13, value_col=14,
            data_start=10, data_end=13,
            anchor_row=40, anchor_col=13,
        )

        pie = spec["spec"]["pieChart"]

        # domain (레이블)
        domain_src = pie["domain"]["sourceRange"]["sources"][0]
        assert domain_src["sheetId"] == 42
        assert domain_src["startRowIndex"] == 10
        assert domain_src["endRowIndex"] == 13
        assert domain_src["startColumnIndex"] == 13
        assert domain_src["endColumnIndex"] == 14

        # series (값)
        series_src = pie["series"]["sourceRange"]["sources"][0]
        assert series_src["startColumnIndex"] == 14
        assert series_src["endColumnIndex"] == 15

    def test_pie_anchor_position(self):
        """Pie 차트 배치 위치 검증"""
        spec = SummaryGenerator._build_pie_chart_spec(
            sheet_id=1, title="test",
            label_col=13, value_col=14,
            data_start=0, data_end=3,
            anchor_row=40, anchor_col=13,
            width=450, height=370,
        )

        pos = spec["position"]["overlayPosition"]
        assert pos["anchorCell"]["rowIndex"] == 40
        assert pos["anchorCell"]["columnIndex"] == 13
        assert pos["widthPixels"] == 450
        assert pos["heightPixels"] == 370
