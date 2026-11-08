# bumpit

`bumpit` is a local-first Go CLI that reads the commits after the last semantic version tag and computes the next version using Conventional Commits and SemVer rules. In a Go repository it versions each module on its own, as the Go modules reference lays them out, and a part of the repository with no `go.mod`, such as a static site or a Helm chart, is versioned the same way once `bumpit.yaml` declares it.

## Why

Most auto-versioning tools are wired into CI/CD. `bumpit` keeps the decision local:

- inspect the repo when you want
- see why the version changes
- create the tag locally without pushing automatically

## Install

```bash
go install github.com/pragmabits/bumpit@latest
```

bumpit needs Go 1.26.2 or newer, the version its `go.mod` declares. Up to `v0.3.0` the command lived in `cmd/bumpit`, so those versions install with `go install github.com/pragmabits/bumpit/cmd/bumpit@v0.3.0`.

## Commands

```bash
bumpit latest
bumpit next
bumpit explain
bumpit tag
bumpit modules
bumpit modules add <directory>...
bumpit modules set|remove <module>
bumpit modules check
bumpit config get|set|unset <key>
bumpit init
bumpit version     # or bumpit --version
bumpit completion  # bash, zsh, fish or powershell
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
| `-c` | `--config` | `latest`, `next`, `explain`, `tag`, `modules`, `modules add`, `modules set`, `modules remove`, `config get`, `config set`, `config unset` | path to `bumpit.yaml` |
| `-r` | `--repo` | `latest`, `next`, `explain`, `tag`, `modules`, `modules add`, `modules set`, `modules remove`, `modules check`, `config get`, `config set`, `config unset`, `init` | path to the git repository |
| `-t` | `--match` | `latest`, `next`, `explain`, `tag` | tag pattern used to filter candidate tags; default `v*`, or the module's tag prefix |
| `-g` | `--module` | `latest`, `next`, `explain`, `tag` | name of the module to version, or a directory of it relative to `--repo`: a Go module, or one `bumpit.yaml` declares |
| `-o` | `--output` | `latest`, `next`, `explain`, `modules`, `modules check`, `init` | output format: `text` or `json` |
| `-a` | `--all` | `latest` | consider every tag, not only the ones reachable from `HEAD` |
| `-n` | `--no-prefix` | `latest` | print the bare version instead of the tag |
| `-s` | `--start-version` | `next`, `explain`, `tag` | version for the first release when no tags exist |
| `-d` | `--allow-dirty` | `next`, `explain`, `tag`, `modules` | allow a dirty working tree |
| `-p` | `--pre` | `next`, `explain`, `tag` | prerelease label to apply |
| `-P` | `--promote` | `next`, `explain`, `tag` | promote the current prerelease to a final release |
| `-V` | `--release-as` | `next`, `explain`, `tag` | release this exact version, the only way to reach `1.0.0` |
| `-m` | `--message` | `tag` | annotated tag message |
| `-M` | `--all-modules` | `tag` | tag every module that has a release, Go modules and the ones `bumpit.yaml` declares, holding the Go modules that require an older version of another module |
| `-e` | `--name` | `modules add`, `modules set` | what `--module`, `shippedBy` and the output call the module; several directories need one |
| `-D` | `--directory` | `modules set` | a directory the module owns, relative to `--repo`, in place of those it has; repeat it for each |
| `-T` | `--tag-prefix` | `modules add`, `modules set` | what every tag of the declared module starts with, its `v` included |
| `-f` | `--path` | `modules add`, `modules set` | a file or directory outside the module that it is built from, relative to `--repo`; repeat it for each |
| `-N` | `--no-paths` | `modules set` | leave out every path the declaration lists |
| `-b` | `--shipped-by` | `modules add`, `modules set` | the name, or the directory relative to `--repo`, of a module whose release also carries this one; repeat it for each |
| `-B` | `--shipped-by-none` | `modules set` | say that no other module's release carries this one |
| `-v` | `--version` | `bumpit` itself | print the version, as `bumpit version` does |

`-t` carries the tag pattern instead of `-m` so that `-m` can keep its usual meaning from `git tag -m` and `git commit -m`. `-p` and `-P` are the mutually exclusive prerelease pair, and `-V` excludes both. `-g` sets the tag pattern from the module, so it excludes `-t`. `-M` tags every module at its own next version, so it excludes `-g`, `-t`, `-V`, `-p` and `-P`. `-a` is not used for it because `latest -a` already means every tag, and `git tag -a` means an annotated tag. `-b` and `-B` exclude each other, and so do `-f` and `-N`; a switch given as `=false`, such as `--no-paths=false`, asks for nothing. `-h` is reserved by the CLI for help.

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

A pattern that starts with `-` is refused, on the command line and as `tagMatch`: git would read it as an option, and creates no tag whose name starts with `-`. A pattern reads every commit, whatever files it touches. `--module` is the way to version one module from its own commits: a Go module, or one `bumpit.yaml` declares.

## Go modules

In a repository with a `go.mod`, bumpit versions one module at a time, following the [Go modules reference](https://go.dev/ref/mod). With neither `--module` nor `--match` it versions the root module, when the repository has a `go.mod` at its top, and `--module` names any module, by the name `bumpit.yaml` gives it or by its directory, relative to `--repo`:

- **Tags.** The tag prefix is the module subdirectory, without the major version suffix, followed by `v`: the root module is tagged `v1.2.3`, the module in `cmd/tool` is tagged `cmd/tool/v1.2.3`, and a module in `sub/v2` whose path is `example.com/repository/sub/v2` is tagged `sub/v2.0.0`. A tag that is the prefix of another module followed by a version is that module's: the root module's `v*` also matches `vite/v0.1.0`, the version of a module in `vite`, and does not read it, neither as a version nor as an ignored tag. A directory git takes in no tag name, as one with a glob character or a space or one that starts with `-`, cannot be versioned until it takes another name: a plan of the module stops, and the other modules go on, though `bumpit tag --all-modules` creates no tag while a plan fails.
- **Commits.** Only the commits touching the module's directory count, less the modules nested in it, Go modules and the ones `bumpit.yaml` declares: a commit under `cmd/tool` does not release the root module.
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

With `--output json` each module carries `directory`, `path`, `current_tag`, `next_tag` and `has_release`, `hold` when its release is held, as `bumpit tag --all-modules` holds it, and `error` in place of the next tag when its plan failed, so one module that cannot be planned does not hide the others.

`bumpit explain` shows the module and who still requires the old version:

```text
Module: example.com/repository
Current tag: v0.1.0
Base release: v0.1.0
Next version: v0.2.0
...

