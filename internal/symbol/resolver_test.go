package symbol

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseMstLines(t *testing.T) {
	// prefix: 단축코드(9 bytes) + 표준코드(12 bytes) = 21 bytes
	// name: "삼성전자" = 12 bytes (UTF-8)
	// fwf suffix: 5 bytes → fwfLen=5
	// Go byte slice: line[21:len(line)-5] = "삼성전자"
	line := "005930   " + "KR7005930003" + "삼성전자" + "XXXXX"
	got := parseMstLines(line+"\n", 5)
	assert.Equal(t, "005930", got["삼성전자"])
}
