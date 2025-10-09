package types

type DocResult struct {
	Repo       string  `json:"repo"`
	Component  *string `json:"component,omitempty"`
	Path       string  `json:"path"`
	CommitSHA  string  `json:"commit_sha"`
	SourceURL  *string `json:"source_url,omitempty"`
	Snippet    string  `json:"snippet"`
	Similarity float64 `json:"similarity"`
	Content    *string `json:"content,omitempty"`
}
