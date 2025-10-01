package config

const (
	KeyPostgresURL          = "postgres_url"
	KeyOllamaURL            = "ollama_url"
	KeyLogLevel             = "log_level"
	KeyAuthFile             = "auth_file"
	KeyCacheDir             = "cache_dir"
	KeyEmbeddingModel       = "embedding_model_name"
	KeyGitHubFetchMax       = "github_fetch_max"
	KeyGitHubFetchStartDate = "github_fetch_start_date"
	KeyRecreateMode         = "recreate"
	KeyExecutionMode        = "execution_mode"
	KeyMaxProcessBatch      = "max_process_batch"
	KeyDiffEnabled          = "diff_analysis_enabled"
	KeyDiffModel            = "diff_analysis_model"
	KeyDiffOllamaURL        = "diff_analysis_ollama_url"
	KeyDiffContext          = "diff_analysis_context_tokens"
	KeyRepoPath             = "aro_hcp_repo_path"
	KeyTraceSkopeo          = "trace_skopeo_path"
	KeyTraceSecret          = "pull_secret"
)
