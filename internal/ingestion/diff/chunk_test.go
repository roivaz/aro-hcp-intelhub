package diff

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/roivaz/aro-hcp-intelhub/internal/logging"
)

func TestSplitDiffIntoFiles(t *testing.T) {
	diff := `diff --git a/file1.txt b/file1.txt
index 123..456 100644
--- a/file1.txt
+++ b/file1.txt
@@ -1 +1 @@
-foo
+bar

diff --git a/file2.txt b/file2.txt
index 789..abc 100644
--- a/file2.txt
+++ b/file2.txt
@@ -1 +1 @@
-baz
+qux
`
	chunks := splitDiffIntoFiles(diff, logging.New(logr.Discard()))
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	if chunks[0][0] != "file1.txt" {
		t.Fatalf("unexpected file path %s", chunks[0][0])
	}
	if chunks[1][0] != "file2.txt" {
		t.Fatalf("unexpected file path %s", chunks[1][0])
	}
}

func TestFilterGeneratedFiles(t *testing.T) {
	patterns := buildIgnorePatterns()
	chunks := [][2]string{{"package-lock.json", "chunk"}, {"file.txt", "chunk"}}
	included, skipped := filterGeneratedFiles(chunks, patterns)
	if len(included) != 1 || included[0][0] != "file.txt" {
		t.Fatalf("expected file.txt included")
	}
	if len(skipped) != 1 || skipped[0][0] != "package-lock.json" {
		t.Fatalf("expected package-lock.json skipped")
	}
}

func TestBuildDocuments_SplitsLargeChunk(t *testing.T) {
	oldEstimate := estimateTokensFunc
	estimateTokensFunc = func(text string) int { return len(text) / 10 }
	defer func() { estimateTokensFunc = oldEstimate }()

	cfg := Config{MaxContextTokens: 20}
	chunks := [][2]string{{"file.txt", longDiff()}}

	docs, stats := buildDocuments(chunks, logging.New(logr.Discard()), cfg)
	if len(docs) == 0 {
		t.Fatalf("expected documents")
	}
	if stats.FilesIncluded != 1 {
		t.Fatalf("expected 1 included file")
	}
	for _, doc := range docs {
		if doc.TokenCount > cfg.MaxContextTokens {
			t.Fatalf("chunk exceeds token limit")
		}
	}
}

func longDiff() string {
	base := "@@ -0,0 +1,0 @@\n"
	body := ""
	for i := 0; i < 200; i++ {
		body += "+line\n"
	}
	return "diff --git a/file.txt b/file.txt\n" + base + body
}
