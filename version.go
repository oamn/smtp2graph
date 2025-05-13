package main

// revision holds the Git commit hash of the build.
// It is set at build time using -ldflags, for example:
//
//	go build -ldflags "-X main.revision=$(git rev-parse --short HEAD)"
//
// If not set, revision will be an empty string.
var revision string
