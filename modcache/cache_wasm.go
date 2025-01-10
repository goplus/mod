//go:build js || wasip1

package modcache

import (
	"os"
	"path/filepath"
)

// getGOMODCACHE returns the Go module cache directory. It follows the same
// logic as the Go command:
//  1. Use GOMODCACHE if set.
//  2. Otherwise use $GOPATH/pkg/mod.
//  3. If GOPATH is not set, default to "/go/pkg/mod".
func getGOMODCACHE() string {
	if dir := os.Getenv("GOMODCACHE"); dir != "" {
		return dir
	}

	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		gopath = "/go"
	}
	return filepath.Join(gopath, "pkg", "mod")
}
