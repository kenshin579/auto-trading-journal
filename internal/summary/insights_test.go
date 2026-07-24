package summary

import (
	"math"
	"testing"

	"github.com/kenshin579/auto-trading-journal/internal/model"
)

// sell 는 테스트용 매도 거래를 만든다.
func sell(date string, profitKRW, profitRate, amountKRW float64) model.Trade {
	return model.Trade{
		Date: date, TradeType: "매도",
		ProfitKRW: profitKRW, ProfitRate: profitRate, AmountKRW: amountKRW,
	}
}

func TestCalcStreaks(t *testing.T) {
	// 손익 부호 +,+,-,+ → 최대 연승 2, 최대 연패 1, 현재 연승 1, 현재 연패 0.
	sells := []model.Trade{
		sell("2026-01-01", 10, 0, 0),
		sell("2026-01-02", 20, 0, 0),
		sell("2026-01-03", -5, 0, 0),
		sell("2026-01-04", 7, 0, 0),
	}
	maxW, maxL, curW, curL := calcStreaks(sells)
	if maxW != 2 || maxL != 1 || curW != 1 || curL != 0 {
		t.Errorf("got (maxW=%d,maxL=%d,curW=%d,curL=%d), want (2,1,1,0)", maxW, maxL, curW, curL)
	}

	// 마지막이 손실로 끝나면: +,-,- → 최대연승1, 최대연패2, 현재연승0, 현재연패2.
	sells2 := []model.Trade{
		sell("2026-01-01", 10, 0, 0),
		sell("2026-01-02", -5, 0, 0),
		sell("2026-01-03", -3, 0, 0),
	}
	maxW, maxL, curW, curL = calcStreaks(sells2)
	if maxW != 1 || maxL != 2 || curW != 0 || curL != 2 {
		t.Errorf("got (maxW=%d,maxL=%d,curW=%d,curL=%d), want (1,2,0,2)", maxW, maxL, curW, curL)
	}

	// profit == 0 은 loss(else 분기)로 취급. (Python `if t.profit_krw > 0`)
	sells3 := []model.Trade{sell("2026-01-01", 0, 0, 0)}
	maxW, maxL, _, curL = calcStreaks(sells3)
	if maxW != 0 || maxL != 1 || curL != 1 {
		t.Errorf("zero-profit: got (maxW=%d,maxL=%d,curL=%d), want (0,1,1)", maxW, maxL, curL)
	}
}

func TestCalcDayOfWeekStats(t *testing.T) {
	// 2026-01-05 = 월요일(weekday 0), 2026-01-06 = 화요일(1).
	sells := []model.Trade{
		sell("2026-01-05", 100, 0, 0), // 월, win
		sell("2026-01-05", -50, 0, 0), // 월, loss
		sell("2026-01-06", 30, 0, 0),  // 화, win
	}
	stats := calcDayOfWeekStats(sells)

	mon, ok := stats[0]
	if !ok {
		t.Fatalf("expected monday stats")
	}
	if mon.count != 2 || mon.profitSum != 50 {
		t.Errorf("mon = (count=%d,profit=%v), want (2,50)", mon.count, mon.profitSum)
	}
	if mon.winRate != 50 { // 1 win / 2 * 100
		t.Errorf("mon winRate = %v, want 50", mon.winRate)
	}

	tue, ok := stats[1]
	if !ok {
		t.Fatalf("expected tuesday stats")
	}
	if tue.count != 1 || tue.profitSum != 30 || tue.winRate != 100 {
		t.Errorf("tue = (count=%d,profit=%v,wr=%v), want (1,30,100)", tue.count, tue.profitSum, tue.winRate)
	}

	if _, ok := stats[2]; ok {
		t.Errorf("did not expect wednesday stats")
	}
}

func TestWeekdayMondayZero(t *testing.T) {
	cases := map[string]int{
		"2026-01-05": 0, // 월
		"2026-01-06": 1, // 화
		"2026-01-10": 5, // 토
		"2026-01-11": 6, // 일
	}
	for date, want := range cases {
		if got := weekdayMondayZero(date); got != want {
			t.Errorf("weekday(%s) = %d, want %d", date, got, want)
		}
	}
	if got := weekdayMondayZero("bad"); got != -1 {
		t.Errorf("weekday(bad) = %d, want -1", got)
	}
}

