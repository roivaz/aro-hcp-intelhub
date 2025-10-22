package db

import (
	"context"
	"database/sql"
	"errors"
	"time"

	pgvector "github.com/pgvector/pgvector-go"
	"github.com/uptrace/bun"

	tooltypes "github.com/roivaz/aro-hcp-intelhub/internal/mcp/tools/types"
)

type SearchRepository struct {
	TraceCacheMax int
	retryFailed   bool
	db            *bun.DB
}

type PRSearchRow struct {
	PREmbedding `bun:",extend"`
	Distance    float64 `bun:"distance"`
}

type DocSearchRow struct {
	DocumentChunk `bun:",extend"`
	Snippet       string  `bun:"snippet"`
	Distance      float64 `bun:"distance"`
}

func NewSearchRepository(database *Database, opts ...func(*SearchRepository)) *SearchRepository {
	repo := &SearchRepository{db: database.Bun()}
	for _, opt := range opts {
		opt(repo)
	}
	return repo
}

func WithTraceCacheMax(n int) func(*SearchRepository) {
	return func(r *SearchRepository) { r.TraceCacheMax = n }
}

func WithRetryFailed(retry bool) func(*SearchRepository) {
	return func(r *SearchRepository) { r.retryFailed = retry }
}

func (r *SearchRepository) LatestMergedPR(ctx context.Context) (time.Time, int, error) {
	var result struct {
		MergedAt sql.NullTime `bun:"merged_at"`
		PRNumber int          `bun:"pr_number"`
	}
	err := r.db.NewSelect().Model((*PREmbedding)(nil)).
		Column("merged_at", "pr_number").
		OrderExpr("merged_at DESC, pr_number DESC").
		Limit(1).
		Scan(ctx, &result)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return time.Time{}, 0, nil
		}
		return time.Time{}, 0, err
	}
	if !result.MergedAt.Valid {
		return time.Time{}, result.PRNumber, nil
	}
	return result.MergedAt.Time, result.PRNumber, nil
}

func (r *SearchRepository) OldestMergedPR(ctx context.Context) (time.Time, int, error) {
	var result struct {
		MergedAt sql.NullTime `bun:"merged_at"`
		PRNumber int          `bun:"pr_number"`
	}
	err := r.db.NewSelect().Model((*PREmbedding)(nil)).
		Column("merged_at", "pr_number").
		OrderExpr("merged_at ASC, pr_number ASC").
		Limit(1).
		Scan(ctx, &result)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return time.Time{}, 0, nil
		}
		return time.Time{}, 0, err
	}
	if !result.MergedAt.Valid {
		return time.Time{}, result.PRNumber, nil
	}
	return result.MergedAt.Time, result.PRNumber, nil
}

func (r *SearchRepository) SearchPRs(ctx context.Context, embedding []float32, limit int) ([]PRSearchRow, error) {
	if limit <= 0 {
		limit = 10
	}
	var results []PRSearchRow
	query := r.db.NewSelect().Model(&results).
		Column(
			"id", "pr_number", "pr_title", "pr_body", "author", "created_at",
			"merged_at", "state", "base_ref", "github_base_sha", "base_merge_base_sha",
			"head_commit_sha", "merge_commit_sha",
		).
		ColumnExpr("embedding <=> ? AS distance", pgvector.NewVector(embedding)).
		Where("embedding IS NOT NULL"). // Only search processed PRs
		OrderExpr("distance")
	query.Limit(limit)

	if err := query.Scan(ctx); err != nil {
		return nil, err
	}
	return results, nil
}

func (r *SearchRepository) SearchDocs(ctx context.Context, embedding []float32, limit int, component, repo *string) ([]DocSearchRow, error) {
	if limit <= 0 {
		limit = 10
	}
	var results []DocSearchRow
	q := r.db.NewSelect().Model(&results).
		Column("id", "repo", "component", "path", "commit_sha", "source_url").
		ColumnExpr("substring(chunk_text for 400) AS snippet").
		ColumnExpr("embedding <=> ? AS distance", pgvector.NewVector(embedding)).
		OrderExpr("distance").
		Limit(limit)
	if component != nil && *component != "" {
		q = q.Where("component = ?", *component)
	}
	if repo != nil && *repo != "" {
		q = q.Where("repo = ?", *repo)
	}
	if err := q.Scan(ctx); err != nil {
		return nil, err
	}
	return results, nil
}

func (r *SearchRepository) GetPRByNumber(ctx context.Context, number int) (*PREmbedding, error) {
	pr := new(PREmbedding)
	err := r.db.NewSelect().Model(pr).Where("pr_number = ?", number).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return pr, nil
}

func (r *SearchRepository) HasPR(ctx context.Context, number int) (bool, error) {
	count, err := r.db.NewSelect().Model((*PREmbedding)(nil)).Where("pr_number = ?", number).Count(ctx)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *SearchRepository) StorePR(ctx context.Context, pr *PREmbedding) error {
	_, err := r.db.NewInsert().Model(pr).On("CONFLICT (pr_number) DO NOTHING").Exec(ctx)
	return err
}

func (r *SearchRepository) GetUnprocessedPRs(ctx context.Context, limit int) ([]*PREmbedding, error) {
	if limit <= 0 {
		limit = 100
	}
	var prs []*PREmbedding
	query := r.db.NewSelect().Model(&prs)

	if r.retryFailed {
		// Include unprocessed PRs OR failed analyses
		query = query.Where("processed_at IS NULL OR analysis_successful = ?", false)
	} else {
		// Only unprocessed PRs
		query = query.Where("processed_at IS NULL")
	}

	err := query.OrderExpr("merged_at DESC").Limit(limit).Scan(ctx)
	return prs, err
}

