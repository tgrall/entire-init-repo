# entire-init

An [Entire CLI](https://docs.entire.io/overview) plugin that adds **`entire init`**, a guided
onboarding command that sets up the current repository on Entire in one pass:

1. **Git repository.** Checks you are in a repo. It offers to run `git init`, and to make an initial commit if there are none yet.
2. **Entire account.** Runs `entire login` if you are not logged in.
3. **Agents & session capture.** Lets you pick which coding agents to capture sessions for. Agents whose CLI is on `PATH` or whose config folder is in the repo are pre-checked. It then runs `entire enable --agent …` and `entire agent add …`.
4. **Entire repository.** Links the repo to Entire, in one of two ways:
   - **Mirror** its GitHub `origin` with `entire repo mirror add /gh/<owner>/<repo>`, on your account's region.
   - **Create** a new Entire repository: `entire repo create` in a project you pick or create, then `entire repo remote add` and `git push`.
5. **Entire Brain (optional).** Installs the brain plugin if missing, runs `entire brain setup --no-backfill --no-daemon` (this spends no agent tokens), then `entire brain connect`. Background distillation spends tokens, so it is a separate opt-in.

It ends with a summary and tailored next steps: connecting Brain, starting a session, creating a trail, and committing the generated config.

Each step checks what is already in place and skips it, so re-running `entire init` is safe.
The plugin never calls Entire APIs itself. Every change is made by an existing `entire`
command, and that command is printed as it runs.

## Usage

```sh
entire init                 # interactive wizard
entire init --dry-run       # show what would change; read-only checks still run
entire init --yes           # accept the recommended defaults
```

Non-interactive / scripted:

```sh
entire init --yes --agents claude-code,codex --repo mirror --no-brain
entire init --yes --repo create --project my-project --repo-name api --visibility private --brain
```

| Flag | Meaning |
|---|---|
| `-y, --yes` | Accept recommended defaults without prompting |
| `--dry-run` | Print the commands that would change things instead of running them |
| `--agents a,b` | Agents to enable (`claude-code, codex, cursor, copilot-cli, opencode, factoryai-droid, pi, antigravity`) |
| `--import-history` | Import the last 30 days of agent history when enabling |
| `--repo mirror\|create\|skip` | How to link the repository |
| `--project`, `--repo-name`, `--visibility` | Options for `--repo create` |
| `--cluster` | Cluster host for a GitHub mirror (default: your account's region) |
| `--brain` / `--no-brain` | Set up Brain without asking, or skip it |
| `--brain-backfill` | Also let Brain distill past sessions and install its watcher (**spends agent tokens**) |

With no terminal (CI, piped), pass `--yes`; otherwise the command exits with code 2.
`entire login` always needs a terminal.
Global `entire` flags are not forwarded to plugins: use `ENTIRE_CONTEXT=<name> entire init`
instead of `entire --context <name> init`.

Exit codes: `0` all steps succeeded or were already done, `1` a step failed, `2` usage error, `130` cancelled.

## Install

From a checkout:

```sh
mise run install        # go build + entire plugin install ./entire-init --force
entire init version
```

From a release (once published):

```sh
entire plugin install https://github.com/entireio/entire-init
```

## Development

```sh
mise run test           # go test ./...
mise run lint           # gofmt + go vet
mise run build          # ./entire-init
```

Layout:

| Path | Purpose |
|---|---|
| `cmd/entire-init` | main |
| `internal/cli` | cobra command and flags |
| `internal/wizard` | one file per step (`preflight`, `auth`, `agents`, `repo`, `brain`) plus `summary` |
| `internal/hostcli` | runs `entire`/`git` as child processes (capture, stream, interactive; dry-run wrapper) with typed wrappers for each subcommand |
| `internal/detect` | git and agent detection, TTY checks |
| `internal/ui` | huh prompts themed like the Entire CLI, plus styled output |

Tests use a fake `Runner` that records commands and returns real captured JSON. An end-to-end test puts a fake `entire` script on `PATH` and pushes to a local bare repo.

Releases: push a `v*` tag. `.github/workflows/release.yml` cross-builds
`entire-init_<version>_<os>_<arch>.tar.gz|.zip` plus `checksums.txt`, the layout
`entire plugin install <repo-url>` expects.
