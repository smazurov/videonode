package api

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"reflect"
	"sort"
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

// TestDenormalizedFanoutCoverage asserts that every model field tagged
// `republish:"<contrib>[,<contrib>...]"` has a matching
// `events.OnLifecycle(server.<contrib>Entity, []string{...all three
// lifecycle actions...}, ...)` call somewhere in the api package.
//
// Owner entity (the one carrying the denormalized field) is derived by
// stripping the conventional "Data" suffix from the parent struct name
// and lowercasing: SourceData → "source", ComposerData → "composer".
// Contributing entities are read straight from the tag.
//
// This catches the bug class "I added a server-denormalized rollup but
// forgot to wire the SSE fan-out, so the UI silently shows stale lists
// after a contributing entity changes".
func TestDenormalizedFanoutCoverage(t *testing.T) {
	fset := token.NewFileSet()

	apiPkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse api package: %v", err)
	}
	apiPkg, ok := apiPkgs["api"]
	if !ok {
		t.Fatal("internal/api package not found")
	}
	apiFiles := make([]*ast.File, 0, len(apiPkg.Files))
	for _, f := range apiPkg.Files {
		apiFiles = append(apiFiles, f)
	}

	modelsPkgs, err := parser.ParseDir(fset, "models", nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse models package: %v", err)
	}
	modelsPkg, ok := modelsPkgs["models"]
	if !ok {
		t.Fatal("internal/api/models package not found")
	}
	modelsFiles := make([]*ast.File, 0, len(modelsPkg.Files))
	for _, f := range modelsPkg.Files {
		modelsFiles = append(modelsFiles, f)
	}

	registered := discoverRegisteredEntities(t, apiFiles)
	if len(registered) == 0 {
		t.Fatal("no entities discovered from events.Register calls")
	}

	fanouts := collectRepublishDeclarations(t, modelsFiles, registered)
	if len(fanouts) == 0 {
		t.Fatal(`no republish:"..." struct tags found — add tags to denormalized fields like SourceData.Consumers`)
	}

	hooks := collectOnLifecycleHooks(apiFiles)

	requiredActions := []string{"created", "updated", "deleted"}
	for _, f := range fanouts {
		for _, contrib := range f.contributors {
			contribField, contribOK := registered[contrib]
			if !contribOK {
				t.Errorf("field %s.%s declares republish:%q but no entity %q is registered in server.go",
					f.ownerStruct, f.fieldName, contrib, contrib)
				continue
			}
			hook, found := hooks[contribField]
			if !found {
				t.Errorf(
					"DENORMALIZED FANOUT MISSING: %s.%s (entity %q) is fed by changes to entity %q, but no `events.OnLifecycle(server.%s, ...)` hook exists in internal/api/. "+
						"Without this hook, %s.%s will not refresh over SSE when a %s is created/updated/deleted — the UI shows stale lists until a manual reload. "+
						"Add the hook in internal/api/server.go next to the existing stream→upstream hook.",
					f.ownerStruct, f.fieldName, f.ownerEntity, contrib, contribField,
					f.ownerStruct, f.fieldName, contrib,
				)
				continue
			}
			for _, action := range requiredActions {
				if !hook.actions[action] {
					t.Errorf(
						"DENORMALIZED FANOUT INCOMPLETE: hook events.OnLifecycle(server.%s, ...) covers %v but is missing action %q. "+
							"%s.%s (entity %q) is fed by %q and will not refresh when a %s is %s.",
						contribField, sortedKeys(hook.actions), action,
						f.ownerStruct, f.fieldName, f.ownerEntity, contrib, contrib, action,
					)
				}
			}
		}
	}
}

// republishDecl is one (owner, field, contributors) declaration.
type republishDecl struct {
	ownerStruct  string   // e.g. "SourceData"
	ownerEntity  string   // e.g. "source"
	fieldName    string   // e.g. "Consumers"
	contributors []string // e.g. ["stream", "composer"]
}

