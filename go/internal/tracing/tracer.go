package tracing

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tidwall/gjson"
	"gopkg.in/yaml.v3"
)

const (
	defaultRepoURL = "https://github.com/Azure/ARO-HCP"
)

var componentMappings = map[string]struct {
	Registry   string
	Repository string
	SourceRepo string
}{
	"Backend": {
		Registry:   "arohcpsvcdev.azurecr.io",
		Repository: "arohcpbackend",
		SourceRepo: "https://github.com/Azure/ARO-HCP",
	},
	"Frontend": {
		Registry:   "arohcpsvcdev.azurecr.io",
		Repository: "arohcpfrontend",
		SourceRepo: "https://github.com/Azure/ARO-HCP",
	},
	"Cluster Service": {
		Registry:   "quay.io",
		Repository: "app-sre/uhc-clusters-service",
		SourceRepo: "https://gitlab.cee.redhat.com/service/uhc-clusters-service",
	},
	"Maestro": {
		Registry:   "quay.io",
		Repository: "redhat-user-workloads/maestro-rhtap-tenant/maestro/maestro",
		SourceRepo: "https://github.com/openshift-online/maestro/",
	},
	"Hypershift": {
		Registry:   "quay.io",
		Repository: "acm-d/rhtap-hypershift-operator",
		SourceRepo: "https://github.com/openshift/hypershift",
	},
	"ACM Operator": {
		Registry:   "quay.io",
		Repository: "redhat-user-workloads/crt-redhat-acm-tenant/acm-operator-bundle-acm-214",
		SourceRepo: "https://github.com/stolostron/acm-operator-bundle",
	},
	"MCE": {
		Registry:   "quay.io",
		Repository: "redhat-user-workloads/crt-redhat-acm-tenant/mce-operator-bundle-mce-29",
		SourceRepo: "https://github.com/stolostron/mce-operator-bundle",
	},
	"OcMirror": {
		Registry:   "arohcpsvcdev.azurecr.io",
		Repository: "image-sync/oc-mirror",
		SourceRepo: "https://github.com/openshift/oc-mirror",
	},
	"Package Operator Package": {
		Registry:   "quay.io",
		Repository: "redhat-user-workloads/redhat-appstudio-tenant/po-package",
		SourceRepo: "https://github.com/package-operator/package-operator",
	},
	"Package Operator Manager": {
		Registry:   "quay.io",
		Repository: "redhat-user-workloads/redhat-appstudio-tenant/po-manager",
		SourceRepo: "https://github.com/package-operator/package-operator",
	},
	"Package Operator Remote Phase Manager": {
		Registry:   "quay.io",
		Repository: "redhat-user-workloads/redhat-appstudio-tenant/po-remote-phase-manager",
		SourceRepo: "https://github.com/package-operator/package-operator",
	},
}

var environmentConfigPaths = map[string]struct {
	File     string
	BasePath []string
}{
	"dev": {
		File:     "config/rendered/dev/dev/westus3.yaml",
		BasePath: []string{"defaults"},
	},
	"int": {
		File:     "config/config.msft.clouds-overlay.yaml",
		BasePath: []string{"clouds", "public", "environments", "int", "defaults"},
	},
	"stg": {
		File:     "config/config.msft.clouds-overlay.yaml",
		BasePath: []string{"clouds", "public", "environments", "stg", "defaults"},
	},
	"prod": {
		File:     "config/config.msft.clouds-overlay.yaml",
		BasePath: []string{"clouds", "public", "environments", "prod", "defaults"},
	},
}

var imageConfigPaths = map[string][]string{
	"Backend":                               {"backend", "image"},
	"Frontend":                              {"frontend", "image"},
	"Cluster Service":                       {"clustersService", "image"},
	"Maestro":                               {"maestro", "image"},
	"Hypershift":                            {"hypershift", "image"},
	"ACM Operator":                          {"acm", "operator", "bundle"},
	"MCE":                                   {"acm", "mce", "bundle"},
	"OcMirror":                              {"imageSync", "ocMirror", "image"},
	"Package Operator Package":              {"pko", "imagePackage"},
	"Package Operator Manager":              {"pko", "imageManager"},
	"Package Operator Remote Phase Manager": {"pko", "remotePhaseManager"},
}

type Tracer struct {
	config Config
}

