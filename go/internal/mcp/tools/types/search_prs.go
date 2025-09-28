package types

type PRResult struct {
	PRNumber        int      `json:"pr_number"`
	Title           string   `json:"title"`
	Body            string   `json:"body"`
	Author          string   `json:"author"`
	State           string   `json:"state"`
	CreatedAt       string   `json:"created_at"`
	MergedAt        *string  `json:"merged_at"`
	GithubURL       string   `json:"github_url"`
	SimilarityScore *float64 `json:"similarity_score,omitempty"`
}
