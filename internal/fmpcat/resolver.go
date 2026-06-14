// Package fmpcat 는 FMP(Financial Modeling Prep) 회사 프로필로 해외 종목의
// 섹터/산업(영문)을 티커로 조회한다. (국내 internal/bizcat 의 해외 대응판.)
package fmpcat

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"sync"

	fmp "github.com/kenshin579/fmp-go"
)

type entry struct {
	Sector   string `json:"sector"`
	Industry string `json:"industry"`
}

type Resolver struct {
	mu        sync.Mutex
	cache     map[string]entry
	failed    map[string]bool // 이번 실행에서 실패한 심볼(negative cache, 비영구)
	cachePath string
	fetch     func(symbol string) (sector, industry string, err error)
	dirty     bool
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

func (r *Resolver) cacheLookup(symbol string) (string, bool) {
	e, ok := r.cache[symbol]
	return e.Sector, ok
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

// Resolve 는 해외 티커+통화로 (섹터, 산업)을 조회한다(영문). 미지원 통화/빈 티커는 공란.
func (r *Resolver) Resolve(ticker, currency string) (string, string) {
	if ticker == "" {
		return "", ""
	}
	suffix, ok := exchangeSuffix(currency)
	if !ok {
		return "", "" // 미지원 통화 → 공란(FMP 호출 안 함)
	}
	symbol := ticker + suffix

	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.cache[symbol]; ok {
		return e.Sector, e.Industry
	}
	if r.failed[symbol] {
		return "", "" // 이번 실행에서 이미 실패 → 재조회 안 함
	}
	if r.fetch == nil {
		r.fetch = fmpFetch()
	}
	sector, industry, err := r.fetch(symbol)
	if err != nil {
		if r.failed == nil {
			r.failed = map[string]bool{}
		}
		r.failed[symbol] = true
		slog.Warn("FMP 프로필 조회 실패, 빈 값 처리", "symbol", symbol, "err", err)
		return "", ""
	}
	// not-found 는 fetch 가 ("","",nil)로 반환 → 빈 값으로 영구 캐시(해외 ETF/미커버, 재조회 안 함).
	r.cache[symbol] = entry{Sector: sector, Industry: industry}
	r.dirty = true
	return sector, industry
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

func fmpFetch() func(string) (string, string, error) {
	client, err := fmp.NewClientFromEnv()
	if err != nil {
		slog.Warn("FMP 클라이언트 생성 실패, 해외 섹터 보강 비활성화", "err", err)
		return func(string) (string, string, error) { return "", "", nil }
	}
	return func(symbol string) (string, string, error) {
		p, err := client.Company.Profile(context.Background(), symbol)
		if err != nil {
			if errors.Is(err, fmp.ErrNotFound) {
				return "", "", nil // 미커버(해외 ETF 등) → 빈 값으로 캐시(재조회 안 함)
			}
			return "", "", err // 일시적 오류 → negative-cache, 다음 실행 재시도
		}
		return p.Sector, p.Industry, nil
	}
}
