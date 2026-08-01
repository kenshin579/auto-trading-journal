// Package etfclass 는 ETF 종목명을 고정 taxonomy 의 카테고리 하나로 분류한다.
// 국내(internal/bizcat)와 해외(internal/fmpcat) 가 같은 체계를 공유하기 위해 분리돼 있다.
package etfclass

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

// Categories 는 모든 ETF 를 종목명으로 분류할 고정 taxonomy.
// 국내 시장/섹터 ETF 와 해외·테마 ETF 를 하나의 일관된 카테고리 체계로 통일한다.
var Categories = []string{
	// 지역/시장대표
	"한국주식", "미국주식", "중국주식", "일본주식", "인도주식", "베트남주식", "글로벌주식",
	// 섹터/테마
	"반도체", "2차전지", "바이오·헬스케어", "AI·로봇", "신재생에너지", "원자력",
	"방위·우주항공", "자동차", "금융", "건설", "필수소비재", "IT·인터넷",
	// 자산군
	"배당", "리츠·부동산", "원자재", "채권", "통화·단기금리",
	// 기타
	"기타테마",
}

// FallbackCategory 는 taxonomy 밖 응답/분류 불가 시 사용.
const FallbackCategory = "기타테마"

var categorySet = func() map[string]bool {
	m := make(map[string]bool, len(Categories))
	for _, c := range Categories {
		m[c] = true
	}
	return m
}()

// Validate 는 분류 결과가 taxonomy 안에 있으면 그대로, 아니면 FallbackCategory 로 정규화한다.
func Validate(s string) string {
	s = strings.TrimSpace(s)
	if categorySet[s] {
		return s
	}
	return FallbackCategory
}

const classifyTimeout = 30 * time.Second

var systemPrompt = fmt.Sprintf(`당신은 ETF 분류 전문가입니다.
ETF 종목명을 보고 아래 카테고리 중 정확히 하나로 분류하세요.

사용 가능한 카테고리: %s

분류 기준:
- 특정 섹터·테마가 핵심이면 지역보다 해당 테마 우선
  (반도체/2차전지/바이오·헬스케어/AI·로봇/신재생에너지/원자력/방위·우주항공/자동차/금융/건설/필수소비재/IT·인터넷).
  예: "미국반도체"→반도체, "글로벌AI"→AI·로봇.
- 시장대표·종합 지수형은 투자 지역(한국주식/미국주식/중국주식/일본주식/인도주식/베트남주식/글로벌주식).
  예: "코스피200"/"코스닥150"→한국주식, "S&P500"/"나스닥100"→미국주식.
- 은행/보험/증권은 금융. 채권/국채/회사채는 채권. 단기자금/CD금리/통화는 통화·단기금리.
- 금·은·원유 등 상품은 원자재. 리츠·부동산·인프라는 리츠·부동산. 배당·커버드콜은 배당.
- 애매하거나 위에 없으면 기타테마.

반드시 JSON 으로만 응답: {"category": "<카테고리>"}`, strings.Join(Categories, ", "))

// New 는 ETF 종목명 → 카테고리 분류 함수를 만든다. apiKey 가 비면 nil 을 반환한다
// (호출부는 nil 이면 각자의 폴백을 사용). 결과는 호출부 영구 캐시에 저장돼 종목당 1회만 호출된다.
func New(apiKey, model string) func(name string) (string, error) {
	if apiKey == "" {
		return nil
	}
	if model == "" {
		model = "gpt-4o-mini"
	}
	client := openai.NewClient(apiKey)
	return func(name string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), classifyTimeout)
		defer cancel()
		resp, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
			Model: model,
			Messages: []openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
				{Role: openai.ChatMessageRoleUser, Content: "ETF 종목명: " + name},
			},
			Temperature: 0,
			ResponseFormat: &openai.ChatCompletionResponseFormat{
				Type: openai.ChatCompletionResponseFormatTypeJSONObject,
			},
		})
		if err != nil || len(resp.Choices) == 0 {
			slog.Warn("ETF OpenAI 분류 실패", "name", name, "err", err)
			return "", fmt.Errorf("etf classify: %w", err)
		}
		var out struct {
			Category string `json:"category"`
		}
		if err := json.Unmarshal([]byte(resp.Choices[0].Message.Content), &out); err != nil {
			slog.Warn("ETF OpenAI 응답 파싱 실패", "name", name, "err", err)
			return "", err
		}
		return Validate(out.Category), nil
	}
}
