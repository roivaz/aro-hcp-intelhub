CREATE TABLE IF NOT EXISTS trace_image_cache (
  commit_sha TEXT NOT NULL,
  environment TEXT NOT NULL,
  response_json JSONB NOT NULL,
  inserted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (commit_sha, environment)
);

CREATE INDEX IF NOT EXISTS trace_image_cache_inserted_idx
  ON trace_image_cache (inserted_at DESC);

