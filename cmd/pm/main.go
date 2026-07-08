package main

import "github.com/b0riswu/profile-manager/cmd"

// version is overridden at release time by goreleaser via -ldflags.
var version = "dev"

func main() {
	cmd.Execute(version)
}
