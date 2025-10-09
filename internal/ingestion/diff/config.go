package diff

import (
	"time"

	"github.com/go-logr/logr"
)

type Config struct {
	Enabled          bool
	ModelName        string
	OllamaURL        string
	RepoPath         string
	MaxContextTokens int
	CallTimeout      time.Duration
	Logger           logr.Logger
}
