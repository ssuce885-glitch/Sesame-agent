package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type UserConfig struct {
	Listen       UserConfigListen    `json:"listen"`
	Provider     string              `json:"provider"`
	Anthropic    UserConfigAnthropic `json:"anthropic"`
	OpenAI       UserConfigOpenAI    `json:"openai"`
	MaxToolSteps int                 `json:"max_tool_steps"`
}

type UserConfigListen struct {
	Addr string `json:"addr"`
}

type UserConfigAnthropic struct {
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
}

type UserConfigOpenAI struct {
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
}

func loadUserConfig() (UserConfig, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return UserConfig{}, nil
	}
	path := filepath.Join(home, ".agentd", "config.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return UserConfig{}, nil
	}
	if err != nil {
		return UserConfig{}, err
	}
	var cfg UserConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return UserConfig{}, fmt.Errorf("~/.agentd/config.json: %w", err)
	}
	return cfg, nil
}
