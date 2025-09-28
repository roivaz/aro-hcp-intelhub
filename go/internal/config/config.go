package config

import (
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func Init(root *cobra.Command) {
	viper.AutomaticEnv()
	_ = godotenv.Load("config-go.env")
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
}

func PostgresURL() string        { return viper.GetString(KeyPostgresURL) }
func OllamaURL() string          { return viper.GetString(KeyOllamaURL) }
func AuthFile() string           { return viper.GetString(KeyAuthFile) }
func CacheDir() string           { return viper.GetString(KeyCacheDir) }
func EmbeddingModel() string     { return viper.GetString(KeyEmbeddingModel) }
func IngestionMode() string      { return viper.GetString(KeyIngestionMode) }
func IngestionLimit() int        { return viper.GetInt(KeyIngestionLimit) }
func IngestionStartDate() string { return viper.GetString(KeyIngestionStart) }
func BatchDirection() string     { return viper.GetString(KeyBatchDirection) }
func RecreateMode() string       { return viper.GetString(KeyRecreateMode) }
