package modfile_test

import (
	"testing"

	"github.com/goplus/mod/gopmod"
)

func TestGopMod(t *testing.T) {
	if path := gopmod.Default.Path(); path != "" {
		t.Fatal("TestGopMod:", path)
	}
}
