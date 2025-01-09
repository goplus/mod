//go:build !(js || wasip1)

package modcache

import (
	"bytes"
	"log"
	"os/exec"
	"strings"
)

// getGOMODCACHE returns the Go module cache directory.
func getGOMODCACHE() string {
	var buf bytes.Buffer
	var stderr bytes.Buffer
	cmd := exec.Command("go", "env", "GOMODCACHE")
	cmd.Stdout = &buf
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		log.Panicln("GOMODCACHE not found:", err)
	}
	return strings.TrimRight(buf.String(), "\n")
}
