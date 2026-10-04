package talos

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// With an empty PATH no subprocess can start: ErrReadOnly must come back
// before any exec is attempted (an exec failure would be a different error).
func TestMutatingMethodsRefuseWhenReadOnly(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	c := New("", "")
	if !c.ReadOnly() {
		t.Fatal("a new client must be read-only")
	}
	ctx := context.Background()
	ch := make(chan string, 4)
	calls := map[string]func() error{
		"Reboot":             func() error { return c.Reboot(ctx, "10.0.0.1") },
		"Shutdown":           func() error { return c.Shutdown(ctx, "10.0.0.1") },
		"UpgradeTalos":       func() error { return c.UpgradeTalos(ctx, "10.0.0.1", UpgradeOptions{Image: "x"}, ch) },
		"UpgradeK8s":         func() error { return c.UpgradeK8s(ctx, "10.0.0.1", "v1.35.0", ch) },
		"ApplyConfig":        func() error { return c.ApplyConfig(ctx, "10.0.0.1", "/nonexistent") },
		"PatchMachineConfig": func() error { return c.PatchMachineConfig(ctx, "10.0.0.1", "/nonexistent") },
	}
	for _, name := range MutatingMethods {
		call, ok := calls[name]
		if !ok {
			t.Fatalf("MutatingMethods lists %s but the test has no call for it", name)
		}
		if err := call(); !errors.Is(err, ErrReadOnly) {
			t.Errorf("%s: err = %v, want ErrReadOnly", name, err)
		}
	}
	if len(calls) != len(MutatingMethods) {
		t.Errorf("test covers %d methods, MutatingMethods lists %d", len(calls), len(MutatingMethods))
	}
	if len(ch) != 0 {
		t.Errorf("a refused call wrote %d lines to the output channel", len(ch))
	}
}

// With --write the guard lets the call through to the subprocess layer. The
// empty PATH makes that fail with an exec error, not ErrReadOnly.
func TestMutatingMethodsRunWhenWritable(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	c := New("", "")
	c.SetWritable(true)
	if err := c.Reboot(context.Background(), "10.0.0.1"); err == nil || errors.Is(err, ErrReadOnly) {
		t.Errorf("writable Reboot: err = %v, want an exec error", err)
	}
}

// Every exported Client method that starts a subprocess through the
// streaming/apply paths must be in MutatingMethods and begin with the guard.
// The check reads the source so a new mutating method cannot be forgotten.
func TestMutatingMethodsCallTheGuardFirst(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "client.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string]bool{}
	for _, n := range MutatingMethods {
		listed[n] = true
	}
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || !fn.Name.IsExported() || fn.Body == nil {
			continue
		}
		if listed[fn.Name.Name] {
			if !startsWithGuard(fn) {
				t.Errorf("%s must start with c.refuseWrite()", fn.Name.Name)
			}
			delete(listed, fn.Name.Name)
		}
	}
	for n := range listed {
		t.Errorf("MutatingMethods lists %s, which is not a Client method in client.go", n)
	}
}

func startsWithGuard(fn *ast.FuncDecl) bool {
	if len(fn.Body.List) == 0 {
		return false
	}
	ifs, ok := fn.Body.List[0].(*ast.IfStmt)
	if !ok {
		return false
	}
	found := false
	ast.Inspect(ifs.Init, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == "refuseWrite" {
			found = true
		}
		return true
	})
	return found
}
