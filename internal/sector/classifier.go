// Package sector 는 OpenAI 기반 종목 섹터 분류기이다.
// Python modules/sector_classifier.py 의 Go 포팅.
//
// 종목명/종목코드를 GICS 기반 한국어 섹터(12종)로 분류하고,
// JSON 파일 캐시({종목명: 섹터})로 반복 API 호출을 방지한다.
package sector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	openai "github.com/sashabaranov/go-openai"

	"github.com/kenshin579/auto-trading-journal/internal/summary"
)

// Sectors 는 사용 가능한 12개 GICS 한국어 섹터. (Python SECTORS, py:16-19)
var Sectors = []string{
	"에너지", "소재", "산업재", "경기소비재", "필수소비재",
	"헬스케어", "금융", "IT", "통신서비스", "유틸리티", "부동산", "기타",
}

// otherSector 는 분류 불가/검증 실패 시 기본값.
const otherSector = "기타"

// systemPrompt 는 OpenAI 시스템 프롬프트. (Python SYSTEM_PROMPT, py:21-30)
const systemPrompt = `당신은 주식 종목 섹터 분류 전문가입니다.
주어진 종목명과 종목코드를 보고 GICS 기반 한국어 섹터로 분류하세요.

사용 가능한 섹터: 에너지, 소재, 산업재, 경기소비재, 필수소비재, 헬스케어, 금융, IT, 통신서비스, 유틸리티, 부동산, 기타

규칙:
- ETF는 주요 투자 대상 섹터로 분류
- 분류 불가 시 "기타"
- 반드시 JSON 형식으로만 응답: {"종목명": "섹터명", ...}
- 다른 텍스트 없이 JSON만 출력`

// apiTimeout 은 OpenAI 호출 타임아웃. (Python timeout=30, py:122)
const apiTimeout = 30 * time.Second

// sectorSet 은 검증용 섹터 집합.
var sectorSet = func() map[string]bool {
	m := make(map[string]bool, len(Sectors))
	for _, s := range Sectors {
		m[s] = true
	}
	return m
}()

// Classifier 는 OpenAI API 를 사용한 종목 섹터 분류기. (Python SectorClassifier)
type Classifier struct {
	client    *openai.Client
	model     string
	cachePath string
	cache     map[string]string
}

// 컴파일 타임 인터페이스 만족 검증 (import cycle 없음을 보장).
var _ summary.SectorClassifier = (*Classifier)(nil)

// New 는 Classifier 를 생성한다. 캐시 파일을 즉시 로드한다.
// (Python SectorClassifier.__init__, py:36-41)
func New(apiKey, model, cachePath string) *Classifier {
	if model == "" {
		model = openai.GPT4oMini
	}
	return &Classifier{
		client:    openai.NewClient(apiKey),
		model:     model,
		cachePath: cachePath,
		cache:     loadCache(cachePath),
	}
}

// loadCache 는 캐시 파일을 읽는다. 없거나 손상되면 빈 맵을 반환한다.
// (Python _load_cache, py:43-56)
func loadCache(path string) map[string]string {
	empty := map[string]string{}
	if path == "" {
		return empty
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("캐시 파일 로드 실패, 초기화", "error", err)
		}
		return empty
	}
	var raw map[string]string
	if err := json.Unmarshal(data, &raw); err != nil {
		slog.Warn("캐시 파일 형식 오류, 초기화", "path", path, "error", err)
		return empty
	}
	return raw
}

// saveCache 는 캐시를 JSON(indent 2, 한글 escape 안 함)으로 저장한다.
// (Python _save_cache, py:58-61)
func (c *Classifier) saveCache() error {
	if c.cachePath == "" {
		return nil
	}
	if dir := filepath.Dir(c.cachePath); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false) // 한글 가독성 유지 (Python ensure_ascii=False)
	enc.SetIndent("", "  ")
	if err := enc.Encode(c.cache); err != nil {
		return err
	}
	return os.WriteFile(c.cachePath, buf.Bytes(), 0o644)
}

