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
	workspaceRoot = strings.TrimSpace(workspaceRoot)

	dataDir := strings.TrimSpace(explicitDataDir)
	if dataDir == "" {
		dataDir = strings.TrimSpace(os.Getenv("SESAME_DATA_DIR"))
	}
	if dataDir == "" {
		if workspaceRoot != "" {
			dataDir = filepath.Join(workspaceRoot, DirName)
		} else {
			dataDir = globalRoot
		}
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

	if workspaceRoot == "" {
		return paths, nil
	}

	paths.WorkspaceRoot = workspaceRoot
	paths.WorkspaceDir = workspaceRoot
	paths.WorkspacePromptFile = filepath.Join(workspaceRoot, "docs", "prompt.md")
	paths.WorkspaceRulesDir = filepath.Join(workspaceRoot, "rules")
	paths.WorkspaceSkillsDir = filepath.Join(workspaceRoot, "skills")
	paths.WorkspaceToolsDir = filepath.Join(workspaceRoot, "resources", "tools")
	return paths, nil
}
