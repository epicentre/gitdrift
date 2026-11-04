#!/usr/bin/env bash
# Build the demo workspace used for the README screenshots.
#
# Usage: scripts/demo-workspace.sh <dir>
#
# Creates <dir>/remotes (bare repositories) and <dir>/work (clones). Every
# remote is a local path, so the script never touches the network. Git
# configuration is isolated from the user's own, like the gittest helpers.
set -euo pipefail

if [ "$#" -ne 1 ]; then
	echo "usage: $0 <dir>" >&2
	exit 2
fi

target=$1

if [ -e "$target" ] && [ -n "$(ls -A "$target" 2>/dev/null)" ]; then
	echo "refusing to use $target: it exists and is not empty" >&2
	exit 1
fi

mkdir -p "$target"
target=$(cd "$target" && pwd)
remotes=$target/remotes
work=$target/work
scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT
mkdir -p "$remotes" "$work"

# Isolated git configuration with a fixed author.
cat >"$scratch/gitconfig" <<'CFG'
[user]
	name = Demo Dev
	email = demo@example.com
[init]
	defaultBranch = main
[commit]
	gpgsign = false
[tag]
	gpgsign = false
[advice]
	detachedHead = false
[core]
	hooksPath = /dev/null
[merge]
	ff = true
CFG
export GIT_CONFIG_GLOBAL=$scratch/gitconfig
export GIT_CONFIG_NOSYSTEM=1
export GIT_TERMINAL_PROMPT=0

# stamp <when>: turn a git approxidate such as "5 hours ago" into the
# "@<epoch> +0000" form that GIT_AUTHOR_DATE accepts.
stamp() {
	local arg
	arg=$(git rev-parse --after="$1")
	echo "@${arg#--max-age=} +0000"
}

# commit <repo> <when> <message>: commit staged changes at an approxidate.
commit() {
	local ts
	ts=$(stamp "$2")
	GIT_AUTHOR_DATE=$ts GIT_COMMITTER_DATE=$ts git -C "$1" commit -q -m "$3"
}

# put <repo> <file> <content>: write a file inside the repository.
put() {
	mkdir -p "$(dirname "$1/$2")"
	printf '%s\n' "$3" >"$1/$2"
	git -C "$1" add -- "$2"
}

# save <repo> <when> <message> <file> <content>: write, stage and commit.
save() {
	put "$1" "$4" "$5"
	commit "$1" "$2" "$3"
}

# bulk <repo> <count> <label> <when>: <count> small "chore: <label> N" commits,
# each adding a tiny Go file, all stamped with <when>.
bulk() {
	local i slug=${3// /_}
	for ((i = 1; i <= $2; i++)); do
		save "$1" "$4" "chore: $3 $i" "internal/changes/${slug}_$i.go" \
			"package changes

// Marker$i records the \"$3\" change $i.
const Marker$i = $i"
	done
}

# refund_go <rounding-expression> [extra-function]: cmd/payments/refund.go.
refund_go() {
	cat <<GO
package main

// Refund returns the amount to give back, in cents, rounded to five.
func Refund(amount int) int {
	return $1
}
${2:-}
GO
}

# partial_refund_go: the PartialRefund function added next to Refund.
partial_refund_go() {
	cat <<'GO'

// PartialRefund returns share percent of amount, refunded like Refund.
func PartialRefund(amount, share int) int {
	return Refund(amount * share / 100)
}
GO
}

# total_go <body>: cmd/payments/total.go.
total_go() {
	cat <<GO
package main

// RoundTotal rounds a total in cents to the nearest five.
func RoundTotal(cents int) int {
$1
}
GO
}

# bump_deps <repo> <when> <version>: "chore: update dependencies" touching
# go.mod and go.sum.
bump_deps() {
	printf 'module example.com/%s\n\ngo 1.24\n\nrequire github.com/go-chi/chi/v5 %s\n' \
		"$(basename "${1%/}")" "$3" >"$1/go.mod"
	printf 'github.com/go-chi/chi/v5 %s h1:3b1f0d9c7a5e\ngithub.com/go-chi/chi/v5 %s/go.mod h1:9a2c4e6f8b0d\n' \
		"$3" "$3" >"$1/go.sum"
	git -C "$1" add -- go.mod go.sum
	commit "$1" "$2" "chore: update dependencies"
}

# repo_dir <name>: where the work clone of a repository lives. The demo
# workspace nests most repositories in folders, one directly in the root.
repo_dir() {
	case $1 in
	payments | billing | ledger | search | auth) echo "$work/services/$1" ;;
	api-gateway | notifications | mobile-bff) echo "$work/services/platform/$1" ;;
	web-app | design-system) echo "$work/web/$1" ;;
	infra) echo "$work/tools/$1" ;;
	*) echo "$work/$1" ;;
	esac
}