func NewTracer(cfg Config) (*Tracer, error) {
	if cfg.RepoPath == "" {
		return nil, fmt.Errorf("repo path is required")
	}
	if cfg.SkopeoPath == "" {
		cfg.SkopeoPath = "skopeo"
	}
	if cfg.RepoURL == "" {
		cfg.RepoURL = defaultRepoURL
	}
	return &Tracer{config: cfg}, nil
}

func (t *Tracer) Trace(ctx context.Context, commitSHA, environment string) (TraceResult, error) {
	result := TraceResult{CommitSHA: commitSHA, Environment: environment}
	if _, ok := environmentConfigPaths[environment]; !ok {
		return result, fmt.Errorf("unsupported environment: %s", environment)
	}

	if err := t.ensureRepo(ctx); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("prepare repo: %v", err))
		return result, nil
	}

	repoPath, err := filepath.Abs(t.config.RepoPath)
	if err != nil {
		return result, fmt.Errorf("resolve repo path: %w", err)
	}

	tempDir, restore, err := t.checkoutCommit(ctx, repoPath, commitSHA)
	if err != nil {
		return result, err
	}
	defer restore()

	images, err := extractImages(tempDir, environment)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("extract images: %v", err))
		return result, nil
	}

	components := make([]Component, 0, len(images))
	var errs []string
	for _, name := range sortedKeys(images) {
		img := images[name]
		component := Component{
			Name:       name,
			Registry:   img.Registry,
			Repository: img.Repository,
			Digest:     img.Digest,
		}
		if mapping, ok := componentMappings[name]; ok {
			if mapping.SourceRepo != "" {
				sourceRepo := mapping.SourceRepo
				component.SourceRepoURL = &sourceRepo
			}
		}

		labels, err := t.inspectImage(ctx, component.Registry, component.Repository, component.Digest)
		if err != nil {
			errMsg := err.Error()
			component.Error = &errMsg
			errs = append(errs, fmt.Sprintf("inspect %s: %v", name, err))
		} else {
			if sha := labels["vcs-ref"]; sha != "" {
				component.SourceSHA = &sha
			}
		}

		components = append(components, component)
	}

	result.Components = components
	result.Errors = errs
	return result, nil
}

func (t *Tracer) ensureRepo(ctx context.Context) error {
	if _, err := os.Stat(t.config.RepoPath); os.IsNotExist(err) {
		return cloneRepo(ctx, t.config.RepoURL, t.config.RepoPath)
	}
	return fetchRepo(ctx, t.config.RepoPath)
}

func (t *Tracer) checkoutCommit(ctx context.Context, repoPath, commit string) (string, func(), error) {
	gitArgs := []string{"rev-parse", commit}
	if err := runGit(ctx, repoPath, gitArgs...); err != nil {
		return "", nil, fmt.Errorf("resolve commit %s: %w", commit, err)
	}

	checkoutDir, err := os.MkdirTemp("", "aro-hcp-checkout-*")
	if err != nil {
		return "", nil, fmt.Errorf("create temp checkout: %w", err)
	}

	cleanup := func() {
		_ = os.RemoveAll(checkoutDir)
	}

	if err := runGit(ctx, repoPath, "worktree", "add", "--detach", checkoutDir, commit); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("create worktree: %w", err)
	}

	return checkoutDir, func() {
		_ = runGit(context.Background(), repoPath, "worktree", "remove", checkoutDir, "--force")
		cleanup()
	}, nil
}

func extractImages(root string, environment string) (map[string]struct {
	Registry   string
	Repository string
	Digest     string
}, error) {
	paths := environmentConfigPaths[environment]
	filePath := filepath.Join(root, paths.File)
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", filePath, err)
	}

	var data map[string]any
	if err := yaml.Unmarshal(content, &data); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", filePath, err)
	}

	baseDefaults, err := loadBaseDefaults(root)
	if err != nil {
		return nil, err
	}

	envSection := getNested(data, paths.BasePath)
	if envSection == nil {
		return nil, fmt.Errorf("path %s not found in config", strings.Join(paths.BasePath, "."))
	}

	images := make(map[string]struct {
		Registry   string
		Repository string
		Digest     string
	})

	for name, path := range imageConfigPaths {
		merged := mergeConfig(getNested(envSection, path), getNested(baseDefaults, path))
		registry := stringFromMap(merged, "registry")
		repo := stringFromMap(merged, "repository")
		digest := stringFromMap(merged, "digest")
		if registry == "" || repo == "" {
			return nil, fmt.Errorf("missing registry or repository for %s", name)
		}
		images[name] = struct {
			Registry   string
			Repository string
			Digest     string
		}{registry, repo, digest}
	}

	return images, nil
}

