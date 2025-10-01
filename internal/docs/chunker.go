package docs

import (
	"github.com/tmc/langchaingo/textsplitter"
)

// mdChunker wraps langchaingo's RecursiveCharacter splitter with markdown-aware separators.
type mdChunker struct {
	s textsplitter.RecursiveCharacter
}

func NewMDChunker(chunkSize, overlap int) mdChunker {
	if chunkSize <= 0 {
		chunkSize = 1000
	}
	if overlap < 0 {
		overlap = 0
	}
	return mdChunker{
		s: textsplitter.NewRecursiveCharacter(
			textsplitter.WithSeparators([]string{
				"\n```", // code fences
				"\n# ", "\n## ", "\n### ",
				"\n- ", "\n* ", // lists
				"\n", // line
				"",   // fallback
			}),
			textsplitter.WithChunkSize(chunkSize),
			textsplitter.WithChunkOverlap(overlap),
		),
	}
}

func (c mdChunker) Split(text string) []string {
	parts, err := c.s.SplitText(text)
	if err != nil || len(parts) == 0 {
		return []string{text}
	}
	return parts
}
