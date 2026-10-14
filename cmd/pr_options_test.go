package cmd

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/agynio/gh-pr-review/internal/ghcli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubGitContext replaces the inference seams for the duration of a test.
func stubGitContext(t *testing.T, repo func() (string, error), pr func() (int, error)) {
	t.Helper()
	originalRepo, originalPR := currentRepo, currentPR
	currentRepo, currentPR = repo, pr
	t.Cleanup(func() { currentRepo, currentPR = originalRepo, originalPR })
}

func inferredRepo(name string) func() (string, error) {
	return func() (string, error) { return name, nil }
}

func inferredPR(number int) func() (int, error) {
	return func() (int, error) { return number, nil }
}

func repoInferenceBanned(t *testing.T) func() (string, error) {
	t.Helper()
	return func() (string, error) {
		t.Error("repository inference should not have run")
		return "", errors.New("banned")
	}
}

func prInferenceBanned(t *testing.T) func() (int, error) {
	t.Helper()
	return func() (int, error) {
		t.Error("pull request inference should not have run")
		return 0, errors.New("banned")
	}
}

// runViewForInference executes `review view` against a recording fake and
// returns the GraphQL variables it was called with.
func runViewForInference(t *testing.T, args ...string) (map[string]interface{}, error) {
	t.Helper()

	originalFactory := apiClientFactory
	t.Cleanup(func() { apiClientFactory = originalFactory })

	fake := &fakeViewAPI{payload: viewResponse, t: t}
	apiClientFactory = func(host string) ghcli.API { return fake }

	root := newRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(append([]string{"review", "view"}, args...))

	err := root.Execute()
	return fake.variables, err
}

func TestInferenceFillsRepoAndPullRequest(t *testing.T) {
	stubGitContext(t, inferredRepo("octo/demo"), inferredPR(7))

	variables, err := runViewForInference(t)
	require.NoError(t, err)
	assert.Equal(t, "octo", variables["owner"])
	assert.Equal(t, "demo", variables["name"])
	assert.Equal(t, 7, variables["number"])
}

func TestExplicitFlagsSkipInference(t *testing.T) {
	stubGitContext(t, repoInferenceBanned(t), prInferenceBanned(t))

	variables, err := runViewForInference(t, "--repo", "other/repo", "--pr", "3")
	require.NoError(t, err)
	assert.Equal(t, "other", variables["owner"])
	assert.Equal(t, "repo", variables["name"])
	assert.Equal(t, 3, variables["number"])
}

// An explicit --repo may name a repository the working directory knows nothing
// about, so the checked out branch must not be used to pick a pull request.
func TestExplicitRepoDoesNotInferPullRequest(t *testing.T) {
	stubGitContext(t, repoInferenceBanned(t), prInferenceBanned(t))

	_, err := runViewForInference(t, "--repo", "other/repo")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must specify a pull request")
}

func TestPullURLSelectorSkipsInference(t *testing.T) {
	stubGitContext(t, repoInferenceBanned(t), prInferenceBanned(t))

	variables, err := runViewForInference(t, "https://github.com/octo/demo/pull/9")
	require.NoError(t, err)
	assert.Equal(t, "octo", variables["owner"])
	assert.Equal(t, "demo", variables["name"])
	assert.Equal(t, 9, variables["number"])
}

func TestRepoInferenceFailureIsReported(t *testing.T) {
	stubGitContext(t, func() (string, error) {
		return "", errors.New("not a git repository")
	}, prInferenceBanned(t))

	_, err := runViewForInference(t)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no --repo given and none could be inferred")
	assert.Contains(t, err.Error(), "not a git repository")
}

func TestPullRequestInferenceFailureIsReported(t *testing.T) {
	stubGitContext(t, inferredRepo("octo/demo"), func() (int, error) {
		return 0, errors.New("no pull requests found for branch fork")
	})

	_, err := runViewForInference(t)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "none could be inferred from the current branch")
	assert.Contains(t, err.Error(), "no pull requests found for branch fork")
}

// Inference lives in the shared options, so every command group gets it.
func TestThreadsListInfersPullRequest(t *testing.T) {
	originalFactory := apiClientFactory
	defer func() { apiClientFactory = originalFactory }()

	stubGitContext(t, inferredRepo("octo/demo"), inferredPR(7))

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
		return assignJSON(result, obj{
			"node": obj{"reviewThreads": obj{"nodes": []obj{}}},
		})
	}
	apiClientFactory = func(host string) ghcli.API { return fake }

	root := newRootCommand()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(io.Discard)
	root.SetArgs([]string{"threads", "list"})

	require.NoError(t, root.Execute())
	assert.Contains(t, restPaths, "repos/octo/demo/pulls/7")
}
