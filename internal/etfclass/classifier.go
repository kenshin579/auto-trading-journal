// Package etfclass 는 ETF 라벨링 계약(섹터 판별자 + 카테고리 taxonomy + 종목명 분류기)을 소유한다.
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
// 읽기 전용 — 런타임에 수정하지 말 것(categorySet/systemPrompt 가 init 시점에 이 값으로 고정된다).
var Categories = []string{
	// 지역/시장대표 (미국은 추종 지수 단위로 분리)
	"한국주식", "S&P500", "나스닥", "미국주식(기타)",
	"중국주식", "일본주식", "인도주식", "베트남주식", "글로벌주식",
	// 섹터/테마
	"반도체", "2차전지", "바이오·헬스케어", "AI·로봇", "신재생에너지", "원자력",
	"방위·우주항공", "자동차", "금융", "건설", "필수소비재", "IT·인터넷",
	// 자산군·전략
	"배당", "팩터·스타일", "리츠·부동산", "원자재", "채권", "통화·단기금리",
	// 기타
	"기타테마",
}

// FallbackCategory 는 taxonomy 밖 응답/분류 불가 시 사용.
const FallbackCategory = "기타테마"

// SectorETF 는 ETF/펀드로 판별된 종목의 섹터 값. bizcat·fmpcat 이 쓰고 summary 가 읽는
// 패키지 간 계약이라 리터럴 대신 이 상수를 쓴다(한쪽만 바뀌면 지수 집계가 조용히 0 이 된다).
//
// 이 값은 config/*_cache.json 과 거래 시트에 문자열로 영속화된다. 바꾸면 기존 캐시·시트가
// 전부 무효가 되고 needsRefresh 가 그걸 감지하지 못한다(체크 자체가 이 값에 의존) —
// 바꿔야 한다면 캐시 삭제 + 시트 재생성이 함께 필요하다.
const SectorETF = "ETF"

var categorySet = func() map[string]bool {
	m := make(map[string]bool, len(Categories))
	for _, c := range Categories {
		m[c] = true
	}
	return m
}()

// categoryNormalizer 는 모델 응답의 표기 흔들림을 흡수한다
// (전각 괄호, ASCII 공백·탭, NBSP U+00A0, 전각 공백 U+3000).
// taxonomy 의 어떤 카테고리도 내부 공백을 갖지 않으므로 공백 제거는 안전하다.
var categoryNormalizer = strings.NewReplacer(
	"（", "(", "）", ")", " ", "", "\t", "", " ", "", "　", "",
)

// Validate 는 분류 결과가 taxonomy 안에 있으면 그대로, 표기만 다르면 정규화해서,
// 그래도 없으면 FallbackCategory 로 돌려준다.
// taxonomy 밖 응답은 경고로 남긴다 — 조용히 기타테마로 흡수되면 지수 비중이 과소평가되는데
// 그 사실을 알 방법이 없어진다.
func Validate(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		slog.Warn("ETF 카테고리가 비어 있음 — 기타테마로 처리")
		return FallbackCategory
	}
	if categorySet[s] {
		return s
	}
	if n := categoryNormalizer.Replace(s); categorySet[n] {
		slog.Warn("ETF 카테고리 표기 정규화", "raw", s, "normalized", n)
		return n
	}
	slog.Warn("ETF 카테고리가 taxonomy 밖 — 기타테마로 처리", "raw", s)
	return FallbackCategory
}

const classifyTimeout = 30 * time.Second

// categoryList 는 프롬프트에 들어가는 카테고리 나열.
var categoryList = strings.Join(Categories, ", ")