Dependents requiring an older version:
- cmd/tool (example.com/repository/cmd/tool) requires v0.1.0
```

`bumpit tag --all-modules` tags every module that has a release in one run, each at its own next version:

```text
.         created tag v0.2.0
api       no release
cmd/tool  held: requires example.com/repository v0.1.0, below v0.2.0; update cmd/tool/go.mod, commit and rerun
```

- **Planned first.** Every module is planned before any tag is created. When a plan fails, no tag is created and every failure is listed.
- **Held releases.** A module that requires another module of the repository below its newest version, the next one when it is released in the same run and its current tag otherwise, is held: tagged now, its release would require the older version while its code may need the newer one. That is the order dependents call for: release the module, then update and release its dependents. Rerunning after the `go.mod` update tags it, and rerunning without one holds it again. A module that waits on another held module says so, and modules whose requirements form a cycle are released one at a time with `--module`, which holds nothing. A placeholder requirement beside a `replace` directive is held as well, since the go command cannot find it once the replace is ignored.
- **Exit code.** The command fails when it creates no tag, as `bumpit tag` does.
- **Flags.** `--module`, `--match`, `--release-as`, `--pre` and `--promote` pick one module or one version, so they cannot be combined with it, whether given on the command line or set in `bumpit.yaml`. `--start-version` applies to every module without tags and must be the bare version, such as `0.5.0`, since each module has its own prefix. `--message` and `tagMessage` go on every tag; without them each tag gets `Release <tag>`.

## Modules without a go.mod

A repository often ships parts that are released on their own but are not Go modules: a static site built into an image of its own, a frontend package, a Helm chart. `bumpit.yaml` declares them under `modules`, and bumpit versions each one as it versions a Go module, from its own tags and its own commits: `--module` names it, `bumpit modules` lists it, and `bumpit tag --all-modules` tags it at its own next version.

```yaml
modules:
  - name: homepage               # default: its directory, the only one it has
    directories:                 # what it owns, relative to the top of the repository
      - web/homepage
      - deploy/homepage
    tagPrefix: homepage/v        # default: the name or the directory followed by /v, or v for .
    paths:                       # what it is built from and does not own
      - web/brand
    shippedBy: []                # required: the modules whose release also carries it