// collectRepublishDeclarations walks the models package for struct
// fields tagged `republish:"a,b"` and pairs each with its owner
// entity type (derived from the struct name by stripping "Data").
func collectRepublishDeclarations(t *testing.T, files []*ast.File, registered map[string]string) []republishDecl {
	t.Helper()
	var out []republishDecl
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				return true
			}
			ownerStruct := ts.Name.Name
			ownerEntity := strings.ToLower(strings.TrimSuffix(ownerStruct, "Data"))
			if _, ownerReg := registered[ownerEntity]; !ownerReg {
				// Not a registered entity model (e.g. nested helper struct);
				// skip — only top-level entity rollups can fan out.
				return true
			}
			for _, field := range st.Fields.List {
				if field.Tag == nil {
					continue
				}
				tagVal := strings.Trim(field.Tag.Value, "`")
				contribs := extractRepublishTag(tagVal)
				if len(contribs) == 0 {
					continue
				}
				for _, name := range field.Names {
					out = append(out, republishDecl{
						ownerStruct:  ownerStruct,
						ownerEntity:  ownerEntity,
						fieldName:    name.Name,
						contributors: contribs,
					})
				}
			}
			return true
		})
	}
	return out
}

// extractRepublishTag pulls the comma-separated values from a
// `republish:"..."` struct tag. Returns nil if the tag is absent.
func extractRepublishTag(raw string) []string {
	tag := reflect.StructTag(raw)
	val, ok := tag.Lookup("republish")
	if !ok || val == "" {
		return nil
	}
	parts := strings.Split(val, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// onLifecycleHook is one parsed events.OnLifecycle call.
type onLifecycleHook struct {
	contribField string          // e.g. "streamEntity"
	actions      map[string]bool // lowercase action names this hook subscribes to
}

// collectOnLifecycleHooks walks the api package for calls of the form
// `events.OnLifecycle(server.<X>Entity, []string{...}, ...)` and indexes
// them by the contributor entity field name.
func collectOnLifecycleHooks(files []*ast.File) map[string]onLifecycleHook {
	out := map[string]onLifecycleHook{}
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
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != "events" || sel.Sel.Name != "OnLifecycle" {
				return true
			}
			if len(call.Args) < 2 {
				return true
			}
			contribField := extractEntityFieldName(call.Args[0])
			if contribField == "" {
				return true
			}
			actions := extractActionsLiteral(call.Args[1])
			prev := out[contribField]
			if prev.actions == nil {
				prev.actions = map[string]bool{}
				prev.contribField = contribField
			}
			for a := range actions {
				prev.actions[a] = true
			}
			out[contribField] = prev
			return true
		})
	}
	return out
}

// extractEntityFieldName pulls "streamEntity" from `server.streamEntity`
// or `s.streamEntity`. Returns "" if the expression isn't a selector
// onto an *Entity field.
func extractEntityFieldName(expr ast.Expr) string {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	if !strings.HasSuffix(sel.Sel.Name, "Entity") {
		return ""
	}
	return sel.Sel.Name
}

// extractActionsLiteral parses the second arg of OnLifecycle, which is
// expected to be `[]string{events.ActionCreated, ...}` or
// `[]string{"created", ...}`. Returns a set of lowercase action names.
func extractActionsLiteral(expr ast.Expr) map[string]bool {
	out := map[string]bool{}
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return out
	}
	for _, elt := range lit.Elts {
		switch v := elt.(type) {
		case *ast.BasicLit:
			if v.Kind == token.STRING {
				out[strings.ToLower(strings.Trim(v.Value, `"`))] = true
			}
		case *ast.SelectorExpr:
			// events.ActionCreated → "created"
			name := strings.TrimPrefix(v.Sel.Name, "Action")
			out[strings.ToLower(name)] = true
		case *ast.Ident:
			name := strings.TrimPrefix(v.Name, "Action")
			out[strings.ToLower(name)] = true
		}
	}
	return out
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
