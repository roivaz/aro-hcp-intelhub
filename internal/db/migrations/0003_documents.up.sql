CREATE TABLE IF NOT EXISTS documents (
  id TEXT PRIMARY KEY,
  repo TEXT NOT NULL,
  component TEXT,
  path TEXT NOT NULL,
  commit_sha TEXT NOT NULL,
  doc_type TEXT NOT NULL,
  chunk_index INT NOT NULL,
  chunk_text TEXT NOT NULL,
  embedding VECTOR(768) NOT NULL,
  embedding_model TEXT NOT NULL,
  updated_at TIMESTAMPTZ DEFAULT now(),
  source_url TEXT
);

CREATE INDEX IF NOT EXISTS documents_component_idx ON documents(component);

CREATE INDEX IF NOT EXISTS documents_hnsw ON documents USING hnsw (embedding vector_cosine_ops);

