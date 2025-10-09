CREATE TABLE IF NOT EXISTS pr_embeddings (
  id BIGSERIAL PRIMARY KEY,
  pr_number INT UNIQUE NOT NULL,
  pr_title TEXT NOT NULL,
  pr_body TEXT NOT NULL,
  author TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  merged_at TIMESTAMPTZ,
  state TEXT NOT NULL,
  base_ref TEXT NOT NULL,
  github_base_sha TEXT,
  base_merge_base_sha TEXT,
  head_commit_sha TEXT,
  merge_commit_sha TEXT,
  embedding VECTOR(768),
  rich_description TEXT,
  analysis_successful BOOLEAN DEFAULT FALSE,
  failure_reason TEXT,
  failure_category TEXT,
  processed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS pr_embeddings_hnsw
  ON pr_embeddings USING hnsw (embedding vector_cosine_ops);

