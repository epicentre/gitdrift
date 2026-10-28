package repo

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func FuzzParseLog(f *testing.F) {
	f.Add([]byte("a\x1fb\x1fp1 p2\x1f1700000000\x1fname\x1fsubject\x00"))

	f.Fuzz(func(t *testing.T, data []byte) {
		commits, err := parseLog(data)
		if err != nil {
			return
		}

		for _, c := range commits {
			if c.SHA == "" && c.Short == "" && c.Subject == "" && c.Author == "" && c.When.IsZero() {
				t.Fatalf("empty commit from %q", data)
			}
		}
	})
}

func TestParseLog(t *testing.T) {
	t.Parallel()

	in := "aaa\x1fa\x1fp1 p2\x1f1700000000\x1fAda\x1fMerge branch x\x00" +
		"bbb\x1fb\x1fp1\x1f1700000100\x1fBob\x1ffix:\tkeep the tab\x00" +
		"ccc\x1fc\x1fp1 p2 p3\x1f1700000200\x1fCy\x1fOctopus\x00" +
		"ddd\x1fd\x1fp1\x1f1700000300\x1fDi\x1fodd\x1fsubject\x00"

	want := []Commit{
		{SHA: "aaa", Short: "a", Subject: "Merge branch x", Author: "Ada", When: time.Unix(1700000000, 0).UTC(), Merge: true},
		{SHA: "bbb", Short: "b", Subject: "fix:\tkeep the tab", Author: "Bob", When: time.Unix(1700000100, 0).UTC()},
		{SHA: "ccc", Short: "c", Subject: "Octopus", Author: "Cy", When: time.Unix(1700000200, 0).UTC(), Merge: true},
		{SHA: "ddd", Short: "d", Subject: "odd\x1fsubject", Author: "Di", When: time.Unix(1700000300, 0).UTC()},
	}

	got, err := parseLog([]byte(in))
	if err != nil {
		t.Fatal(err)
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("mismatch (-want +got):\n%s", diff)
	}
}

func TestParseLogErrors(t *testing.T) {
	t.Parallel()

	for name, in := range map[string]string{
		"too few fields": "a\x1fb\x00",
		"bad time":       "a\x1fb\x1fp\x1fnope\x1fn\x1fs\x00",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, err := parseLog([]byte(in)); err == nil {
				t.Fatal("want error")
			}
		})
	}
}