```

- **Name.** What `--module`, `shippedBy` and the output call the module. It is optional with one directory, which then names the module, and several directories need one, since none of them says what the module is. A name is any text that cannot be read as another path or as a flag: it is not `.`, does not start with `-`, is clean, stays inside the repository and holds no glob character, and it cannot be the directory of another module, nor lead to one through a link the repository tracks, since `--module` reads it from the top of the repository as a path too. A link only one working tree holds leaves the file as it is in every clone: there, `--module` refuses the name read from the top, and the directory still calls the module. `--module`, `modules set` and `modules remove` take the name or any of the directories, relative to `--repo`. A name that from `--repo` is also the directory of another module is refused, and `./name` says the directory.
- **Tags.** Every tag of the module starts with `tagPrefix`, its `v` included: `homepage/v0.1.0`. Without one, the prefix is the name followed by `/v`, or else the directory, as for a Go module in that directory.
- **Commits.** A file belongs to the deepest module whose directory holds it, Go or declared, and a module reads the commits that touch its own files, in any of its directories. The root module therefore no longer reads `web/homepage` or `deploy/homepage`, as it does not read a nested Go module. A directory belongs to one module, and one that lies in another directory of the same module is refused. A declared module also reads the commits of each of its `paths`, whoever owns them: a `fix:` in `web/brand` releases the root module, which owns it, and the homepage, which is built from it. A path may name a module nested in the directory, which is then read too. The `.go` files of a Go module that lie in a declared directory stay the Go module's code: their commits are its own, at their own level, whatever `shippedBy` says. The files no build compiles are left out of that: those under `testdata` or `vendor`, those whose name or whose directory's name starts with `_` or a dot, and those whose build constraint needs the `ignore` tag. `explain` names the declared directories whose Go files a module reads, as `Reads the Go files in`. Each directory goes by the Go module around it, which may not be the same for two directories of a module.
- **Node packages.** A declared module with a `package.json` in one of its directories also reads, without listing them, what its install reads outside its directory. The packages that the `package.json` files in its directory depend on are read, and those they depend on in turn: by path, written bare or after `workspace:`, `file:`, `link:` or `portal:`, and by name otherwise, the name being that of the dependency or of the alias after `workspace:` or `npm:`. A name is looked up among every `package.json` of the repository, so a package outside the workspace that bears it is read too, and a dependency on the module or on one of its ancestors is left out. So are the overrides of npm, pnpm and yarn, those of the module itself when it is a workspace root and those of the roots above it, that point at a package of the repository. The files of every workspace root above it are read as well, the roots being the directories with a `pnpm-workspace.yaml` or a `package.json` that has `workspaces`: their `package.json`, `pnpm-workspace.yaml`, lockfiles, `.npmrc`, `.yarnrc`, `.yarnrc.yml`, `.pnpmfile.cjs`, `.pnpmfile.mjs`, `bunfig.toml`, `.yarn` and `patches`, and every `.yarnrc.yml` above. This works the same for pnpm, npm, yarn and bun. bumpit never sets the version a `package.json` carries, and does not see the files a build script reads on its own. A `package.json` the module, a root above it or a path it follows needs that cannot be read stops the plans that read the module.
- **Shipped modules.** A module that `shippedBy` names carries the declared module in its release, so it reads every commit the declared module reads, and those of the modules that one carries in turn. `shippedBy` calls a module by its name, or by its directory when it has none, and calls each once. A breaking change that reaches it only that way is a minor change for it: the contract it breaks is the carried module's, and a breaking change of the carrier is marked in a commit of its own. A cycle, a module that through its carriers carries itself, is refused.
- **No major version check, no holds.** Nothing reads a declared module by a module path, so it takes any version, and no `go.mod` requires it.

`shippedBy` is required of a module with no `go.mod` because bumpit cannot always tell whether a module carries another. Where it can, it checks: a Go module ships a declared module when one of its `//go:embed` patterns reaches the files of any of its directories. bumpit reads the patterns whatever the build constraints of a file and whether cgo is enabled or not, and leaves out the files no build compiles: test files, files whose constraint needs the `ignore` tag, as `//go:build ignore` does, and files the go command cannot read, such as a generator's template. A pattern that names a directory does not reach a Go module nested in it, which the go command never embeds, nor, without the `all:` prefix, what lies below it under a name that starts with `.` or `_`. bumpit then refuses the declaration unless `shippedBy` names that Go module:

