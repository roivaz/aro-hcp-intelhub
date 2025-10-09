package traceimages

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tidwall/gjson"
	"sigs.k8s.io/yaml"

	"github.com/roivaz/aro-hcp-intelhub/internal/gitrepo"
	"github.com/roivaz/aro-hcp-intelhub/internal/logging"
)

const defaultRepoURL = "https://github.com/Azure/ARO-HCP"

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
	// "Package Operator Package": {
	// 	Registry:   "quay.io",
	// 	Repository: "redhat-user-workloads/redhat-appstudio-tenant/po-package",
	// 	SourceRepo: "https://github.com/package-operator/package-operator",
	// },
	// "Package Operator Manager": {
	// 	Registry:   "quay.io",
	// 	Repository: "redhat-user-workloads/redhat-appstudio-tenant/po-manager",
	// 	SourceRepo: "https://github.com/package-operator/package-operator",
	// },
	// "Package Operator Remote Phase Manager": {
	// 	Registry:   "quay.io",
	// 	Repository: "redhat-user-workloads/redhat-appstudio-tenant/po-remote-phase-manager",
	// 	SourceRepo: "https://github.com/package-operator/package-operator",
	// },
}

var imageConfigPaths = map[string][]string{
	"Backend":         {"backend", "image"},
	"Frontend":        {"frontend", "image"},
	"Cluster Service": {"clustersService", "image"},
	"Maestro":         {"maestro", "image"},
	"Hypershift":      {"hypershift", "image"},
	"ACM Operator":    {"acm", "operator", "bundle"},
	"MCE":             {"acm", "mce", "bundle"},
	"OcMirror":        {"imageSync", "ocMirror", "image"},
	// "Package Operator Package":              {"pko", "imagePackage"},
	// "Package Operator Manager":              {"pko", "imageManager"},
	// "Package Operator Remote Phase Manager": {"pko", "remotePhaseManager"},
}

type envFile struct {
	Path     string
	BasePath []string
}

var environmentConfigSources = map[string]envFile{
	"dev": {
		Path:     filepath.Join("config", "rendered", "dev", "dev", "westus3.yaml"),
		BasePath: nil,
	},
	"int": {
		Path:     filepath.Join("config", "config.msft.clouds-overlay.yaml"),
		BasePath: []string{"clouds", "public", "environments", "int", "defaults"},
	},
	"stg": {
		Path:     filepath.Join("config", "config.msft.clouds-overlay.yaml"),
		BasePath: []string{"clouds", "public", "environments", "stg", "defaults"},
	},
	"prod": {
		Path:     filepath.Join("config", "config.msft.clouds-overlay.yaml"),
		BasePath: []string{"clouds", "public", "environments", "prod", "defaults"},
	},
}

type Config struct {
	RepoPath   string
	SkopeoPath string
	PullSecret string
	RepoURL    string
	Logger     logging.Logger
}

type Tracer struct {
	cfg  Config
	repo *gitrepo.Repo
	log  logging.Logger
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

	log := cfg.Logger
	if log.Logr().GetSink() == nil {
		log = logging.New(logging.DefaultLogger())
	}
	log = log.WithName("traceimages.tracer")

	repo := gitrepo.New(gitrepo.RepoConfig{URL: cfg.RepoURL, Path: cfg.RepoPath})

	return &Tracer{cfg: cfg, repo: repo, log: log}, nil
}

