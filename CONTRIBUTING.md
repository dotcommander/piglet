# Contributing to Piglet

Thanks for your interest in contributing!

## Getting Started

```bash
git clone https://github.com/dotcommander/piglet
cd piglet
go build ./...
go test -race ./...
```

## Development

- Go 1.26+
- No external tools required beyond the Go toolchain
- Run `go vet ./...` before submitting

## Pull Requests

1. Fork the repo and create a branch from `main`
2. Add tests for new functionality
3. Ensure `go build ./...` and `go test -race ./...` pass
4. Use [conventional commits](https://www.conventionalcommits.org/): `feat:`, `fix:`, `docs:`, `test:`, `chore:`
5. Keep PRs focused — one feature or fix per PR

## Code Style

- Short functions (80 lines max)
- `t.Parallel()` on all tests
- Table-driven tests for multiple cases
- `fmt.Errorf` with `%w` for error wrapping
- No `init()` or mutable package globals

## Extension Development

See [`docs/extensions.md`](docs/extensions.md) for the extension API. The `examples/extensions/` directory has working examples.

## Reporting Issues

Open an issue with:
- What you expected
- What happened
- Steps to reproduce
- Go version and OS

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
