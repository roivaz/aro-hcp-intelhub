package config

const (
	KeyPostgresURL    = "postgres_url"
	KeyOllamaURL      = "ollama_url"
	KeyLogLevel       = "log_level"
	KeyAuthFile       = "auth_file"
	KeyCacheDir       = "cache_dir"
	KeyEmbeddingModel = "embedding_model_name"
	KeyIngestionMode  = "ingestion_mode"
	KeyIngestionLimit = "ingestion_limit"
	KeyIngestionStart = "ingestion_start_date"
	KeyBatchDirection = "batch_mode_direction"
	KeyRecreateMode   = "recreate"
	KeyDiffEnabled    = "diff_analysis_enabled"
	KeyDiffModel      = "diff_analysis_model"
	KeyDiffOllamaURL  = "diff_analysis_ollama_url"
	KeyDiffContext    = "diff_analysis_context_tokens"
	KeyRepoPath       = "aro_hcp_repo_path"
	KeyTraceSkopeo    = "trace_skopeo_path"
	KeyTraceSecret    = "pull_secret"
)
