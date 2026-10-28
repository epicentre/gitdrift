// Package badge turns the files a pull would bring in into short labels such
// as deps or mig. It is pure: no git and no I/O.
package badge

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"
)

var labelPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,11}$`)

// Rule labels files that match one matcher. Exactly one of Name, Path and
// Dir is set.
type Rule struct {
	// Label is the badge text.
	Label string
	// Name is a path.Match glob applied to the base name at any depth.
	Name string
	// Path is a path.Match glob applied to the full slash-separated path.
	Path string
	// Dir is slash-separated directory components that must appear
	// consecutively anywhere in the directory part of the path.
	Dir string
}

// Defaults returns the built-in rules: deps, mig and env.
func Defaults() []Rule {
	deps := []string{
		"composer.json", "composer.lock", "package.json", "package-lock.json",
		"npm-shrinkwrap.json", "yarn.lock", "pnpm-lock.yaml", "bun.lock",
		"go.mod", "go.sum", "Cargo.toml", "Cargo.lock", "Gemfile", "Gemfile.lock",
		"requirements*.txt", "Pipfile", "Pipfile.lock", "poetry.lock", "uv.lock",
		"pyproject.toml", "pom.xml", "build.gradle", "build.gradle.kts", "gradle.lockfile",
	}
	envs := []string{".env.example", ".env.sample", ".env.dist", ".env.template"}
	migs := []string{"migrations", "migrate", "alembic/versions"}

	rules := make([]Rule, 0, len(deps)+len(migs)+len(envs))

	for _, n := range deps {
		rules = append(rules, Rule{Label: "deps", Name: n})
	}

	for _, d := range migs {
		rules = append(rules, Rule{Label: "mig", Dir: d})
	}

	for _, n := range envs {
		rules = append(rules, Rule{Label: "env", Name: n})
	}

	return rules
}

// ParseRule parses "<label>=<pattern>". A pattern without a slash matches
// base names; otherwise it matches the full path.
func ParseRule(v string) (Rule, error) {
	label, pattern, ok := strings.Cut(v, "=")
	if !ok {
		return Rule{}, fmt.Errorf("badge: %q is not <label>=<pattern>", v)
	}

	if !labelPattern.MatchString(label) {
		return Rule{}, fmt.Errorf("badge: invalid label %q", label)
	}

	if pattern == "" {
		return Rule{}, errors.New("badge: empty pattern")
	}

	if _, err := path.Match(pattern, ""); err != nil {
		return Rule{}, fmt.Errorf("badge: invalid pattern %q: %w", pattern, err)
	}

	if strings.Contains(pattern, "/") {
		return Rule{Label: label, Path: pattern}, nil
	}

	return Rule{Label: label, Name: pattern}, nil
}

// Match returns the unique labels of the rules that match at least one file,
// in rule order. Files are slash-separated paths.
func Match(rules []Rule, files []string) []string {
	var labels []string

	for _, r := range rules {
		if slices.Contains(labels, r.Label) {
			continue
		}

		if slices.ContainsFunc(files, r.matches) {
			labels = append(labels, r.Label)
		}
	}

	return labels
}

func (r Rule) matches(file string) bool {
	switch {
	case r.Name != "":
		ok, _ := path.Match(r.Name, path.Base(file))

		return ok
	case r.Path != "":
		ok, _ := path.Match(r.Path, file)

		return ok
	case r.Dir != "":
		return hasDir(file, r.Dir)
	default:
		return false
	}
}

// hasDir reports whether the directory components of file contain the
// components of dir consecutively.
func hasDir(file, dir string) bool {
	parts := strings.Split(file, "/")
	if len(parts) < 2 {
		return false
	}

	want := strings.Split(dir, "/")
	dirs := parts[:len(parts)-1]

	for i := 0; i+len(want) <= len(dirs); i++ {
		if slices.Equal(dirs[i:i+len(want)], want) {
			return true
		}
	}

	return false
}
