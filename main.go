package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/phaseant/mcpgen/internal/generate"
	"github.com/phaseant/mcpgen/internal/spec"
)

func run(args []string) (*flag.FlagSet, error) {
	f := flag.NewFlagSet("mcpgen", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	input := f.String("spec", "mcp.yaml", "MCP specification file")
	output := f.String("out", "internal/mcpapi", "generated package directory")
	pkg := f.String("package", "mcpapi", "generated Go package name")
	check := f.Bool("check", false, "check generation drift without writing files")
	if err := f.Parse(args); err != nil {
		return f, err
	}
	if f.NArg() != 0 {
		return f, fmt.Errorf("unexpected arguments: %v", f.Args())
	}
	data, err := os.ReadFile(*input)
	if err != nil {
		return f, fmt.Errorf("%s: %w", *input, err)
	}
	doc, err := spec.Parse(*input, data)
	if err != nil {
		return f, err
	}
	files, err := generate.Generate(doc, *pkg)
	if err != nil {
		return f, fmt.Errorf("%s:%w", *input, err)
	}
	if *check {
		return f, generate.Check(*output, files)
	}
	return f, generate.Write(*output, files)
}

func execute(args []string, stderr io.Writer) int {
	f, err := run(args)
	if err == nil {
		return 0
	}
	if !errors.Is(err, flag.ErrHelp) {
		fmt.Fprintln(stderr, "mcpgen:", err)
	}
	f.SetOutput(stderr)
	f.Usage()
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	return 1
}

func main() {
	if code := execute(os.Args[1:], os.Stderr); code != 0 {
		os.Exit(code)
	}
}
