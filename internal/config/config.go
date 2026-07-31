// Package config loads the static cluster/endpoint list clustertop polls.
package config

import (
	"errors"
	"os"

	"gopkg.in/yaml.v3"
)

// Cluster is one fleet cluster's read-only kubectl-proxy endpoint.
type Cluster struct {
	Name     string `yaml:"name"`
	Endpoint string `yaml:"endpoint"`
	Route    string `yaml:"route"`
	Notes    string `yaml:"notes,omitempty"`
}

// Config is the top-level clusters.yaml document.
type Config struct {
	Clusters []Cluster `yaml:"clusters"`
}

// LoadClusters reads and parses a clusters.yaml file at path. Unknown YAML
// keys are ignored (lenient decode) so a stale field never breaks an
// already-released binary. An empty cluster list is the only hard error —
// there is nothing to display.
func LoadClusters(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	if len(cfg.Clusters) == 0 {
		return Config{}, errors.New("config: clusters list is empty")
	}
	return cfg, nil
}
