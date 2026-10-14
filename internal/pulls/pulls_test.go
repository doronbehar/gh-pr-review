package pulls

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type fakeAPI struct {
	t         *testing.T
	query     string
	variables map[string]interface{}
	nodes     []map[string]interface{}
	err       error
}

func (f *fakeAPI) REST(string, string, map[string]string, interface{}, interface{}) error {
	f.t.Fatal("unexpected REST call")
	return nil
}

func (f *fakeAPI) GraphQL(query string, variables map[string]interface{}, result interface{}) error {
	f.query = query
	f.variables = variables
	if f.err != nil {
		return f.err
	}
	payload, err := json.Marshal(map[string]interface{}{
		"repository": map[string]interface{}{
			"pullRequests": map[string]interface{}{"nodes": f.nodes},
		},
	})
	if err != nil {
		f.t.Fatalf("marshal payload: %v", err)
	}
	return json.Unmarshal(payload, result)
}

func TestFindByBranchReturnsTheSingleMatch(t *testing.T) {
	api := &fakeAPI{t: t, nodes: []map[string]interface{}{{"number": 42}}}

	number, err := FindByBranch(api, "octo", "demo", "feature")
	if err != nil {
		t.Fatalf("find by branch: %v", err)
	}
	if number != 42 {
		t.Fatalf("expected 42, got %d", number)
	}

	if !strings.Contains(api.query, "states: OPEN") {
		t.Fatalf("expected the query to filter open pull requests: %s", api.query)
	}
	if api.variables["owner"] != "octo" || api.variables["name"] != "demo" {
		t.Fatalf("unexpected repository variables: %#v", api.variables)
	}
	if api.variables["headRefName"] != "feature" {
		t.Fatalf("unexpected headRefName: %#v", api.variables["headRefName"])
	}
}

func TestFindByBranchTrimsInput(t *testing.T) {
	api := &fakeAPI{t: t, nodes: []map[string]interface{}{{"number": 3}}}

	if _, err := FindByBranch(api, " octo ", " demo ", " feature "); err != nil {
		t.Fatalf("find by branch: %v", err)
	}
	if api.variables["headRefName"] != "feature" {
		t.Fatalf("expected trimmed headRefName, got %#v", api.variables["headRefName"])
	}
}

func TestFindByBranchWithoutMatches(t *testing.T) {
	api := &fakeAPI{t: t}

	_, err := FindByBranch(api, "octo", "demo", "feature")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if !strings.Contains(err.Error(), `branch "feature" in octo/demo`) {
		t.Fatalf("expected the branch and repository in the message, got %v", err)
	}
}

// Forks pushing the same branch name produce several matches, and picking one
// would mean reviewing a stranger's pull request.
func TestFindByBranchWithSeveralMatches(t *testing.T) {
	api := &fakeAPI{t: t, nodes: []map[string]interface{}{
		{"number": 11, "headRepositoryOwner": map[string]interface{}{"login": "alice"}},
		{"number": 12, "headRepositoryOwner": map[string]interface{}{"login": "bob"}},
	}}

	_, err := FindByBranch(api, "octo", "demo", "patch-1")

	var ambiguous *AmbiguousError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("expected AmbiguousError, got %v", err)
	}
	if len(ambiguous.Candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %#v", ambiguous.Candidates)
	}
	if ambiguous.Candidates[0].HeadOwner != "alice" || ambiguous.Candidates[1].Number != 12 {
		t.Fatalf("unexpected candidates: %#v", ambiguous.Candidates)
	}
	for _, want := range []string{`branch "patch-1"`, "#11 from alice", "#12 from bob"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected %q in %v", want, err)
		}
	}
}

func TestFindByBranchRequiresRepositoryAndBranch(t *testing.T) {
	api := &fakeAPI{t: t, nodes: []map[string]interface{}{{"number": 1}}}

	if _, err := FindByBranch(api, "", "demo", "feature"); err == nil {
		t.Fatal("expected an error for a missing owner")
	}
	if _, err := FindByBranch(api, "octo", "demo", "  "); err == nil {
		t.Fatal("expected an error for a missing branch")
	}
	if api.query != "" {
		t.Fatal("expected no API call for invalid input")
	}
}

func TestFindByBranchPropagatesAPIErrors(t *testing.T) {
	api := &fakeAPI{t: t, err: errors.New("bad credentials")}

	if _, err := FindByBranch(api, "octo", "demo", "feature"); err == nil || !strings.Contains(err.Error(), "bad credentials") {
		t.Fatalf("expected the API error, got %v", err)
	}
}
