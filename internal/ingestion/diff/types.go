package diff

import "time"

type Analysis struct {
	RichDescription    string `json:"rich_description"`
	AnalysisSuccessful bool   `json:"analysis_successful"`
	FailureReason      string `json:"failure_reason,omitempty"`
}

type PRMetadata struct {
	Number         int
	Title          string
	Body           string
	Author         string
	BaseRef        string
	BaseCommitSHA  string
	HeadCommitSHA  string
	MergeCommitSHA string
	CreatedAt      time.Time
	MergedAt       *time.Time
}

type Document struct {
	FilePath   string
	Content    string
	TokenCount int
}

type DocumentStats struct {
	FilesTotal    int
	FilesIncluded int
	FilesFiltered int
	MaxTokens     float64
	MedianTokens  float64
}
