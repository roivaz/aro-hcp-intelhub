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

// DocumentChunk represents an embedded chunk of a documentation file.
type DocumentChunk struct {
	bun.BaseModel `bun:"table:documents"`

	ID             string          `bun:"id,pk"` // sha256(repo|path|commit|idx|text)
	Repo           string          `bun:"repo"`
	Component      *string         `bun:"component,nullzero"`
	Path           string          `bun:"path"` // repo-relative path
	CommitSHA      string          `bun:"commit_sha"`
	DocType        string          `bun:"doc_type"` // readme|docs|adr|runbook|other
	ChunkIndex     int             `bun:"chunk_index"`
	ChunkText      string          `bun:"chunk_text"`
	Embedding      pgvector.Vector `bun:"embedding"` // vector(768)
	EmbeddingModel string          `bun:"embedding_model"`
	UpdatedAt      time.Time       `bun:"updated_at,nullzero,default:now()"`
	SourceURL      *string         `bun:"source_url,nullzero"`
}

func (DocumentChunk) TableName() string { return "documents" }
