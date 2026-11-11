package repo

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func FuzzParseStatusV2(f *testing.F) {
	f.Add([]byte("# branch.oid (initial)\x00# branch.head main\x00"))

	f.Fuzz(func(t *testing.T, data []byte) {
		st, err := parseStatusV2(data)
		if err != nil {
			return
		}

		if st.ahead < 0 || st.behind < 0 || st.tracked < 0 || st.untracked < 0 {
			t.Fatalf("negative count in %+v", st)
		}

		// Every tracked entry records its path, a rename or copy its
		// original path too.
		if len(st.changed) < st.tracked || st.unmerged > st.tracked {
			t.Fatalf("%d changed paths and %d unmerged for %d tracked entries", len(st.changed), st.unmerged, st.tracked)
		}
	})
}

func TestParseStatusV2(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		want    statusV2
		wantErr bool
	}{
		{"unborn", "# branch.oid (initial)\x00# branch.head main\x00", statusV2{unborn: true, branch: "main"}, false},
		{
			"gone upstream has no ab",
			"# branch.oid abc\x00# branch.head x\x00# branch.upstream origin/x\x00",
			statusV2{oid: "abc", branch: "x", upstream: "origin/x"},
			false,
		},
		{
			"rename records both paths",
			"# branch.head main\x002 R. N... 1 1 1 a b R100 new\x00old\x00? other\x00",
			statusV2{branch: "main", tracked: 1, untracked: 1, changed: []string{"new", "old"}},
			false,
		},
		{
			"changed path keeps its spaces",
			"1 .M N... 100644 100644 100644 a b dir/my file.txt\x00",
			statusV2{tracked: 1, changed: []string{"dir/my file.txt"}},
			false,
		},
		{
			"unmerged entry",
			"u UU N... 100644 100644 100644 100644 a b c both.txt\x00",
			statusV2{tracked: 1, unmerged: 1, changed: []string{"both.txt"}},
			false,
		},
		{"changed entry without a path", "1 .M N... 100644 100644 100644 a b\x00", statusV2{}, true},
		{"unmerged entry without a path", "u UU N... 100644 100644 100644 100644 a b c\x00", statusV2{}, true},
		{"rename without its original path", "2 R. N... 1 1 1 a b R100 new\x00", statusV2{}, true},
		{"ignored entries are not counted", "! junk.log\x00", statusV2{}, false},
		{"malformed ab", "# branch.ab +x -1\x00", statusV2{}, true},
		{"negative ab", "# branch.ab +-1 -1\x00", statusV2{}, true},
		{"unknown entry", "Z what\x00", statusV2{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseStatusV2([]byte(tt.in))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}

			if err != nil {
				return
			}

			if diff := cmp.Diff(tt.want, got, cmp.AllowUnexported(statusV2{})); diff != "" {
				t.Errorf("parseStatusV2 mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestParseStatusV2HasAB(t *testing.T) {
	t.Parallel()

	got, err := parseStatusV2([]byte(strings.Join([]string{"# branch.upstream o/x", "# branch.ab +3 -4", ""}, "\x00")))
	if err != nil || !got.hasAB || got.ahead != 3 || got.behind != 4 {
		t.Fatalf("got %+v, err %v", got, err)
	}
}