func (t *Tracer) Trace(ctx context.Context, commitSHA, environment string) (TraceResult, error) {
	result := TraceResult{CommitSHA: commitSHA, Environment: environment}

	source, ok := environmentConfigSources[environment]
	if !ok {
		return result, fmt.Errorf("unsupported environment: %s", environment)
	}

	if err := t.ensureRepo(ctx); err != nil {
		t.log.Error(err, "prepare repo failed")
		result.Errors = append(result.Errors, fmt.Sprintf("prepare repo: %v", err))
		return result, nil
	}

	repoPath, err := filepath.Abs(t.cfg.RepoPath)
	if err != nil {
		return result, fmt.Errorf("resolve repo path: %w", err)
	}

	checkoutDir, restore, err := t.checkoutCommit(ctx, repoPath, commitSHA)
	if err != nil {
		return result, err
	}
	defer restore()

	envConfig, err := loadEnvironmentConfig(checkoutDir, source)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("extract images: %v", err))
		return result, nil
	}

	components := make([]Component, 0, len(imageConfigPaths))
	var errs []string

	for _, name := range sortedKeys(imageConfigPaths) {
		section := getNested(envConfig, imageConfigPaths[name])
		registry := stringFromMap(section, "registry")
		repository := stringFromMap(section, "repository")
		digest := stringFromMap(section, "digest")

		component := Component{
			Name:       name,
			Registry:   registry,
			Repository: repository,
			Digest:     digest,
		}

		if mapping, ok := componentMappings[name]; ok {
			if component.Registry == "" && mapping.Registry != "" {
				component.Registry = mapping.Registry
			}
			if component.Repository == "" && mapping.Repository != "" {
				component.Repository = mapping.Repository
			}
			if mapping.SourceRepo != "" {
				src := mapping.SourceRepo
				component.SourceRepoURL = &src
			}
		}

		if component.Registry == "" || component.Repository == "" {
			err := fmt.Errorf("missing registry or repository for %s", name)
			errs = append(errs, err.Error())
			msg := err.Error()
			component.Error = &msg
			components = append(components, component)
			continue
		}

		labels, err := t.inspectImage(ctx, component.Registry, component.Repository, component.Digest)
		if err != nil {
			t.log.Error(err, "inspect image failed", "component", name)
			msg := err.Error()
			component.Error = &msg
			errs = append(errs, fmt.Sprintf("inspect %s: %v", name, err))
		} else if sha := labels["vcs-ref"]; sha != "" {
			component.SourceSHA = &sha
		}

		components = append(components, component)
	}

	result.Components = components
	result.Errors = errs

	return result, nil
}

func loadEnvironmentConfig(root string, src envFile) (map[string]any, error) {
	configPath := filepath.Join(root, src.Path)
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", configPath, err)
	}

	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", configPath, err)
	}

	if len(src.BasePath) == 0 {
		return raw, nil
	}

	section := getNested(raw, src.BasePath)
	if section == nil {
		return nil, fmt.Errorf("path %s not found in %s", strings.Join(src.BasePath, "."), configPath)
	}

	return section, nil
}

func (t *Tracer) ensureRepo(ctx context.Context) error {
	_, err := t.repo.Ensure(ctx)
	return err
}

func (t *Tracer) checkoutCommit(ctx context.Context, repoPath, commit string) (string, func(), error) {
	if _, err := t.repo.Run(ctx, "rev-parse", commit); err != nil {
		return "", nil, fmt.Errorf("resolve commit %s: %w", commit, err)
	}

	checkoutDir, err := os.MkdirTemp("", "aro-hcp-checkout-*")
	if err != nil {
		return "", nil, fmt.Errorf("create temp checkout: %w", err)
	}

	cleanup := func() {
		if err := os.RemoveAll(checkoutDir); err != nil {
			t.log.Error(err, "cleanup checkout dir failed", "dir", checkoutDir)
		}
	}

	if err := t.repo.WorktreeAddDetach(ctx, checkoutDir, commit); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("create worktree: %w", err)
	}

	return checkoutDir, func() {
		if err := t.repo.WorktreeRemove(context.Background(), checkoutDir); err != nil {
			t.log.Error(err, "remove worktree failed", "dir", checkoutDir)
		}
		cleanup()
	}, nil
}

func (t *Tracer) inspectImage(ctx context.Context, registry, repository, digest string) (map[string]string, error) {
	imageRef := fmt.Sprintf("%s/%s@%s", registry, repository, digest)
	args := []string{"inspect", "--raw"}
	if t.cfg.PullSecret != "" {
		args = append(args, "--authfile", t.cfg.PullSecret)
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
	if t.cfg.PullSecret != "" {
		configArgs = append(configArgs, "--authfile", t.cfg.PullSecret)
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
	cmd := exec.CommandContext(ctx, t.cfg.SkopeoPath, args...)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(output))
		if trimmed != "" {
			t.log.Debug("skopeo stderr", "output", trimmed)
		}
		t.log.Error(err, "skopeo command failed", "args", args)
		return nil, fmt.Errorf("skopeo %s: %v: %s", strings.Join(args, " "), err, trimmed)
	}
	return output, nil
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

func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