```text
modules: web/homepage: site.go:5:12 embeds web/homepage/dist: the module . then ships it; add . to its shippedBy
```

What bumpit cannot check is left to the declaration: a module that carries another through a Dockerfile or a script, and a `paths` list that misses a file the module is built from, whose commits then release nothing.

bumpit also refuses a path that names no tracked file, as a misspelt one would; a path inside the module's own directory, which it reads already; a carrier that is no module of the repository, and a cycle of carriers; and a tag prefix that cannot tell the module's tags from another module's: the same prefix, or one that continues the other with what can start a version, as `v1` continues `v` and would make `v10.0.0` a version of both. A tag prefix that starts with `-` is refused too, since git would take the tag for an option.

A Go module can be listed too, with an entry whose directory holds its `go.mod`. It needs none: a `go.mod` no entry lists is a module all the same, so a repository with no `bumpit.yaml` works as ever. The entry lists it and can give it a name, and takes nothing else, since a Go module has one directory and its `go.mod` says the rest: `tagPrefix`, `paths` and `shippedBy` are refused there, and a name leaves its tags as the go command reads them. An entry whose `go.mod` is gone is a declared module with no `shippedBy`, which is refused: `bumpit modules remove` takes it out.

```yaml
modules:
  - directories: [.]
  - name: stack
    directories: [cmd/stack]
```

`explain` shows what a declared module reads, who carries it, and the path or module each commit came in through:

```text
Module: web/homepage (declared in bumpit.yaml)
Paths: deploy/homepage
Paths from the Node packages: web/brand, web/package.json, web/pnpm-lock.yaml, web/pnpm-workspace.yaml
Current tag: homepage/v0.1.0
...
- 3f2a1c9 fix: sharpen the logo [patch: fix commit] via web/brand
```

The module around it says `Leaves out: web/homepage`, or `Ships: web/homepage` when `shippedBy` names it. A module with a name of its own also says `Directories`, and a Go module with one says `Name`. In JSON, `bumpit modules`, `next` and `explain` give a declared module `name` and `directories` in place of the `path` and `directory` of a Go module, and a Go module `name` when it has one, and `next` and `explain` also carry `paths`, `derived_paths`, `shipped_by`, `go_files_in`, `ships`, `leaves_out` and, on each commit, `via`.

The declarations describe the repository, as its `go.mod` files do, so bumpit reads them from the `bumpit.yaml` at the top of the repository, wherever `--repo` points: a run from a subdirectory reads them too, while its other settings still come from the config found in `--repo`. A config given with `--config` that declares `modules`, an empty list included, takes their place; one that does not leaves them to the top. A config found in a subdirectory cannot list modules of its own, though it may say `modules: []`. Of the top `bumpit.yaml`, a run from a subdirectory reads the `modules` key alone, so that its other keys, which are not the run's, cannot stop it; modules it cannot read stop the run, since they would be lost.

### Declaring modules from the command line

`bumpit modules add`, `set` and `remove` edit the declarations, so `bumpit.yaml` need not be written by hand:

```bash
bumpit modules add web/homepage deploy/homepage -e homepage          # declare it
bumpit modules set homepage -b .                                     # . ships it now
bumpit modules set homepage -f web/brand                             # it is built from web/brand
bumpit modules set homepage -e site                                  # another name, the same tags
bumpit modules add cmd/stack -e stack                                # name a Go module
bumpit modules remove site
```

