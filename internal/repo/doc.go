// Package repo inspects git repositories without changing them: it locates a
// worktree root and reads its status. Every command runs through git.Runner,
// so the package can only read; it never writes to a repository.
package repo