# new_repo <name> <age>: a bare remote plus a work clone on main with a few
# base commits pushed to it. Prints nothing.
new_repo() {
	local name=$1 age=$2 dir
	dir=$(repo_dir "$name")
	mkdir -p "$(dirname "$dir")"
	git init -q --bare "$remotes/$name.git"
	git init -q "$dir"
	git -C "$dir" remote add origin "$remotes/$name.git"
	save "$dir" "$age" "chore: initial commit" README.md "# $name"
	put "$dir" go.mod "module example.com/$name

go 1.24"
	save "$dir" "$age" "chore: add project layout" "cmd/$name/main.go" \
		"package main

func main() {}"
	save "$dir" "$age" "ci: add continuous integration" ci/pipeline.txt "ci"
	git -C "$dir" push -q -u origin main
	git -C "$dir" remote set-head origin main >/dev/null
}

# pusher <name> <author>: clone the remote into a scratch directory so other
# people's work can be pushed, committing as the teammate <author>; prints the
# clone path. Work made on this machine stays with the fixed author.
pusher() {
	local p=$scratch/pusher-$1 first=${2%% *}
	git clone -q "$remotes/$1.git" "$p" 2>/dev/null
	git -C "$p" config user.name "$2"
	git -C "$p" config user.email "$(printf '%s' "$first" | tr '[:upper:]' '[:lower:]')@example.com"
	echo "$p"
}

# ---------------------------------------------------------------- payments
# Feature branch ahead 3 and behind 12 of its upstream, 27 behind its base,
# local changes, a stash, two gone branches (one squash-merged and carrying
# 2 local-only commits) and incoming dependency and migration files.
p=$(repo_dir payments)
new_repo payments "60 days ago"
save "$p" "45 days ago" "feat: add refund model" internal/refund/model.go \
	"package refund

// Refund is a refund issued against a payment.
type Refund struct {
	PaymentID string
	Amount    int
}"
git -C "$p" push -q origin main

git -C "$p" switch -q -c feature/cash-rounding
save "$p" "20 days ago" "feat: round cash totals to five cents" cmd/payments/total.go \
	"$(total_go '	return (cents + 2) / 5 * 5')"
git -C "$p" push -q -u origin feature/cash-rounding

git -C "$p" switch -q -c feature/qr-login main
save "$p" "18 days ago" "feat: add QR code login" internal/auth/qrlogin.go \
	"package auth

// QRLogin starts a login that is confirmed by scanning a QR code.
func QRLogin(user string) string {
	return \"qr:\" + user
}"
git -C "$p" push -q -u origin feature/qr-login
git -C "$p" switch -q main

# The checkout work builds on the rounding commit, so that commit stays
# reachable from the feature branch upstream after the rounding branch is gone.
git -C "$p" switch -q -c feature/checkout feature/cash-rounding
save "$p" "9 days ago" "feat: start checkout flow" cmd/payments/refund.go \
	"$(refund_go 'amount / 5 * 5')"
save "$p" "8 days ago" "feat: add checkout totals" cmd/payments/summary.go \
	"package main

// Summary is the total shown at the end of the checkout.
type Summary struct {
	Items int
	Total int
}"
git -C "$p" push -q -u origin feature/checkout

