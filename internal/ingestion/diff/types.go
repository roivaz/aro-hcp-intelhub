package diff

import (
	"context"
	"errors"
	"strings"
	"time"
)

type FailureCategory string

const (
	FailureCategoryLargeDiff FailureCategory = "large_diff"
	FailureCategoryTimeout   FailureCategory = "timeout"
	FailureCategoryError     FailureCategory = "error"
)

type Analysis struct {
	RichDescription    string          `json:"rich_description"`
	AnalysisSuccessful bool            `json:"analysis_successful"`
	FailureReason      string          `json:"failure_reason,omitempty"`
	FailureCategory    FailureCategory `json:"failure_category,omitempty"`
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

func GetFailureDetails(err error) (reason string, category FailureCategory) {
	if err == nil {
		return "", ""
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout: " + strings.TrimSpace(err.Error()), FailureCategoryTimeout
	}
	msg := strings.TrimSpace(err.Error())
	if msg == "" {
		msg = "unknown failure"
	}
	return msg, FailureCategoryError
}

func GetFailureCategory(err error) FailureCategory {
	_, category := GetFailureDetails(err)
	return category
}
