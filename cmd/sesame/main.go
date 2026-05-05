package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go-agent/internal/config"
	"go-agent/internal/skillcatalog"
	v2app "go-agent/internal/v2/app"
	v2client "go-agent/internal/v2/client"
	v2tools "go-agent/internal/v2/tools"
	"go-agent/internal/v2/tui"
)

const defaultV2Addr = "127.0.0.1:8421"

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "skill" {
		return runSkillCommand(args[1:], stdout, stderr)
	}
	return runApp(ctx, args, stderr)
}

func runApp(ctx context.Context, args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("sesame", flag.ContinueOnError)
	fs.SetOutput(stderr)

	workspace := fs.String("workspace", "", "workspace root")
	daemon := fs.Bool("daemon", false, "run daemon")
	dataDir := fs.String("data-dir", "", "sesame data directory")
	addr := fs.String("addr", defaultV2Addr, "daemon listen address")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(stderr, "unexpected arguments: %s\n", strings.Join(fs.Args(), " "))
		return 2
	}

	if *workspace == "" {
		if wd, err := os.Getwd(); err == nil {
			*workspace = wd
		}
	}

	cfg, err := config.ResolveCLIStartupConfig(config.CLIStartupOverrides{
		WorkspaceRoot: *workspace,
		DataDir:       *dataDir,
		Addr:          *addr,
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if *daemon {
		if err := v2app.Run(ctx, cfg); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}

	baseURL := fmt.Sprintf("http://%s", connectAddr(cfg.Addr))
	if err := ensureDaemon(ctx, baseURL, cfg); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	client := v2client.New(baseURL)
	if err := tui.Run(ctx, client, *workspace); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func runSkillCommand(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printSkillUsage(stderr)
		return 2
	}

	switch args[0] {
	case "lint":
		return runSkillLintCommand(args[1:], stdout, stderr)
	case "test":
		return runSkillTestCommand(args[1:], stdout, stderr)
	case "install":
		return runSkillInstallCommand(args[1:], stdout, stderr)
	case "pack":
		return runSkillPackCommand(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown skill subcommand %q\n", args[0])
		printSkillUsage(stderr)
		return 2
	}
}

func runSkillLintCommand(args []string, stdout, stderr io.Writer) int {
	return runSkillValidationCommand(
		args,
		stdout,
		stderr,
		"usage: sesame skill lint <path...> [--workspace <root>]",
		skillcatalog.LintSkillFile,
	)
}

func runSkillTestCommand(args []string, stdout, stderr io.Writer) int {
	return runSkillValidationCommand(
		args,
		stdout,
		stderr,
		"usage: sesame skill test <path...> [--workspace <root>]",
		skillcatalog.TestSkillFile,
	)
}

func runSkillValidationCommand(args []string, stdout, stderr io.Writer, usage string, validate func(string, []string) ([]skillcatalog.LintFinding, error)) int {
	flags, err := parseSkillCommandFlags(args, skillCommandFlagWorkspace)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if len(flags.positional) == 0 {
		fmt.Fprintln(stderr, usage)
		return 2
	}

	toolNames := availableToolNames()
	hasErrors := false
	for _, input := range flags.positional {
		path, err := resolveInputPath(input, flags.workspace)
		if err != nil {
			hasErrors = true
			fmt.Fprintln(stderr, err)
			continue
		}

		findings, err := validate(path, toolNames)
		if err != nil {
			hasErrors = true
			fmt.Fprintf(stderr, "ERROR %s: %v\n", path, err)
			continue
		}
		if len(findings) == 0 {
			fmt.Fprintf(stdout, "OK %s\n", path)
			continue
		}
		if writeSkillFindings(stdout, path, findings) {
			hasErrors = true
		}
	}
	if hasErrors {
		return 1
	}
	return 0
}

func runSkillInstallCommand(args []string, stdout, stderr io.Writer) int {
	workspace, positional, err := parseWorkspaceFlag(args)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if len(positional) != 1 {
		fmt.Fprintln(stderr, "usage: sesame skill install <template-path-or-name> --workspace <root>")
		return 2
	}
	if strings.TrimSpace(workspace) == "" {
		fmt.Fprintln(stderr, "--workspace is required")
		return 2
	}

	sourcePath, err := resolveSkillTemplateSource(positional[0], workspace)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	installedPath, err := skillcatalog.InstallSkillTemplate(sourcePath, workspace)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "installed %s\n", installedPath)
	return 0
}

func runSkillPackCommand(args []string, stdout, stderr io.Writer) int {
	flags, err := parseSkillCommandFlags(args, skillCommandFlagWorkspace|skillCommandFlagOut)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if len(flags.positional) != 1 {
		fmt.Fprintln(stderr, "usage: sesame skill pack <template-path-or-name> --out <zip-path> [--workspace <root>]")
		return 2
	}
	if strings.TrimSpace(flags.out) == "" {
		fmt.Fprintln(stderr, "--out is required")
		return 2
	}

	sourcePath, err := resolveSkillTemplateSource(flags.positional[0], flags.workspace)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	skillPath, findings, err := skillcatalog.TestSkillTemplate(sourcePath, availableToolNames())
	if err != nil {
		fmt.Fprintf(stderr, "ERROR %s: %v\n", sourcePath, err)
		return 1
	}
	if len(findings) != 0 {
		writeSkillFindings(stdout, skillPath, findings)
		return 1
	}

	packedPath, err := skillcatalog.PackSkillTemplate(sourcePath, flags.out)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "packed %s\n", packedPath)
	return 0
}