# Upstream work on the feature branch, oldest first: one test per commit
# appended to the refund tests, then the refund fixes and the dependency bump.
pp=$(pusher payments "Dana Lee")
git -C "$pp" switch -q feature/checkout
for i in 1 2 3 4 5 6 7 8; do
	f=$pp/cmd/payments/refund_test.go
	if [ "$i" -eq 1 ]; then
		printf 'package main\n\nimport "testing"\n' >"$f"
	fi
	printf '\nfunc TestRefundCase%s(t *testing.T) {\n\tif got := Refund(%s0); got != %s0 {\n\t\tt.Fatalf("Refund(%s0) = %%d", got)\n\t}\n}\n' \
		"$i" "$i" "$i" "$i" >>"$f"
	git -C "$pp" add -- cmd/payments/refund_test.go
	commit "$pp" "$((8 - (i + 1) / 2)) days ago" "test: add refund case $i"
done
save "$pp" "3 days ago" "fix: refund rounding" cmd/payments/refund.go \
	"$(refund_go '(amount + 2) / 5 * 5')"
save "$pp" "3 days ago" "feat: partial refunds" cmd/payments/refund.go \
	"$(refund_go '(amount + 2) / 5 * 5' "$(partial_refund_go)")"
bump_deps "$pp" "2 days ago" v5.1.0
save "$pp" "1 day ago" "feat: add refunds table" migrations/0007_add_refunds_table.sql \
	"CREATE TABLE refunds (
	id         BIGSERIAL PRIMARY KEY,
	payment_id TEXT NOT NULL,
	amount     INTEGER NOT NULL
);"
git -C "$pp" push -q origin feature/checkout

git -C "$pp" switch -q main
bulk "$pp" 12 "sync config" "6 days ago"
git -C "$pp" fetch -q origin feature/qr-login
ts=$(stamp "5 days ago")
GIT_AUTHOR_DATE=$ts GIT_COMMITTER_DATE=$ts \
	git -C "$pp" merge -q --no-ff -m "Merge feature/qr-login" origin/feature/qr-login
git -C "$pp" fetch -q origin feature/cash-rounding
git -C "$pp" merge -q --squash origin/feature/cash-rounding >/dev/null 2>&1
commit "$pp" "4 days ago" "feat: round cash totals (#41)"
bulk "$pp" 12 "tune settings" "4 hours ago"
git -C "$pp" push -q origin main
git -C "$pp" push -q origin --delete feature/cash-rounding feature/qr-login

# Two commits that exist only on this machine, on the squash-merged branch.
git -C "$p" switch -q feature/cash-rounding
save "$p" "7 hours ago" "fix: handle negative totals" cmd/payments/total.go \
	"$(total_go '	if cents < 0 {
		return -RoundTotal(-cents)
	}
	return (cents + 2) / 5 * 5')"
save "$p" "5 hours ago" "test: add rounding tests" cmd/payments/total_test.go \
	"package main

import \"testing\"

