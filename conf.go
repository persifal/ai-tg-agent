package main

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

func NewConf() (*Conf, error) {
	conf, err := load("conf.yaml")
	if err != nil {
		return nil, err
	}

	return conf, nil
}

type Conf struct {
	Debug bool `yaml:"debug"`
	Bot   struct {
		MaxContentLen int      `yaml:"max-content-len"`
		Key           string   `yaml:"key"`
		Whitelist     []string `yaml:"whitelist"`
	} `yaml:"bot"`
	Anthropic struct {
        System string `yaml:"system"`
		Key   string `yaml:"key"`
		Proxy struct {
			Enabled bool   `yaml:"enabled"`
			Url     string `yaml:"url"`
		} `yaml:"proxy"`
	} `yaml:"anthropic"`
}

func load(path string) (*Conf, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("error resolving config path: %w", err)
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	config := &Conf{}

	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("error parsing yaml: %w", err)
	}

	if err := validate(config); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return config, nil
}

func validate(config *Conf) error {
	if config.Bot.Key == "" {
		return fmt.Errorf("telegram token is required")
	}
	if len(config.Bot.Whitelist) == 0 {
		return fmt.Errorf("whitelist is empty")
	}
	if config.Anthropic.Key == "" {
		return fmt.Errorf("anthropic API key is required")
	}

	return nil
}
