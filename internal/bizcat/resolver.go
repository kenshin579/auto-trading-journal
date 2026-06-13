// Package bizcat 는 KIS 지수업종(대분류=섹터 / 중분류=산업)을 종목코드로 조회한다.
// (기존 internal/sector(OpenAI, 대시보드 GICS)와 별개 — per-row 열 전용.)
package bizcat

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"sync"

	kis "github.com/kenshin579/korea-investment-stock"
)

type entry struct {
	Sector   string `json:"sector"`
	Industry string `json:"industry"`
}

type Resolver struct {
	mu        sync.Mutex
	cache     map[string]entry
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
	if e, ok := r.cache[code]; ok {
		return e.Sector, e.Industry
	}
	if r.fetch == nil {
		r.fetch = kisFetch()
	}
	sector, industry, err := r.fetch(code)
	if err != nil {
		slog.Warn("업종 조회 실패, 빈 값 처리", "code", code, "err", err)
		return "", ""
	}
	r.cache[code] = entry{Sector: sector, Industry: industry}
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

func kisFetch() func(string) (string, string, error) {
	client, err := kis.NewClientFromEnv()
	if err != nil {
		slog.Warn("KIS 클라이언트 생성 실패, 업종 보강 비활성화", "err", err)
		return func(string) (string, string, error) { return "", "", nil }
	}
	return func(code string) (string, string, error) {
		info, err := client.Domestic.SearchStockInfo(context.Background(), code, "300")
		if err != nil {
			return "", "", err
		}
		sector, industry := pickSectorIndustry(info.IdxBztpMclsCdName, info.StdIdstClsfCdName)
		return sector, industry, nil
	}
}

// pickSectorIndustry 는 KIS 주식기본조회 업종 필드에서 섹터/산업을 선택한다.
//
//   - 섹터  = 지수업종 중분류(idx_bztp_mcls_cd_name, 예 "전기,전자"/"서비스업").
//     대분류는 "시가총액규모"라 업종이 아니므로 미사용. 지수업종 미분류 종목/ETF 는 빈값.
//   - 산업  = 표준산업분류(std_idst_clsf_cd_name, 예 "의료용 기기 제조업").
//     지수업종(중/소분류)이 비는 일반 종목(클래시스·솔루엠 등)도 표준산업분류는 채워져
//     커버리지가 넓다. (소분류 idx_bztp_scls_cd_name 은 중분류와 동일값이라 미사용.)
func pickSectorIndustry(idxBztpMcls, stdIdstClsf string) (sector, industry string) {
	return idxBztpMcls, stdIdstClsf
}
