package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLI(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.yaml")
	output := filepath.Join(dir, "api")
	source := "mcp: '1'\ninfo: {name: cli, version: '1'}\ntools:\n  echo:\n    input: {type: object}\n    output: {type: object}\n"
	if err := os.WriteFile(path, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	args := []string{"-spec", path, "-out", output, "-package", "customapi"}
	if err := run(args); err != nil {
		t.Fatal(err)
	}
	generated, err := os.ReadFile(filepath.Join(output, "handlers.gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(generated), "package customapi") || !strings.Contains(string(generated), "Echo(context.Context") {
		t.Fatalf("handlers=%s", generated)
	}
	if err := run(append(args, "-check")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(source, "name: cli", "name: changed", 1)), 0644); err != nil {
		t.Fatal(err)
	}
	if err := run(append(args, "-check")); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("drift error=%v", err)
	}
	if err := run(append(args, "-package", "bad-name")); err == nil {
		t.Fatal("invalid package accepted")
	}
}
