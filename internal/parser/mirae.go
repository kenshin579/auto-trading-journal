package parser

import (
	"encoding/csv"
	"fmt"
	"os"

	"github.com/kenshin579/auto-trading-journal/internal/model"
)

// MiraeDomestic: 미래에셋증권 국내계좌 파서 (11컬럼, 헤더+서브헤더 2행 건너뜀)
type MiraeDomestic struct{}

func (MiraeDomestic) Name() string { return "MiraeDomesticParser" }

func (MiraeDomestic) CanParse(h []string) bool {
	return hasAll(h, "일자", "종목명", "기간 중 매수")
}

func (MiraeDomestic) Parse(path, account string) ([]model.Trade, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	var out []model.Trade
	for i := 2; i < len(rows); i++ {
		row := rows[i]
		if len(row) < 11 {
			continue
		}
		dateRaw := trimSpace(row[0])
		name := trimSpace(row[1])
		if dateRaw == "" && name == "" {
			continue
		}
		if dateRaw == "" {
			return nil, fmt.Errorf("날짜가 비어있습니다: %s, %d행", base(path), i+1)
		}
		if name == "" {
			continue
		}
		date := convertDate(dateRaw)
		buyQty, buyPrice, buyAmt := parseFloat(row[2]), parseFloat(row[3]), parseFloat(row[4])
		sellQty, sellPrice, sellAmt := parseFloat(row[5]), parseFloat(row[6]), parseFloat(row[7])
		fee, profit, profitRate := parseFloat(row[8]), parseFloat(row[9]), parseFloat(row[10])
		if buyQty > 0 {
			out = append(out, model.Trade{Date: date, TradeType: "매수", StockName: name,
				Quantity: buyQty, Price: buyPrice, Amount: buyAmt, Currency: "KRW",
				ExchangeRate: 1, AmountKRW: buyAmt, Account: account})
		}
		if sellQty > 0 {
			out = append(out, model.Trade{Date: date, TradeType: "매도", StockName: name,
				Quantity: sellQty, Price: sellPrice, Amount: sellAmt, Currency: "KRW",
				ExchangeRate: 1, AmountKRW: sellAmt, Fee: fee, Profit: profit,
				ProfitKRW: profit, ProfitRate: profitRate, Account: account})
		}
	}
	return out, nil
}

// MiraeForeign: 미래에셋증권 해외계좌 파서 (25컬럼, 헤더 1행 건너뜀)
type MiraeForeign struct{}

func (MiraeForeign) Name() string { return "MiraeForeignParser" }

func (MiraeForeign) CanParse(h []string) bool {
	return hasAll(h, "매매일", "통화", "종목번호")
}

func (MiraeForeign) Parse(path, account string) ([]model.Trade, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	var out []model.Trade
	for i := 1; i < len(rows); i++ {
		row := rows[i]
		if len(row) < 25 {
			continue
		}
		dateRaw := trimSpace(row[0])
		currency := trimSpace(row[1])
		stockCode := trimSpace(row[2])
		name := trimSpace(row[3])
		if dateRaw == "" && name == "" {
			continue
		}
		if dateRaw == "" {
			return nil, fmt.Errorf("날짜가 비어있습니다: %s, %d행", base(path), i+1)
		}
		if name == "" {
			continue
		}
		date := convertDate(dateRaw)
		exchangeRate := parseFloat(row[6])
		buyQty, buyPrice, buyAmt, buyAmtKRW := parseFloat(row[7]), parseFloat(row[8]), parseFloat(row[9]), parseFloat(row[10])
		sellQty, sellPrice, sellAmt, sellAmtKRW := parseFloat(row[11]), parseFloat(row[12]), parseFloat(row[13]), parseFloat(row[14])
		fee, tax := parseFloat(row[15]), parseFloat(row[16])
		profit, profitKRW, profitRate := parseFloat(row[19]), parseFloat(row[22]), parseFloat(row[23])
		if buyQty > 0 {
			out = append(out, model.Trade{Date: date, TradeType: "매수", StockName: name,
				StockCode: stockCode, Quantity: buyQty, Price: buyPrice, Amount: buyAmt,
				Currency: currency, ExchangeRate: exchangeRate, AmountKRW: buyAmtKRW,
				Account: account})
		}
		if sellQty > 0 {
			out = append(out, model.Trade{Date: date, TradeType: "매도", StockName: name,
				StockCode: stockCode, Quantity: sellQty, Price: sellPrice, Amount: sellAmt,
				Currency: currency, ExchangeRate: exchangeRate, AmountKRW: sellAmtKRW,
				Fee: fee, Tax: tax, Profit: profit, ProfitKRW: profitKRW,
				ProfitRate: profitRate, Account: account})
		}
	}
	return out, nil
}