func printSkillUsage(w io.Writer) {
	fmt.Fprintln(w, "usage:")
	fmt.Fprintln(w, "  sesame skill lint <path...> [--workspace <root>]")
	fmt.Fprintln(w, "  sesame skill test <path...> [--workspace <root>]")
	fmt.Fprintln(w, "  sesame skill install <template-path-or-name> --workspace <root>")
	fmt.Fprintln(w, "  sesame skill pack <template-path-or-name> --out <zip-path> [--workspace <root>]")
}

func parseWorkspaceFlag(args []string) (string, []string, error) {
	flags, err := parseSkillCommandFlags(args, skillCommandFlagWorkspace)
	if err != nil {
		return "", nil, err
	}
	return flags.workspace, flags.positional, nil
}

type skillCommandFlags struct {
	workspace  string
	out        string
	positional []string
}

type skillCommandFlag uint8

const (
	skillCommandFlagWorkspace skillCommandFlag = 1 << iota
	skillCommandFlagOut
)

func parseSkillCommandFlags(args []string, allowed skillCommandFlag) (skillCommandFlags, error) {
	var flags skillCommandFlags
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch {
		case arg == "--workspace" || arg == "-workspace":
			if allowed&skillCommandFlagWorkspace == 0 {
				return skillCommandFlags{}, fmt.Errorf("unknown flag %q", arg)
			}
			if i+1 >= len(args) {
				return skillCommandFlags{}, fmt.Errorf("%s requires a value", arg)
			}
			flags.workspace = strings.TrimSpace(args[i+1])
			i++
		case strings.HasPrefix(arg, "--workspace="):
			if allowed&skillCommandFlagWorkspace == 0 {
				return skillCommandFlags{}, fmt.Errorf("unknown flag %q", arg)
			}
			flags.workspace = strings.TrimSpace(strings.TrimPrefix(arg, "--workspace="))
		case strings.HasPrefix(arg, "-workspace="):
			if allowed&skillCommandFlagWorkspace == 0 {
				return skillCommandFlags{}, fmt.Errorf("unknown flag %q", arg)
			}
			flags.workspace = strings.TrimSpace(strings.TrimPrefix(arg, "-workspace="))
		case arg == "--out" || arg == "-out":
			if allowed&skillCommandFlagOut == 0 {
				return skillCommandFlags{}, fmt.Errorf("unknown flag %q", arg)
			}
			if i+1 >= len(args) {
				return skillCommandFlags{}, fmt.Errorf("%s requires a value", arg)
			}
			flags.out = strings.TrimSpace(args[i+1])
			i++
		case strings.HasPrefix(arg, "--out="):
			if allowed&skillCommandFlagOut == 0 {
				return skillCommandFlags{}, fmt.Errorf("unknown flag %q", arg)
			}
			flags.out = strings.TrimSpace(strings.TrimPrefix(arg, "--out="))
		case strings.HasPrefix(arg, "-out="):
			if allowed&skillCommandFlagOut == 0 {
				return skillCommandFlags{}, fmt.Errorf("unknown flag %q", arg)
			}
			flags.out = strings.TrimSpace(strings.TrimPrefix(arg, "-out="))
		case strings.HasPrefix(arg, "-"):
			return skillCommandFlags{}, fmt.Errorf("unknown flag %q", arg)
		default:
			flags.positional = append(flags.positional, arg)
		}
	}
	return flags, nil
}

func availableToolNames() []string {
	reg := v2tools.NewRegistry()
	v2tools.RegisterAllTools(reg, nil, skillcatalog.Catalog{})

	defs := reg.AllToolDefinitions()
	names := make([]string, 0, len(defs))
	for _, def := range defs {
		if trimmed := strings.TrimSpace(def.Name); trimmed != "" {
			names = append(names, trimmed)
		}
	}
	sort.Strings(names)
	return names
}

