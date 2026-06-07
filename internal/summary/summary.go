// Package summary 는 "대시보드" 단일 시트를 생성한다.
// Python modules/summary_generator.py 의 Go 포팅.
//
// 6개 섹션: 포트폴리오 요약, 월별 성과, 투자 지표, 매매 인사이트,
// 월별 성과 추이, 종목별 현황. (현재 Task 15/16 범위: 골격 + 1~3섹션)
package summary

import (
	"context"
	"log/slog"

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

	// 차트/포맷 수집 상태 (Task 18/19 가 채움)
	dashboardSheetID int64
}

// New 는 Generator 를 생성한다. sc 는 nil 일 수 있다.
// (Python SummaryGenerator.__init__)
func New(client *sheets.Client, w *writer.Writer, sc SectorClassifier) *Generator {
	return &Generator{client: client, writer: w, sc: sc}
}

// GenerateAll 은 대시보드 시트를 초기화 후 재작성한다. (Python generate_all)
func (g *Generator) GenerateAll(ctx context.Context, trades []model.Trade) error {
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

	// 아래는 Task 18/19 가 채운다 (포맷/색상/차트/거래건수 데이터).
	// 현재는 컴파일/실행이 되도록 변수만 참조해 둔다.
	_ = monthlyStart
	_ = metricsStart
	_ = insightsStart
	_ = trendStart
	_ = stockStart

	// Task 18 fills this: 헤더 색상/포맷 요청 수집
	//   g.collectHeaderColors(monthlyStart, trendStart, stockStart)
	//   g.collectDashboardFormats(monthlyStart, metricsStart, insightsStart,
	//       trendStart, stockStart, currentRow)
	// Task 18 fills this: 차트용 거래건수 데이터 작성
	//   g.writeTradeCountData(ctx, trades)
	// Task 18 fills this: 수집된 포맷/색상 요청 1회 전송
	//   g.flushPendingRequests(ctx)
	// Task 19 fills this: 차트 생성
	//   g.createCharts(ctx, trendStart, stockStart-1)

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

// ── Task 17 stubs ──────────────────────────────────────────
// Task 17 fills these: 투자지표/매매인사이트/월별추이.
// 현재는 startRow 를 그대로 반환한다(no-op).

// writeInvestmentMetrics: 섹션 4. (Python _write_investment_metrics, py:269-)
func (g *Generator) writeInvestmentMetrics(_ context.Context, _ []model.Trade, startRow int) (int, error) {
	return startRow, nil // Task 17 fills this
}

// writeTradingInsights: 섹션 5. (Python _write_trading_insights)
func (g *Generator) writeTradingInsights(_ context.Context, _ []model.Trade, startRow int) (int, error) {
	return startRow, nil // Task 17 fills this
}

// writeMonthlyTrend: 섹션 6(월별 성과 추이). (Python _write_monthly_trend)
func (g *Generator) writeMonthlyTrend(_ context.Context, _ []model.Trade, startRow int) (int, error) {
	return startRow, nil // Task 17 fills this
}