- **Which file.** The edit goes where the runs read the declarations from: the config `--config` names when it declares `modules`, else the `bumpit.yaml` or `.bumpit.yaml` at the top of the repository, created when there is none. A `--config` that declares no `modules` is refused, since the runs read them from the top, unless it is the top config itself.
- **Paths.** The directories, `--path` and `--shipped-by` are relative to `--repo`, as `--module` is, or absolute, and are written relative to the top of the repository. An empty one is refused, since it names nothing, and `.` says the directory `--repo` gives. `--shipped-by` also takes a name, and writes what `shippedBy` calls the module.
- **add** refuses a directory declared already. On a directory with no `go.mod`, without `--shipped-by` it writes `shippedBy: []`, which says no module carries the module, and prints what that rests on: which modules around it stop reading it, and that bumpit sees only `//go:embed`. On a directory that holds a `go.mod` it writes an entry with no `shippedBy`. **set** changes what its flags give and leaves the rest: `--tag-prefix ""` returns to the default prefix, `--name ""` drops the name, and `--no-paths` and `--shipped-by-none` empty their lists. `--directory` replaces the directories, and is refused on a Go module, whose directory is where its `go.mod` is. **remove** takes the comments inside the declaration with it; removing the last one leaves `modules: []`.
- **The rest of the file stays as it is**, comments, blank lines, indentation and line endings included: a new declaration goes after the last one, in its indentation, with its keys in the order `name`, `directories`, `tagPrefix`, `paths`, `shippedBy`. `set` writes the declaration anew, so it refuses one that holds a comment. A file the edit cannot change line by line is refused: several YAML documents, a root that is no block mapping, `modules` written as a list in `[...]` that has items or as an empty value on a line of its own, a block scalar (`|` or `>`), an anchor or an alias in the modules, and a key written twice, since keys are compared without case.
- **A module keeps its tags and its carriers.** A new name or a move that would change the tag prefix of a module with no `go.mod` writes the old prefix in `tagPrefix`. A move out of `.` needs `--tag-prefix`, since `v` is the prefix of the module in `.` alone, and leaves the tags of that prefix behind. Any edit that changes what a module is called, `add` and `remove` on a named Go module included, changes it in the `shippedBy` of the modules that call it, and only that key of those entries, whose comments stay; a `shippedBy` written over several lines is refused, naming the entry to edit by hand. A name another module also answers to, as a name that is the directory of another module does, stays in `shippedBy`, since the file cannot say which of the two it meant, and the command says which entries keep it.
- **Proof before writing.** The file is written only once its new text loads as the config it was with the edit applied, and says nothing wrong that it did not say already, with every check a run makes: what the edit is about gains no problem of any kind, even one another module has too, and nothing else gains one more. A file with several problems can thus be repaired one at a time. A module whose text does not hold is compared with the repository like any other, so that repairing it answers only for what the edit adds, and the next `bumpit modules check` says what remains; only a module with no directory is left out, since it is nowhere in the repository. The file is then written at once, by renaming the new text onto it, keeping its mode and, when it is a link, the link. When the proof fails the file is left as it was, and so it is when git fails during the proof, which then says that the edit could not be checked, not that it is refused.

### Checking the modules

`bumpit modules check` reads the config a run reads, as a run reads it, and lists all a run would stop at, where a run stops at the first, with what is wrong, why, how to resolve it, and the command that does when that takes no choice:

```text
blocks the runs:
  web/homepage: site.go:5:12 embeds web/homepage/dist
    why: the module . then ships it
    how: add . to its shippedBy
    run: bumpit modules set web/homepage -b .
  docs: tag prefix site/v is taken by another module
    why: it is the one of admin too, so their tags could not be told apart
    how: give one of them another prefix, which leaves its tags behind
to review:
  cmd/stack: go.mod with no entry in bumpit.yaml
    why: it is versioned all the same, but the file does not list it
    how: list it, as bumpit init also does
    run: bumpit modules add cmd/stack
  release-: 3 tags no module owns, the newest release-1.2.0
    why: they may be the releases of a module not declared yet
    how: declare its module with bumpit modules add and --tag-prefix
not verified that no module ships homepage: bumpit sees only //go:embed: check that no Dockerfile, CI job or other build copies it into the release of another module
1 problem blocks the runs
```

