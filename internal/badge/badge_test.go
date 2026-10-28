package badge_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/epicentre/gitdrift/internal/badge"
)

const (
	labelDeps = "deps"
	labelMig  = "mig"
	labelEnv  = "env"
)

func TestMatchDefaults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		files []string
		want  []string
	}{
		{"lock file", []string{"composer.lock"}, []string{labelDeps}},
		{"nested manifest", []string{"packages/api/package.json"}, []string{labelDeps}},
		{"requirements glob", []string{"requirements-dev.txt"}, []string{labelDeps}},
		{"migrations dir", []string{"database/migrations/2026_10_26_add_refunds.php"}, []string{labelMig}},
		{"migrate dir", []string{"db/migrate/20261026_add.rb"}, []string{labelMig}},
		{"alembic versions", []string{"backend/alembic/versions/abc.py"}, []string{labelMig}},
		{"env example", []string{".env.example"}, []string{labelEnv}},
		{"source file", []string{"src/main.go"}, nil},
		{"file named like dir", []string{"migrations.md"}, nil},
		{"alembic without versions", []string{"alembic/env.py"}, nil},
		{
			"rule order",
			[]string{".env.example", "composer.lock", "database/migrations/x.php"},
			[]string{labelDeps, labelMig, labelEnv},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := badge.Match(badge.Defaults(), tt.files)
			if diff := cmp.Diff(tt.want, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("Match mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestParseRule(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in      string
		want    badge.Rule
		wantErr bool
	}{
		{in: "docker=Dockerfile", want: badge.Rule{Label: "docker", Name: "Dockerfile"}},
		{in: "ci=.github/workflows/*.yml", want: badge.Rule{Label: "ci", Path: ".github/workflows/*.yml"}},
		{in: "", wantErr: true},
		{in: "x", wantErr: true},
		{in: "=x", wantErr: true},
		{in: "docker=", wantErr: true},
		{in: "Bad Label=x", wantErr: true},
		{in: "averyveryverylonglabel=x", wantErr: true},
		{in: "bad=[", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()

			got, err := badge.ParseRule(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseRule(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("ParseRule mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestMatchUserRuleAfterDefaults(t *testing.T) {
	t.Parallel()

	user, err := badge.ParseRule("docker=Dockerfile")
	if err != nil {
		t.Fatal(err)
	}

	rules := append(badge.Defaults(), user)

	got := badge.Match(rules, []string{"Dockerfile", "go.mod"})
	if diff := cmp.Diff([]string{labelDeps, "docker"}, got); diff != "" {
		t.Errorf("Match mismatch (-want +got):\n%s", diff)
	}
}
