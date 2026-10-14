// Package pulls locates pull requests that are not addressed directly.
package pulls

import (
	"errors"
	"fmt"
	"strings"

	"github.com/agynio/gh-pr-review/internal/ghapi"
)

const branchQuery = `query PullRequestsForBranch(
  $owner: String!,
  $name: String!,
  $headRefName: String!,
  $first: Int!
) {
  repository(owner: $owner, name: $name) {
    pullRequests(
      headRefName: $headRefName,
      states: OPEN,
      first: $first,
      orderBy: { field: CREATED_AT, direction: DESC }
    ) {
      nodes {
        number
        headRepositoryOwner { login }
      }
    }
  }
}`

// candidateLimit caps the lookup: a second match already makes it ambiguous,
// the rest only make the message longer.
const candidateLimit = 10

// ErrNotFound reports that no open pull request has the branch as its head.
var ErrNotFound = errors.New("no open pull request found")

// Candidate is one open pull request whose head ref matched.
type Candidate struct {
	Number    int
	HeadOwner string
}

// AmbiguousError reports several open pull requests sharing one head branch,
// which happens when forks push the same branch name.
type AmbiguousError struct {
	Branch     string
	Candidates []Candidate
}

func (e *AmbiguousError) Error() string {
	parts := make([]string, 0, len(e.Candidates))
	for _, candidate := range e.Candidates {
		if candidate.HeadOwner == "" {
			parts = append(parts, fmt.Sprintf("#%d", candidate.Number))
			continue
		}
		parts = append(parts, fmt.Sprintf("#%d from %s", candidate.Number, candidate.HeadOwner))
	}
	return fmt.Sprintf("branch %q heads %d open pull requests (%s)", e.Branch, len(e.Candidates), strings.Join(parts, ", "))
}

// FindByBranch returns the number of the one open pull request whose head ref
// is branch. Several matches are reported rather than guessed at: a review
// posted to the wrong pull request cannot be taken back.
func FindByBranch(api ghapi.API, owner, name, branch string) (int, error) {
	owner = strings.TrimSpace(owner)
	name = strings.TrimSpace(name)
	branch = strings.TrimSpace(branch)
	if owner == "" || name == "" {
		return 0, errors.New("owner and repository name are required")
	}
	if branch == "" {
		return 0, errors.New("branch is required")
	}

	var response struct {
		Repository struct {
			PullRequests struct {
				Nodes []struct {
					Number              int `json:"number"`
					HeadRepositoryOwner *struct {
						Login string `json:"login"`
					} `json:"headRepositoryOwner"`
				} `json:"nodes"`
			} `json:"pullRequests"`
		} `json:"repository"`
	}

	variables := map[string]interface{}{
		"owner":       owner,
		"name":        name,
		"headRefName": branch,
		"first":       candidateLimit,
	}
	if err := api.GraphQL(branchQuery, variables, &response); err != nil {
		return 0, err
	}

	nodes := response.Repository.PullRequests.Nodes
	switch len(nodes) {
	case 0:
		return 0, fmt.Errorf("%w for branch %q in %s/%s", ErrNotFound, branch, owner, name)
	case 1:
		if nodes[0].Number <= 0 {
			return 0, fmt.Errorf("pull request lookup for branch %q returned no number", branch)
		}
		return nodes[0].Number, nil
	}

	candidates := make([]Candidate, 0, len(nodes))
	for _, node := range nodes {
		candidate := Candidate{Number: node.Number}
		if node.HeadRepositoryOwner != nil {
			candidate.HeadOwner = node.HeadRepositoryOwner.Login
		}
		candidates = append(candidates, candidate)
	}
	return 0, &AmbiguousError{Branch: branch, Candidates: candidates}
}
