package db

import (
	"time"

	"github.com/pgvector/pgvector-go"
	"github.com/uptrace/bun"
)

type PREmbedding struct {
	bun.BaseModel `bun:"table:pr_embeddings"`

	ID                 int64            `bun:"id,pk,autoincrement"`
	PRNumber           int              `bun:"pr_number,unique"`
	PRTitle            string           `bun:"pr_title"`
	PRBody             string           `bun:"pr_body"`
	Author             string           `bun:"author"`
	CreatedAt          time.Time        `bun:"created_at"`
	MergedAt           *time.Time       `bun:"merged_at"`
	State              string           `bun:"state"`
	BaseRef            string           `bun:"base_ref"`
	GithubBaseSHA      *string          `bun:"github_base_sha"`
	BaseMergeBaseSHA   *string          `bun:"base_merge_base_sha"`
	HeadCommitSHA      *string          `bun:"head_commit_sha"`
	MergeCommitSHA     *string          `bun:"merge_commit_sha"`
	Embedding          *pgvector.Vector `bun:"embedding"` // Nullable: NULL = not processed yet
	RichDescription    *string          `bun:"rich_description"`
	AnalysisSuccessful bool             `bun:"analysis_successful"`
	FailureReason      *string          `bun:"failure_reason"`
	ProcessedAt        *time.Time       `bun:"processed_at"` // NULL = needs processing
}
