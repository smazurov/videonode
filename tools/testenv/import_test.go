package main

import (
	"go/build"
	"strings"
	"testing"
)

// TestNoDirectInternalImports enforces the layering rule: cmd/ and
// internal/mcpsrv/ must go through internal/envctl/ for business
// logic, never importing store/slots/spawn/reaper directly.
//
// This prevents CLI and MCP from having parallel implementations that
// can silently disagree.
func TestNoDirectInternalImports(t *testing.T) {
	forbidden := map[string]bool{
		"github.com/smazurov/videonode/tools/testenv/internal/store":  true,
		"github.com/smazurov/videonode/tools/testenv/internal/slots":  true,
		"github.com/smazurov/videonode/tools/testenv/internal/spawn":  true,
		"github.com/smazurov/videonode/tools/testenv/internal/reaper": true,
		"github.com/smazurov/videonode/tools/testenv/internal/config": true,
	}

	// Packages that must NOT import the forbidden set directly.
	restricted := []string{
		"github.com/smazurov/videonode/tools/testenv/cmd",
		"github.com/smazurov/videonode/tools/testenv/internal/mcpsrv",
	}

	// context.go in cmd/ imports store for OpenStore — that's allowed
	// because envctl will own the store handle and pass it through.
	// Once the refactor lands this test will catch regressions.

	for _, pkg := range restricted {
		p, err := build.Default.Import(pkg, ".", 0)
		if err != nil {
			t.Logf("skip %s: %v", pkg, err)
			continue
		}
		for _, imp := range p.Imports {
			if forbidden[imp] {
				t.Errorf("%s imports %s directly — must go through internal/envctl instead",
					shortPkg(pkg), shortPkg(imp))
			}
		}
	}
}

func shortPkg(p string) string {
	const prefix = "github.com/smazurov/videonode/tools/testenv/"
	return strings.TrimPrefix(p, prefix)
}
