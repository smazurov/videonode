package api

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"strings"
	"testing"
)

// TestLifecyclePublishCoverage asserts that every entity registered
// in NewServer has a corresponding *.PublishCreated, *.PublishUpdated,
// AND *.PublishDeleted call site somewhere in the internal/api
// package. This catches the failure mode SelfCheck cannot — handler
// exists, route exists, registration exists, but the publish was
// forgotten so live-sync silently degrades for that entity.
//
// The test discovers registered entities by scanning server.go for
// `events.Register(opts.EventRegistry, events.Registration[X]{Type:
// "name", ...})` calls and matches the Type string against the
// convention field name `<type>Entity`. Add a new entity, regen, and
// this test will iterate it automatically.
//
// Failure messages name the exact missing call so the fix is one
// search-and-paste away.
func TestLifecyclePublishCoverage(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse api package: %v", err)
	}
	pkg, ok := pkgs["api"]
	if !ok {
		t.Fatal("internal/api package not found")
	}

	files := make([]*ast.File, 0, len(pkg.Files))
	for _, f := range pkg.Files {
		files = append(files, f)
	}

	registered := discoverRegisteredEntities(t, files)
	if len(registered) == 0 {
		t.Fatal("no entities discovered from events.Register calls — has the registration pattern changed? See internal/api/server.go for examples.")
	}

	publishCalls := collectPublishCalls(files)

	for entityType, fieldName := range registered {
		receiver := "s." + fieldName
		actions := publishCalls[receiver]
		for _, want := range []string{"Created", "Updated", "Deleted"} {
			if !actions[want] {
				t.Errorf(
					"entity %q (field %s) is registered in server.go but no `%s.Publish%s(...)` call site exists in internal/api/. "+
						"Add `if %s != nil { %s.Publish%s(...) }` to the %s handler in internal/api/%ss.go after the mutation succeeds.",
					entityType, fieldName,
					receiver, want,
					receiver, receiver, want,
					strings.ToLower(want), entityType,
				)
			}
		}
	}
}

// discoverRegisteredEntities walks the api package for
// `events.Register(_, events.Registration[T]{Type: "name", ...})` and
// returns a map: entity type → expected field name on Server.
//
// Convention: an entity of type "source" lives on Server.sourceEntity
// (lowercase type + "Entity"). Tests fail with a clear pointer if a
// future contributor breaks the convention.
func discoverRegisteredEntities(t *testing.T, files []*ast.File) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			assign, ok := n.(*ast.AssignStmt)
			if !ok {
				return true
			}
			// Looking for: server.<X>Entity = events.Register(opts.EventRegistry, events.Registration[...]{Type: "name", ...})
			if len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
				return true
			}
			lhs, ok := assign.Lhs[0].(*ast.SelectorExpr)
			if !ok {
				return true
			}
			fieldName := lhs.Sel.Name
			if !strings.HasSuffix(fieldName, "Entity") {
				return true
			}
			call, ok := assign.Rhs[0].(*ast.CallExpr)
			if !ok {
				return true
			}
			if !isEventsRegisterCall(call) {
				return true
			}
			if len(call.Args) < 2 {
				return true
			}
			// Second arg is a Registration[T] composite literal; find its
			// Type field's string value.
			typeName := extractRegistrationType(call.Args[1])
			if typeName == "" {
				return true
			}
			out[typeName] = fieldName
			return true
		})
	}
	return out
}

func isEventsRegisterCall(call *ast.CallExpr) bool {
	// Either `events.Register[...](...)` or `Register[...](...)` if
	// dot-imported (we don't dot-import, but be defensive).
	switch fun := call.Fun.(type) {
	case *ast.SelectorExpr:
		x, ok := fun.X.(*ast.Ident)
		return ok && x.Name == "events" && fun.Sel.Name == "Register"
	case *ast.IndexExpr:
		// `events.Register[T]` form
		sel, ok := fun.X.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		x, ok := sel.X.(*ast.Ident)
		return ok && x.Name == "events" && sel.Sel.Name == "Register"
	}
	return false
}

func extractRegistrationType(expr ast.Expr) string {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return ""
	}
	for _, elt := range lit.Elts {
		kv, isKV := elt.(*ast.KeyValueExpr)
		if !isKV {
			continue
		}
		key, isIdent := kv.Key.(*ast.Ident)
		if !isIdent || key.Name != "Type" {
			continue
		}
		bl, isLit := kv.Value.(*ast.BasicLit)
		if !isLit || bl.Kind != token.STRING {
			continue
		}
		return strings.Trim(bl.Value, `"`)
	}
	return ""
}

// collectPublishCalls walks the api package for any `<receiver>.Publish<Action>(...)`
// call and returns receiver → set of actions seen.
func collectPublishCalls(files []*ast.File) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if !strings.HasPrefix(sel.Sel.Name, "Publish") {
				return true
			}
			action := strings.TrimPrefix(sel.Sel.Name, "Publish")
			recv := exprString(sel.X)
			if out[recv] == nil {
				out[recv] = map[string]bool{}
			}
			out[recv][action] = true
			return true
		})
	}
	return out
}

// exprString prints an AST expression as it appears in source. Good
// enough for `s.sourceEntity`-style receivers; falls back to the
// expression's literal text via a tiny visitor.
func exprString(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return exprString(e.X) + "." + e.Sel.Name
	}
	return fmt.Sprintf("%T", expr)
}
