package ingestion

import (
	"context"
	"net/http"
	"time"

	"github.com/google/go-github/v66/github"
	"golang.org/x/oauth2"
)

func NewGitHubClient(token string) *github.Client {
	if token == "" {
		return github.NewClient(&http.Client{Timeout: 30 * time.Second})
	}
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(context.Background(), ts)
	tc.Timeout = 30 * time.Second
	return github.NewClient(tc)
}

type PRChange struct {
	Number         int
	Title          string
	Body           string
	Author         string
	CreatedAt      time.Time
	MergedAt       *time.Time
	State          string
	HeadCommitSHA  string
	BaseRef        string
	BaseSHA        string
	MergeCommitSHA string
}

type GitHubFetcher struct {
	client *github.Client
	owner  string
	repo   string
}

func NewGitHubFetcher(client *github.Client, owner, repo string) *GitHubFetcher {
	return &GitHubFetcher{client: client, owner: owner, repo: repo}
}

func buildPRChange(pr *github.PullRequest) PRChange {
	mergedAt := pr.GetMergedAt().Time
	return PRChange{
		Number:         pr.GetNumber(),
		Title:          pr.GetTitle(),
		Body:           pr.GetBody(),
		Author:         pr.GetUser().GetLogin(),
		CreatedAt:      pr.GetCreatedAt().Time,
		MergedAt:       &mergedAt,
		State:          pr.GetState(),
		HeadCommitSHA:  pr.GetHead().GetSHA(),
		BaseRef:        pr.GetBase().GetRef(),
		BaseSHA:        pr.GetBase().GetSHA(),
		MergeCommitSHA: pr.GetMergeCommitSHA(),
	}
}

type FetchResult struct {
	PRs       []PRChange
	NextPage  int
	HasMore   bool
	PageCount int
}

func (f *GitHubFetcher) FetchBatch(ctx context.Context, page int) (*FetchResult, error) {
	if page <= 0 {
		page = 1
	}

	opts := &github.PullRequestListOptions{
		State:       "closed",
		Base:        "main",
		Sort:        "updated",
		Direction:   "desc",
		ListOptions: github.ListOptions{PerPage: 100, Page: page},
	}

	prs, resp, err := f.client.PullRequests.List(ctx, f.owner, f.repo, opts)
	if err != nil {
		return nil, err
	}

	var results []PRChange
	for _, pr := range prs {
		if pr.MergedAt == nil {
			continue
		}
		results = append(results, buildPRChange(pr))
	}

	return &FetchResult{
		PRs:       results,
		NextPage:  resp.NextPage,
		HasMore:   resp.NextPage != 0,
		PageCount: len(prs),
	}, nil
}
