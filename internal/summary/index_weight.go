package summary

// 그룹(상위 묶음). 미분류는 지수/나머지 어디에도 속하지 않는 독립 그룹이다.
const (
	groupIndex   = "지수"
	groupOther   = "나머지"
	groupUnknown = "미분류"
)

// 버킷(표시 줄 라벨).
const (
	bucketSP500      = "S&P500"
	bucketNasdaq     = "나스닥"
	bucketKorea      = "한국(코스피·코스닥)"
	bucketOtherIndex = "기타 지역·전세계"
	bucketStock      = "개별종목"
	bucketTheme      = "테마·섹터 ETF"
	bucketDividend   = "배당·전략 ETF"
	bucketBondGold   = "채권·금·현금성 ETF"
	bucketUnknown    = "미분류"
)

// etfBuckets 는 ETF 카테고리(etfclass.Categories) → (그룹, 버킷) 매핑.
// etfclass 에 카테고리를 추가하면 여기에도 넣어야 한다(TestETFBuckets_CoversTaxonomy 가 강제).
var etfBuckets = map[string][2]string{
	"S&P500":     {groupIndex, bucketSP500},
	"나스닥":        {groupIndex, bucketNasdaq},
	"한국주식":       {groupIndex, bucketKorea},
	"미국주식(기타)":   {groupIndex, bucketOtherIndex},
	"중국주식":       {groupIndex, bucketOtherIndex},
	"일본주식":       {groupIndex, bucketOtherIndex},
	"인도주식":       {groupIndex, bucketOtherIndex},
	"베트남주식":      {groupIndex, bucketOtherIndex},
	"글로벌주식":      {groupIndex, bucketOtherIndex},
	"반도체":        {groupOther, bucketTheme},
	"2차전지":       {groupOther, bucketTheme},
	"바이오·헬스케어":   {groupOther, bucketTheme},
	"AI·로봇":      {groupOther, bucketTheme},
	"신재생에너지":     {groupOther, bucketTheme},
	"원자력":        {groupOther, bucketTheme},
	"방위·우주항공":    {groupOther, bucketTheme},
	"자동차":        {groupOther, bucketTheme},
	"금융":         {groupOther, bucketTheme},
	"건설":         {groupOther, bucketTheme},
	"필수소비재":      {groupOther, bucketTheme},
	"IT·인터넷":     {groupOther, bucketTheme},
	"리츠·부동산":     {groupOther, bucketTheme},
	"기타테마":       {groupOther, bucketTheme},
	"배당":         {groupOther, bucketDividend},
	"팩터·스타일":     {groupOther, bucketDividend},
	"채권":         {groupOther, bucketBondGold},
	"원자재":        {groupOther, bucketBondGold},
	"통화·단기금리":    {groupOther, bucketBondGold},
}

// bucketOf 는 거래의 (섹터, 산업)으로 표시 그룹/버킷을 정한다.
//   - 섹터가 비면 미분류(FMP 미커버/미지원 통화/키 없음).
//   - 섹터가 "ETF" 가 아니면 개별종목.
//   - ETF 인데 산업이 taxonomy 밖이면(분류기 없을 때의 KIS 지수명·FMP 산업 폴백 등) 미분류 —
//     임의로 테마에 넣지 않는다. 지수인지 아닌지 모르는 것을 아는 척하면 배분 판단이 틀어진다.
func bucketOf(sector, industry string) (group, bucket string) {
	if sector == "" {
		return groupUnknown, bucketUnknown
	}
	if sector != "ETF" {
		return groupOther, bucketStock
	}
	if gb, ok := etfBuckets[industry]; ok {
		return gb[0], gb[1]
	}
	return groupUnknown, bucketUnknown
}