func loadBaseDefaults(root string) (map[string]any, error) {
	filePath := filepath.Join(root, "config", "config.yaml")
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read base config: %w", err)
	}

	replaced := strings.ReplaceAll(string(content), "{{ .ev2.availabilityZoneCount }}", "2")

	var data map[string]any
	if err := yaml.Unmarshal([]byte(replaced), &data); err != nil {
		return nil, fmt.Errorf("parse base config: %w", err)
	}
	defaults, _ := data["defaults"].(map[string]any)
	return defaults, nil
}

func mergeConfig(primary, fallback map[string]any) map[string]any {
	result := make(map[string]any)
	for k, v := range fallback {
		result[k] = v
	}
	for k, v := range primary {
		result[k] = v
	}
	return result
}

func getNested(source any, path []string) map[string]any {
	current, ok := source.(map[string]any)
	if !ok {
		return nil
	}
	for _, key := range path {
		next, ok := current[key]
		if !ok {
			return nil
		}
		current, ok = next.(map[string]any)
		if !ok {
			return nil
		}
	}
	return current
}

func stringFromMap(source map[string]any, key string) string {
	if source == nil {
		return ""
	}
	if value, ok := source[key]; ok {
		if str, ok := value.(string); ok {
			return str
		}
	}
	return ""
}

func (t *Tracer) inspectImage(ctx context.Context, registry, repository, digest string) (map[string]string, error) {
	imageRef := fmt.Sprintf("%s/%s@%s", registry, repository, digest)
	args := []string{"inspect", "--raw"}
	if t.config.PullSecret != "" {
		args = append(args, "--authfile", t.config.PullSecret)
	}
	args = append(args, "docker://"+imageRef)
	output, err := t.runSkopeo(ctx, args...)
	if err != nil {
		return nil, err
	}
	manifestJSON := string(output)
	configRef, err := resolveConfigReference(manifestJSON, registry, repository, digest)
	if err != nil {
		return nil, err
	}

	configArgs := []string{"inspect", "--config"}
	if t.config.PullSecret != "" {
		configArgs = append(configArgs, "--authfile", t.config.PullSecret)
	}
	configArgs = append(configArgs, configRef)
	configData, err := t.runSkopeo(ctx, configArgs...)
	if err != nil {
		return nil, err
	}

	labels := make(map[string]string)
	labelsJSON := string(configData)
	gjson.Get(labelsJSON, "config.Labels").ForEach(func(key, value gjson.Result) bool {
		if key.Str != "" && value.Str != "" {
			labels[key.Str] = value.Str
		}
		return true
	})

	return labels, nil
}

func resolveConfigReference(manifest string, registry, repository, digest string) (string, error) {
	mediaType := gjson.Get(manifest, "mediaType").Str
	switch mediaType {
	case "application/vnd.docker.distribution.manifest.v2+json", "application/vnd.oci.image.manifest.v1+json":
		return fmt.Sprintf("docker://%s/%s@%s", registry, repository, digest), nil
	case "application/vnd.docker.distribution.manifest.list.v2+json", "application/vnd.oci.image.index.v1+json":
		entries := gjson.Get(manifest, "manifests").Array()
		var fallback string
		for idx, entry := range entries {
			if dig := entry.Get("digest").Str; dig != "" {
				if idx == 0 {
					fallback = dig
				}
				arch := entry.Get("platform.architecture").Str
				osName := entry.Get("platform.os").Str
				if arch == "amd64" && osName == "linux" {
					return fmt.Sprintf("docker://%s/%s@%s", registry, repository, dig), nil
				}
			}
		}
		if fallback != "" {
			return fmt.Sprintf("docker://%s/%s@%s", registry, repository, fallback), nil
		}
		return "", fmt.Errorf("manifest list missing usable entries")
	default:
		return fmt.Sprintf("docker://%s/%s@%s", registry, repository, digest), nil
	}
}

func (t *Tracer) runSkopeo(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, t.config.SkopeoPath, args...)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("skopeo %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

func runGit(ctx context.Context, repoPath string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repoPath}, args...)...)
	cmd.Env = os.Environ()
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func cloneRepo(ctx context.Context, repoURL, path string) error {
	cmd := exec.CommandContext(ctx, "git", "clone", repoURL, path)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git clone: %v: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func fetchRepo(ctx context.Context, path string) error {
	return runGit(ctx, path, "fetch", "origin")
}

func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
