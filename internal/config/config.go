package config

import (
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func Init(root *cobra.Command) {
	viper.AutomaticEnv()
	_ = godotenv.Load("manifests/config.env")
	if root != nil {
		_ = viper.BindPFlags(root.PersistentFlags())
	}
	setDefaults()
}

func setDefaults() {
	viper.SetDefault(KeyOllamaURL, "http://localhost:11434")
	viper.SetDefault(KeyLogLevel, "info")
	viper.SetDefault(KeyCacheDir, "ignore/cache")
	viper.SetDefault(KeyEmbeddingModel, "nomic-embed-text")
	viper.SetDefault(KeyIngestionMode, "INCREMENTAL")
	viper.SetDefault(KeyIngestionLimit, 100)
	viper.SetDefault(KeyBatchDirection, "backwards")
	viper.SetDefault(KeyRecreateMode, "no")
	viper.SetDefault(KeyDiffEnabled, false)
	viper.SetDefault(KeyDiffModel, "phi3")
	viper.SetDefault(KeyDiffOllamaURL, "http://localhost:11434")
	viper.SetDefault(KeyDiffContext, 4096)
	viper.SetDefault(KeyRepoPath, "./ignore/aro-hcp-repo")
	viper.SetDefault(KeyTraceSkopeo, "skopeo")
}

func PostgresURL() string            { return viper.GetString(KeyPostgresURL) }
func OllamaURL() string              { return viper.GetString(KeyOllamaURL) }
func AuthFile() string               { return viper.GetString(KeyAuthFile) }
func CacheDir() string               { return viper.GetString(KeyCacheDir) }
func EmbeddingModel() string         { return viper.GetString(KeyEmbeddingModel) }
func IngestionMode() string          { return viper.GetString(KeyIngestionMode) }
func IngestionLimit() int            { return viper.GetInt(KeyIngestionLimit) }
func IngestionStartDate() string     { return viper.GetString(KeyIngestionStart) }
func BatchDirection() string         { return viper.GetString(KeyBatchDirection) }
func RecreateMode() string           { return viper.GetString(KeyRecreateMode) }
func DiffAnalysisEnabled() bool      { return viper.GetBool(KeyDiffEnabled) }
func DiffAnalysisModel() string      { return viper.GetString(KeyDiffModel) }
func DiffAnalysisOllamaURL() string  { return viper.GetString(KeyDiffOllamaURL) }
func DiffAnalysisContextTokens() int { return viper.GetInt(KeyDiffContext) }
func RepoPath() string               { return viper.GetString(KeyRepoPath) }
func TraceSkopeoPath() string        { return viper.GetString(KeyTraceSkopeo) }
func TracePullSecret() string        { return viper.GetString(KeyTraceSecret) }
