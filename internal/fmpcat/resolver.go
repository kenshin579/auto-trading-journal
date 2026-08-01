// Package fmpcat 는 FMP(Financial Modeling Prep) 회사 프로필로 해외 종목의
// 섹터/산업을 티커로 조회한다. (국내 internal/bizcat 의 해외 대응판.)
//
// ETF/펀드는 FMP 의 IsEtf/IsFund 플래그로 판별해 국내와 같은 표기로 통일한다
// (섹터="ETF", 산업=etfclass taxonomy 카테고리). 산업 문자열("Asset Management")로
// 판별하면 BDC·자산운용사가 ETF 로 오분류되므로 플래그를 쓴다.
//
// 빈 결과(미커버·미지원 통화)는 파일에 남기지 않아 다음 실행에 1회 재시도된다
// (국내 bizcat 은 빈 결과도 영구화한다 — 두 패키지의 정책이 다르다).
package fmpcat

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"sync"

	fmp "github.com/kenshin579/fmp-go"

	"github.com/kenshin579/auto-trading-journal/internal/etfclass"
)

// cacheVersion 은 현재 캐시 스키마 버전. 이보다 낮은 항목은 1회 재조회한다.
// v2: ETF 판별(isEtf)과 taxonomy 산업 도입. v0(버전 필드 없음)=구버전.
const cacheVersion = 2

type entry struct {
	Sector   string `json:"sector"`
	Industry string `json:"industry"`
	IsETF    bool   `json:"isEtf,omitempty"`
	Version  int    `json:"v,omitempty"`
}

// profile 은 FMP 프로필에서 이 패키지가 쓰는 필드만 추린 것.
type profile struct {
	Sector   string
	Industry string
	Name     string
	IsETF    bool
	IsFund   bool
}

type Resolver struct {
	mu          sync.Mutex
	cache       map[string]entry
	failed      map[string]bool // 이번 실행에서 실패한 심볼(negative cache, 비영구)
	cachePath   string
	fetch       func(symbol string) (profile, error)
	classifyETF etfclass.Classifier // ETF 종목명 → taxonomy 카테고리 분류기(optional, nil 가능)
	dirty       bool
}

func New(cachePath string) *Resolver {
	r := &Resolver{cache: map[string]entry{}, cachePath: cachePath}
	if data, err := os.ReadFile(cachePath); err == nil {
		var m map[string]entry
		if json.Unmarshal(data, &m) == nil && m != nil {
			r.cache = m
		} else {
			slog.Warn("fmpcat 캐시 로드 실패, 빈 캐시로 시작", "path", cachePath)
		}
	}
	return r
}

// EnableETFClassifier 는 해외 ETF 종목명의 OpenAI 카테고리 분류를 활성화한다.
// apiKey 가 비면 분류기는 nil 로 남아 FMP 원본 산업을 그대로 쓴다.
func (r *Resolver) EnableETFClassifier(apiKey, model string) {
	r.classifyETF = etfclass.New(apiKey, model)
}

func (r *Resolver) cacheLookup(symbol string) (string, bool) {
	e, ok := r.cache[symbol]
	return e.Sector, ok
}

// needsRefresh 는 캐시 히트라도 재조회가 필요한지 판단한다.
// 구버전 캐시는 ETF 판별 정보(isEtf)가 없어 지수 집계에 쓸 수 없다.
func needsRefresh(e entry) bool {
	return e.Version < cacheVersion
}

// exchangeSuffix 는 통화로 FMP 거래소 접미사를 정한다. 현재 US/JP 만 지원
// (그 외 통화는 미지원 → 공란). 보유 국가 추가 시 여기에 매핑을 늘린다.
func exchangeSuffix(currency string) (string, bool) {
	switch currency {
	case "USD":
		return "", true // 미국: 접미사 없음
	case "JPY":
		return ".T", true // 도쿄
	default:
		return "", false
	}
}

