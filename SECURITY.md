# Security policy

## Supported versions

Only the latest release of gitdrift receives security fixes.

## Reporting a vulnerability

Please report vulnerabilities privately through GitHub's private
vulnerability reporting: open the **Security** tab of
[epicentre/gitdrift](https://github.com/epicentre/gitdrift/security) and choose
**Report a vulnerability**. Do not open a public issue for a security problem.

## What counts as a security issue

Besides the usual classes of vulnerabilities, any bug that could make gitdrift
lose a user's code or work is treated as a security issue. That includes a
writing git command running outside the documented allowlist, a merge that is
not fast-forward only, and a branch deletion that skips its pre-checks.
