// Command atj 는 증권사별 CSV 매매내역을 파싱해 Google Sheets 매매일지에 자동
// 입력하는 메인 오케스트레이션이다. (Python main.py 포팅)
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kenshin579/auto-trading-journal/internal/bizcat"
	"github.com/kenshin579/auto-trading-journal/internal/config"
	"github.com/kenshin579/auto-trading-journal/internal/model"
	"github.com/kenshin579/auto-trading-journal/internal/parser"
	"github.com/kenshin579/auto-trading-journal/internal/sector"
	"github.com/kenshin579/auto-trading-journal/internal/sheets"
	"github.com/kenshin579/auto-trading-journal/internal/summary"
	"github.com/kenshin579/auto-trading-journal/internal/symbol"
	"github.com/kenshin579/auto-trading-journal/internal/writer"
	"golang.org/x/text/unicode/norm"
)

const (
	configPath = "config/config.yaml"
	inputDir   = "input"
)

// resolver 는 종목명 → 종목코드 조회를 추상화한다(테스트용 스텁 주입 가능).
type resolver interface {
	Resolve(name string) string
}

// csvFile 은 스캔된 CSV 한 건을 나타낸다. (증권사명, 계좌유형, 파일경로)
type csvFile struct {
	broker      string
	accountType string
	path        string
}

// enrichDomesticCodes 는 국내 거래 중 종목코드가 비어있는 항목을 KRX 마스터로
// 보강한다(in-place). 해외 거래와 이미 코드가 있는 거래는 건드리지 않는다.
// (Python enrich_domestic_codes)
func enrichDomesticCodes(trades []model.Trade, r resolver) {
	for i := range trades {
		t := &trades[i]
		if t.IsDomestic() && t.StockCode == "" {
			if code := r.Resolve(t.StockName); code != "" {
				t.StockCode = code
			}
		}
	}
}

// bizcatResolver 는 종목코드 → (섹터, 산업) 조회를 추상화한다(테스트 스텁 주입용).
type bizcatResolver interface {
	Resolve(code string) (sector, industry string)
}

// enrichSectors 는 국내 거래(코드 있음)에 섹터/산업을 채운다(in-place). 해외/무코드는 스킵.
func enrichSectors(trades []model.Trade, r bizcatResolver) {
	for i := range trades {
		t := &trades[i]
		if t.IsDomestic() && t.StockCode != "" {
			t.Sector, t.Industry = r.Resolve(t.StockCode)
		}
	}
}

// scanCSVFiles 는 input/ 하위 CSV 파일을 스캔한다. 디렉토리/파일 모두 정렬되며,
// 폴더명/파일명(확장자 제외)은 macOS NFD 대응을 위해 NFC 로 정규화한다.
// (Python scan_csv_files)
func scanCSVFiles(root string) ([]csvFile, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Warn("input/ 디렉토리가 존재하지 않습니다")
			return nil, nil
		}
		return nil, err
	}

	// 디렉토리 정렬 (Python sorted(input_dir.iterdir()))
	dirNames := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			dirNames = append(dirNames, e.Name())
		}
	}
	sort.Strings(dirNames)

	var results []csvFile
	for _, dirName := range dirNames {
		brokerPath := filepath.Join(root, dirName)
		brokerEntries, err := os.ReadDir(brokerPath)
		if err != nil {
			return nil, err
		}
		// CSV 파일 정렬 (Python sorted(broker_dir.glob("*.csv")))
		var csvNames []string
		for _, fe := range brokerEntries {
			if fe.IsDir() {
				continue
			}
			if strings.EqualFold(filepath.Ext(fe.Name()), ".csv") {
				csvNames = append(csvNames, fe.Name())
			}
		}
		sort.Strings(csvNames)

		for _, name := range csvNames {
			brokerName := norm.NFC.String(dirName)
			stem := strings.TrimSuffix(name, filepath.Ext(name))
			accountType := norm.NFC.String(stem)
			results = append(results, csvFile{
				broker:      brokerName,
				accountType: accountType,
				path:        filepath.Join(brokerPath, name),
			})
		}
	}

	slog.Info(fmt.Sprintf("CSV 파일 %d개 발견", len(results)))
	return results, nil
}

// processor 는 주식 데이터 처리기. (Python StockDataProcessor)
type processor struct {
	dryRun     bool
	client     *sheets.Client
	writer     *writer.Writer
	summary    *summary.Generator
	symbolRes  resolver
	bizcatRes  bizcatResolver
	bizcatStore *bizcat.Resolver
}

