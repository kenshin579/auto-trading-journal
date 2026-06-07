package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectParser(t *testing.T) {
	p, err := DetectParser("../../testdata/mirae_domestic.csv")
	require.NoError(t, err)
	assert.Equal(t, "MiraeDomesticParser", p.Name())

	p, err = DetectParser("../../testdata/hankook_domestic.csv")
	require.NoError(t, err)
	assert.Equal(t, "HankookDomesticParser", p.Name())

	_, err = DetectParser("../../testdata/unsupported.csv")
	require.Error(t, err)
}
