package config

import (
	"os"
	"path/filepath"
	"strings"
)

const DirName = ".sesame"

type Paths struct {
	GlobalRoot          string
	GlobalConfigFile    string
	GlobalCLIConfigFile string
	GlobalSkillsDir     string
	GlobalToolsDir      string
	WorkspaceRoot       string
	WorkspaceDir        string
	WorkspacePromptFile string
	WorkspaceRulesDir   string
	WorkspaceSkillsDir  string
	WorkspaceToolsDir   string
	DataDir             string
	DatabaseFile        string
	PIDFile             string
}

func ResolvePaths(workspaceRoot string, explicitDataDir string) (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, err
	}
	globalRoot := filepath.Join(home, DirName)

	dataDir := strings.TrimSpace(explicitDataDir)
	if dataDir == "" {
		dataDir = strings.TrimSpace(os.Getenv("SESAME_DATA_DIR"))
	}
	if dataDir == "" {
		dataDir = globalRoot
	}

	paths := Paths{
		GlobalRoot:          globalRoot,
		GlobalConfigFile:    filepath.Join(globalRoot, "config.json"),
		GlobalCLIConfigFile: filepath.Join(globalRoot, "cli.json"),
		GlobalSkillsDir:     filepath.Join(globalRoot, "skills"),
		GlobalToolsDir:      filepath.Join(globalRoot, "tools"),
		DataDir:             dataDir,
		DatabaseFile:        filepath.Join(dataDir, "sesame.db"),
		PIDFile:             filepath.Join(dataDir, "sesame.pid"),
	}

	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if workspaceRoot == "" {
		return paths, nil
	}

	workspaceDir := filepath.Join(workspaceRoot, DirName)
	paths.WorkspaceRoot = workspaceRoot
	paths.WorkspaceDir = workspaceDir
	paths.WorkspacePromptFile = filepath.Join(workspaceDir, "prompt.md")
	paths.WorkspaceRulesDir = filepath.Join(workspaceDir, "rules")
	paths.WorkspaceSkillsDir = filepath.Join(workspaceDir, "skills")
	paths.WorkspaceToolsDir = filepath.Join(workspaceDir, "tools")
	return paths, nil
}
