package cli

import (
	"flag"
	"io"
	"strings"
)

type Options struct {
	ResumeID       string
	ShowStatus     bool
	PrintOnly      bool
	ShowVersion    bool
	DataDir        string
	Model          string
	PermissionMode string
	InitialPrompt  string
}

func ParseOptions(args []string) (Options, error) {
	fs := flag.NewFlagSet("sesame-agent", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var opts Options
	fs.StringVar(&opts.ResumeID, "resume", "", "resume a specific session")
	fs.BoolVar(&opts.ShowStatus, "status", false, "print daemon status and exit")
	fs.BoolVar(&opts.PrintOnly, "print", false, "submit one prompt and exit")
	fs.BoolVar(&opts.ShowVersion, "version", false, "print version and exit")
	fs.StringVar(&opts.DataDir, "data-dir", "", "override agent data directory")
	fs.StringVar(&opts.Model, "model", "", "override model for launched daemon")
	fs.StringVar(&opts.PermissionMode, "permission-mode", "", "override permission profile")

	if err := fs.Parse(args); err != nil {
		return Options{}, err
	}

	opts.InitialPrompt = strings.TrimSpace(strings.Join(fs.Args(), " "))
	return opts, nil
}
