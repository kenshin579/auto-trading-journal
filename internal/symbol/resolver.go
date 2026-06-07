package symbol

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/transform"
)

const (
	kospiURL     = "https://new.real.download.dws.co.kr/common/master/kospi_code.mst.zip"
	kosdaqURL    = "https://new.real.download.dws.co.kr/common/master/kosdaq_code.mst.zip"
	kospiFWFLen  = 227
	kosdaqFWFLen = 221
	cacheTTL     = 7 * 24 * time.Hour
)

// Resolver maps stock names to short codes (단축코드).
// It is lazy-loaded on first Resolve call and resilient to network failures.
type Resolver struct {
	m      map[string]string
	loaded bool
	warned map[string]bool
}

// New creates a new Resolver.
func New() *Resolver { return &Resolver{warned: map[string]bool{}} }

// Resolve returns the short code (단축코드) for the given stock name.
// Returns "" if not found. Network failures are silently handled.
func (r *Resolver) Resolve(name string) string {
	r.ensure()
	return r.m[strings.TrimSpace(name)]
}

func (r *Resolver) ensure() {
	if r.loaded {
		return
	}
	r.loaded = true
	r.m = map[string]string{}
	for _, src := range []struct {
		url, cache string
		fwf        int
	}{
		{kospiURL, "kospi_code.mst.zip", kospiFWFLen},
		{kosdaqURL, "kosdaq_code.mst.zip", kosdaqFWFLen},
	} {
		zipBytes, err := fetchZip(src.url, src.cache)
		if err != nil {
			continue
		}
		text, err := extractMst(zipBytes)
		if err != nil {
			continue
		}
		for name, code := range parseMstLines(text, src.fwf) {
			if _, ok := r.m[name]; !ok {
				r.m[name] = code
			}
		}
	}
}

// parseMstLines extracts {한글명: 단축코드} from a decoded .mst text.
// Each line layout: [0:9]=단축코드, [9:21]=표준코드(ISIN), [21:len-fwfLen]=한글명.
// First occurrence wins for duplicate names.
// NOTE: operates on byte offsets (the text is expected to be a decoded string,
// so byte indices match character boundaries only for ASCII prefixes — the Korean
// name is extracted by byte slicing which is valid since the prefix is all ASCII).
func parseMstLines(text string, fwfLen int) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		if len(line) < fwfLen+21 {
			continue
		}
		code := strings.TrimSpace(line[0:9])
		name := strings.TrimSpace(line[21 : len(line)-fwfLen])
		if code != "" && name != "" {
			if _, ok := out[name]; !ok {
				out[name] = code
			}
		}
	}
	return out
}

func cacheDir() string {
	d, _ := os.UserHomeDir()
	p := filepath.Join(d, ".cache", "auto-trading-journal")
	os.MkdirAll(p, 0o755)
	return p
}

func fetchZip(url, cacheName string) ([]byte, error) {
	path := filepath.Join(cacheDir(), cacheName)
	if fi, err := os.Stat(path); err == nil && time.Since(fi.ModTime()) < cacheTTL {
		return os.ReadFile(path)
	}
	resp, err := http.Get(url) //nolint:gosec
	if err == nil {
		defer resp.Body.Close()
		data, rerr := io.ReadAll(resp.Body)
		if rerr == nil {
			if _, zerr := zip.NewReader(bytes.NewReader(data), int64(len(data))); zerr == nil {
				os.WriteFile(path, data, 0o644) //nolint:errcheck
				return data, nil
			}
		}
	}
	// Fall back to stale cache if present
	if b, ferr := os.ReadFile(path); ferr == nil {
		return b, nil
	}
	return nil, err
}

func extractMst(zipBytes []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return "", err
	}
	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, ".mst") {
			rc, err := f.Open()
			if err != nil {
				return "", err
			}
			defer rc.Close()
			// .mst files are cp949/EUC-KR encoded; korean.EUCKR handles cp949 as a superset
			dec := transform.NewReader(rc, korean.EUCKR.NewDecoder())
			b, err := io.ReadAll(dec)
			if err != nil {
				return "", err
			}
			return string(b), nil
		}
	}
	return "", io.EOF
}
