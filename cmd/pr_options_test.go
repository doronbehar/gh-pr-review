package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/agynio/gh-pr-review/internal/ghapi"
	"github.com/agynio/gh-pr-review/internal/gitctx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubGitContext replaces the inference seams for the duration of a test.
func stubGitContext(t *testing.T, repo func() (gitctx.Repo, error), branch func() (string, error)) {
	t.Helper()
	originalRepo, originalBranch := currentRepo, currentBranch
	currentRepo, currentBranch = repo, branch
	t.Cleanup(func() { currentRepo, currentBranch = originalRepo, originalBranch })
}

func inferredRepo(owner, name, host string) func() (gitctx.Repo, error) {
	return func() (gitctx.Repo, error) {
		return gitctx.Repo{Owner: owner, Name: name, Host: host}, nil
	}
}

func inferredBranch(name string) func() (string, error) {
	return func() (string, error) { return name, nil }
}

func repoInferenceBanned(t *testing.T) func() (gitctx.Repo, error) {
	t.Helper()
	return func() (gitctx.Repo, error) {
		t.Error("repository inference should not have run")
		return gitctx.Repo{}, errors.New("banned")
	}
}

func branchInferenceBanned(t *testing.T) func() (string, error) {
	t.Helper()
	return func() (string, error) {
		t.Error("branch inference should not have run")
		return "", errors.New("banned")
	}
}

// inferenceRun records what `review view` asked of the API while resolving.
type inferenceRun struct {
	hosts            []string
	reportVariables  map[string]interface{}
	lookupVariables  map[string]interface{}
	lookupNumbers    []int
	lookupHeadOwners []string
	err              error
}

// runViewForInference executes `review view`, answering the branch lookup with
// lookupNumbers and the report with the canned payload.
func runViewForInference(t *testing.T, run *inferenceRun, args ...string) {
	t.Helper()

	originalFactory := apiClientFactory
	t.Cleanup(func() { apiClientFactory = originalFactory })

	fake := &commandFakeAPI{}
	fake.graphqlFunc = func(query string, variables map[string]interface{}, result interface{}) error {
		if strings.Contains(query, "PullRequestsForBranch") {
			run.lookupVariables = variables
			nodes := make([]obj, 0, len(run.lookupNumbers))
			for i, number := range run.lookupNumbers {
				node := obj{"number": number}
				if i < len(run.lookupHeadOwners) {
					node["headRepositoryOwner"] = obj{"login": run.lookupHeadOwners[i]}
				}
				nodes = append(nodes, node)
			}
			return assignJSON(result, obj{
				"repository": obj{"pullRequests": obj{"nodes": nodes}},
			})
		}
		run.reportVariables = variables
		return json.Unmarshal(viewResponse, result)
	}
	apiClientFactory = func(host string) ghapi.API {
		run.hosts = append(run.hosts, host)
		return fake
	}

	root := newRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(append([]string{"review", "view"}, args...))
	run.err = root.Execute()
}

func TestInferenceFillsRepoAndPullRequest(t *testing.T) {
	stubGitContext(t, inferredRepo("octo", "demo", "github.com"), inferredBranch("feature"))

	run := &inferenceRun{lookupNumbers: []int{7}}
	runViewForInference(t, run)

	require.NoError(t, run.err)
	assert.Equal(t, "octo", run.lookupVariables["owner"])
	assert.Equal(t, "demo", run.lookupVariables["name"])
	assert.Equal(t, "feature", run.lookupVariables["headRefName"])
	assert.Equal(t, "octo", run.reportVariables["owner"])
	assert.Equal(t, "demo", run.reportVariables["name"])
	assert.Equal(t, 7, run.reportVariables["number"])
}

// The remote decides the host when GH_HOST does not, so an enterprise checkout
// resolves without extra configuration.
func TestInferredRemoteHostIsUsed(t *testing.T) {
	t.Setenv("GH_HOST", "")
	stubGitContext(t, inferredRepo("octo", "demo", "github.example.com"), inferredBranch("feature"))

	run := &inferenceRun{lookupNumbers: []int{7}}
	runViewForInference(t, run)

	require.NoError(t, run.err)
	require.NotEmpty(t, run.hosts)
	for _, host := range run.hosts {
		assert.Equal(t, "github.example.com", host)
	}
}

func TestExplicitFlagsSkipInference(t *testing.T) {
	stubGitContext(t, repoInferenceBanned(t), branchInferenceBanned(t))

	run := &inferenceRun{}
	runViewForInference(t, run, "--repo", "other/repo", "--pr", "3")

	require.NoError(t, run.err)
	assert.Nil(t, run.lookupVariables, "branch lookup should not have run")
	assert.Equal(t, "other", run.reportVariables["owner"])
	assert.Equal(t, 3, run.reportVariables["number"])
}

