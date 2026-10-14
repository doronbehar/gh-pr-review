// Package ghapi talks to the GitHub API over HTTP, authenticating the way gh
// itself does: GH_TOKEN and friends, then the token in hosts.yml, then gh's
// secure storage. Requests go out in process; the gh binary is not involved.
package ghapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	gogh "github.com/cli/go-gh/v2/pkg/api"
)

// apiVersion pins the REST API version the payloads here were written against.
const apiVersion = "2022-11-28"

// API defines the subset of GitHub API interactions required by the command logic.
type API interface {
	REST(method, path string, params map[string]string, body interface{}, result interface{}) error
	GraphQL(query string, variables map[string]interface{}, result interface{}) error
}

// GraphQLErrorEntry captures a single GraphQL error payload.
type GraphQLErrorEntry struct {
	Message string        `json:"message"`
	Path    []interface{} `json:"path,omitempty"`
}

// GraphQLError represents GraphQL-level errors returned alongside a response.
type GraphQLError struct {
	Errors []GraphQLErrorEntry
}

func (e *GraphQLError) Error() string {
	if len(e.Errors) == 0 {
		return "graphql returned errors"
	}
	if len(e.Errors) == 1 {
		return fmt.Sprintf("graphql error: %s", e.Errors[0].Message)
	}
	parts := make([]string, 0, len(e.Errors))
	for _, err := range e.Errors {
		parts = append(parts, err.Message)
	}
	return fmt.Sprintf("graphql errors: %s", strings.Join(parts, "; "))
}

// Client implements API against a single GitHub host. The zero value talks to
// the host gh treats as default.
type Client struct {
	// Host is the GitHub host to address, such as github.com or a GitHub
	// Enterprise Server hostname.
	Host string
	// Transport replaces the HTTP transport, for tests.
	Transport http.RoundTripper

	setup sync.Once
	rest  *gogh.RESTClient
	gql   *gogh.GraphQLClient
	err   error
}

// clients builds the REST and GraphQL clients once per Client. Building them
// resolves the host's authentication token, which is why it can fail.
func (c *Client) clients() error {
	c.setup.Do(func() {
		opts := gogh.ClientOptions{
			Host:      strings.TrimSpace(c.Host),
			Transport: c.Transport,
			Headers:   map[string]string{"X-GitHub-Api-Version": apiVersion},
		}

		rest, err := gogh.NewRESTClient(opts)
		if err != nil {
			c.err = err
			return
		}
		gql, err := gogh.NewGraphQLClient(opts)
		if err != nil {
			c.err = err
			return
		}
		c.rest, c.gql = rest, gql
	})
	return c.err
}

// REST invokes the REST API. params are sent as query parameters, body is
// JSON-encoded, and result, when not nil, must be a pointer to unmarshal into.
func (c *Client) REST(method, path string, params map[string]string, body interface{}, result interface{}) error {
	if err := c.clients(); err != nil {
		return err
	}

	var payload io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		payload = bytes.NewReader(data)
	}

	if err := c.rest.Do(method, withParams(path, params), payload, result); err != nil {
		return err
	}
	return nil
}

// GraphQL issues a GraphQL operation, unmarshaling the response's data field
// into result.
func (c *Client) GraphQL(query string, variables map[string]interface{}, result interface{}) error {
	if err := c.clients(); err != nil {
		return err
	}

	if result == nil {
		var discard json.RawMessage
		result = &discard
	}

	err := c.gql.Do(query, variables, result)

	// Report GraphQL errors in this package's own shape: callers surface the
	// individual entries, and gh's own type should not leak into their output.
	var remote *gogh.GraphQLError
	if errors.As(err, &remote) {
		entries := make([]GraphQLErrorEntry, 0, len(remote.Errors))
		for _, entry := range remote.Errors {
			entries = append(entries, GraphQLErrorEntry{Message: entry.Message, Path: entry.Path})
		}
		return &GraphQLError{Errors: entries}
	}

	return err
}

// withParams appends params to path as query parameters.
func withParams(path string, params map[string]string) string {
	if len(params) == 0 {
		return path
	}

	query := url.Values{}
	for key, value := range params {
		query.Set(key, value)
	}

	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return path + separator + query.Encode()
}