// Classify 는 종목 리스트를 섹터로 분류한다. (Python classify, py:63-102)
func (c *Classifier) Classify(ctx context.Context, stocks []summary.SectorStock) (map[string]string, error) {
	result := make(map[string]string)
	var uncached []summary.SectorStock

	for _, s := range stocks {
		if sec, ok := c.cache[s.Name]; ok {
			result[s.Name] = sec
		} else {
			uncached = append(uncached, s)
		}
	}

	// 전부 캐시면 네트워크 접근 없이 즉시 반환. (Python py:81-83)
	if len(uncached) == 0 {
		slog.Info("섹터 분류: 전체 캐시 사용", "count", len(stocks))
		return result, nil
	}

	slog.Info("섹터 분류: OpenAI 호출", "uncached", len(uncached), "cached", len(result))

	// 국내(KRW)/해외 분리하여 배치 처리. (Python py:88-89)
	var domestic, foreign []summary.SectorStock
	for _, s := range uncached {
		if s.Currency == "KRW" {
			domestic = append(domestic, s)
		} else {
			foreign = append(foreign, s)
		}
	}

	if len(domestic) > 0 {
		classified := c.callOpenAI(ctx, domestic, true)
		for k, v := range classified {
			result[k] = v
			c.cache[k] = v
		}
	}
	if len(foreign) > 0 {
		classified := c.callOpenAI(ctx, foreign, false)
		for k, v := range classified {
			result[k] = v
			c.cache[k] = v
		}
	}

	if err := c.saveCache(); err != nil {
		slog.Warn("캐시 저장 실패", "error", err)
	}
	return result, nil
}

// callOpenAI 는 OpenAI 호출로 섹터를 분류한다. 실패 시 전부 "기타".
// (Python _call_openai, py:104-146)
func (c *Classifier) callOpenAI(ctx context.Context, stocks []summary.SectorStock, isDomestic bool) map[string]string {
	market := "해외"
	if isDomestic {
		market = "한국"
	}

	var sb strings.Builder
	for i, s := range stocks {
		if i > 0 {
			sb.WriteByte('\n')
		}
		fmt.Fprintf(&sb, "- %s (%s)", s.Name, s.Code)
	}
	userPrompt := fmt.Sprintf("다음 %s 주식 종목들의 섹터를 분류해주세요:\n%s", market, sb.String())

	names := make([]string, len(stocks))
	for i, s := range stocks {
		names[i] = s.Name
	}

	callCtx, cancel := context.WithTimeout(ctx, apiTimeout)
	defer cancel()

	resp, err := c.client.CreateChatCompletion(callCtx, openai.ChatCompletionRequest{
		Model: c.model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
			{Role: openai.ChatMessageRoleUser, Content: userPrompt},
		},
		Temperature: 0,
		ResponseFormat: &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONObject,
		},
	})
	if err != nil || len(resp.Choices) == 0 {
		slog.Error("OpenAI 섹터 분류 실패", "error", err)
		return allOther(names)
	}

	var classified map[string]string
	if err := json.Unmarshal([]byte(resp.Choices[0].Message.Content), &classified); err != nil {
		slog.Error("OpenAI 응답 파싱 실패", "error", err)
		return allOther(names)
	}

	return validateSectors(classified, names)
}

// validateSectors 는 응답을 검증한다: 알 수 없는 섹터→"기타", 누락 종목→"기타".
// (Python py:127-142)
func validateSectors(classified map[string]string, names []string) map[string]string {
	valid := make(map[string]string, len(names))
	for name, sec := range classified {
		if sectorSet[sec] {
			valid[name] = sec
		} else {
			slog.Warn("알 수 없는 섹터 → 기타 처리", "sector", sec, "name", name)
			valid[name] = otherSector
		}
	}
	for _, name := range names {
		if _, ok := valid[name]; !ok {
			slog.Warn("OpenAI 응답에서 누락된 종목 → 기타 처리", "name", name)
			valid[name] = otherSector
		}
	}
	return valid
}

// allOther 는 모든 종목을 "기타"로 매핑한다. (Python py:146)
func allOther(names []string) map[string]string {
	out := make(map[string]string, len(names))
	for _, n := range names {
		out[n] = otherSector
	}
	return out
}