func resolveInputPath(input string, workspace string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", fmt.Errorf("path is required")
	}
	candidates := []string{input}
	if strings.TrimSpace(workspace) != "" && !filepath.IsAbs(input) {
		candidates = append(candidates, filepath.Join(workspace, input))
	}
	path, ok := firstExistingPath(candidates...)
	if !ok {
		return "", fmt.Errorf("path %q not found", input)
	}
	return path, nil
}

func resolveSkillTemplateSource(input string, workspace string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", fmt.Errorf("template path or name is required")
	}
	if !looksLikePath(input) {
		for _, root := range exampleSkillRoots(workspace) {
			if path, ok := firstExistingPath(filepath.Join(root, input)); ok {
				return path, nil
			}
		}
	}
	if path, ok := firstExistingPath(skillTemplateSourceCandidates(input, workspace)...); ok {
		return path, nil
	}
	return "", fmt.Errorf("skill template %q not found", input)
}

func looksLikePath(input string) bool {
	return strings.ContainsAny(input, `/\`)
}

func skillTemplateSourceCandidates(input string, workspace string) []string {
	var candidates []string
	for _, variant := range normalizedPathVariants(input) {
		candidates = append(candidates, variant)
		if strings.TrimSpace(workspace) != "" && !filepath.IsAbs(variant) {
			candidates = append(candidates, filepath.Join(workspace, variant))
		}
	}
	return dedupeStrings(candidates)
}

func normalizedPathVariants(input string) []string {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}

	normalized := strings.NewReplacer(`/`, string(filepath.Separator), `\`, string(filepath.Separator)).Replace(input)
	if normalized == input {
		return []string{input}
	}
	return dedupeStrings([]string{input, normalized})
}

func exampleSkillRoots(workspace string) []string {
	var roots []string
	if cwd, err := os.Getwd(); err == nil {
		roots = append(roots, filepath.Join(cwd, "examples", "skills"))
	}
	if trimmed := strings.TrimSpace(workspace); trimmed != "" {
		roots = append(roots, filepath.Join(trimmed, "examples", "skills"))
	}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		roots = append(roots,
			filepath.Join(exeDir, "examples", "skills"),
			filepath.Join(exeDir, "..", "examples", "skills"),
		)
	}
	return dedupeStrings(roots)
}

func firstExistingPath(paths ...string) (string, bool) {
	for _, candidate := range paths {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if _, err := os.Stat(candidate); err != nil {
			continue
		}
		abs, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		return abs, true
	}
	return "", false
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func writeSkillFindings(w io.Writer, path string, findings []skillcatalog.LintFinding) bool {
	hasErrors := false
	for _, finding := range findings {
		severity := strings.TrimSpace(finding.Severity)
		if severity == "" {
			severity = skillcatalog.LintSeverityError
		}
		if severity == skillcatalog.LintSeverityError {
			hasErrors = true
		}
		if field := strings.TrimSpace(finding.Field); field != "" {
			fmt.Fprintf(w, "%s %s %s: %s\n", strings.ToUpper(severity), path, field, finding.Message)
			continue
		}
		fmt.Fprintf(w, "%s %s: %s\n", strings.ToUpper(severity), path, finding.Message)
	}
	return hasErrors
}

func connectAddr(listenAddr string) string {
	host, port, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return listenAddr
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}

func ensureDaemon(ctx context.Context, baseURL string, cfg config.Config) error {
	if daemonReady(ctx, baseURL) {
		return nil
	}
	for i := 0; i < 3; i++ {
		time.Sleep(300 * time.Millisecond)
		if daemonReady(ctx, baseURL) {
			return nil
		}
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	args := []string{"-daemon", "-workspace", cfg.Paths.WorkspaceRoot, "-addr", cfg.Addr}
	if cfg.DataDir != "" {
		args = append(args, "-data-dir", cfg.DataDir)
	}
	cmd := exec.Command(exe, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	var logFile *os.File
	if cfg.DataDir != "" {
		_ = os.MkdirAll(cfg.DataDir, 0o755)
		if f, openErr := os.OpenFile(filepath.Join(cfg.DataDir, "sesame-v2.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); openErr == nil {
			logFile = f
			cmd.Stdout = logFile
			cmd.Stderr = logFile
		}
	}
	if err := cmd.Start(); err != nil {
		if logFile != nil {
			_ = logFile.Close()
		}
		return err
	}
	if logFile != nil {
		_ = logFile.Close()
	}
	if err := cmd.Process.Release(); err != nil {
		return err
	}
	return waitForDaemon(ctx, baseURL, 5*time.Second)
}

func waitForDaemon(ctx context.Context, baseURL string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if daemonReady(ctx, baseURL) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for v2 daemon at %s", baseURL)
		}
		timer := time.NewTimer(200 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func daemonReady(ctx context.Context, baseURL string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/v2/status", nil)
	if err != nil {
		return false
	}
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
