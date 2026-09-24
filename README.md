# bumpit

`bumpit` is a local-first Go CLI that reads the commits after the last semantic version tag and computes the next version using Conventional Commits and SemVer rules. In a Go repository it versions each module on its own, as the Go modules reference lays them out.

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
bumpit modules
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
bumpit latest --module cmd/tool   # cmd/tool/v0.3.0
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
| `-t` | `--match` | `latest`, `next`, `explain`, `tag` | tag pattern used to filter candidate tags; default `v*`, or the Go module's tag prefix |
| `-g` | `--module` | `latest`, `next`, `explain`, `tag` | Go module directory to version, relative to `--repo` |
| `-o` | `--output` | `latest`, `next`, `explain`, `modules` | output format: `text` or `json` |
| `-a` | `--all` | `latest` | consider every tag, not only the ones reachable from `HEAD` |
| `-n` | `--no-prefix` | `latest` | print the bare version instead of the tag |
| `-s` | `--start-version` | `next`, `explain`, `tag` | version for the first release when no tags exist |
| `-d` | `--allow-dirty` | `next`, `explain`, `tag`, `modules` | allow a dirty working tree |
| `-p` | `--pre` | `next`, `explain`, `tag` | prerelease label to apply |
| `-P` | `--promote` | `next`, `explain`, `tag` | promote the current prerelease to a final release |
| `-V` | `--release-as` | `next`, `explain`, `tag` | release this exact version, the only way to reach `1.0.0` |
| `-m` | `--message` | `tag` | annotated tag message |

`-t` carries the tag pattern instead of `-m` so that `-m` can keep its usual meaning from `git tag -m` and `git commit -m`. `-p` and `-P` are the mutually exclusive prerelease pair, and `-V` excludes both. `-g` sets the tag pattern from the module, so it excludes `-t`. `-h` is reserved by the CLI for help.

Short flags can be bundled. A flag that takes a value must come last in the bundle, because it consumes the rest of the group:

```bash
bumpit next -dP
bumpit latest -ar /path/to/repo   # -a, then -r takes the path
bumpit latest -ra /path/to/repo   # wrong: -r consumes the literal "a"
```

## Rules