// Resolve 는 해외 티커+통화로 (섹터, 산업)을 조회한다. 미지원 통화/빈 티커는 공란.
// ETF/펀드는 ("ETF", taxonomy 카테고리), 일반 종목은 FMP 영문 섹터/산업.
func (r *Resolver) Resolve(ticker, currency string) (string, string) {
	if ticker == "" {
		return "", ""
	}
	suffix, ok := exchangeSuffix(currency)
	if !ok {
		return "", "" // 미지원 통화 → FMP 호출 안 함
	}
	symbol := ticker + suffix

	r.mu.Lock()
	defer r.mu.Unlock()
	cached, hasCached := r.cache[symbol]
	if hasCached && !needsRefresh(cached) {
		return cached.Sector, cached.Industry
	}
	if r.failed[symbol] {
		// 이번 실행에서 이미 실패 → 재조회 안 함. 자가치유 대상이면 기존 캐시 보존.
		return cached.Sector, cached.Industry
	}
	if r.fetch == nil {
		r.fetch = fmpFetch()
	}
	p, err := r.fetch(symbol)
	if err != nil {
		r.markFailed(symbol)
		slog.Warn("FMP 프로필 조회 실패, 빈 값 처리", "symbol", symbol, "err", err)
		return cached.Sector, cached.Industry // 자가치유 재조회 실패 시 기존 캐시 보존
	}

	// not-found 는 fetch 가 zero profile+nil 로 반환 → 빈 값 캐시(saveCache 가 파일에서 제외).
	e := entry{Sector: p.Sector, Industry: p.Industry, Version: cacheVersion}
	if p.IsETF || p.IsFund {
		industry, err := r.etfIndustry(p, symbol)
		if err != nil {
			// 분류 실패를 영구 캐시하면 그 종목이 다음 실행에도 재조회되지 않아
			// 대시보드에서 영구히 미분류로 남는다. 캐시하지 않고 다음 실행에 재시도한다.
			r.markFailed(symbol)
			slog.Warn("ETF 카테고리 분류 실패, 캐시하지 않음(다음 실행 재시도)", "symbol", symbol, "err", err)
			return cached.Sector, cached.Industry
		}
		e.IsETF = true
		e.Sector = "ETF"
		e.Industry = industry
	}
	r.cache[symbol] = e
	r.dirty = true
	return e.Sector, e.Industry
}

// markFailed 는 심볼을 이번 실행의 negative cache 에 넣는다(비영구).
func (r *Resolver) markFailed(symbol string) {
	if r.failed == nil {
		r.failed = map[string]bool{}
	}
	r.failed[symbol] = true
}

// etfIndustry 는 해외 ETF 의 산업(taxonomy 카테고리)을 정한다.
//   - 분류기 미설정(nil) → FMP 원본 산업 폴백. 설정 상태이므로 에러가 아니다.
//   - 분류기가 있는데 실패 → 에러 전파. taxonomy 밖 폴백 값을 영구 캐시하면 그 종목이
//     다음 실행에도 재조회되지 않아 대시보드에서 영구히 미분류로 남기 때문이다.
func (r *Resolver) etfIndustry(p profile, symbol string) (string, error) {
	if r.classifyETF == nil {
		return p.Industry, nil
	}
	name := p.Name
	if name == "" {
		name = symbol
	}
	cat, err := r.classifyETF(name)
	if err != nil {
		return "", err
	}
	// etfclass.New 로 만든 분류기는 Validate 를 거쳐 빈 값을 반환하지 않지만,
	// classifyETF 는 주입되는 함수라 그 보장을 가정하지 않는다.
	if cat == "" {
		return p.Industry, nil
	}
	return cat, nil
}

func (r *Resolver) Save() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.dirty {
		return
	}
	if err := r.saveCache(); err != nil {
		slog.Warn("fmpcat 캐시 저장 실패", "err", err)
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
	// 빈 결과(미커버/미지원 통화)는 파일에 영구화하지 않는다(노이즈 방지).
	// 인메모리 캐시엔 남아 같은 실행 내 재조회는 막고, 다음 실행에 1회 재시도된다.
	out := make(map[string]entry, len(r.cache))
	for k, v := range r.cache {
		if v.Sector == "" && v.Industry == "" {
			continue
		}
		out[k] = v
	}
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func fmpFetch() func(string) (profile, error) {
	client, err := fmp.NewClientFromEnv()
	if err != nil {
		slog.Warn("FMP 클라이언트 생성 실패, 해외 섹터 보강 비활성화", "err", err)
		return func(string) (profile, error) { return profile{}, nil }
	}
	return func(symbol string) (profile, error) {
		p, err := client.Company.Profile(context.Background(), symbol)
		if err != nil {
			if errors.Is(err, fmp.ErrNotFound) {
				return profile{}, nil // 미커버 → 빈 값(재조회는 다음 실행에 1회)
			}
			return profile{}, err // 일시적 오류 → negative-cache, 다음 실행 재시도
		}
		return profile{
			Sector:   p.Sector,
			Industry: p.Industry,
			Name:     p.CompanyName,
			IsETF:    p.IsEtf,
			IsFund:   p.IsFund,
		}, nil
	}
}