- **Blocks the runs.** The check fails when there is any. It goes in two stages: the file first, its keys and the text of its modules, and then, once the file holds, what the repository contradicts of the modules, among them a `package.json` a plan could not read and a Go module in a directory git takes in no tag name. A file that does not hold stops it before the repository, since nothing could be said of modules that cannot be read. A file that cannot be read at all, such as a key no version of bumpit takes, is one problem, which says what the file takes.
- **To review.** What stops no run and may need declaring: the Go modules no entry lists, the tags no module owns and the manifests with a version of their own, as `bumpit init` finds them, and a `Chart.yaml` it cannot read. The tags a run reads as its own, by the `tagMatch` of the config at the top or of the config it loads, are no tags to declare. A Go module in a directory git takes in no tag name, as one with a glob character, is listed only under what blocks the runs: the new name for the directory that item asks for comes before an entry can list it. A tag line that names the directory of a declared module with no tags of its own, as one left behind when a `go.mod` goes and a name gives the module another prefix, comes with the `bumpit modules set -T` that gives it to that module, or says why that command would be refused. It does not fail for any of these. A proposal also shows the `//go:embed` patterns behind its `-b` and the directories it left out, as `bumpit init` does.
- **Not verified.** A module that says no module ships it rests on what bumpit cannot see. A clean check does not prove that no Dockerfile, CI job or script carries it.
- **Commands.** Each `run` is the literal command, for where the check ran, with its `--repo` and its `--config` when the command needs them. bumpit runs it without writing before it prints it, over what the commands before it would have written, and prints why it would be refused in its place, so it is safe to follow them in the order printed. There is none when resolving takes a choice, such as a new tag prefix, or which of `preRelease` and `promote` goes. A command that gives carriers gives those the module has too, since `--shipped-by` replaces the list.
- **What it does not read.** Tags and commits: a tag with no version can still stop a plan. It writes nothing. A repository with no `bumpit.yaml` has nothing to check, and the check says so and passes. `--output json` gives `config`, `file_only`, `blocking`, `review` and `unverified`, each item with `rule`, `subject`, `other` for a conflict between two modules or two keys, `what`, `why`, `how`, `command` and, for a proposal, `embeds` and `left_out`.
- **When it cannot ask.** Outside a repository, with a `go.mod` it cannot read, or with any other failure to read the repository, it fails with the reason, and prints only what it had found until then: it never says there is nothing to resolve. git failing, or a file it cannot read, while a command runs before it is printed is such a failure, not a reason to refuse the command. Outside a repository, the keys of the file that hold a problem come first, since `repository` may be why there is none.

`bumpit init` starts a repository off: it creates `bumpit.yaml` at the top of the repository, where the declarations are read from, when it has neither `bumpit.yaml` nor `.bumpit.yaml`. It lists there the Go modules no entry lists, since a `go.mod` says for certain what a module is, and declares nothing else: what it can only suppose it proposes, each with the `bumpit modules add` that declares it, to run from where `init` ran. It stops on a file that holds a problem, over which it could promise nothing, as `output: xml`, a `repository` that leads nowhere, a declaration with no `shippedBy` or a config in `--repo` that declares modules below the top, and says which `bumpit modules check` lists them all:

```text
created /path/to/repo/bumpit.yaml
listed the Go modules ., cmd/stack in /path/to/repo/bumpit.yaml
tags no module owns:
  homepage/v: 4 tags, the newest homepage/v0.4.0
    bumpit modules add deploy/homepage web/homepage -e homepage -b .
      site.go:5:12 embeds web/homepage/index.html
    leave out a directory that is no part of the module, with the -b only its patterns ask for: bumpit cannot tell a module from directories that share a name
manifests with a version of their own:
  deploy/chart/Chart.yaml: version 1.4.0
    bumpit modules add deploy/chart
a command with no -b says no module ships the directory, but bumpit sees only //go:embed: check that no Dockerfile, CI job or other build copies it into the release of another module, and give that module with -b if one does
commit the file so that every clone of the repository reads the change
```

