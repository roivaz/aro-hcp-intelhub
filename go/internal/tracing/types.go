package tracing

type Component struct {
	Name          string
	Registry      string
	Repository    string
	Digest        string
	SourceSHA     *string
	SourceRepoURL *string
	Error         *string
}

type TraceResult struct {
	CommitSHA   string
	Environment string
	Components  []Component
	Errors      []string
}
