package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/phaseant/mcpgen/internal/generate"
	"github.com/phaseant/mcpgen/internal/spec"
)

func run(args []string) error {
	f := flag.NewFlagSet("mcpgen", flag.ContinueOnError)
	input := f.String("spec", "mcp.yaml", "MCP specification file")
	output := f.String("out", "internal/mcpapi", "generated package directory")
	pkg := f.String("package", "mcpapi", "generated Go package name")
	check := f.Bool("check", false, "check generation drift without writing files")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %v", f.Args())
	}
	data, err := os.ReadFile(*input)
	if err != nil {
		return fmt.Errorf("%s: %w", *input, err)
	}
	doc, err := spec.Parse(*input, data)
	if err != nil {
		return err
	}
	files, err := generate.Generate(doc, *pkg)
	if err != nil {
		return fmt.Errorf("%s:%w", *input, err)
	}
	if *check {
		return generate.Check(*output, files)
	}
	return generate.Write(*output, files)
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "mcpgen:", err)
		os.Exit(1)
	}
}