- **Tags no module owns.** A tag is the release of a module when it is that module's tag prefix followed by a semantic version. The tags a run with no module reads are owned too: `v` followed by a version, and those of the `tagMatch` of the top `bumpit.yaml` or of the config in `--repo`. The prefix of any other tag is the text before its version, which starts at the first digit after a character that is no digit: `release-10.0.0` has the prefix `release-`, not `release-1`. A tag line proposes one module, since a tag prefix belongs to one: its directories are the tracked ones whose last elements are the name of the prefix, the separators at its end dropped, and a `v` before them when a separator or nothing comes before it: `homepage/v` and `homepage-v` propose every directory named `homepage`, while `kv` proposes those named `kv`. The directories of modules, and those inside `vendor`, `testdata` or `node_modules`, are not proposed; when the directory is that of one declared module with no tags of its own, the line proposes the `bumpit modules set -T` that gives it to that module, or says why it would refuse. One inside another that is proposed, or that no module can own, is listed as left out, with why. The module takes the name of the prefix, unless its only directory already says it, another module has it or it is, through the links the repository tracks, the directory of another module, and `-T` appears only when the prefix is not the one the module would take by itself. Tags with no prefix, such as `1.2.3`, are listed as `(no prefix)` with nothing to propose, since a declared module's tags start with a prefix.
- **Manifests.** A `Chart.yaml` that gives a `version` and is no subchart, in the `charts` directory of another chart, and a `package.json` that gives a `version` and is not `private`, read as npm reads it, so that `"private": "true"` is private too. Each must lie in a directory that is no module, is not inside `vendor`, `testdata` or `node_modules`, and that no tag line proposes already. A `Chart.yaml` there that cannot be read is listed as such, and stops nothing.
- **Refusals.** bumpit prints no command it would refuse. The command of each proposal runs without writing, flags, config file and proof included, over what the Go modules and the commands printed before it would write, and the file is written last, so that nothing is written when the proposals cannot be made up. git failing, or a file it cannot read, while a command runs is no reason to refuse it: `init` stops with that failure and writes nothing. A command it would refuse, such as one with a directory with a glob character, a tag prefix that cannot be told from another module's or a file written as a `modules` list in `[...]`, is listed with why in its place, and takes no directory from the proposals after it. A path that starts with `-`, or that is the name of a module, is written after `./`, so that the command reads it as a path, and a module whose name from `--repo`, links followed, is the directory of another is called by its path.
- **Shipping.** A proposal says `-b` for each Go module whose `//go:embed` patterns reach one of its directories, with the patterns and where they are written, and no `-b` when none does. bumpit sees nothing else, so a proposal with no `-b` is to be checked. Paths are relative to `--repo`, those of the patterns included.
- **Go modules.** They go in one edit by the same editor as `bumpit modules add`, which keeps the rest of the file as it is, and only those no entry lists. A file that editor cannot change is left as it is, `init` says why, and carries on; git failing, or a file it cannot read, while it proves the edit stops `init`. A directory no entry can name, as one with a glob character, is left out of the edit, and `init` says why. So is one git takes in no tag name, as one with a space or that starts with `-`, which is to take another name before bumpit can version its module. A `go.mod` added later is a module all the same, and the next `init` lists it.
- Running `init` again lists what is still not listed and proposes what is still not declared, and says when it leaves the file as it is. `init` reads the config in `--repo` as `bumpit modules add` does, the `repository` it gives included, so that what it prints means there what it meant here. `--output json` gives `config`, `created`, `declared` and `unlisted`, as the file writes them, relative to the top of the repository, `unlisted_reason`, `unnameable` and `untaggable`, each with `directory` and `reason`, `tag_lines`, each with its `candidate`, or the `command` that gives it to a declared module or why that is `refused`, and `manifests`, each proposal with `name`, `directories`, `left_out`, `tag_prefix`, `shipped_by`, `embeds` and either `command` or `refused`, and `unreadable` lists the manifests that could not be read.

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
bumpit next --module web/homepage         # a module bumpit.yaml declares
bumpit modules --output json              # bumpit modules -o json
bumpit tag --all-modules                  # bumpit tag -M
bumpit init                               # create bumpit.yaml, list the Go modules, propose the others
bumpit modules add web/homepage -b .      # bumpit modules add web/homepage --shipped-by .
bumpit modules check                      # what needs resolving in bumpit.yaml, why and how
bumpit next --module homepage             # by the name bumpit.yaml gives it
bumpit config set tagMessage "Release"    # write a key of bumpit.yaml
```

## Configuration

`bumpit` looks for `bumpit.yaml` or `.bumpit.yaml` in the target repository. You can also pass an explicit path with `--config`. The `modules` key is read from the config at the top of the repository; see "Modules without a go.mod".

`bumpit config get`, `set` and `unset` read and write one key of the config `--config` names, else of the one found in `--repo`; `set` creates `bumpit.yaml` there when there is none. Keys are compared without case. `get` prints the value as the file writes it, an empty line for a key written with no value, and `get` and `unset` fail on a key the file does not hold, with `<key> is not set in <file>`. `allowDirty` and `promote` take `true` or `false`; the other values are written as YAML strings, quoted when YAML would read them as something else. `set` keeps the comment after the value, and both edit the file as `bumpit modules add` does, line by line and only once the new text is proved, so they can repair a value the config cannot take. The `modules` are edited with `bumpit modules add`, `set` and `remove`.

Precedence is:

- explicit CLI flags
- `bumpit.yaml`
- built-in defaults

Example:

```yaml
repository: .
tagMatch: ""  # empty: "v*", or the root module's tag prefix
startVersion: ""
allowDirty: false
output: text
tagMessage: ""
preRelease: ""
promote: false
modules: []   # modules with no go.mod; see "Modules without a go.mod"
```

## Notes

- the default tag pattern is `v*`, or the root module's tag prefix in a Go repository or when `bumpit.yaml` declares `.`
- the latest tag is always resolved by SemVer precedence, never by tag name or creation date
- merge commits are ignored
- the tool creates annotated local tags only
- `bumpit tag --all-modules` tags every module that has a release, Go modules and the ones `bumpit.yaml` declares, holds the Go modules that require an older version of another module of the repository, and creates no tag when a module cannot be planned
- the tool never pushes tags automatically
- the CLI is built with Cobra
- `bumpit next` returns exit code `0` and prints `no release` when no releasable commits exist, and `bumpit tag` then fails with `no releasable commits found; refusing to create a tag`
- `--pre` applies an explicit prerelease label such as `beta` or `rc.1`
- `--promote` removes the current prerelease suffix and produces the final release for the same base version
- `--start-version` and `--release-as` take the version with or without the tag prefix
- `explain` reports the base release, the last final release whose commits it reads, and the tags it ignored, in a Go module the module and its dependents, and in a declared module what it reads and who carries it
- the JSON payload of `next` and `explain` carries `base_tag`, `ignored_tags`, `module` and `dependents` beside the fields above, and, when `bumpit.yaml` declares modules, `name`, `directories`, `paths`, `derived_paths` and `shipped_by` for a declared module, `name` for a Go module with one, `go_files_in`, `ships` and `leaves_out` for the module around one, and `via` on the commits that came in through a path or a carried module

## Version

`bumpit version` and `bumpit --version` report the version set at link time, when the build sets one, as `make build` and `make install` do with `git describe`:

```bash
go build -ldflags "-X github.com/pragmabits/bumpit/internal/app.buildVersion=v1.2.3" .
```

Without it they report the version the go command records in every binary: the version asked of `go install github.com/pragmabits/bumpit@<version>`, and for a build from a clone the version git gives, such as `v0.4.0` at a clean tagged commit or `v0.4.0+dirty` with uncommitted changes. A build that records neither reports `dev`, which is also what every release up to `v0.3.0` reports.

## Development

```bash
make test      # go test ./...
make lint      # golangci-lint run ./..., with .golangci.yml; needs golangci-lint installed
make build     # ./bin/bumpit, with the version from git describe
make install   # go install, with the same version
make help      # every target
```

The end-to-end tests build the command and run it against temporary git repositories, so they need `git` on the `PATH`.