func TestCalcMonthlyProfits(t *testing.T) {
	sells := []model.Trade{
		sell("2026-02-10", 50, 0, 0),
		sell("2026-01-15", 20, 0, 0),
		sell("2026-02-20", 30, 0, 0),
	}
	out := calcMonthlyProfits(sells)
	if len(out) != 2 {
		t.Fatalf("expected 2 months, got %d", len(out))
	}
	// 사전식 정렬: 2026-01 먼저.
	if out[0].month != "2026-01" || out[0].profit != 20 {
		t.Errorf("out[0] = (%s,%v), want (2026-01,20)", out[0].month, out[0].profit)
	}
	if out[1].month != "2026-02" || out[1].profit != 80 {
		t.Errorf("out[1] = (%s,%v), want (2026-02,80)", out[1].month, out[1].profit)
	}
}

func TestCalcTradeFrequency(t *testing.T) {
	// 2026-01: 1건, 2026-02: 3건, 2026-03: 2건. avg = 6/3 = 2.
	sells := []model.Trade{
		sell("2026-01-15", 0, 0, 0),
		sell("2026-02-01", 0, 0, 0),
		sell("2026-02-02", 0, 0, 0),
		sell("2026-02-03", 0, 0, 0),
		sell("2026-03-01", 0, 0, 0),
		sell("2026-03-02", 0, 0, 0),
	}
	f := calcTradeFrequency(sells)
	if f.avgMonthly != 2 {
		t.Errorf("avgMonthly = %v, want 2", f.avgMonthly)
	}
	if f.maxMonth != "2026-02" || f.maxCount != 3 {
		t.Errorf("max = (%s,%d), want (2026-02,3)", f.maxMonth, f.maxCount)
	}
	if f.minMonth != "2026-01" || f.minCount != 1 {
		t.Errorf("min = (%s,%d), want (2026-01,1)", f.minMonth, f.minCount)
	}

	// 빈 입력.
	e := calcTradeFrequency(nil)
	if e.maxMonth != "-" || e.minMonth != "-" || e.avgMonthly != 0 {
		t.Errorf("empty freq = %+v, want zero/dashes", e)
	}
}

func TestCalcMonthlyTrend(t *testing.T) {
	// 2026-01: 매도 2건, profit +100/-40, amount 1000/600.
	// 2026-02: 매도 1건, profit +60, amount 400.
	sells := []model.Trade{
		sell("2026-01-10", 100, 20, 1000), // win, rate 20%
		sell("2026-01-20", -40, -10, 600), // loss, rate -10%
		sell("2026-02-05", 60, 15, 400),   // win, rate 15%
	}
	out := calcMonthlyTrend(sells)
	if len(out) != 2 {
		t.Fatalf("expected 2 months, got %d", len(out))
	}

	jan := out[0]
	if jan.month != "2026-01" || jan.sellCount != 2 {
		t.Fatalf("jan = (%s,%d), want (2026-01,2)", jan.month, jan.sellCount)
	}
	// profit_krw = 100-40 = 60.
	if jan.profitKRW != 60 {
		t.Errorf("jan profitKRW = %v, want 60", jan.profitKRW)
	}
	// return_rate = 60 / (1000+600) = 0.0375.
	if math.Abs(jan.returnRate-0.0375) > 1e-9 {
		t.Errorf("jan returnRate = %v, want 0.0375", jan.returnRate)
	}
	// win_rate = 1/2 = 0.5.
	if jan.winRate != 0.5 {
		t.Errorf("jan winRate = %v, want 0.5", jan.winRate)
	}
	// avg_profit_rate = 20/100 = 0.2; avg_loss_rate = -10/100 = -0.1.
	if math.Abs(jan.avgProfitRate-0.2) > 1e-9 || math.Abs(jan.avgLossRate-(-0.1)) > 1e-9 {
		t.Errorf("jan rates = (%v,%v), want (0.2,-0.1)", jan.avgProfitRate, jan.avgLossRate)
	}
	// pl_ratio = abs(0.2 / -0.1) = 2.0.
	if jan.plRatio != 2.0 {
		t.Errorf("jan plRatio = %v, want 2", jan.plRatio)
	}
	// profit_factor = 100 / 40 = 2.5.
	if jan.profitFactor != 2.5 {
		t.Errorf("jan profitFactor = %v, want 2.5", jan.profitFactor)
	}
	// expectancy = (avg_profit_amount * win) - (avg_loss_amount * loss)
	//   = (100 * 0.5) - (40 * 0.5) = 50 - 20 = 30.
	if math.Abs(jan.expectancy-30) > 1e-9 {
		t.Errorf("jan expectancy = %v, want 30", jan.expectancy)
	}
	// 첫 달은 mom_change 없음.
	if jan.momChangeOK {
		t.Errorf("jan should have no mom_change")
	}

	feb := out[1]
	// mom_change = (60 - 60) / abs(60) = 0.
	if !feb.momChangeOK || feb.momChange != 0 {
		t.Errorf("feb mom = (ok=%v,%v), want (true,0)", feb.momChangeOK, feb.momChange)
	}
}

