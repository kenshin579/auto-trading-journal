// Package bizcat 는 KIS 지수업종(대분류=섹터 / 중분류=산업)을 종목코드로 조회한다.
// (기존 internal/sector(OpenAI, 대시보드 GICS)와 별개 — per-row 열 전용.)
package bizcat

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"sync"

	kis "github.com/kenshin579/korea-investment-stock"
	"github.com/kenshin579/korea-investment-stock/domestic"
)

type entry struct {
	Sector   string `json:"sector"`
	Industry string `json:"industry"`
}

type Resolver struct {
	mu        sync.Mutex
	cache     map[string]entry
	failed    map[string]bool // 이번 실행에서 실패한 코드(negative cache, 비영구)
	cachePath string
	fetch     func(code string) (sector, industry string, err error)
	dirty     bool
}

func New(cachePath string) *Resolver {
	r := &Resolver{cache: map[string]entry{}, cachePath: cachePath}
	if data, err := os.ReadFile(cachePath); err == nil {
		var m map[string]entry
		if json.Unmarshal(data, &m) == nil && m != nil {
			r.cache = m
		} else {
			slog.Warn("bizcat 캐시 로드 실패, 빈 캐시로 시작", "path", cachePath)
		}
	}
	return r
}

func (r *Resolver) cacheLookup(code string) (string, bool) {
	e, ok := r.cache[code]
	return e.Sector, ok
}

func (r *Resolver) Resolve(code string) (string, string) {
	if code == "" {
		return "", ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	cached, hasCached := r.cache[code]
	if hasCached && !needsRefresh(cached) {
		return cached.Sector, cached.Industry
	}
	if r.failed[code] {
		// 이번 실행에서 이미 실패 → 재조회 안 함. 자가치유 대상이면 기존 캐시 보존.
		return cached.Sector, cached.Industry
	}
	if r.fetch == nil {
		r.fetch = kisFetch()
	}
	sector, industry, err := r.fetch(code)
	if err != nil {
		if r.failed == nil {
			r.failed = map[string]bool{}
		}
		r.failed[code] = true
		slog.Warn("업종 조회 실패, 빈 값 처리", "code", code, "err", err)
		return cached.Sector, cached.Industry // 자가치유 재조회 실패 시 기존 캐시 보존
	}
	r.cache[code] = entry{Sector: sector, Industry: industry}
	r.dirty = true
	return sector, industry
}

// needsRefresh 는 캐시 히트라도 재조회가 필요한지 판단한다.
// 구버전 캐시는 ETF 를 {섹터:"ETF", 산업:""} 로 저장했는데, 이제 산업 열에
// ETF 대표 업종명을 채우므로 미스로 간주해 재조회한다(별도 마이그레이션 불필요).
func needsRefresh(e entry) bool {
	return e.Sector == "ETF" && e.Industry == ""
}

func (r *Resolver) Save() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.dirty {
		return
	}
	if err := r.saveCache(); err != nil {
		slog.Warn("bizcat 캐시 저장 실패", "err", err)
	} else {
		r.dirty = false
	}
}

func (r *Resolver) saveCache() error {
	f, err := os.Create(r.cachePath)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(r.cache)
}

// kisCallsPerSec 는 bizcat 업종 조회의 KIS 호출 빈도 상한.
// 코드당 2회 호출(InquirePrice + SearchStockInfo, ETF 는 InquireEtfPrice 까지 3회)이라
// SDK 기본(15/s)에선 초당 제한(EGW00201)에 걸린다. 보수적으로 낮춰 한 실행에 빠짐없이 채워지도록 한다.
const kisCallsPerSec = 4

func kisFetch() func(string) (string, string, error) {
	client, err := kis.NewClientFromEnv(kis.WithRateLimit(kisCallsPerSec))
	if err != nil {
		slog.Warn("KIS 클라이언트 생성 실패, 업종 보강 비활성화", "err", err)
		return func(string) (string, string, error) { return "", "", nil }
	}
	return func(code string) (string, string, error) {
		ctx := context.Background()
		price, err := client.Domestic.InquirePrice(ctx, code)
		if err != nil {
			return "", "", err
		}
		info, err := client.Domestic.SearchStockInfo(ctx, code, "300")
		if err != nil {
			return "", "", err
		}
		// ETF 는 표준산업분류가 비므로, 대표 업종명(etf_rprs_bstp_kor_isnm)을 추가 조회해 산업 열에 채운다.
		var etfIndustry string
		if isETF(price, info) {
			if etf, err := client.Domestic.InquireEtfPrice(ctx, domestic.InquireEtfPriceParams{Symbol: code}); err != nil {
				slog.Warn("ETF 대표 업종 조회 실패, 산업 빈 값 처리", "code", code, "err", err)
			} else {
				etfIndustry = etf.Output.EtfRprsBstpKorIsnm
			}
		}
		sector, industry := extractSectorIndustry(price, info, etfIndustry)
		return sector, industry, nil
	}
}

// isETF 는 종목이 ETF 인지 판별한다. SearchStockInfo 의 ETF 구분 코드(etf_dvsn_cd)를 1차
// 신호로 쓰되, 비어 오는 경우를 대비해 업종 한글명 "ETF…" 접두사도 함께 본다.
func isETF(price *domestic.Price, info *domestic.StockInfo) bool {
	return info.EtfDvsnCd != "" || strings.HasPrefix(price.BstpKorIsnm, "ETF")
}

// extractSectorIndustry 는 KIS 응답에서 섹터/산업을 추출한다.
//
//   - 일반 종목
//   - 섹터  = InquirePrice 의 업종 한글명(bstp_kor_isnm, 예 "전기·전자"/"IT 서비스"/"의료·정밀기기").
//     지수업종 중분류가 비는 일반 종목(클래시스·솔루엠 등)도 채워져 커버리지가 가장 넓다.
//     moneyflow 의 sector_detail 과 동일 소스.
//   - 산업  = search-stock-info 의 표준산업분류(std_idst_clsf_cd_name, 예 "의료용 기기 제조업").
//   - ETF: 섹터 = "ETF"(고정), 산업 = etfIndustry(InquireEtfPrice 의 대표 업종명, 예 "반도체").
//     표준산업분류가 비는 ETF 의 산업 열을 추종 섹터로 채운다.
func extractSectorIndustry(price *domestic.Price, info *domestic.StockInfo, etfIndustry string) (sector, industry string) {
	if isETF(price, info) {
		return "ETF", etfIndustry
	}
	return price.BstpKorIsnm, info.StdIdstClsfCdName
}
