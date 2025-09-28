package config

import (
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func Init(root *cobra.Command) {
	viper.SetEnvPrefix("AROHCP")
	viper.AutomaticEnv()
	_ = godotenv.Load("manifests/config.env")
	_ = viper.BindPFlags(root.PersistentFlags())
	setDefaults()
}

func setDefaults() {
	viper.SetDefault(KeyOllamaURL, "http://localhost:11434")
	viper.SetDefault(KeyLogLevel, "info")
	viper.SetDefault(KeyCacheDir, "ignore/cache")
	viper.SetDefault(KeyMaxNewPRsPerRun, 100)
}

func PostgresURL() string  { return viper.GetString(KeyPostgresURL) }
func OllamaURL() string    { return viper.GetString(KeyOllamaURL) }
func AuthFile() string     { return viper.GetString(KeyAuthFile) }
func CacheDir() string     { return viper.GetString(KeyCacheDir) }
func MaxNewPRsPerRun() int { return viper.GetInt(KeyMaxNewPRsPerRun) }
func PRStartDate() string  { return viper.GetString(KeyPRStartDate) }