func TestGroupConsecutiveRows(t *testing.T) {
	got := groupConsecutiveRows([]int{2, 3, 4, 7, 9, 10})
	want := [][2]int{{2, 4}, {7, 7}, {9, 10}}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("group[%d] = %v, want %v", i, got[i], want[i])
		}
	}
	if groupConsecutiveRows(nil) != nil {
		t.Errorf("nil input should give nil")
	}
}

func TestFormatThousands(t *testing.T) {
	cases := map[float64]string{
		0:        "0",
		1234:     "1,234",
		-1234567: "-1,234,567",
		999:      "999",
		1000:     "1,000",
	}
	for in, want := range cases {
		if got := formatThousands(in); got != want {
			t.Errorf("formatThousands(%v) = %q, want %q", in, got, want)
		}
	}
}

// tr 는 테스트용 거래를 만든다(계좌별 종목수 집계용).
func tr(account, tradeType, code, name, currency string, qty float64) model.Trade {
	return model.Trade{
		Account: account, TradeType: tradeType,
		StockCode: code, StockName: name, Currency: currency, Quantity: qty,
	}
}

func TestAggregateAccountStockCount(t *testing.T) {
	trades := []model.Trade{
		// 한국투자: 삼성전자 10주 매수 후 10주 전량 매도 → 거래O, 보유X
		tr("한국투자증권_국내계좌", "매수", "005930", "삼성전자", "KRW", 10),
		tr("한국투자증권_국내계좌", "매도", "005930", "삼성전자", "KRW", 10),
		// 한국투자: SK하이닉스 5주 매수 후 2주 매도 → 부분매도, 보유O
		tr("한국투자증권_국내계좌", "매수", "000660", "SK하이닉스", "KRW", 5),
		tr("한국투자증권_국내계좌", "매도", "000660", "SK하이닉스", "KRW", 2),
		// 미래에셋: AAPL 3주 매수(미매도) → 보유O
		tr("미래에셋증권_해외계좌", "매수", "", "AAPL", "USD", 3),
		// 미래에셋: TSLA 1주 매수(미매도), 코드 없음 → 이름으로 구분되어야 함
		tr("미래에셋증권_해외계좌", "매수", "", "TSLA", "USD", 1),
	}

	got := aggregateAccountStockCount(trades)

	want := []accountStockCount{
		{account: "미래에셋증권_해외계좌", held: 2, total: 2},
		{account: "한국투자증권_국내계좌", held: 1, total: 2},
	}
	if len(got) != len(want) {
		t.Fatalf("len: got %d, want %d (%+v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d]: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestAggregateAccountStockCountEmpty(t *testing.T) {
	if got := aggregateAccountStockCount(nil); len(got) != 0 {
		t.Errorf("empty input: got %+v, want empty", got)
	}
}
