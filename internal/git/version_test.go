package git

import "testing"

func TestParseVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in      string
		want    Version
		wantErr bool
	}{
		{"git version 2.54.0 (Apple Git-157)", Version{2, 54, 0}, false},
		{"git version 2.30.1.windows.1", Version{2, 30, 1}, false},
		{"git version 2.45.0.rc1", Version{2, 45, 0}, false},
		{"git version 2.30", Version{2, 30, 0}, false},
		{"nonsense", Version{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()

			got, err := ParseVersion(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}

			if got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVersionAtLeast(t *testing.T) {
	t.Parallel()

	tests := []struct {
		v, o Version
		want bool
	}{
		{Version{2, 30, 0}, MinVersion, true},
		{Version{2, 29, 9}, MinVersion, false},
		{Version{3, 0, 0}, MinVersion, true},
		{Version{2, 30, 1}, MinVersion, true},
	}

	for _, tt := range tests {
		if got := tt.v.AtLeast(tt.o); got != tt.want {
			t.Errorf("%v.AtLeast(%v) = %v, want %v", tt.v, tt.o, got, tt.want)
		}
	}

	if s := (Version{2, 30, 1}).String(); s != "2.30.1" {
		t.Errorf("String() = %q", s)
	}
}

func TestRunnerVersionRealGit(t *testing.T) {
	t.Parallel()

	r, err := NewRunner()
	if err != nil {
		t.Skipf("git not available: %v", err)
	}

	v, err := r.Version(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	if !v.AtLeast(MinVersion) {
		t.Fatalf("git %v older than %v", v, MinVersion)
	}
}
