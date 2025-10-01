package diff

import "regexp"

var ignorePatternMap = map[string]string{
	"package-lock":         `package-lock\.json$`,
	"yarn-lock":            `yarn\.lock$`,
	"pnpm-lock":            `pnpm-lock\.yaml$`,
	"npm-shrinkwrap":       `npm-shrinkwrap\.json$`,
	"go-sum":               `go\.sum$`,
	"go-work-sum":          `go\.work\.sum$`,
	"gomodcache":           `(^|/)vendor/`,
	"node_modules":         `(^|/)node_modules/`,
	"generated-go":         `\.(?:pb|pb\.gw|pb\.json|pb\.grpc)\.go$`,
	"generated-client":     `\.generated\.(?:ts|js|py|go|rs|java)$`,
	"typescript-snapshots": `\.snap$`,
	"openapi-json":         `api/common-types/.*\.json$`,
	"rendered-config":      `config/rendered/.*`,
	"digests":              `config/.*\.digests\.yaml$`,
	"bicep-cache":          `dev-infrastructure/.+\.bicepparam$`,
	"helm-render":          `.*chart\.lock$`,
	"lockfiles":            `\.lock$`,
	"generated-json":       `.*\.swagger\.json$`,
}

func buildIgnorePatterns() map[string]*regexp.Regexp {
	compiled := make(map[string]*regexp.Regexp, len(ignorePatternMap))
	for reason, pattern := range ignorePatternMap {
		compiled[reason] = regexp.MustCompile(pattern)
	}
	return compiled
}

func shouldIgnoreFile(path string, patterns map[string]*regexp.Regexp) (bool, string) {
	for reason, rx := range patterns {
		if rx.MatchString(path) {
			return true, reason
		}
	}
	return false, ""
}
