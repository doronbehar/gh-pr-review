package cmd

import (
	"github.com/agynio/gh-pr-review/internal/ghapi"
	"github.com/agynio/gh-pr-review/internal/gitctx"
)

var apiClientFactory = func(host string) ghapi.API {
	return &ghapi.Client{Host: host}
}

// currentRepo and currentBranch read the git context that --repo and --pr are
// inferred from when they are omitted. Tests replace them.
var (
	currentRepo   = gitctx.CurrentRepo
	currentBranch = gitctx.CurrentBranch
)
