// Package syncclusters regenerates clusters.yaml from a declarative-config
// checkout, so the endpoint list can't silently drift the way it did for
// ardenone-hub (stale in docs for weeks after decommission). See
// docs/research/sync-clusters-source-manifests.md.
package syncclusters

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/jedarden/clustertop/internal/config"
)

const (
	kubectlProxyRelPath  = "devpod-observer/kubectl-proxy.yml"
	traefikSvcRelPath    = "traefik/tailscale-service.yml"
	tailscaleHostnameKey = "tailscale.com/hostname"
	kubectlTCPPortName   = "kubectl-tcp"
	proxyPort            = "8001"
)

// k8sObject is a minimal partial-unmarshal of just the fields sync.go
// needs — not a full manifest schema.
type k8sObject struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Annotations map[string]string `yaml:"annotations"`
	} `yaml:"metadata"`
	Spec struct {
		Ports []struct {
			Name string `yaml:"name"`
		} `yaml:"ports"`
	} `yaml:"spec"`
}

// findService decodes a multi-document YAML file and returns the first
// Service object found, if any.
func findService(data []byte) (k8sObject, bool, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	for {
		var obj k8sObject
		err := dec.Decode(&obj)
		if err != nil {
			// io.EOF ends the stream normally; any other decode error on one
			// document in a multi-doc stream is treated the same way — "no
			// Service found here" — rather than aborting the whole scan.
			break
		}
		if obj.Kind == "Service" {
			return obj, true, nil
		}
	}
	return k8sObject{}, false, nil
}

func hasKubectlTCPPort(obj k8sObject) bool {
	for _, p := range obj.Spec.Ports {
		if p.Name == kubectlTCPPortName {
			return true
		}
	}
	return false
}

// resolveCluster determines one cluster directory's endpoint, or returns
// found=false if neither known manifest shape is present.
func resolveCluster(clusterDir string) (endpoint, route string, found bool) {
	proxyPath := filepath.Join(clusterDir, kubectlProxyRelPath)
	if data, err := os.ReadFile(proxyPath); err == nil {
		if svc, ok, _ := findService(data); ok {
			if host, ok := svc.Metadata.Annotations[tailscaleHostnameKey]; ok && host != "" {
				return fmt.Sprintf("http://%s.tail1b1987.ts.net:%s", host, proxyPort), "direct-tailscale-operator", true
			}
		}
	}

	traefikPath := filepath.Join(clusterDir, traefikSvcRelPath)
	if data, err := os.ReadFile(traefikPath); err == nil {
		if svc, ok, _ := findService(data); ok {
			host, hasHost := svc.Metadata.Annotations[tailscaleHostnameKey]
			if hasHost && host != "" && hasKubectlTCPPort(svc) {
				return fmt.Sprintf("http://%s.tail1b1987.ts.net:%s", host, proxyPort), "traefik-kubectl-tcp", true
			}
		}
	}

	return "", "", false
}

// Scan walks <declarativeConfigPath>/k8s/*, resolving each cluster
// directory's endpoint. Directories matching neither known manifest shape
// are skipped with a logged warning, not a fatal error.
func Scan(declarativeConfigPath string) (config.Config, error) {
	k8sDir := filepath.Join(declarativeConfigPath, "k8s")
	entries, err := os.ReadDir(k8sDir)
	if err != nil {
		return config.Config{}, fmt.Errorf("reading %s: %w", k8sDir, err)
	}

	var cfg config.Config
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		clusterDir := filepath.Join(k8sDir, name)

		if _, err := os.Stat(filepath.Join(clusterDir, kubectlProxyRelPath)); err != nil {
			// No read-only proxy manifest at all — not every k8s/ subdirectory
			// is a cluster (e.g. argocd/ is a loose Application file).
			continue
		}

		endpoint, route, found := resolveCluster(clusterDir)
		if !found {
			log.Printf("sync-clusters: skipping %s — has a kubectl-proxy manifest but neither known hostname-annotation shape was found", name)
			continue
		}
		cfg.Clusters = append(cfg.Clusters, config.Cluster{
			Name:     name,
			Endpoint: endpoint,
			Route:    route,
		})
	}
	return cfg, nil
}

func writeClustersYAML(path string, cfg config.Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// Run is the sync-clusters subcommand entrypoint.
func Run(args []string) error {
	fs := flag.NewFlagSet("sync-clusters", flag.ContinueOnError)
	path := fs.String("declarative-config-path", "", "path to a declarative-config checkout")
	out := fs.String("out", "clusters.yaml", "output path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *path == "" {
		return fmt.Errorf("sync-clusters: --declarative-config-path is required")
	}

	cfg, err := Scan(*path)
	if err != nil {
		return err
	}
	if len(cfg.Clusters) == 0 {
		return fmt.Errorf("sync-clusters: found 0 clusters under %s/k8s — refusing to write an empty clusters.yaml", *path)
	}
	return writeClustersYAML(*out, cfg)
}