// classifyRules 는 분류 규칙 본문. 카테고리 나열과 분리해 둔다 — 규칙 문구를 검증하는 테스트가
// 카테고리 이름 때문에 무조건 통과하는 동어반복이 되지 않도록.
const classifyRules = `먼저 아래 예외에 해당하는지 보세요. 해당하면 이름에 지수명이 들어 있어도
시장대표 지수로 분류하지 마세요(지수를 그대로 추종하지 않기 때문입니다).
여러 예외에 해당하면 위에 있는 것을 적용하세요.
- 커버드콜·프리미엄인컴: "커버드콜"/"타겟프리미엄"/Covered Call/Equity Premium Income/
  Enhanced Income/Buffer → 배당.  예: "JPMorgan Nasdaq Equity Premium Income ETF"→배당
- 배당이 핵심인 펀드: "배당"/"고배당"/"배당성장"/Dividend/Dividend Growth → 배당.
  예: "Schwab U.S. Dividend Equity ETF"→배당, "TIGER 미국배당다우존스"→배당.
  추종 지수 이름에 다우존스가 들어가도 배당 펀드이면 배당입니다.
- 레버리지·인버스: 배수·방향 표기가 이름에 붙은 상품("2X"/"3X"/"곱버스"/"인버스"/
  Ultra/UltraPro/Bull/Bear/Inverse) → 기타테마
- 팩터·스타일: 성장주/가치주/퀄리티/모멘텀/저변동/동일가중/Growth/Value/Quality/Momentum/
  Low Volatility/Equal Weight → 팩터·스타일
- 소수 종목 집중 바스켓("Magnificent Seven"/TOP10 등) → 기타테마

예외가 아니면 아래 기준으로 분류하세요.
- 특정 섹터·테마가 핵심이면 지역보다 해당 테마 우선
  (반도체/2차전지/바이오·헬스케어/AI·로봇/신재생에너지/원자력/방위·우주항공/자동차/금융/건설/필수소비재/IT·인터넷).
  예: "미국반도체"→반도체, "글로벌AI"→AI·로봇.
- 시장 전체를 담는 대표 지수형은 추종 지수로 분류합니다.
  "S&P500"/"S&P 500"/"SPDR S&P 500"→S&P500,  "나스닥100"/"NASDAQ 100"/"QQQ"→나스닥,
  "코스피200"/"코스닥150"/"KRX300"→한국주식,
  러셀2000·다우존스산업평균·미국 토탈마켓 등 그 외 미국 시장대표→미국주식(기타),
  그 외 국가·지역은 중국주식/일본주식/인도주식/베트남주식/글로벌주식.
- 은행/보험/증권은 금융. 채권/국채/회사채는 채권. 단기자금/CD금리/통화는 통화·단기금리.
- 원자재는 금·은·원유 등 실물·선물에 직접 투자하는 경우만입니다. 채굴·정유·에너지·소재 등
  관련 기업 주식에 투자하면 원자재가 아니라 해당 테마(원자력 등)이며, 전통 에너지·정유·소재처럼
  맞는 테마가 없으면 기타테마입니다(신재생에너지로 분류하지 마세요).
- 리츠·부동산·인프라는 리츠·부동산.
- 애매하거나 위에 없으면 기타테마.`

var systemPrompt = fmt.Sprintf(`당신은 ETF 분류 전문가입니다.
ETF 종목명(한글 펀드명 또는 영문 펀드명)을 보고 아래 카테고리 중 정확히 하나로 분류하세요.

사용 가능한 카테고리: %s

%s

반드시 JSON 으로만 응답: {"category": "<카테고리>"}`, categoryList, classifyRules)

// Classifier 는 ETF 종목명을 카테고리로 분류하는 함수. nil 은 "분류기 비활성"을 뜻하며,
// 호출부는 nil 일 때 각자의 폴백을 쓴다.
type Classifier func(name string) (string, error)

// New 는 ETF 종목명 → 카테고리 분류기를 만든다. apiKey 가 비면 nil 을 반환한다.
// 결과는 호출부 영구 캐시에 저장돼 종목당 1회만 호출된다.
func New(apiKey, model string) Classifier {
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
