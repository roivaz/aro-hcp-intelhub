package types

type ComponentTraceInfo struct {
	Name          string  `json:"name"`
	Registry      string  `json:"registry"`
	Repository    string  `json:"repository"`
	Digest        string  `json:"digest"`
	SourceSHA     *string `json:"source_sha"`
	SourceRepoURL *string `json:"source_repo_url"`
	Error         *string `json:"error"`
}

type TraceImagesResponse struct {
	CommitSHA   string               `json:"commit_sha"`
	Environment string               `json:"environment"`
	Components  []ComponentTraceInfo `json:"components"`
	Errors      []string             `json:"errors"`
}
