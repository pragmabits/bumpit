# bumpit

`bumpit` is a local-first Go CLI that reads the commits after the last semantic version tag and computes the next version using Conventional Commits and SemVer rules.

## Why

Most auto-versioning tools are wired into CI/CD. `bumpit` keeps the decision local:

- inspect the repo when you want
- see why the version changes
- create the tag locally without pushing automatically

## Commands

```bash
bumpit next
bumpit explain
bumpit tag
bumpit version
bumpit help
bumpit next --help
```

## Rules

- `BREAKING CHANGE:` footer or `type(scope)!:` header => major
- `feat:` => minor
- `fix:`, `perf:`, `refactor:` => patch
- `docs:`, `test:`, `chore:`, `ci:`, `build:` => no bump

## Examples

```bash
bumpit next --repo /path/to/repo
bumpit explain --output json
bumpit tag --message "Release v1.4.0"
bumpit next --config /path/to/bumpit.yaml
bumpit next --pre beta
bumpit next --promote
```

## Configuration

`bumpit` looks for `bumpit.yaml` or `.bumpit.yaml` in the target repository. You can also pass an explicit path with `--config`.

Precedence is:

- explicit CLI flags
- `bumpit.yaml`
- built-in defaults

Example:

```yaml
repository: .
tagMatch: "v*"
startVersion: ""
allowDirty: false
output: text
tagMessage: ""
preRelease: ""
promote: false
```

## Notes

- default tag lookup pattern is `v*`
- merge commits are ignored
- the tool creates annotated local tags only
- the tool never pushes tags automatically
- the CLI is built with Cobra
- `bumpit next` returns exit code `0` and prints `no release` when no releasable commits exist
- `--pre` applies an explicit prerelease label such as `beta` or `rc.1`
- `--promote` removes the current prerelease suffix and produces the final release for the same base version

To embed a build version:

```bash
go build -ldflags "-X github.com/pragmabits/bumpit/internal/app.buildVersion=v1.2.3" ./cmd/bumpit
```
