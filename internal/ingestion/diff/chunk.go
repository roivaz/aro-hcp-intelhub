package diff

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/roivaz/aro-hcp-intelhub/internal/logging"
	"github.com/tmc/langchaingo/textsplitter"
)

var diffHeaderRegexp = regexp.MustCompile(`(?m)^diff --git a/(?P<old>.*?) b/(?P<new>.*?)$`)

func splitDiffIntoFiles(diffText string, log logging.Logger) [][2]string {
	if strings.TrimSpace(diffText) == "" {
		return nil
	}

	matches := diffHeaderRegexp.FindAllStringIndex(diffText, -1)
	if len(matches) == 0 {
		return nil
	}

	results := make([][2]string, 0, len(matches))
	for i, loc := range matches {
		start := loc[0]
		end := len(diffText)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		chunk := strings.TrimSpace(diffText[start:end])
		header := diffHeaderRegexp.FindStringSubmatch(chunk)
		if header == nil {
			preview := chunk
			if len(preview) > 80 {
				preview = preview[:80]
			}
			log.Debug("skip chunk without header", "chunk", preview)
			continue
		}
		oldPath := header[diffHeaderRegexp.SubexpIndex("old")]
		newPath := header[diffHeaderRegexp.SubexpIndex("new")]
		file := newPath
		if file == "/dev/null" {
			file = oldPath
		}
		results = append(results, [2]string{file, chunk})
	}
	return results
}

func filterGeneratedFiles(chunks [][2]string, patterns map[string]*regexp.Regexp) (included [][2]string, skipped [][2]string) {
	for _, chunk := range chunks {
		path := chunk[0]
		if ign, reason := shouldIgnoreFile(path, patterns); ign {
			skipped = append(skipped, [2]string{path, reason})
			continue
		}
		included = append(included, chunk)
	}
	return included, skipped
}

func buildDocuments(chunks [][2]string, log logging.Logger, cfg Config) ([]Document, DocumentStats) {
	docs := make([]Document, 0, len(chunks))
	tokenCounts := make([]int, 0, len(chunks))

	chunkSize := cfg.MaxContextTokens
	if chunkSize == 0 {
		chunkSize = 4096
	}
	targetTokens := chunkSize * 3 / 4
	splitter := textsplitter.NewRecursiveCharacter(
		textsplitter.WithSeparators([]string{"\n@@", "\ndiff --git", "\n", ""}),
		textsplitter.WithChunkSize(targetTokens*approxCharsPerToken),
		textsplitter.WithChunkOverlap(400*approxCharsPerToken),
	)

	for _, chunk := range chunks {
		path := chunk[0]
		content := chunk[1]
		docsForFile, counts := splitChunkRecursive(content, path, splitter, log, targetTokens)
		docs = append(docs, docsForFile...)
		tokenCounts = append(tokenCounts, counts...)
	}

	stats := DocumentStats{
		FilesTotal:    len(chunks),
		FilesIncluded: len(chunks),
		FilesFiltered: 0,
		MaxTokens:     0,
	}

	if len(tokenCounts) > 0 {
		sort.Ints(tokenCounts)
		stats.MaxTokens = float64(tokenCounts[len(tokenCounts)-1])
		stats.MedianTokens = float64(tokenCounts[len(tokenCounts)/2])
	}

	return docs, stats
}

func splitChunkRecursive(content, path string, splitter textsplitter.RecursiveCharacter, log logging.Logger, targetTokens int) ([]Document, []int) {
	tokens := estimateTokens(content)

	var chunks []string
	if tokens <= targetTokens {
		chunks = []string{content}
	} else {
		parts, err := splitter.SplitText(content)
		if err != nil || len(parts) == 0 {
			log.Error(err, "splitText failed; using original chunk", "file", path)
			chunks = []string{content}
		} else {
			chunks = parts
		}
	}

	docs := make([]Document, 0, len(chunks))
	counts := make([]int, 0, len(chunks))

	for idx, chunk := range chunks {
		annotated := annotateChunk(chunk, path, idx, len(chunks))
		tokenCount := estimateTokens(annotated)
		docs = append(docs, Document{FilePath: path, Content: annotated, TokenCount: tokenCount})
		counts = append(counts, tokenCount)
	}

	return docs, counts
}

func annotateChunk(content, path string, index, total int) string {
	header := []string{fmt.Sprintf("File: %s", path)}
	if total > 1 {
		header = append(header, fmt.Sprintf("Chunk: %d/%d", index+1, total))
	}
	header = append(header, "")
	return strings.Join(header, "\n") + strings.TrimSpace(content)
}
