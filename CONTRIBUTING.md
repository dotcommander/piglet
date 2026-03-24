# Contributing to Piglet

Thanks for your interest in contributing!

## Prerequisites

- Go 1.26+
- Git

## Build & Test

```bash
git clone https://github.com/dotcommander/piglet
cd piglet
go build ./...
go test -race ./...
go vet ./...
```

All three must pass before submitting a PR. CI runs them automatically on every push and PR.

## Architecture

Piglet is extension-first. The core agent loop (`core/`) imports nothing from piglet — all functionality registers through `ext.App`. An architecture test enforces dependency boundaries:

```bash
go test ./ext/... -run TestArchitecture
```

**Key rule**: new functionality should be an extension, not a core modification. See [CLAUDE.md](CLAUDE.md) for the full architecture overview.

### Package structure

```
core/       Agent loop, streaming, types (imports nothing from piglet)
ext/        Registration surface (ext.App)
tool/       Built-in tools (read, write, edit, bash, grep, find, ls)
command/    Built-in slash commands
prompt/     System prompt builder
provider/   OpenAI, Anthropic, Google streaming
sdk/        Go Extension SDK (standalone module)
tui/        Bubble Tea v2 terminal UI
```

## Writing Extensions

Extensions are the preferred way to add functionality. They run as standalone binaries communicating via JSON-RPC over stdin/stdout.

```bash
# Scaffold a new extension from inside piglet:
/ext-init my-extension
```

See [docs/extensions.md](docs/extensions.md) for the SDK reference and [examples/extensions/](examples/extensions/) for working code.

Official extensions live in a separate repo: [piglet-extensions](https://github.com/dotcommander/piglet-extensions).

## Pull Requests

1. Fork the repo and create a branch from `main`
2. Make your changes
3. Add tests for new functionality
4. Ensure `go build ./...`, `go test -race ./...`, and `go vet ./...` pass
5. Submit a pull request

### Commit messages

Use [conventional commits](https://www.conventionalcommits.org/):

```
feat(tool): add support for binary file detection
fix(provider): handle empty SSE data lines
docs: update extension SDK examples
test(session): add fork/branch coverage
```

### What makes a good PR

- **Focused** — one feature or fix per PR
- **Tested** — new code has tests, existing tests still pass
- **Clean** — no unrelated changes, no debug output, no commented-out code

### What to avoid

- Modifying `core/` without a strong reason (everything should be an extension)
- Adding dependencies without checking if an existing one covers the need
- Hardcoding configuration data in Go source — prompts, defaults, and behavioral text belong in config files under `~/.config/piglet/`
- Committing secrets, local paths, or binary artifacts

## Code Style

- Short functions (80 lines max)
- Pointer receivers by default
- `context.Context` as first param
- `fmt.Errorf` with `%w` for error wrapping
- No `init()`, no mutable package globals
- `t.Parallel()` on all tests
- Table-driven tests for multiple cases

See [CLAUDE.md](CLAUDE.md) for the complete conventions.

## Reporting Issues

Open an issue with:

- What you expected
- What happened
- Steps to reproduce
- Go version and OS

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).
