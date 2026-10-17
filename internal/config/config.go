// Package config loads the static cluster/endpoint list clustertop polls.
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

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
// already-released binary. An empty cluster list and duplicate cluster names
// are hard errors: there is nothing to display in the former case, and fetch
// results could not be routed unambiguously in the latter.
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
	seenNames := make(map[string]struct{}, len(cfg.Clusters))
	nameOrder := make([]string, 0, len(cfg.Clusters))
	duplicateNames := make(map[string]struct{})
	for _, cluster := range cfg.Clusters {
		if _, exists := seenNames[cluster.Name]; exists {
			duplicateNames[cluster.Name] = struct{}{}
			continue
		}
		seenNames[cluster.Name] = struct{}{}
		nameOrder = append(nameOrder, cluster.Name)
	}
	if len(duplicateNames) == 1 {
		for _, name := range nameOrder {
			if _, exists := duplicateNames[name]; exists {
				return Config{}, fmt.Errorf("config: duplicate cluster name %q", name)
			}
		}
	}
	if len(duplicateNames) > 1 {
		quotedNames := make([]string, 0, len(duplicateNames))
		for _, name := range nameOrder {
			if _, exists := duplicateNames[name]; exists {
				quotedNames = append(quotedNames, fmt.Sprintf("%q", name))
			}
		}
		return Config{}, errors.New("config: duplicate cluster names " + strings.Join(quotedNames, ", "))
	}
	return cfg, nil
}
