package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type FocusArea string

const (
	FocusLogic       FocusArea = "logic"
	FocusSecurity    FocusArea = "security"
	FocusPerformance FocusArea = "performance"
	FocusTypes       FocusArea = "types"
	FocusTests       FocusArea = "tests"
)

type OutputFormat string

const (
	FormatMarkdown    OutputFormat = "markdown"
	FormatJSON        OutputFormat = "json"
	FormatAnnotations OutputFormat = "annotations"
)

type Config struct {
	Agents             int          `json:"agents"`
	Focus              []FocusArea  `json:"focus"`
	Model              string       `json:"model"`
	ConfidenceThreshold float64     `json:"confidence_threshold"`
	Output             string       `json:"output"`
	MaxTokensPerAgent  int          `json:"max_tokens_per_agent"`
	MaxCostUSD         float64      `json:"max_cost_usd,omitempty"`
	GitHubToken        string       `json:"github_token,omitempty"`
	GitLabToken        string       `json:"gitlab_token,omitempty"`
	GitLabHost         string       `json:"gitlab_host,omitempty"`
	BitbucketToken     string       `json:"bitbucket_token,omitempty"` // "username:app_password"
	ConcurrentAgents   int          `json:"concurrent_agents"`

	// Runtime-only (not persisted)
	Format  OutputFormat `json:"-"`
	Fix     bool         `json:"-"`
	Verbose bool         `json:"-"`
}

var Defaults = Config{
	Agents:              4,
	Focus:               []FocusArea{FocusLogic, FocusSecurity, FocusPerformance, FocusTypes, FocusTests},
	Model:               "claude-haiku-4-5-20251001",
	ConfidenceThreshold: 0.80,
	Output:              "REVIEW.md",
	MaxTokensPerAgent:   4000,
	MaxCostUSD:          0,
	GitLabHost:          "https://gitlab.com",
	ConcurrentAgents:    3,
	Format:              FormatMarkdown,
}

func Load(configPath string) (*Config, error) {
	cfg := Defaults

	// Load from config file (project-level)
	projectConfig := "claude-review.config.json"
	if configPath != "" {
		projectConfig = configPath
	}
	if err := mergeFile(&cfg, projectConfig); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	// Load from global user config
	home, err := os.UserHomeDir()
	if err == nil {
		globalConfig := filepath.Join(home, ".claude-review.json")
		if err := mergeFile(&cfg, globalConfig); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("reading global config: %w", err)
		}
	}

	// Environment variable overrides
	if key := os.Getenv("GITHUB_TOKEN"); key != "" && cfg.GitHubToken == "" {
		cfg.GitHubToken = key
	}
	if key := os.Getenv("GITLAB_TOKEN"); key != "" && cfg.GitLabToken == "" {
		cfg.GitLabToken = key
	}
	if key := os.Getenv("BITBUCKET_TOKEN"); key != "" && cfg.BitbucketToken == "" {
		cfg.BitbucketToken = key
	}
	if host := os.Getenv("GITLAB_HOST"); host != "" && cfg.GitLabHost == "https://gitlab.com" {
		cfg.GitLabHost = host
	}

	return &cfg, nil
}

func mergeFile(cfg *Config, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, cfg)
}

func (c *Config) Validate() error {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		return fmt.Errorf("ANTHROPIC_API_KEY environment variable is not set\n\nGet your API key at https://console.anthropic.com/")
	}
	if c.Agents < 1 || c.Agents > 10 {
		return fmt.Errorf("agents must be between 1 and 10, got %d", c.Agents)
	}
	if c.ConfidenceThreshold < 0 || c.ConfidenceThreshold > 1 {
		return fmt.Errorf("confidence_threshold must be between 0 and 1")
	}
	return nil
}

func (c *Config) AllFocusAreas() []FocusArea {
	return []FocusArea{FocusLogic, FocusSecurity, FocusPerformance, FocusTypes, FocusTests}
}
