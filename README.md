# bumpit

`bumpit` is a local-first Go CLI that reads the commits after the last semantic version tag and computes the next version using Conventional Commits and SemVer rules.

## Why

Most auto-versioning tools are wired into CI/CD. `bumpit` keeps the decision local:

- inspect the repo when you want
- see why the version changes
- create the tag locally without pushing automatically

## Commands

```bash
bumpit latest
bumpit next
bumpit explain
bumpit tag
bumpit version
bumpit help
bumpit next --help
```

### `bumpit latest`

Prints the highest semantic version tag in the repository.

Git cannot answer this on its own: `git tag -l` sorts lexicographically, so `v1.9.0` ranks above `v1.10.0`; `git tag --sort=-v:refname` and `sort -V` rank prereleases above their final release; and `git describe --abbrev=0` returns the most recently *created* tag, which loses to any tag added out of order. `bumpit latest` parses every matching tag and orders them by SemVer precedence.

```bash
bumpit latest              # v1.10.0
bumpit latest --no-prefix  # 1.10.0
bumpit latest --all
bumpit latest --match "release-*"
bumpit latest --output json
```

By default only tags reachable from `HEAD` are considered, which is the same set used by `next`, `explain` and `tag`. Pass `--all` to consider every tag in the repository, including tags that live only on other branches. When no matching tag exists the command prints `no tag` and exits with code `0`.

`--no-prefix` prints the bare version instead of the tag, which is what you want when feeding the value into a build:

```bash
go build -ldflags "-X main.version=$(bumpit latest -n)" ./...
```

The flag only changes the text output. The JSON payload always carries both forms:

```json
{
  "tag": "v1.10.0",
  "version": "1.10.0",
  "found": true
}
```

## Flags

Every flag has a short form, and a letter always means the same thing across commands.

| Short | Long | Commands | Description |
| --- | --- | --- | --- |
| `-c` | `--config` | all | path to `bumpit.yaml` |
| `-r` | `--repo` | all | path to the git repository |
| `-t` | `--match` | all | tag pattern used to filter candidate tags |
| `-o` | `--output` | `latest`, `next`, `explain` | output format: `text` or `json` |
| `-a` | `--all` | `latest` | consider every tag, not only the ones reachable from `HEAD` |
| `-n` | `--no-prefix` | `latest` | print the bare version instead of the tag |
| `-s` | `--start-version` | `next`, `explain`, `tag` | version for the first release when no tags exist |
| `-d` | `--allow-dirty` | `next`, `explain`, `tag` | allow a dirty working tree |
| `-p` | `--pre` | `next`, `explain`, `tag` | prerelease label to apply |
| `-P` | `--promote` | `next`, `explain`, `tag` | promote the current prerelease to a final release |
| `-m` | `--message` | `tag` | annotated tag message |

`-t` carries the tag pattern instead of `-m` so that `-m` can keep its usual meaning from `git tag -m` and `git commit -m`. `-p` and `-P` are the mutually exclusive prerelease pair. `-h` is reserved by the CLI for help.

Short flags can be bundled. A flag that takes a value must come last in the bundle, because it consumes the rest of the group:

```bash
bumpit next -dP
bumpit latest -ar /path/to/repo   # -a, then -r takes the path
bumpit latest -ra /path/to/repo   # wrong: -r consumes the literal "a"
```

## Rules

- `BREAKING CHANGE:` footer or `type(scope)!:` header => major
- `feat:` => minor
- `fix:`, `perf:`, `refactor:` => patch
- `docs:`, `test:`, `chore:`, `ci:`, `build:` => no bump

## Examples

```bash
bumpit next --repo /path/to/repo          # bumpit next -r /path/to/repo
bumpit explain --output json              # bumpit explain -o json
bumpit tag --message "Release v1.4.0"     # bumpit tag -m "Release v1.4.0"
bumpit next --config /path/to/bumpit.yaml # bumpit next -c /path/to/bumpit.yaml
bumpit next --pre beta                    # bumpit next -p beta
bumpit next --promote                     # bumpit next -P
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
- the latest tag is always resolved by SemVer precedence, never by tag name or creation date
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
