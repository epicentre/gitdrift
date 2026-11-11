package repo

import (
	"bytes"
	"context"
	"fmt"

	"github.com/epicentre/gitdrift/internal/badge"
	"github.com/epicentre/gitdrift/internal/git"
)

// configNoMatchExit is the exit code of git config when the key is not set.
const configNoMatchExit = 1

// IncomingFiles lists the files a pull would change: those that differ
// between the merge base of HEAD and the upstream and the upstream tip. A
// rename lists both its paths, since a pull removes one and adds the other.
// It returns nil when the branch is not behind.
func IncomingFiles(ctx context.Context, g *git.Runner, loc Location, st Status) ([]string, error) {
	if st.Behind == 0 || st.UpstreamSHA == "" {
		return nil, nil
	}

	out, err := g.Run(ctx, loc.Path, "diff", "--no-color", "--no-ext-diff", "--no-renames", "--name-only", "-z",
		"HEAD..."+st.UpstreamSHA, "--")
	if err != nil {
		return nil, fmt.Errorf("repo: incoming files of %s: %w", loc.Name, err)
	}

	var files []string

	for name := range bytes.SplitSeq(out, []byte{0}) {
		if len(name) > 0 {
			files = append(files, string(name))
		}
	}

	return files, nil
}

// BadgeRules returns the default badge rules followed by the valid
// gitdrift.badge values from git config; invalid values are skipped.
func BadgeRules(ctx context.Context, g *git.Runner, loc Location) ([]badge.Rule, error) {
	rules := badge.Defaults()

	out, err := g.Run(ctx, loc.Path, "config", "--get-all", "gitdrift.badge")
	if err != nil {
		if git.ExitCode(err) == configNoMatchExit {
			return rules, nil
		}

		return nil, fmt.Errorf("repo: read gitdrift.badge of %s: %w", loc.Name, err)
	}

	for line := range bytes.Lines(out) {
		rule, perr := badge.ParseRule(string(bytes.TrimRight(line, "\r\n")))
		if perr != nil {
			continue
		}

		rules = append(rules, rule)
	}

	return rules, nil
}
