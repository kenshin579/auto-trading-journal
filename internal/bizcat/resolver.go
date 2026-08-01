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

	"github.com/kenshin579/auto-trading-journal/internal/etfclass"
	kis "github.com/kenshin579/korea-investment-stock"
	"github.com/kenshin579/korea-investment-stock/domestic"
)

type entry struct {
	Sector   string `json:"sector"`
	Industry string `json:"industry"`
	Version  int    `json:"v,omitempty"` // 분류 스키마 버전(ETF 하이브리드 분류 도입). 0=구버전.
}

// etfCacheVersion 은 현재 ETF 산업 분류 스키마 버전. 이보다 낮은 ETF 캐시는 재조회한다.
// v2: 코드분류(KRX명)+9999(OpenAI) 하이브리드. v3: 전부 OpenAI taxonomy 로 통일.
// v4: 미국 시장대표를 S&P500/나스닥/미국주식(기타)로 세분(지수 비중 집계용).
// v5: 분류 실패 시 폴백 값을 영구 캐시하던 버그 이전 항목 무효화 + 팩터·스타일 카테고리 추가.
const etfCacheVersion = 5

type Resolver struct {
	mu          sync.Mutex
	cache       map[string]entry
	failed      map[string]bool // 이번 실행에서 실패한 코드(negative cache, 비영구)
	cachePath   string
	fetch       func(code, name string) (sector, industry string, err error)
	classifyETF etfclass.Classifier // ETF 종목명 → taxonomy 카테고리 분류기(optional, nil 가능)
	dirty       bool
}

// EnableETFClassifier 는 ETF 종목명의 OpenAI 카테고리 분류를 활성화한다.
// apiKey 가 비면 분류기는 nil 로 남아 지수명 폴백을 사용한다.
func (r *Resolver) EnableETFClassifier(apiKey, model string) {
	r.classifyETF = etfclass.New(apiKey, model)
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

func (r *Resolver) Resolve(code, name string) (string, string) {
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
		r.fetch = kisFetch(r.classifyETF)
	}
	sector, industry, err := r.fetch(code, name)
	if err != nil {
		if r.failed == nil {
			r.failed = map[string]bool{}
		}
		r.failed[code] = true
		slog.Warn("업종 조회 실패, 빈 값 처리", "code", code, "err", err)
		return cached.Sector, cached.Industry // 자가치유 재조회 실패 시 기존 캐시 보존
	}
	r.cache[code] = entry{Sector: sector, Industry: industry, Version: etfCacheVersion}
	r.dirty = true
	return sector, industry
}

// needsRefresh 는 캐시 히트라도 재조회가 필요한지 판단한다.
// ETF 산업 분류 스키마가 바뀌면(구버전=지수명/빈값) 미스로 간주해 재조회한다.
// 비-ETF 주식 캐시는 영향받지 않는다(전체 삭제 불필요).
func needsRefresh(e entry) bool {
	return e.Sector == etfclass.SectorETF && e.Version < etfCacheVersion
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
// SDK 기본(15/s)에선 초당 제한(EGW00201)에 걸린다. ETF 3콜 버스트로 4/s 에서도 간헐
// 초과가 나, 한 실행에 빠짐없이 채워지도록 3/s 로 더 낮춘다.
const kisCallsPerSec = 3

func kisFetch(classifyETF etfclass.Classifier) func(code, name string) (string, string, error) {
	client, err := kis.NewClientFromEnv(kis.WithRateLimit(kisCallsPerSec))
	if err != nil {
		slog.Warn("KIS 클라이언트 생성 실패, 업종 보강 비활성화", "err", err)
		return func(code, name string) (string, string, error) { return "", "", nil }
	}
	return func(code, name string) (string, string, error) {
		ctx := context.Background()
		price, err := client.Domestic.InquirePrice(ctx, code)
		if err != nil {
			return "", "", err
		}
		info, err := client.Domestic.SearchStockInfo(ctx, code, "300")
		if err != nil {
			return "", "", err
		}
		// ETF 는 표준산업분류가 비므로, 종목명을 OpenAI taxonomy 로 분류한다(분류기 없으면 지수명 폴백).
		var etfIndustry string
		if isETF(price, info) {
			etf, err := client.Domestic.InquireEtfPrice(ctx, domestic.InquireEtfPriceParams{Symbol: code})
			if err != nil {
				// 일시적 실패(레이트리밋 등)를 빈 값으로 영구 캐시하지 않도록 fetch 실패로 전파한다
				// (negative-cache 처리 → 다음 실행에 재시도).
				return "", "", err
			}
			etfIndustry, err = resolveETFIndustry(etf.Output.EtfRprsBstpKorIsnm, name, classifyETF)
			if err != nil {
				return "", "", err
			}
		}
		sector, industry := extractSectorIndustry(price, info, etfIndustry)
		return sector, industry, nil
	}
}

// resolveETFIndustry 는 ETF 산업 분류를 결정한다.
//   - 분류기가 있으면 코드 분류 여부와 무관하게 펀드 종목명을 taxonomy 로 분류해 통일한다
//     (예 "S&P500"/"반도체"/"원자재"). 코드 분류된 verbose 한 지수명도 정규화됨.
//   - 분류기 미설정(nil) → KIS 지수명(stripKRXPrefix) 폴백. 설정 상태이므로 에러가 아니다.
//   - 분류기가 있는데 실패 → 에러를 전파한다. 폴백 값(taxonomy 밖 지수명)을 영구 캐시하면
//     그 종목이 다음 실행에도 재조회되지 않아 대시보드에서 영구히 미분류로 남기 때문이다.
func resolveETFIndustry(rprsName, fundName string, classify etfclass.Classifier) (string, error) {
	if classify == nil {
		return stripKRXPrefix(rprsName), nil
	}
	cat, err := classify(fundName)
	if err != nil {
		return "", err
	}
	// etfclass.New 로 만든 분류기는 Validate 를 거쳐 빈 값을 반환하지 않지만,
	// classify 는 주입되는 함수라 그 보장을 가정하지 않는다.
	if cat == "" {
		return stripKRXPrefix(rprsName), nil
	}
	return cat, nil
}

// stripKRXPrefix 는 KRX 업종명의 "KRX " 접두사를 제거한다. 예 "KRX 반도체"→"반도체", "종합"→"종합".
func stripKRXPrefix(s string) string {
	return strings.TrimSpace(strings.TrimPrefix(s, "KRX "))
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
		return etfclass.SectorETF, etfIndustry
	}
	return price.BstpKorIsnm, info.StdIdstClsfCdName
}