func TestRoundTotal(t *testing.T) {
	if got := RoundTotal(-12); got != -10 {
		t.Fatalf(\"RoundTotal(-12) = %d\", got)
	}
}"
git -C "$p" switch -q feature/checkout

# Three local commits on the feature branch, then a stash and local changes.
save "$p" "2 days ago" "feat: add receipt view" web/templates/receipt.html "<h1>Receipt</h1>"
save "$p" "1 day ago" "fix: receipt currency format" web/templates/receipt.html \
	"<h1>Receipt</h1>
<p>{{ printf \"%.2f\" .Total }}</p>"
save "$p" "3 hours ago" "refactor: extract receipt totals" cmd/payments/receipt.go \
	"package main

// ReceiptTotal is the amount printed on a receipt.
func ReceiptTotal(s Summary) int {
	return RoundTotal(s.Total)
}"
printf '%s\n' "<!-- wip: receipt layout -->" >>"$p/web/templates/receipt.html"
ts=$(stamp "5 days ago")
GIT_AUTHOR_DATE=$ts GIT_COMMITTER_DATE=$ts git -C "$p" stash push -q -m "wip: receipt layout"
printf '%s\n' "wip" >>"$p/README.md"
printf '%s\n' "scratch notes" >"$p/notes.txt"
printf '%s\n' "// TODO: receipt" >>"$p/cmd/payments/refund.go"
git -C "$p" fetch -q --prune origin

# ----------------------------------------------------------------- billing
# Feature branch, up to date with its upstream, 27 behind its base.
p=$(repo_dir billing)
new_repo billing "50 days ago"
git -C "$p" switch -q -c feature/invoice-export
save "$p" "5 days ago" "feat: add invoice export" internal/invoice/export.go \
	"package invoice

// Export renders an invoice for download.
func Export(id string) string {
	return \"invoice-\" + id
}"
save "$p" "4 days ago" "feat: add CSV columns" internal/invoice/csv.go \
	"package invoice

// Columns are the CSV columns of an invoice export.
var Columns = []string{\"id\", \"customer\", \"total\"}"
git -C "$p" push -q -u origin feature/invoice-export
pp=$(pusher billing "Sam Park")
bulk "$pp" 27 "update invoice templates" "2 days ago"
git -C "$pp" push -q origin main
git -C "$p" fetch -q --prune origin

# -------------------------------------------------------------- api-gateway
# On main, behind 12, with an incoming dependency bump.
p=$(repo_dir api-gateway)
new_repo api-gateway "40 days ago"
pp=$(pusher api-gateway "Alex Morgan")
for r in users orders invoices payments search health ready metrics auth sessions webhooks; do
	cap="$(printf %s "${r:0:1}" | tr a-z A-Z)${r:1}"
	save "$pp" "2 days ago" "feat: add $r route" "internal/routes/$r.go" \
		"package routes

// $cap is the path prefix of the $r API.
const $cap = \"/$r\""
done
bump_deps "$pp" "5 hours ago" v5.2.0
git -C "$pp" push -q origin main
git -C "$p" fetch -q --prune origin

# ----------------------------------------------------------- notifications
# On main, behind 4, with an incoming migration and an env example.
p=$(repo_dir notifications)
new_repo notifications "35 days ago"
pp=$(pusher notifications "Robin Hale")
digest_go() {
	printf 'package mail\n\n// DigestSubject is the subject line of the daily digest.\nfunc DigestSubject() string {\n\treturn "%s"\n}\n' "$1"
}
save "$pp" "3 days ago" "feat: add digest mail" internal/mail/digest.go "$(digest_go 'Your digest')"
save "$pp" "2 days ago" "feat: add digests table" migrations/0003_create_digests_table.sql \
	"CREATE TABLE digests (
	id      BIGSERIAL PRIMARY KEY,
	user_id BIGINT NOT NULL,
	sent_at TIMESTAMPTZ
);"
save "$pp" "1 day ago" "docs: document digest settings" .env.example "DIGEST_HOUR=8"
save "$pp" "6 hours ago" "fix: digest subject" internal/mail/digest.go "$(digest_go 'Your daily digest')"
git -C "$pp" push -q origin main
git -C "$p" fetch -q --prune origin

# -------------------------------------------------------------------- auth
# Up to date, one commit that is not pushed.
p=$(repo_dir auth)
new_repo auth "30 days ago"
save "$p" "2 hours ago" "feat: add session timeout" internal/session/timeout.go \
	"package session

import \"time\"

// Timeout is how long an idle session stays valid.
const Timeout = 30 * time.Minute"

# ------------------------------------------------------------------- infra
# Up to date, three branches deleted on the remote after being merged.
p=$(repo_dir infra)
new_repo infra "55 days ago"
day=14
for b in dns-cleanup cache-tuning log-retention; do
	git -C "$p" switch -q -c "chore/$b"
	save "$p" "$day days ago" "chore: apply $b" "internal/ops/${b//-/_}.go" \
		"package ops

// Applied records that $b was applied.
const Applied = true"
	day=$((day - 1))
	git -C "$p" push -q -u origin "chore/$b"
	git -C "$p" switch -q main
	git -C "$p" merge -q --ff-only "chore/$b"
	git -C "$p" push -q origin main
	git -C "$p" push -q origin --delete "chore/$b"
done
git -C "$p" fetch -q --prune origin

# ------------------------------------------------------- up-to-date repos
age=10
for name in docs ledger search web-app mobile-bff design-system; do
	p=$(repo_dir "$name")
	new_repo "$name" "$((age + 20)) days ago"
	save "$p" "$age days ago" "docs: update $name notes" notes/update.txt "$name"
	git -C "$p" push -q origin main
	age=$((age + 3))
done

echo "demo workspace ready: $work"