// newProcessor 는 config 를 로드하고 모든 의존성을 조립한다.
// (Python StockDataProcessor.__init__)
func newProcessor(ctx context.Context, dryRun bool, cfg *config.Config) (*processor, error) {
	if dryRun {
		slog.Info("=== 드라이런 모드로 실행 중 (실제 데이터 입력 없음) ===")
	}

	spreadsheetID := cfg.SpreadsheetID()
	if spreadsheetID == "" {
		return nil, fmt.Errorf("환경변수 GOOGLE_SPREADSHEET_ID 또는 설정 파일에 spreadsheet_id가 없습니다")
	}

	client, err := sheets.New(ctx, spreadsheetID, cfg.ServiceAccountPath)
	if err != nil {
		return nil, err
	}
	w := writer.New(client)

	// OpenAI 섹터 분류 (optional). nil 가능.
	var sc summary.SectorClassifier
	if cfg.OpenAIAPIKey() != "" {
		sc = sector.New(cfg.OpenAIAPIKey(), cfg.OpenAI.Model, cfg.OpenAI.SectorCacheFile)
		slog.Info("OpenAI 섹터 분류 활성화")
	} else {
		slog.Info("STOCK_DATA_OPENAI_API_KEY 미설정 → 섹터 분류 비활성화")
	}

	bc := bizcat.New("config/bizcat_cache.json")

	return &processor{
		dryRun:      dryRun,
		client:      client,
		writer:      w,
		summary:     summary.New(client, w, sc),
		symbolRes:   symbol.New(),
		bizcatRes:   bc,
		bizcatStore: bc,
	}, nil
}

// processFile 은 CSV 파일 하나를 처리해 시트에 삽입하고, 요약용으로 전체 Trade
// 리스트를 반환한다. (Python process_file)
func (p *processor) processFile(ctx context.Context, f csvFile) ([]model.Trade, error) {
	sheetName := fmt.Sprintf("%s_%s", f.broker, f.accountType)
	account := sheetName
	isForeign := strings.Contains(f.accountType, "해외")

	// 1. 파서 감지 및 파싱
	prs, err := parser.DetectParser(f.path)
	if err != nil {
		slog.Error(fmt.Sprintf("파서 감지 실패: %v", err))
		return nil, nil
	}

	trades, err := prs.Parse(f.path, account)
	if err != nil {
		return nil, err
	}
	enrichDomesticCodes(trades, p.symbolRes)
	enrichSectors(trades, p.bizcatRes)
	if len(trades) == 0 {
		slog.Warn(fmt.Sprintf("파싱 결과 없음: %s", filepath.Base(f.path)))
		return nil, nil
	}

	// 날짜순 정렬 (안정 정렬로 Python list.sort 동등)
	sort.SliceStable(trades, func(i, j int) bool {
		return trades[i].Date < trades[j].Date
	})

	// 2. 시트 존재 확인 → 없으면 생성
	created, err := p.writer.EnsureSheetExists(ctx, sheetName, isForeign)
	if err != nil {
		return nil, err
	}
	if created {
		slog.Info(fmt.Sprintf("새 시트 생성됨: %s", sheetName))
	}

	// 3. 중복 필터링
	existingKeys, err := p.writer.GetExistingKeys(ctx, sheetName, isForeign)
	if err != nil {
		return nil, err
	}
	newTrades := make([]model.Trade, 0, len(trades))
	for _, t := range trades {
		if !existingKeys[t.DuplicateKey()] {
			newTrades = append(newTrades, t)
		}
	}
	skipped := len(trades) - len(newTrades)
	if skipped > 0 {
		slog.Info(fmt.Sprintf("%d건 중복 건너뜀 (%s)", skipped, sheetName))
	}

	if len(newTrades) == 0 {
		slog.Info(fmt.Sprintf("신규 거래 없음: %s", sheetName))
		return trades, nil // 요약용으로 전체 반환
	}

	// 4. 시트에 삽입
	if !p.dryRun {
		inserted, err := p.writer.InsertTrades(ctx, sheetName, newTrades, isForeign)
		if err != nil {
			return nil, err
		}
		slog.Info(fmt.Sprintf("%s: %d건 삽입 완료", sheetName, inserted))
	} else {
		slog.Info(fmt.Sprintf("[DRY-RUN] %s: %d건 삽입 예정", sheetName, len(newTrades)))
	}

	return trades, nil // 요약용으로 전체 반환
}

