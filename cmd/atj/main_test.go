package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kenshin579/auto-trading-journal/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/unicode/norm"
)

type stubResolver map[string]string

func (s stubResolver) Resolve(name string) string { return s[name] }

func TestEnrichDomesticCodes(t *testing.T) {
	trades := []model.Trade{
		{StockName: "삼성전자", Account: "미래에셋증권_국내계좌"},
		{StockName: "AAPL", StockCode: "AAPL", Account: "미래에셋증권_해외계좌"},
		{StockName: "이미코드", StockCode: "000000", Account: "미래에셋증권_국내계좌"},
	}
	enrichDomesticCodes(trades, stubResolver{"삼성전자": "005930"})
	assert.Equal(t, "005930", trades[0].StockCode) // 보강됨
	assert.Equal(t, "AAPL", trades[1].StockCode)   // 해외 미보강
	assert.Equal(t, "000000", trades[2].StockCode) // 기존 코드 유지
}

func TestEnrichDomesticCodes_NoMatchKeepsEmpty(t *testing.T) {
	trades := []model.Trade{
		{StockName: "없는종목", Account: "미래에셋증권_국내계좌"},
	}
	enrichDomesticCodes(trades, stubResolver{})
	assert.Equal(t, "", trades[0].StockCode)
}

func TestParseLogLevel(t *testing.T) {
	cases := map[string]bool{
		"DEBUG": true, "info": true, "Warning": true, "ERROR": true, "bogus": false,
	}
	for in, ok := range cases {
		_, err := parseLogLevel(in)
		if ok {
			assert.NoError(t, err, in)
		} else {
			assert.Error(t, err, in)
		}
	}
}

func TestScanCSVFiles_NFCAndSorted(t *testing.T) {
	root := t.TempDir()

	// 두 증권사 디렉토리 (정렬 순서 검증을 위해 역순으로 생성)
	mkCSV := func(broker, file string) {
		dir := filepath.Join(root, broker)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, file), []byte("h\n"), 0o644))
	}
	// NFD 로 분해된 한글 폴더명 (macOS 시뮬레이션). 한 broker 의 파일 정렬과
	// NFC 정규화를 검증한다.
	nfdBroker := norm.NFD.String("미래에셋증권")
	mkCSV(nfdBroker, "해외계좌.csv")
	mkCSV(nfdBroker, "국내계좌.csv")
	mkCSV(nfdBroker, "메모.txt") // CSV 아닌 파일은 무시

	files, err := scanCSVFiles(root)
	require.NoError(t, err)
	require.Len(t, files, 2)

	// 파일 정렬: "국내계좌.csv" < "해외계좌.csv"
	assert.Equal(t, "국내계좌", files[0].accountType)
	assert.Equal(t, "해외계좌", files[1].accountType)

	// NFC 정규화 확인: 디스크가 NFD 였어도 반환 broker/accountType 은 NFC.
	assert.Equal(t, "미래에셋증권", files[0].broker)
	assert.True(t, norm.NFC.IsNormalString(files[0].broker))
	assert.True(t, norm.NFC.IsNormalString(files[0].accountType))
}

func TestScanCSVFiles_MissingDir(t *testing.T) {
	files, err := scanCSVFiles(filepath.Join(t.TempDir(), "nope"))
	require.NoError(t, err)
	assert.Nil(t, files)
}

type stubBizcat map[string][2]string

func (s stubBizcat) Resolve(code string) (string, string) {
	v, ok := s[code]
	if !ok {
		return "", ""
	}
	return v[0], v[1]
}

func TestEnrichSectors_DomesticOnly(t *testing.T) {
	trades := []model.Trade{
		{StockCode: "005930", Account: "미래에셋증권_국내계좌"},
		{StockCode: "AAPL", Account: "미래에셋증권_해외계좌"},
		{StockCode: "", Account: "미래에셋증권_국내계좌"}, // 코드 없음 → 미보강
	}
	enrichSectors(trades, stubBizcat{"005930": {"전기·전자", "반도체"}})
	assert.Equal(t, "전기·전자", trades[0].Sector)
	assert.Equal(t, "반도체", trades[0].Industry)
	assert.Equal(t, "", trades[1].Sector) // 해외 미보강
	assert.Equal(t, "", trades[2].Sector) // 코드 없음
}
