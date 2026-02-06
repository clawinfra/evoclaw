# Contributing to EvoClaw

Thanks for your interest in contributing to EvoClaw! 🧬

## Ways to Contribute

- **Bug reports** — Found a bug? Open an issue
- **Feature requests** — Have an idea? Open a discussion
- **Code** — Fix bugs, add features, improve tests
- **Documentation** — Fix typos, add examples, improve clarity
- **Testing** — Run on different platforms, report edge cases

## Getting Started

1. Fork the repository
2. Set up your development environment (see [Development Setup](development.md))
3. Create a feature branch: `git checkout -b feat/my-feature`
4. Make your changes
5. Run tests: `go test ./...` and `cd edge-agent && cargo test`
6. Commit with a descriptive message
7. Push and open a Pull Request

## Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add new strategy type for grid trading
fix: handle nil pointer in agent metrics
docs: add deployment guide for Raspberry Pi
test: add integration test for MQTT reconnect
refactor: extract model selection into separate function
chore: update dependencies
```

## Code Style

### Go

- Follow standard Go conventions (`gofmt`, `go vet`)
- Use `slog` for logging (structured logging)
- Keep packages focused and small
- Write table-driven tests

### Rust

- Follow `rustfmt` formatting
- Use `clippy` for linting: `cargo clippy`
- Prefer `Result` over panicking
- Write unit tests in the same file (`#[cfg(test)]`)

## Pull Request Guidelines

1. **One feature per PR** — Keep PRs focused
2. **Tests required** — New code needs tests
3. **Documentation** — Update docs if behavior changes
4. **No breaking changes** — Unless discussed first
5. **CI must pass** — All tests green before merge

## Architecture

Before making significant changes, understand the [architecture](../architecture/overview.md) and read the [Architecture Decision Records](architecture-decisions.md).

## Testing

```bash
# Go tests
go test ./...

# Go tests with coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Rust tests
cd edge-agent
cargo test

# Rust tests with coverage
cargo llvm-cov
```

Current coverage targets: Go ≥85%, Rust ≥90%.

## License

By contributing, you agree that your contributions will be licensed under the MIT License.

## Code of Conduct

Be kind. Be respectful. We're all here to build something cool.

## Questions?

Open a Discussion on GitHub or reach out to the maintainers.
