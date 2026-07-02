# Verification workflow

After any code change, run all of:

```sh
go test ./... && go vet ./... && go build -o pm ./cmd/pm && go install ./cmd/pm
```

`go install ./cmd/pm` is not optional: the `pm` the user actually runs comes from
mise's go bin (`which pm`), so skipping it ships stale behavior even though
`./pm` looks updated.

For changes to `internal/runner` or `cmd/run.go`, verify end-to-end with a
stub claude that echoes what it received — this checks argv and env without
launching the real claude or making API calls:

```sh
mkdir -p /tmp/stubbin
printf '#!/bin/sh\necho "ARGS: $@"\necho "BASE_URL: $ANTHROPIC_BASE_URL"\n' > /tmp/stubbin/claude
chmod +x /tmp/stubbin/claude
PATH=/tmp/stubbin:$PATH ./pm run <profile>
```

Note that stub runs still write real usage state (`recent`, `last_models`)
into `~/.pm.yaml`; run E2E tests from a scratch directory so per-directory
model memory for the user's real project directories is not polluted.
