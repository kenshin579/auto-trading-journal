// Package summary 는 "대시보드" 단일 시트를 생성한다.
// Python modules/summary_generator.py 의 Go 포팅.
//
// 6개 섹션: 포트폴리오 요약, 월별 성과, 투자 지표, 매매 인사이트,
// 월별 성과 추이, 종목별 현황. (현재 Task 15/16 범위: 골격 + 1~3섹션)
package summary

import (
	"context"
	"log/slog"

	gsheets "google.golang.org/api/sheets/v4"

	"github.com/kenshin579/auto-trading-journal/internal/model"
	"github.com/kenshin579/auto-trading-journal/internal/sheets"
	"github.com/kenshin579/auto-trading-journal/internal/writer"
)

// DashboardSheet 는 대시보드 시트 이름. (Python DASHBOARD_SHEET)
const DashboardSheet = "대시보드"

// 차트 배치 상수 (0-based 열/행 인덱스). (Python CHART_COL_START 등)
const (
	chartColStart     = 13 // N열
	chartColSecondary = 20 // U열
	chartRowSpacing   = 20 // 차트 간 행 간격
)

// SectorStock 은 섹터 분류 입력 단위. (Task 20 internal/sector 가 사용)
type SectorStock struct {
	Name, Code, Currency string
}

// SectorClassifier 는 종목을 섹터로 분류한다(Task 20 internal/sector 가 구현).
// Generator 가 sector 패키지에 빌드 의존하지 않도록 인터페이스로 분리한다.
type SectorClassifier interface {
	Classify(ctx context.Context, stocks []SectorStock) (map[string]string, error)
}

// Generator 는 대시보드 시트 생성기. (Python SummaryGenerator)
type Generator struct {
	client *sheets.Client
	writer *writer.Writer
	sc     SectorClassifier // nil 가능

	// 차트/포맷 수집 상태.
	dashboardSheetID int64
	pendingRequests  []*gsheets.Request // 포맷/색상 요청 누적 (#57 1회 batchUpdate)

	// 차트가 참조할 데이터 범위 (start,end 1-based, ok=true 일 때 유효).
	pieDataRange        rowRange // 파이 차트용 데이터 (계좌별 투자비중)
	tradeCountDataRange rowRange // 월별 매수/매도 건수·금액
}

// rowRange 는 차트가 참조할 행 범위 (1-based, inclusive). ok=false 면 데이터 없음.
type rowRange struct {
	start, end int
	ok         bool
}

// New 는 Generator 를 생성한다. sc 는 nil 일 수 있다.
// (Python SummaryGenerator.__init__)
func New(client *sheets.Client, w *writer.Writer, sc SectorClassifier) *Generator {
	return &Generator{client: client, writer: w, sc: sc}
}

// GenerateAll 은 대시보드 시트를 초기화 후 재작성한다. (Python generate_all)
func (g *Generator) GenerateAll(ctx context.Context, trades []model.Trade) error {
	g.pendingRequests = nil
	if err := g.EnsureDashboardSheet(ctx); err != nil {
		return err
	}

	// sheet_id 1회 조회 후 캐시.
	sheetID, _, err := g.client.GetSheetID(ctx, DashboardSheet)
	if err != nil {
		return err
	}
	g.dashboardSheetID = sheetID

	currentRow := 1
	if currentRow, err = g.writePortfolioSummary(ctx, trades, currentRow); err != nil {
		return err
	}
	currentRow++ // 빈 행
	monthlyStart := currentRow
	if currentRow, err = g.writeMonthlySummary(ctx, trades, currentRow); err != nil {
		return err
	}
	currentRow++ // 빈 행
	metricsStart := currentRow
	if currentRow, err = g.writeInvestmentMetrics(ctx, trades, currentRow); err != nil {
		return err
	}
	currentRow++ // 빈 행
	insightsStart := currentRow
	if currentRow, err = g.writeTradingInsights(ctx, trades, currentRow); err != nil {
		return err
	}
	currentRow++ // 빈 행
	trendStart := currentRow
	if currentRow, err = g.writeMonthlyTrend(ctx, trades, currentRow); err != nil {
		return err
	}
	currentRow++ // 빈 행
	stockStart := currentRow
	if currentRow, err = g.writeStockSummary(ctx, trades, currentRow); err != nil {
		return err
	}

	// 포맷/색상 요청을 pendingRequests 에 수집. (Python generate_all py:69-73)
	g.collectHeaderColors(monthlyStart, trendStart, stockStart)
	g.collectDashboardFormats(monthlyStart, metricsStart, insightsStart,
		trendStart, stockStart, currentRow)

	// 차트용 거래건수 데이터 작성 (포맷 요청도 수집됨). (Python py:76)
	if err := g.writeTradeCountData(ctx, trades); err != nil {
		return err
	}

	// 수집된 포맷/색상 요청을 1회 batchUpdate 로 전송. (#57) (Python py:79)
	if err := g.flushPendingRequests(ctx); err != nil {
		return err
	}

	// 차트 생성. (Python py:81-85)
	if err := g.createCharts(ctx, trendStart, stockStart-1); err != nil {
		return err
	}

	slog.Info("대시보드 시트 갱신 완료")
	return nil
}

// EnsureDashboardSheet 는 대시보드 시트를 확보한다(없으면 생성, 있으면 초기화).
// (Python _ensure_dashboard_sheet)
func (g *Generator) EnsureDashboardSheet(ctx context.Context) error {
	names, err := g.client.ListSheets(ctx)
	if err != nil {
		return err
	}
	exists := false
	for _, n := range names {
		if n == DashboardSheet {
			exists = true
			break
		}
	}

	if !exists {
		return g.client.CreateSheet(ctx, DashboardSheet)
	}

	// 데이터 삭제 (values:clear — 별도 엔드포인트). Python clear_sheet(start_row=1) → "A1:Z".
	if err := g.client.ClearValues(ctx, DashboardSheet+"!A1:Z"); err != nil {
		return err
	}

	// 배경색 초기화 (기본 1000행/26열).
	if err := g.client.ClearBackgroundColors(ctx, DashboardSheet, 1000, 26); err != nil {
		return err
	}
	// 숫자 포맷 초기화.
	if err := g.client.ClearNumberFormats(ctx, DashboardSheet, 1000, 26); err != nil {
		return err
	}
	// 차트 삭제.
	if err := g.client.DeleteAllCharts(ctx, DashboardSheet); err != nil {
		return err
	}
	return nil
}
