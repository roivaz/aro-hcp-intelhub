package db

import (
	"context"
	"database/sql"
	"errors"
	"time"

	pgvector "github.com/pgvector/pgvector-go"
	"github.com/uptrace/bun"
)

type SearchRepository struct {
	db *bun.DB
}

type PRSearchRow struct {
	PREmbedding `bun:",extend"`
	Distance    float64 `bun:"distance"`
}

func NewSearchRepository(database *Database) *SearchRepository {
	return &SearchRepository{db: database.Bun()}
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
		OrderExpr("distance")
	query.Limit(limit)

	if err := query.Scan(ctx); err != nil {
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
