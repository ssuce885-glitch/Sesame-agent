package cli

import (
	"flag"
	"fmt"
	"io"
	"strings"
)

type SkillCommand struct {
	Action string
	Scope  string
	Name   string
	Source string
	Repo   string
	Path   string
	Ref    string
}

type AutomationCommand struct {
	Action        string
	ID            string
	File          string
	WorkspaceRoot string
	WatcherID     string
	StateFile     string
}

type TriggerCommand struct {
	Action string
	File   string
}

type IncidentCommand struct {
	Action        string
	ID            string
	AutomationID  string
	WorkspaceRoot string
	Status        string
	Limit         int
}

type Options struct {
	ResumeID       string
	DaemonRef      string
	ListDaemons    bool
	ShowStatus     bool
	PrintOnly      bool
	ShowVersion    bool
	WorkspaceRoot  string
	DataDir        string
	Addr           string
	Model          string
	PermissionMode string
	InitialPrompt  string
	Skill          *SkillCommand
	Automation     *AutomationCommand
	Trigger        *TriggerCommand
	Incident       *IncidentCommand
}

func ParseOptions(args []string) (Options, error) {
	if len(args) > 0 {
		switch args[0] {
		case "skill":
			return parseSkillOptions(args[1:])
		case "automation":
			return parseAutomationOptions(args[1:])
		case "trigger":
			return parseTriggerOptions(args[1:])
		case "incident":
			return parseIncidentOptions(args[1:])
		}
	}

	fs := flag.NewFlagSet("sesame", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var opts Options
	fs.StringVar(&opts.ResumeID, "resume", "", "resume a specific session")
	fs.StringVar(&opts.DaemonRef, "daemon", "", "use a historical daemon by id or 'latest'; default creates a new daemon")
	fs.BoolVar(&opts.ListDaemons, "list-daemons", false, "list historical daemons and exit")
	fs.BoolVar(&opts.ShowStatus, "status", false, "print daemon status and exit")
	fs.BoolVar(&opts.PrintOnly, "print", false, "submit one prompt and exit")
	fs.BoolVar(&opts.ShowVersion, "version", false, "print version and exit")
	fs.StringVar(&opts.WorkspaceRoot, "workspace", "", "workspace root (defaults to current directory)")
	fs.StringVar(&opts.DataDir, "data-dir", "", "override sesame data directory")
	fs.StringVar(&opts.Model, "model", "", "override model for launched daemon")
	fs.StringVar(&opts.PermissionMode, "permission-mode", "", "override permission profile")

	if err := fs.Parse(args); err != nil {
		return Options{}, err
	}

	opts.InitialPrompt = strings.TrimSpace(strings.Join(fs.Args(), " "))
	return opts, nil
}

func parseSkillOptions(args []string) (Options, error) {
	if len(args) == 0 {
		return Options{}, fmt.Errorf("usage: sesame skill <list|inspect|install|remove> ...")
	}

	cmd := &SkillCommand{Action: strings.ToLower(strings.TrimSpace(args[0]))}
	fs := flag.NewFlagSet("sesame skill "+cmd.Action, flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	switch cmd.Action {
	case "list":
		fs.StringVar(&cmd.Scope, "scope", "", "filter local skills by scope: system|global|workspace")
		fs.StringVar(&cmd.Repo, "repo", "", "GitHub owner/repo for remote listing")
		fs.StringVar(&cmd.Path, "path", "", "GitHub path for remote listing")
		fs.StringVar(&cmd.Ref, "ref", defaultSkillRef, "Git ref for remote listing")
	case "inspect":
		fs.StringVar(&cmd.Scope, "scope", "global", "planned install scope: global or workspace")
		fs.StringVar(&cmd.Repo, "repo", "", "GitHub owner/repo source")
		fs.StringVar(&cmd.Path, "path", "", "optional path inside the GitHub repo")
		fs.StringVar(&cmd.Ref, "ref", defaultSkillRef, "Git ref for GitHub inspection")
	case "install":
		fs.StringVar(&cmd.Scope, "scope", "global", "install scope: global or workspace")
		fs.StringVar(&cmd.Name, "name", "", "override installed directory name")
		fs.StringVar(&cmd.Repo, "repo", "", "GitHub owner/repo source")
		fs.StringVar(&cmd.Path, "path", "", "path inside the GitHub repo")
		fs.StringVar(&cmd.Ref, "ref", defaultSkillRef, "Git ref for GitHub installs")
	case "remove":
		fs.StringVar(&cmd.Scope, "scope", "global", "remove scope: global or workspace")
	default:
		return Options{}, fmt.Errorf("unknown skill command %q", cmd.Action)
	}

	if err := fs.Parse(reorderInterspersedArgs(args[1:])); err != nil {
		return Options{}, err
	}

	rest := fs.Args()
	switch cmd.Action {
	case "list":
		if len(rest) > 0 {
			return Options{}, fmt.Errorf("usage: sesame skill list [--scope ...] [--repo ... --path ... --ref ...]")
		}
		hasRepo := strings.TrimSpace(cmd.Repo) != ""
		hasPath := strings.TrimSpace(cmd.Path) != ""
		if hasRepo != hasPath {
			return Options{}, fmt.Errorf("usage: sesame skill list [--scope ...] [--repo ... --path ... --ref ...]")
		}
	case "inspect":
		if len(rest) > 1 {
			return Options{}, fmt.Errorf("usage: sesame skill inspect <source> [--scope ...] [--repo ...] [--path ...] [--ref ...]")
		}
		if len(rest) == 1 {
			cmd.Source = strings.TrimSpace(rest[0])
		}
		if strings.TrimSpace(cmd.Source) != "" && strings.TrimSpace(cmd.Repo) != "" {
			return Options{}, fmt.Errorf("provide either a source argument or --repo, not both")
		}
		if strings.TrimSpace(cmd.Source) == "" && strings.TrimSpace(cmd.Repo) == "" {
			return Options{}, fmt.Errorf("usage: sesame skill inspect <source> [--scope ...] [--repo ...] [--path ...] [--ref ...]")
		}
	case "install":
		if len(rest) > 1 {
			return Options{}, fmt.Errorf("usage: sesame skill install <source> [--scope ...] [--name ...] [--repo ... --path ... --ref ...]")
		}
		if len(rest) == 1 {
			cmd.Source = strings.TrimSpace(rest[0])
		}
		if strings.TrimSpace(cmd.Source) != "" && strings.TrimSpace(cmd.Repo) != "" {
			return Options{}, fmt.Errorf("provide either a source argument or --repo, not both")
		}
		if strings.TrimSpace(cmd.Source) == "" && strings.TrimSpace(cmd.Repo) == "" {
			return Options{}, fmt.Errorf("usage: sesame skill install <source> [--scope ...] [--name ...] [--repo ... --path ... --ref ...]")
		}
	case "remove":
		if len(rest) != 1 {
			return Options{}, fmt.Errorf("usage: sesame skill remove <name> [--scope ...]")
		}
		cmd.Name = strings.TrimSpace(rest[0])
	}

	return Options{Skill: cmd}, nil
}

func parseAutomationOptions(args []string) (Options, error) {
	if len(args) == 0 {
		return Options{}, fmt.Errorf("usage: sesame automation <apply|list|get|pause|resume|remove|install|reinstall|watcher|run> ...")
	}

	cmd := &AutomationCommand{Action: strings.ToLower(strings.TrimSpace(args[0]))}
	fs := flag.NewFlagSet("sesame automation "+cmd.Action, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	switch cmd.Action {
	case "apply":
		fs.StringVar(&cmd.File, "file", "", "json file containing the automation request")
	case "list":
		fs.StringVar(&cmd.WorkspaceRoot, "workspace-root", "", "optional workspace root filter")
	case "run":
		fs.StringVar(&cmd.WatcherID, "watcher-id", "", "watcher runtime identifier")
		fs.StringVar(&cmd.StateFile, "state-file", "", "watcher state file path")
	case "get", "pause", "resume", "remove", "install", "reinstall", "watcher":
	default:
		return Options{}, fmt.Errorf("unknown automation command %q", cmd.Action)
	}
	if err := fs.Parse(reorderInterspersedArgs(args[1:])); err != nil {
		return Options{}, err
	}

	rest := fs.Args()
	switch cmd.Action {
	case "apply":
		if cmd.File == "" || len(rest) != 0 {
			return Options{}, fmt.Errorf("usage: sesame automation apply --file <path>")
		}
	case "list":
		if len(rest) != 0 {
			return Options{}, fmt.Errorf("usage: sesame automation list [--workspace-root <path>]")
		}
	case "run":
		if len(rest) != 1 {
			return Options{}, fmt.Errorf("usage: sesame automation run [--watcher-id <id>] [--state-file <path>] <automation-id>")
		}
		cmd.ID = strings.TrimSpace(rest[0])
	case "get", "pause", "resume", "remove", "install", "reinstall", "watcher":
		if len(rest) != 1 {
			return Options{}, fmt.Errorf("usage: sesame automation %s <id>", cmd.Action)
		}
		cmd.ID = strings.TrimSpace(rest[0])
	}

	return Options{Automation: cmd}, nil
}

func parseTriggerOptions(args []string) (Options, error) {
	if len(args) == 0 {
		return Options{}, fmt.Errorf("usage: sesame trigger <emit|heartbeat> --file <path>")
	}

	cmd := &TriggerCommand{Action: strings.ToLower(strings.TrimSpace(args[0]))}
	fs := flag.NewFlagSet("sesame trigger "+cmd.Action, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&cmd.File, "file", "", "json file containing the trigger request")
	if err := fs.Parse(reorderInterspersedArgs(args[1:])); err != nil {
		return Options{}, err
	}

	switch cmd.Action {
	case "emit", "heartbeat":
		if cmd.File == "" || len(fs.Args()) != 0 {
			return Options{}, fmt.Errorf("usage: sesame trigger %s --file <path>", cmd.Action)
		}
	default:
		return Options{}, fmt.Errorf("unknown trigger command %q", cmd.Action)
	}

	return Options{Trigger: cmd}, nil
}

func parseIncidentOptions(args []string) (Options, error) {
	if len(args) == 0 {
		return Options{}, fmt.Errorf("usage: sesame incident <list|get> ...")
	}

	cmd := &IncidentCommand{Action: strings.ToLower(strings.TrimSpace(args[0]))}
	fs := flag.NewFlagSet("sesame incident "+cmd.Action, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	switch cmd.Action {
	case "list":
		fs.StringVar(&cmd.AutomationID, "automation-id", "", "optional automation filter")
		fs.StringVar(&cmd.WorkspaceRoot, "workspace-root", "", "optional workspace root filter")
		fs.StringVar(&cmd.Status, "status", "", "optional incident status filter")
		fs.IntVar(&cmd.Limit, "limit", 0, "optional limit")
	case "get":
	default:
		return Options{}, fmt.Errorf("unknown incident command %q", cmd.Action)
	}
	if err := fs.Parse(reorderInterspersedArgs(args[1:])); err != nil {
		return Options{}, err
	}

	rest := fs.Args()
	switch cmd.Action {
	case "list":
		if len(rest) != 0 {
			return Options{}, fmt.Errorf("usage: sesame incident list [--automation-id <id>] [--workspace-root <path>] [--status <status>] [--limit <n>]")
		}
		if cmd.Limit < 0 {
			return Options{}, fmt.Errorf("usage: sesame incident list [--automation-id <id>] [--workspace-root <path>] [--status <status>] [--limit <n>]")
		}
	case "get":
		if len(rest) != 1 {
			return Options{}, fmt.Errorf("usage: sesame incident get <id>")
		}
		cmd.ID = strings.TrimSpace(rest[0])
	}

	return Options{Incident: cmd}, nil
}

func reorderInterspersedArgs(args []string) []string {
	flags := make([]string, 0, len(args))
	positionals := make([]string, 0, len(args))
	for idx := 0; idx < len(args); idx++ {
		arg := args[idx]
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
			if strings.Contains(arg, "=") {
				continue
			}
			if idx+1 < len(args) && !strings.HasPrefix(args[idx+1], "-") {
				flags = append(flags, args[idx+1])
				idx++
			}
			continue
		}
		positionals = append(positionals, arg)
	}
	return append(flags, positionals...)
}

const defaultSkillRef = "main"