// An explicit --repo may name a repository the working directory knows nothing
// about, so the checked out branch must not pick a pull request there.
func TestExplicitRepoDoesNotInferPullRequest(t *testing.T) {
	stubGitContext(t, repoInferenceBanned(t), branchInferenceBanned(t))

	run := &inferenceRun{}
	runViewForInference(t, run, "--repo", "other/repo")

	require.Error(t, run.err)
	assert.Contains(t, run.err.Error(), "must specify a pull request")
}

func TestPullURLSelectorSkipsInference(t *testing.T) {
	stubGitContext(t, repoInferenceBanned(t), branchInferenceBanned(t))

	run := &inferenceRun{}
	runViewForInference(t, run, "https://github.com/octo/demo/pull/9")

	require.NoError(t, run.err)
	assert.Nil(t, run.lookupVariables, "branch lookup should not have run")
	assert.Equal(t, "octo", run.reportVariables["owner"])
	assert.Equal(t, 9, run.reportVariables["number"])
}

func TestRepoInferenceFailureIsReported(t *testing.T) {
	stubGitContext(t, func() (gitctx.Repo, error) {
		return gitctx.Repo{}, errors.New("no git remotes configured")
	}, branchInferenceBanned(t))

	run := &inferenceRun{}
	runViewForInference(t, run)

	require.Error(t, run.err)
	assert.Contains(t, run.err.Error(), "no --repo given and none could be inferred from the git remotes")
	assert.Contains(t, run.err.Error(), "no git remotes configured")
}

func TestDetachedHeadIsReported(t *testing.T) {
	stubGitContext(t, inferredRepo("octo", "demo", "github.com"), func() (string, error) {
		return "", gitctx.ErrNoBranch
	})

	run := &inferenceRun{}
	runViewForInference(t, run)

	require.Error(t, run.err)
	assert.Contains(t, run.err.Error(), "the current branch could not be determined")
	assert.Contains(t, run.err.Error(), "HEAD is not on a branch")
}

func TestBranchWithoutPullRequestIsReported(t *testing.T) {
	stubGitContext(t, inferredRepo("octo", "demo", "github.com"), inferredBranch("feature"))

	run := &inferenceRun{}
	runViewForInference(t, run)

	require.Error(t, run.err)
	assert.Contains(t, run.err.Error(), `no open pull request found for branch "feature" in octo/demo`)
}

func TestAmbiguousBranchIsReported(t *testing.T) {
	stubGitContext(t, inferredRepo("octo", "demo", "github.com"), inferredBranch("patch-1"))

	run := &inferenceRun{lookupNumbers: []int{11, 12}, lookupHeadOwners: []string{"alice", "bob"}}
	runViewForInference(t, run)

	require.Error(t, run.err)
	assert.Contains(t, run.err.Error(), `branch "patch-1" heads 2 open pull requests`)
	assert.Contains(t, run.err.Error(), "#11 from alice")
	assert.Contains(t, run.err.Error(), "#12 from bob")
}

// Inference lives in the shared options, so every command group gets it.
func TestThreadsListInfersPullRequest(t *testing.T) {
	originalFactory := apiClientFactory
	defer func() { apiClientFactory = originalFactory }()

	stubGitContext(t, inferredRepo("octo", "demo", "github.com"), inferredBranch("feature"))

	var restPaths []string
	fake := &commandFakeAPI{}
	fake.restFunc = func(method, path string, params map[string]string, body interface{}, result interface{}) error {
		restPaths = append(restPaths, path)
		switch path {
		case "repos/octo/demo":
			return assignJSON(result, obj{"full_name": "octo/demo"})
		case "repos/octo/demo/pulls/7":
			return assignJSON(result, obj{"node_id": "PR_node"})
		default:
			return errors.New("unexpected path " + path)
		}
	}
	fake.graphqlFunc = func(query string, variables map[string]interface{}, result interface{}) error {
		if strings.Contains(query, "PullRequestsForBranch") {
			return assignJSON(result, obj{
				"repository": obj{"pullRequests": obj{"nodes": []obj{{"number": 7}}}},
			})
		}
		return assignJSON(result, obj{
			"node": obj{"reviewThreads": obj{"nodes": []obj{}}},
		})
	}
	apiClientFactory = func(host string) ghapi.API { return fake }

	root := newRootCommand()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(io.Discard)
	root.SetArgs([]string{"threads", "list"})

	require.NoError(t, root.Execute())
	assert.Contains(t, restPaths, "repos/octo/demo/pulls/7")
}