func (r *SearchRepository) UpdatePRProcessing(ctx context.Context, prNumber int, embedding *pgvector.Vector, richDesc *string, analysisSuccess bool, failureReason *string, failureCategory *string) error {
	now := time.Now()
	_, err := r.db.NewUpdate().
		Model((*PREmbedding)(nil)).
		Set("embedding = ?", embedding).
		Set("rich_description = ?", richDesc).
		Set("analysis_successful = ?", analysisSuccess).
		Set("failure_reason = ?", failureReason).
		Set("failure_category = ?", failureCategory).
		Set("processed_at = ?", now).
		Where("pr_number = ?", prNumber).
		Exec(ctx)
	return err
}

func (r *SearchRepository) CountUnprocessedPRs(ctx context.Context) (int, error) {
	query := r.db.NewSelect().Model((*PREmbedding)(nil))

	if r.retryFailed {
		// Count unprocessed PRs OR failed analyses
		query = query.Where("processed_at IS NULL OR analysis_successful = ?", false)
	} else {
		// Only unprocessed PRs
		query = query.Where("processed_at IS NULL")
	}

	count, err := query.Count(ctx)
	return count, err
}

func (r *SearchRepository) TraceImageCacheGet(ctx context.Context, commitSHA, environment string) (*TraceImageCache, error) {
	entry := new(TraceImageCache)
	err := r.db.NewSelect().Model(entry).
		Where("commit_sha = ? AND environment = ?", commitSHA, environment).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return entry, nil
}

func (r *SearchRepository) TraceImageCacheUpsert(ctx context.Context, commitSHA, environment string, resp tooltypes.TraceImagesResponse) error {
	if r.TraceCacheMax <= 0 {
		return nil
	}
	entry := &TraceImageCache{
		CommitSHA:   commitSHA,
		Environment: environment,
		Response:    resp,
	}
	_, err := r.db.NewInsert().
		Model(entry).
		On("CONFLICT (commit_sha, environment) DO UPDATE SET response_json = EXCLUDED.response_json, inserted_at = now()").
		Exec(ctx)
	if err != nil {
		return err
	}
	_, err = r.db.NewDelete().
		Model((*TraceImageCache)(nil)).
		Where("ctid IN (SELECT ctid FROM trace_image_cache ORDER BY inserted_at DESC OFFSET ?)", r.TraceCacheMax).
		Exec(ctx)
	return err
}

// DocumentBatchWriter provides atomic replace of all documents for a repository.
// Documents are buffered until Commit() is called, at which point they atomically
// replace all existing documents for the repository.
type DocumentBatchWriter interface {
	// Add a document chunk to the batch
	Add(ctx context.Context, doc *DocumentChunk) error

	// Commit atomically replaces old documents with new ones
	Commit(ctx context.Context) error

	// Rollback discards all buffered documents
	Rollback() error

	// Count returns number of documents added so far
	Count() int
}

// NewDocumentBatchWriter creates a batch writer for atomically replacing
// all documents for a given repository.
func (r *SearchRepository) NewDocumentBatchWriter(ctx context.Context, repo string) (DocumentBatchWriter, error) {
	return newPGDocumentBatchWriter(ctx, r.db, repo)
}

// pgDocumentBatchWriter implements DocumentBatchWriter using PostgreSQL temp tables
type pgDocumentBatchWriter struct {
	db         bun.IDB
	tx         bun.Tx
	repo       string
	count      int
	committed  bool
	rolledBack bool
}

func newPGDocumentBatchWriter(ctx context.Context, db bun.IDB, repo string) (*pgDocumentBatchWriter, error) {
	// Start transaction
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}

	// Create temp table (raw SQL needed for DDL)
	_, err = tx.NewRaw(`CREATE TEMP TABLE documents_temp (LIKE documents INCLUDING ALL) 
         ON COMMIT DROP`).Exec(ctx)
	if err != nil {
		tx.Rollback()
		return nil, err
	}

	return &pgDocumentBatchWriter{
		db:   db,
		tx:   tx,
		repo: repo,
	}, nil
}

func (w *pgDocumentBatchWriter) Add(ctx context.Context, doc *DocumentChunk) error {
	if w.committed {
		return errors.New("cannot add after commit")
	}
	if w.rolledBack {
		return errors.New("cannot add after rollback")
	}

	_, err := w.tx.NewInsert().
		Model(doc).
		Table("documents_temp").
		Exec(ctx)
	if err != nil {
		return err
	}

	w.count++
	return nil
}

func (w *pgDocumentBatchWriter) Commit(ctx context.Context) error {
	if w.committed {
		return errors.New("already committed")
	}
	if w.rolledBack {
		return errors.New("already rolled back")
	}

	// Delete old documents for this repo using Bun's query builder
	_, err := w.tx.NewDelete().
		Model((*DocumentChunk)(nil)).
		Where("repo = ?", w.repo).
		Exec(ctx)
	if err != nil {
		w.tx.Rollback()
		return err
	}

	// Copy from temp to main table (raw SQL needed for INSERT FROM SELECT)
	_, err = w.tx.NewRaw("INSERT INTO documents SELECT * FROM documents_temp").Exec(ctx)
	if err != nil {
		w.tx.Rollback()
		return err
	}

	// Commit transaction (temp table auto-drops)
	if err := w.tx.Commit(); err != nil {
		return err
	}

	w.committed = true
	return nil
}

func (w *pgDocumentBatchWriter) Rollback() error {
	if w.committed {
		return errors.New("already committed")
	}
	if w.rolledBack {
		return nil // Idempotent
	}

	err := w.tx.Rollback()
	w.rolledBack = true
	return err
}

func (w *pgDocumentBatchWriter) Count() int {
	return w.count
}
