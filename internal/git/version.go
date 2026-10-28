package git

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
)

// Version is a git release number.
type Version struct{ Major, Minor, Patch int }

// MinVersion is the oldest git release gitdrift supports.
var MinVersion = Version{2, 30, 0}

var versionRe = regexp.MustCompile(`(\d+)\.(\d+)(?:\.(\d+))?`)

// ParseVersion extracts the version from `git version` output.
func ParseVersion(out string) (Version, error) {
	m := versionRe.FindStringSubmatch(out)
	if m == nil {
		return Version{}, fmt.Errorf("cannot parse git version from %q", out)
	}

	var nums [3]int

	for i := range nums {
		if m[i+1] == "" {
			continue
		}

		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			return Version{}, fmt.Errorf("cannot parse git version from %q: %w", out, err)
		}

		nums[i] = n
	}

	return Version{nums[0], nums[1], nums[2]}, nil
}

// AtLeast reports whether v is the same as or newer than o.
func (v Version) AtLeast(o Version) bool {
	if v.Major != o.Major {
		return v.Major > o.Major
	}

	if v.Minor != o.Minor {
		return v.Minor > o.Minor
	}

	return v.Patch >= o.Patch
}

// String formats v as MAJOR.MINOR.PATCH.
func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// Version runs `git version` and parses it.
func (r *Runner) Version(ctx context.Context) (Version, error) {
	out, err := r.Run(ctx, "", "version")
	if err != nil {
		return Version{}, err
	}

	return ParseVersion(string(out))
}
