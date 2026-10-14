package ghapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

// stubTransport answers requests from a canned response and records the last
// request it saw.
type stubTransport struct {
	status  int
	body    string
	request *http.Request
	rawBody string
}

func (s *stubTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	s.request = req
	if req.Body != nil {
		raw, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		s.rawBody = string(raw)
	}

	status := s.status
	if status == 0 {
		status = http.StatusOK
	}
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(s.body)),
		Request:    req,
	}, nil
}

// newClient builds a Client wired to the stub. GH_TOKEN stands in for the
// token the host's configuration would otherwise provide.
func newClient(t *testing.T, transport *stubTransport) *Client {
	t.Helper()
	t.Setenv("GH_TOKEN", "stub-token")
	return &Client{Host: "github.com", Transport: transport}
}

func TestRESTSendsParamsAsQuery(t *testing.T) {
	transport := &stubTransport{body: `{"full_name":"octo/demo"}`}
	client := newClient(t, transport)

	var result struct {
		FullName string `json:"full_name"`
	}
	if err := client.REST("GET", "repos/octo/demo", map[string]string{"per_page": "100", "page": "2"}, nil, &result); err != nil {
		t.Fatalf("REST: %v", err)
	}

	if result.FullName != "octo/demo" {
		t.Fatalf("unexpected result: %#v", result)
	}
	query := transport.request.URL.Query()
	if query.Get("per_page") != "100" || query.Get("page") != "2" {
		t.Fatalf("unexpected query: %s", transport.request.URL.RawQuery)
	}
	if got := transport.request.URL.Path; got != "/repos/octo/demo" {
		t.Fatalf("unexpected path: %s", got)
	}
	if got := transport.request.Header.Get("X-GitHub-Api-Version"); got != apiVersion {
		t.Fatalf("unexpected api version header: %q", got)
	}
	if got := transport.request.Header.Get("Authorization"); !strings.Contains(got, "stub-token") {
		t.Fatalf("expected the resolved token in the request, got %q", got)
	}
}

func TestRESTSendsJSONBody(t *testing.T) {
	transport := &stubTransport{body: `{}`}
	client := newClient(t, transport)

	if err := client.REST("POST", "repos/octo/demo/issues", nil, map[string]string{"title": "hi"}, nil); err != nil {
		t.Fatalf("REST: %v", err)
	}

	if transport.request.Method != "POST" {
		t.Fatalf("unexpected method: %s", transport.request.Method)
	}
	if transport.rawBody != `{"title":"hi"}` {
		t.Fatalf("unexpected body: %s", transport.rawBody)
	}
}

func TestRESTReportsHTTPErrors(t *testing.T) {
	transport := &stubTransport{status: http.StatusNotFound, body: `{"message":"Not Found"}`}
	client := newClient(t, transport)

	err := client.REST("GET", "repos/octo/missing", nil, nil, nil)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Fatalf("expected the status in the message, got %v", err)
	}
}

func TestGraphQLUnwrapsData(t *testing.T) {
	transport := &stubTransport{body: `{"data":{"repository":{"name":"demo"}}}`}
	client := newClient(t, transport)

	var result struct {
		Repository struct {
			Name string `json:"name"`
		} `json:"repository"`
	}
	if err := client.GraphQL("query Q { repository { name } }", map[string]interface{}{"n": 1}, &result); err != nil {
		t.Fatalf("GraphQL: %v", err)
	}

	if result.Repository.Name != "demo" {
		t.Fatalf("unexpected result: %#v", result)
	}

	var sent struct {
		Query     string                 `json:"query"`
		Variables map[string]interface{} `json:"variables"`
	}
	if err := json.Unmarshal([]byte(transport.rawBody), &sent); err != nil {
		t.Fatalf("parse request body: %v", err)
	}
	if !strings.Contains(sent.Query, "repository") || sent.Variables["n"] != float64(1) {
		t.Fatalf("unexpected request: %#v", sent)
	}
	if got := transport.request.URL.Path; got != "/graphql" {
		t.Fatalf("unexpected path: %s", got)
	}
}

// Callers surface the individual entries, so they must come back in this
// package's own shape rather than the library's.
func TestGraphQLReportsQueryErrors(t *testing.T) {
	transport := &stubTransport{body: `{"errors":[{"message":"mutation failed","path":["mutation","submitPullRequestReview"]},{"message":"second"}]}`}
	client := newClient(t, transport)

	err := client.GraphQL("mutation M { x }", nil, &struct{}{})

	var gqlErr *GraphQLError
	if !errors.As(err, &gqlErr) {
		t.Fatalf("expected *GraphQLError, got %T: %v", err, err)
	}
	if len(gqlErr.Errors) != 2 {
		t.Fatalf("expected 2 entries, got %#v", gqlErr.Errors)
	}
	if gqlErr.Errors[0].Message != "mutation failed" {
		t.Fatalf("unexpected first entry: %#v", gqlErr.Errors[0])
	}
	if len(gqlErr.Errors[0].Path) != 2 || gqlErr.Errors[0].Path[0] != "mutation" {
		t.Fatalf("expected the path to survive, got %#v", gqlErr.Errors[0].Path)
	}
	if !strings.Contains(err.Error(), "mutation failed") || !strings.Contains(err.Error(), "second") {
		t.Fatalf("expected both messages in %v", err)
	}
}

func TestGraphQLToleratesNoResult(t *testing.T) {
	transport := &stubTransport{body: `{"data":{"ok":true}}`}
	client := newClient(t, transport)

	if err := client.GraphQL("query Q { ok }", nil, nil); err != nil {
		t.Fatalf("GraphQL: %v", err)
	}
}

func TestClientWithoutTokenFails(t *testing.T) {
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_CONFIG_DIR", t.TempDir())
	t.Setenv("PATH", t.TempDir())

	client := &Client{Host: "github.com", Transport: &stubTransport{body: "{}"}}
	if err := client.REST("GET", "user", nil, nil, nil); err == nil {
		t.Fatal("expected an authentication error")
	}
}

func TestGraphQLErrorMessages(t *testing.T) {
	cases := []struct {
		name     string
		err      *GraphQLError
		expected string
	}{
		{"empty", &GraphQLError{}, "graphql returned errors"},
		{"single", &GraphQLError{Errors: []GraphQLErrorEntry{{Message: "boom"}}}, "graphql error: boom"},
		{"several", &GraphQLError{Errors: []GraphQLErrorEntry{{Message: "a"}, {Message: "b"}}}, "graphql errors: a; b"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := testCase.err.Error(); got != testCase.expected {
				t.Fatalf("expected %q, got %q", testCase.expected, got)
			}
		})
	}
}
