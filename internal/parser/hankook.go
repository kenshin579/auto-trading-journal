package parser

import (
	"fmt"

	"github.com/kenshin579/auto-trading-journal/internal/model"
)

// HankookDomestic: 한국투자증권 국내계좌 파서 (17컬럼, 쌍따옴표, 헤더 1행 건너뜀)
type HankookDomestic struct{}

func (HankookDomestic) Name() string { return "HankookDomesticParser" }

func (HankookDomestic) CanParse(h []string) bool {
	return hasAll(h, "매매일자", "종목코드", "매입단가")
}

func (HankookDomestic) Parse(path, account string) ([]model.Trade, error) {
	rows, err := readCSVRows(path)
	if err != nil {
		return nil, err
	}
	var out []model.Trade
	for i := 1; i < len(rows); i++ {
		row := rows[i]
		if len(row) < 17 {
			continue
		}
		// name checked before date (matches Python behavior)
		name := trimSpace(row[1])
		if name == "" {
			continue
		}
		dateRaw := trimSpace(row[0])
		if dateRaw == "" {
			return nil, fmt.Errorf("날짜가 비어있습니다: %s, %d행", base(path), i+1)
		}
		date := convertDate(dateRaw)
		stockCode := trimSpace(row[2])
		buyPrice := parseFloat(row[6])   // 매입단가
		buyQty := parseFloat(row[7])     // 매수수량
		sellPrice := parseFloat(row[8])  // 매도단가
		sellQty := parseFloat(row[9])    // 매도수량
		buyAmount := parseFloat(row[10]) // 매수금액
		sellAmount := parseFloat(row[11]) // 매도금액
		realizedProfit := parseFloat(row[12])
		profitRate := parseFloat(row[13])
		commission := parseFloat(row[14])
		tax := parseFloat(row[16])
		if buyQty > 0 && buyAmount > 0 {
			out = append(out, model.Trade{Date: date, TradeType: "매수", StockName: name,
				StockCode: stockCode, Quantity: buyQty, Price: buyPrice, Amount: buyAmount,
				Currency: "KRW", ExchangeRate: 1, AmountKRW: buyAmount, Account: account})
		}
		if sellQty > 0 && sellAmount > 0 {
			totalFee := commission + tax
			out = append(out, model.Trade{Date: date, TradeType: "매도", StockName: name,
				StockCode: stockCode, Quantity: sellQty, Price: sellPrice, Amount: sellAmount,
				Currency: "KRW", ExchangeRate: 1, AmountKRW: sellAmount, Fee: totalFee,
				Profit: realizedProfit, ProfitKRW: realizedProfit, ProfitRate: profitRate,
				Account: account})
		}
	}
	return out, nil
}