bumpit reads each commit as a [Conventional Commit](https://www.conventionalcommits.org/) and releases by [SemVer 2.0.0](https://semver.org/):

- a `BREAKING CHANGE:` or `BREAKING-CHANGE:` footer, or `type(scope)!:` in the header => major
- `feat:` => minor, and so does a `Deprecated:` footer, since SemVer requires a minor release when public API is deprecated
- `fix:`, `perf:`, `refactor:` => patch
- `docs:`, `test:`, `chore:`, `ci:`, `build:` => no bump

A commit that is not conventional, which means a header without `type: description` and the space after the colon, asks for no bump, whatever its body says. Footers are the last paragraph of the message, read the way git reads trailers, so `BREAKING CHANGE:` in the middle of a sentence is not one. `BREAKING CHANGE` must be uppercase; the other tokens are read in any case.

### Major version zero

In `0.y.z` anything may change, and `1.0.0` is the release that declares the public API stable, which no commit can decide:

- a breaking change bumps the minor version: `v0.4.2` becomes `v0.5.0`
- the first release is `v0.1.0`, whatever the commits ask for, unless `--start-version` says otherwise
- `v1.0.0` is reached only with `--release-as 1.0.0`

### Prereleases

A prerelease belongs to its normal version: `v1.1.0-beta.2` is a prerelease of `1.1.0`. The level comes from the commits since the last final release, so commits after a prerelease continue it while they ask for no more than its normal version, and move it once they ask for more:

| Last final release | Current | Commit after it | Next |
| --- | --- | --- | --- |
| `v1.0.0` | `v1.1.0-beta.2` | `fix:` | `v1.1.0-beta.3` |
| `v1.0.0` | `v1.1.0-beta.2` | `feat:` | `v1.1.0-beta.3` |
| `v1.0.0` | `v1.1.0-beta.2` | `feat!:` | `v2.0.0-beta` |

`--pre` with the current label increments it, `--pre` with another label replaces it, and `--promote` releases the normal version.

### What a next version must satisfy

The next version must have higher precedence than the current one and must not be tagged anywhere in the repository, since a released version never changes. bumpit refuses anything else: `--pre alpha` over `v1.1.0-beta.2` is an error, not `v1.1.0-alpha`.

### Tag prefixes

A tag is a prefix followed by a semantic version, and only the version is SemVer: `v1.2.3` itself is not a semantic version. The prefix is the literal part of `--match` before its first wildcard:

| `--match` | Prefix |
| --- | --- |
| `v*` | `v` |
| `release-*` | `release-` |
| `cmd/tool/v*` | `cmd/tool/v` |

A pattern that opens with a wildcard fixes no prefix, and then a tag may carry the customary `v` or none. The version after the prefix is read strictly: `v01.2.3` and `v1.0.0-rc.01` are not versions. `explain` lists such tags as ignored, with the reason, and when every matching tag is one of them bumpit stops instead of planning a first release.

A pattern reads every commit, whatever files it touches. In a Go repository, `--module` is the way to version one module from its own commits.

## Go modules

In a repository with a `go.mod`, bumpit versions one module at a time, following the [Go modules reference](https://go.dev/ref/mod). With neither `--module` nor `--match` it versions the root module, when the repository has a `go.mod` at its top, and `--module <dir>` names any module, relative to `--repo`:

- **Tags.** The tag prefix is the module subdirectory, without the major version suffix, followed by `v`: the root module is tagged `v1.2.3`, the module in `cmd/tool` is tagged `cmd/tool/v1.2.3`, and a module in `sub/v2` whose path is `example.com/repository/sub/v2` is tagged `sub/v2.0.0`.
- **Commits.** Only the commits touching the module's directory count, less the modules nested in it: a commit under `cmd/tool` does not release the root module.
- **Major version.** The next version must fit the module path. A path without a major version suffix takes `v0` and `v1` only, and a path ending in `/vN` takes `vN` only. A breaking change in `v1.x` of `example.com/repository` is an error until the `go.mod` says `module example.com/repository/v2`, and a `go.mod` moved to `/v2` in a commit that does not mark a breaking change is an error too. The go command refuses any other tag of a module that has a `go.mod`, so it would be a version nobody can require.
- **Dependents.** `explain` lists the modules of the repository that require the module at a version below the next one. bumpit reports them and changes no `go.mod`: release the module, then update and release its dependents.

`--match` turns all of this off and reads tags by pattern alone. A `go.mod` under `vendor`, `testdata` or a directory starting with `.` or `_` is not a module of the repository, as for the go command.

The type of a commit applies to every module it touches: a `feat!:` that also edits `cmd/tool/go.mod` is a breaking change for the tool as well, so keep one module per commit.

`bumpit modules` plans every module of the repository, the root one first:

```text
DIRECTORY  MODULE                           CURRENT          NEXT
.          example.com/repository           v0.1.0           v0.2.0
cmd/tool   example.com/repository/cmd/tool  cmd/tool/v0.1.0  no release
```

and `bumpit explain` shows the module and who still requires the old version:

```text
Module: example.com/repository
Current tag: v0.1.0
Base release: v0.1.0
Next version: v0.2.0
...

Dependents requiring an older version:
- cmd/tool (example.com/repository/cmd/tool) requires v0.1.0
```

## Examples

```bash
bumpit next --repo /path/to/repo          # bumpit next -r /path/to/repo
bumpit explain --output json              # bumpit explain -o json
bumpit tag --message "Release v1.4.0"     # bumpit tag -m "Release v1.4.0"
bumpit next --config /path/to/bumpit.yaml # bumpit next -c /path/to/bumpit.yaml
bumpit next --pre beta                    # bumpit next -p beta
bumpit next --promote                     # bumpit next -P
bumpit next --release-as 1.0.0            # bumpit next -V 1.0.0
bumpit next --module cmd/tool             # bumpit next -g cmd/tool
bumpit modules --output json              # bumpit modules -o json
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
tagMatch: ""  # empty: "v*", or the root Go module's tag prefix
startVersion: ""
allowDirty: false
output: text
tagMessage: ""
preRelease: ""
promote: false
```

## Notes

- the default tag pattern is `v*`, or the root Go module's tag prefix in a Go repository
- the latest tag is always resolved by SemVer precedence, never by tag name or creation date
- merge commits are ignored
- the tool creates annotated local tags only
- the tool never pushes tags automatically
- the CLI is built with Cobra
- `bumpit next` returns exit code `0` and prints `no release` when no releasable commits exist
- `--pre` applies an explicit prerelease label such as `beta` or `rc.1`
- `--promote` removes the current prerelease suffix and produces the final release for the same base version
- `--start-version` and `--release-as` take the version with or without the tag prefix
- `explain` reports the base release, the last final release whose commits it reads, and the tags it ignored, and in a Go module the module and its dependents
- the JSON payload of `next` and `explain` carries `base_tag`, `ignored_tags`, `module` and `dependents` beside the fields above

To embed a build version:

```bash
go build -ldflags "-X github.com/pragmabits/bumpit/internal/app.buildVersion=v1.2.3" ./cmd/bumpit
```
