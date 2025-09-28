package ingestion

import (
	"context"
	"net/http"
	"sort"
	"strings"
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

func (f *GitHubFetcher) FetchSince(ctx context.Context, mergedAfter time.Time, lastNumber int, limit int) ([]PRChange, error) {
	if limit <= 0 {
		limit = 100
	}
	var results []PRChange
	opts := &github.PullRequestListOptions{
		State:       "closed",
		Base:        "main",
		Sort:        "updated",
		Direction:   "asc",
		ListOptions: github.ListOptions{PerPage: 100},
	}
	for len(results) < limit {
		prs, resp, err := f.client.PullRequests.List(ctx, f.owner, f.repo, opts)
		if err != nil {
			return nil, err
		}
		for _, pr := range prs {
			if pr.MergedAt == nil {
				continue
			}
			mergedAt := pr.GetMergedAt().Time
			if !mergedAt.After(mergedAfter) {
				if mergedAt.Equal(mergedAfter) && pr.GetNumber() <= lastNumber {
					continue
				}
				if mergedAfter.IsZero() {
					// no watermark yet; allow all
				} else {
					continue
				}
			}
			results = append(results, buildPRChange(pr))
			if len(results) >= limit {
				break
			}
		}
		if resp.NextPage == 0 || len(results) >= limit {
			break
		}
		opts.Page = resp.NextPage
	}
	return results, nil
}

func (f *GitHubFetcher) FetchBatch(ctx context.Context, start time.Time, direction string, limit int) ([]PRChange, error) {
	if limit <= 0 {
		limit = 100
	}
	var results []PRChange
	dir := "desc"
	if strings.EqualFold(direction, "onwards") {
		dir = "asc"
	}
	opts := &github.PullRequestListOptions{
		State:       "closed",
		Base:        "main",
		Sort:        "updated",
		Direction:   dir,
		ListOptions: github.ListOptions{PerPage: 100},
	}
	for len(results) < limit {
		prs, resp, err := f.client.PullRequests.List(ctx, f.owner, f.repo, opts)
		if err != nil {
			return nil, err
		}
		for _, pr := range prs {
			if pr.MergedAt == nil {
				continue
			}
			mergedAt := pr.GetMergedAt().Time
			if !start.IsZero() {
				if dir == "desc" && mergedAt.After(start) {
					continue
				}
				if dir == "asc" && mergedAt.Before(start) {
					continue
				}
			}
			results = append(results, buildPRChange(pr))
			if len(results) >= limit {
				break
			}
		}
		if resp.NextPage == 0 || len(results) >= limit {
			break
		}
		opts.Page = resp.NextPage
	}

	if dir == "desc" {
		sort.Slice(results, func(i, j int) bool {
			return results[i].MergedAt.After(*results[j].MergedAt)
		})
	} else {
		sort.Slice(results, func(i, j int) bool {
			return results[i].MergedAt.Before(*results[j].MergedAt)
		})
	}
	return results, nil
}
