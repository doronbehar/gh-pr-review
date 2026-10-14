package cmd

import "github.com/agynio/gh-pr-review/internal/ghcli"

var apiClientFactory = func(host string) ghcli.API {
	return &ghcli.Client{Host: host}
}

// currentRepo and currentPR read the git context that --repo and --pr are
// inferred from when they are omitted. Tests replace them.
var (
	currentRepo = ghcli.CurrentRepo
	currentPR   = ghcli.CurrentPR
)
