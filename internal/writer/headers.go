// Package writer 는 Google Sheets 매매일지 시트의 생성/중복필터/삽입/읽기를 담당한다.
// Python modules/sheet_writer.py 의 Go 포팅.
package writer

import "github.com/kenshin579/auto-trading-journal/internal/sheets"

// DomesticHeaders 는 국내계좌 시트 헤더(10컬럼). (Python DOMESTIC_HEADERS)
var DomesticHeaders = []string{
	"일자", "구분", "종목코드", "종목명", "수량", "단가", "금액",
	"수수료", "손익금액", "수익률(%)",
}

// OldDomesticHeadersV1 는 종목코드 컬럼 추가 이전(9컬럼) 헤더(마이그레이션 감지용).
// (Python OLD_DOMESTIC_HEADERS_V1)
var OldDomesticHeadersV1 = []string{
	"일자", "구분", "종목명", "수량", "단가", "금액",
	"수수료", "손익금액", "수익률(%)",
}

// ForeignHeaders 는 해외계좌 시트 헤더(15컬럼). (Python FOREIGN_HEADERS)
var ForeignHeaders = []string{
	"일자", "구분", "통화", "종목코드", "종목명", "수량", "단가",
	"금액(외화)", "환율", "금액(원화)", "수수료", "세금",
	"손익(외화)", "손익(원화)", "수익률(%)",
}

// DomesticFormats 는 국내계좌 컬럼별 숫자 포맷(1-based col). (Python DOMESTIC_FORMATS)
var DomesticFormats = []sheets.ColumnFormat{
	{Col: 5, Pattern: "#,##0"},                   // E: 수량
	{Col: 6, Pattern: "₩#,##0"},                  // F: 단가
	{Col: 7, Pattern: "₩#,##0"},                  // G: 금액
	{Col: 8, Pattern: "₩#,##0"},                  // H: 수수료
	{Col: 9, Pattern: "₩#,##0"},                  // I: 손익금액
	{Col: 10, Pattern: "0.00%", Type: "PERCENT"}, // J: 수익률
}

// ForeignFormatsCommon 은 해외계좌 통화 무관 컬럼 포맷(1-based col). (Python FOREIGN_FORMATS_COMMON)
var ForeignFormatsCommon = []sheets.ColumnFormat{
	{Col: 6, Pattern: "#,##0"},                   // F: 수량
	{Col: 9, Pattern: "#,##0.00"},                // I: 환율
	{Col: 10, Pattern: "₩#,##0"},                 // J: 금액(원화)
	{Col: 14, Pattern: "₩#,##0"},                 // N: 손익(원화)
	{Col: 15, Pattern: "0.00%", Type: "PERCENT"}, // O: 수익률
}

// ForeignCurrencyCols 는 해외계좌 통화별 외화 컬럼(1-based). (Python FOREIGN_CURRENCY_COLS)
// G:단가, H:금액외화, K:수수료, L:세금, M:손익외화
var ForeignCurrencyCols = []int{7, 8, 11, 12, 13}

// CurrencyPatterns 는 통화별 포맷 패턴. (Python CURRENCY_PATTERNS)
var CurrencyPatterns = map[string]string{
	"USD": "$#,##0.00",
	"JPY": "¥#,##0",
	"EUR": "€#,##0.00",
	"GBP": "£#,##0.00",
	"CNY": "¥#,##0.00",
}

// CurrencyPatternDefault 는 통화 매핑이 없을 때 기본 패턴. (Python CURRENCY_PATTERN_DEFAULT)
const CurrencyPatternDefault = "#,##0.00"

// indexOf 는 문자열 슬라이스에서 값의 0-based 위치를 반환한다(없으면 -1).
func indexOf(ss []string, v string) int {
	for i, s := range ss {
		if s == v {
			return i
		}
	}
	return -1
}
