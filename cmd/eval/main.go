package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"go-agent/cmd/eval/internal/evalcore"
	evalsuites "go-agent/cmd/eval/suites"
)

type EvalSuite = evalcore.EvalSuite
type EvalTurn = evalcore.EvalTurn
type EvalResponse = evalcore.EvalResponse
type EvalResult = evalcore.EvalResult

func main() {
	opts := parseFlags()
	if opts.Quick && opts.Long {
		fmt.Fprintln(os.Stderr, "-quick and -long cannot both be set")
		os.Exit(2)
	}

	logWriter := io.Writer(os.Stdout)
	if opts.JSON {
		logWriter = os.Stderr
	}
	logf := func(format string, args ...any) {
		if !opts.JSON || opts.Verbose {
			_, _ = fmt.Fprintf(logWriter, format, args...)
		}
	}

	suites := evalsuites.All(evalcore.SuiteOptions{
		Quick:   opts.Quick,
		Long:    opts.Long,
		Verbose: opts.Verbose,
	})
	report, err := evalcore.RunSuites(context.Background(), suites, opts, logf)
	if opts.JSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(report)
	} else {
		printSummary(report)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
		os.Exit(1)
	}
}

func parseFlags() evalcore.RunnerOptions {
	var opts evalcore.RunnerOptions
	flag.StringVar(&opts.WorkspaceRoot, "workspace", "", "path to eval workspace root; suite workspaces are created under this directory")
	flag.StringVar(&opts.SuiteFilter, "suite", "", "suite name to run, or comma-separated suite names; default runs all suites")
	flag.BoolVar(&opts.Quick, "quick", false, "run reduced turn counts for faster checks")
	flag.BoolVar(&opts.Long, "long", false, "run expanded turn counts for stress checks")
	flag.BoolVar(&opts.JSON, "json", false, "write machine-readable JSON results to stdout")
	flag.BoolVar(&opts.Keep, "keep", false, "keep suite workspaces after the run")
	flag.BoolVar(&opts.Verbose, "verbose", false, "print progress while -json is enabled")
	flag.Parse()
	return opts
}

func printSummary(report evalcore.RunReport) {
	fmt.Println("=== Eval Summary ===")
	fmt.Printf("Suites: %d, passed=%v, duration=%dms\n", len(report.Suites), report.Passed, report.DurationMS)
	for _, suite := range report.Suites {
		status := "FAIL"
		if suite.Passed {
			status = "PASS"
		}
		fmt.Printf("%s %s: %.0f%% (%d checks)\n", status, suite.Name, suite.PassRate*100, len(suite.Results))
		if suite.Error != "" {
			fmt.Printf("  error: %s\n", suite.Error)
		}
		for _, result := range suite.Results {
			resultStatus := "FAIL"
			if result.Passed {
				resultStatus = "PASS"
			}
			fmt.Printf("  %s %s: %s\n", resultStatus, result.Name, result.Detail)
		}
	}
}
