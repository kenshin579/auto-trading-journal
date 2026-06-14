package bizcat

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

// etfCategories 는 9999 미분류 ETF(해외·테마)를 분류할 고정 taxonomy.
// KIS 업종코드로 분류되지 않는 해외 지수/자산군/테마 ETF 를 종목명으로 분류한다.
var etfCategories = []string{
	"미국주식", "중국주식", "일본주식", "인도주식", "베트남주식", "글로벌주식",
	"반도체", "2차전지", "바이오·헬스케어", "AI·로봇", "신재생에너지", "원자력",
	"방위·우주항공", "배당", "리츠·부동산", "원자재", "채권", "통화·단기금리", "기타테마",
}

// etfFallbackCategory 는 taxonomy 밖 응답/분류 불가 시 사용.
const etfFallbackCategory = "기타테마"

var etfCategorySet = func() map[string]bool {
	m := make(map[string]bool, len(etfCategories))
	for _, c := range etfCategories {
		m[c] = true
	}
	return m
}()

// validateETFCategory 는 분류 결과가 taxonomy 안에 있으면 그대로, 아니면 기타테마로 정규화한다.
func validateETFCategory(s string) string {
	s = strings.TrimSpace(s)
	if etfCategorySet[s] {
		return s
	}
	return etfFallbackCategory
}

const etfClassifyTimeout = 30 * time.Second

var etfSystemPrompt = fmt.Sprintf(`당신은 한국 상장 ETF 분류 전문가입니다.
ETF 종목명을 보고 아래 카테고리 중 정확히 하나로 분류하세요.

사용 가능한 카테고리: %s

분류 기준:
- 해외 주식형은 투자 지역 우선(미국주식/중국주식/일본주식/인도주식/베트남주식/글로벌주식).
- 특정 섹터·테마가 핵심이면 해당 테마(반도체/2차전지/바이오·헬스케어/AI·로봇/신재생에너지/원자력/방위·우주항공) 우선.
- 채권/국채/단기자금/CD금리는 채권 또는 통화·단기금리.
- 금·은·원유 등 상품은 원자재. 리츠·부동산·인프라는 리츠·부동산. 배당·커버드콜은 배당.
- 애매하거나 위에 없으면 기타테마.

반드시 JSON 으로만 응답: {"category": "<카테고리>"}`, strings.Join(etfCategories, ", "))

// newETFClassifier 는 ETF 종목명 → 카테고리 분류 함수를 만든다. apiKey 가 비면 nil 을 반환한다
// (호출부는 nil 이면 지수명 폴백을 사용). 결과는 bizcat 영구 캐시에 저장돼 코드당 1회만 호출된다.
func newETFClassifier(apiKey, model string) func(name string) (string, error) {
	if apiKey == "" {
		return nil
	}
	if model == "" {
		model = "gpt-4o-mini"
	}
	client := openai.NewClient(apiKey)
	return func(name string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), etfClassifyTimeout)
		defer cancel()
		resp, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
			Model: model,
			Messages: []openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleSystem, Content: etfSystemPrompt},
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
		return validateETFCategory(out.Category), nil
	}
}
