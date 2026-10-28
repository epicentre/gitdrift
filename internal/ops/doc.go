// Package ops holds every writing operation gitdrift performs on a
// repository. Each operation re-checks the repository state right before it
// writes, so a stale view can never cause a change the user did not review.
package ops
