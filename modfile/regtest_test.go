package modfile_test

import (
	"testing"

	"github.com/goplus/mod/gopmod"
)

func TestGopMod(t *testing.T) {
	mod := gopmod.Default
	if mod.HasModfile() {
		t.Fatal("gopmod.Default HasModfile?")
	}
	if path := mod.Modfile(); path != "" {
		t.Fatal("gopmod.Default.Modfile?", path)
	}
	if path := mod.Path(); path != "" {
		t.Fatal("gopmod.Default.Path?", path)
	}
	if path := mod.Root(); path != "" {
		t.Fatal("gopmod.Default.Root?", path)
	}
	if pt := mod.PkgType("foo"); pt != gopmod.PkgtStandard {
		t.Fatal("PkgType foo:", pt)
	}
}
