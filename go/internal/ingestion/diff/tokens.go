package diff

import (
	"sync"

	"github.com/pkoukk/tiktoken-go"
)

const approxCharsPerToken = 4

var (
	tokenEncoderOnce sync.Once
	tokenEncoder     *tiktoken.Tiktoken

	estimateTokensFunc = defaultEstimateTokens
)

func estimateTokens(text string) int {
	return estimateTokensFunc(text)
}

func defaultEstimateTokens(text string) int {
	enc := getTokenEncoder()
	if enc != nil {
		tokens := enc.Encode(text, nil, nil)
		if len(tokens) > 0 {
			return len(tokens)
		}
	}
	return maxInt(1, len(text)/approxCharsPerToken)
}

func getTokenEncoder() *tiktoken.Tiktoken {
	tokenEncoderOnce.Do(func() {
		enc, err := tiktoken.EncodingForModel("gpt-4o-mini")
		if err != nil {
			enc, err = tiktoken.GetEncoding("cl100k_base")
		}
		tokenEncoder = enc
	})
	return tokenEncoder
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