// run 은 메인 실행: CSV 스캔 → 파일별 처리 → 대시보드 갱신. (Python run)
func (p *processor) run(ctx context.Context) error {
	slog.Info("=== 매매일지 구글 시트 입력 시작 (v2) ===")
	defer slog.Info("스크립트 실행 완료")
	if p.bizcatStore != nil {
		defer p.bizcatStore.Save()
	}

	// 1. CSV 파일 스캔 및 시트 삽입
	csvFiles, err := scanCSVFiles(inputDir)
	if err != nil {
		return err
	}
	totalCSVTrades := 0

	if len(csvFiles) > 0 {
		for i, f := range csvFiles {
			name := filepath.Base(f.path)
			slog.Info(fmt.Sprintf("[%d/%d] %s 처리 중...", i+1, len(csvFiles), name))
			trades, err := p.processFile(ctx, f)
			if err != nil {
				slog.Error(fmt.Sprintf("[%d/%d] %s 처리 실패: %v", i+1, len(csvFiles), name, err))
				continue
			}
			totalCSVTrades += len(trades)
			slog.Info(fmt.Sprintf("[%d/%d] %s 완료 (%d건)", i+1, len(csvFiles), name, len(trades)))
		}
	} else {
		slog.Info("처리할 CSV 파일이 없습니다")
	}

	// 2. 대시보드 갱신 (매매일지 시트에서 전체 데이터 읽기)
	if !p.dryRun {
		slog.Info("=== 대시보드 갱신 중 ===")
		allTrades, err := p.writer.ReadAllTrades(ctx)
		if err != nil {
			return err
		}
		if len(allTrades) > 0 {
			if err := p.summary.GenerateAll(ctx, allTrades); err != nil {
				return err
			}
		} else {
			slog.Warn("매매일지 시트에 데이터가 없어 대시보드를 갱신하지 않습니다")
		}
	} else {
		slog.Info(fmt.Sprintf("[DRY-RUN] 대시보드 갱신 예정 (CSV %d건 처리)", totalCSVTrades))
	}

	// 3. 결과 출력
	slog.Info("=== 전체 처리 결과 ===")
	slog.Info(fmt.Sprintf("CSV %d건 처리 완료", totalCSVTrades))
	return nil
}

// parseLogLevel 은 문자열(DEBUG/INFO/WARNING/ERROR)을 slog.Level 로 매핑한다.
func parseLogLevel(s string) (slog.Level, error) {
	switch strings.ToUpper(s) {
	case "DEBUG":
		return slog.LevelDebug, nil
	case "INFO":
		return slog.LevelInfo, nil
	case "WARNING":
		return slog.LevelWarn, nil
	case "ERROR":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("잘못된 로그 레벨: %s (DEBUG/INFO/WARNING/ERROR 중 하나)", s)
	}
}

// ensureKISFileToken 은 atj 프로세스가 KIS 토큰을 파일에 캐시하도록 강제한다.
// 사용자 env(KOREA_INVESTMENT_TOKEN_STORAGE=redis)가 로컬 Redis 를 가리켜도 Redis 미동작 시
// 매 호출 토큰 재발급 → KIS 발급 제한(403/500)에 걸린다. atj 는 파일 토큰으로 동작해 Redis
// 의존을 제거한다. os.Setenv 는 현재 프로세스에만 적용되어 다른 KIS 도구 env 에는 영향 없다.
func ensureKISFileToken() {
	os.Setenv("KOREA_INVESTMENT_TOKEN_STORAGE", "file")
	if os.Getenv("KOREA_INVESTMENT_TOKEN_FILE") == "" {
		home, _ := os.UserHomeDir()
		os.Setenv("KOREA_INVESTMENT_TOKEN_FILE",
			filepath.Join(home, ".cache", "auto-trading-journal", "kis_token.json"))
	}
}

func main() {
	dryRun := flag.Bool("dry-run", false, "실제 데이터 입력 없이 시뮬레이션만 수행합니다.")
	logLevel := flag.String("log-level", "INFO", "로그 레벨을 설정합니다 (DEBUG/INFO/WARNING/ERROR, 기본값: INFO)")
	flag.Parse()

	ensureKISFileToken()

	level, err := parseLogLevel(*logLevel)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	ctx := context.Background()

	cfg, err := config.Load(configPath)
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}

	p, err := newProcessor(ctx, *dryRun, cfg)
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}

	if err := p.run(ctx); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}
