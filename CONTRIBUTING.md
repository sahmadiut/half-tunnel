# Contributing to Half-Tunnel

Thank you for your interest in contributing to Half-Tunnel! This document provides guidelines and information for contributors.

## Code of Conduct

By participating in this project, you agree to maintain a respectful and inclusive environment. Please be courteous and constructive in all interactions.

## How to Contribute

### Reporting Bugs

If you find a bug, please open an issue with:

1. A clear, descriptive title
2. Steps to reproduce the issue
3. Expected behavior vs actual behavior
4. Your environment (OS, Go version, Half-Tunnel version)
5. Any relevant logs or error messages

### Suggesting Features

For feature requests, please:

1. Check if a similar feature has been requested
2. Open an issue with a clear description
3. Explain the use case and benefits
4. Consider if you'd be willing to implement it

### Pull Requests

1. Fork the repository
2. Create a feature branch from `main`
3. Make your changes
4. Write or update tests
5. Ensure all tests pass
6. Submit a pull request

## Development Setup

### Prerequisites

- Go 1.21 or later
- golangci-lint (for linting)
- Docker (optional, for containerized testing)

### Getting Started

```bash
# Clone your fork
git clone https://github.com/YOUR_USERNAME/half-tunnel.git
cd half-tunnel

# Add upstream remote
git remote add upstream https://github.com/sahmadiut/half-tunnel.git

# Install dependencies
go mod download

# Build
make build

# Run tests
make test

# Run linter
make lint
```

### Running Locally

```bash
# Start the server
./bin/ht-server -config configs/config.example.yaml

# In another terminal, start the client
./bin/ht-client -config configs/config.example.yaml

# Test with curl
curl --socks5 127.0.0.1:1080 https://example.com
```

## Code Style

### Go Guidelines

- Follow standard Go conventions and idioms
- Use `gofmt` or `goimports` for formatting
- Write clear, self-documenting code
- Add comments for complex logic
- Keep functions focused and small

### Naming Conventions

- Use descriptive names
- Package names should be lowercase, single-word
- Exported names should be clear without package prefix
- Use camelCase for local variables
- Use PascalCase for exported names

### Error Handling

- Always handle errors explicitly
- Use custom error types from `internal/errors`
- Wrap errors with context using `fmt.Errorf("context: %w", err)`
- Log errors at the appropriate level

## Testing

### Writing Tests

- Write unit tests for all new functionality
- Place tests in the same package with `_test.go` suffix
- Use table-driven tests where appropriate
- Mock external dependencies
- Test edge cases and error conditions

### Test Organization

```
internal/
├── protocol/
│   ├── packet.go
│   └── packet_test.go      # Unit tests
test/
├── integration/             # Integration tests
└── e2e/                     # End-to-end tests
```

### Running Tests

```bash
# Run all tests
make test

# Run tests with coverage
make test-coverage

# Run a specific package
go test -v ./internal/protocol/...

# Run integration tests
go test -v ./test/integration/...

# Skip integration tests in short mode
go test -v -short ./...
```

## Project Structure

```
half-tunnel/
├── cmd/                    # Application entry points
│   ├── client/             # Client binary
│   ├── server/             # Server binary
│   └── half-tunnel/        # CLI tool
├── internal/               # Private application code
│   ├── protocol/           # Wire protocol
│   ├── transport/          # WebSocket transport
│   ├── session/            # Session management
│   ├── mux/                # Multiplexing
│   └── config/             # Configuration
├── pkg/                    # Public libraries
│   ├── crypto/             # Encryption utilities
│   └── logger/             # Logging wrapper
├── api/                    # API definitions
├── configs/                # Sample configurations
├── deployments/            # Docker files
├── docs/                   # Documentation
├── scripts/                # Build/deploy scripts
└── test/                   # Integration/E2E tests
```

## Commit Messages

Follow conventional commit format:

```
<type>(<scope>): <subject>

<body>

<footer>
```

Types:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `style`: Code style changes (formatting)
- `refactor`: Code refactoring
- `test`: Adding or updating tests
- `chore`: Maintenance tasks

Examples:
```
feat(protocol): add reconnection packet type

Implements RECONNECT flag for session resumption after connection loss.
Includes exponential backoff logic.

Closes #123
```

```
fix(transport): handle WebSocket close properly

Ensure all goroutines are cleaned up when WebSocket connection closes.
```

## Pull Request Process

1. **Before submitting:**
   - Ensure your code builds: `make build`
   - Run tests: `make test`
   - Run linter: `make lint`
   - Update documentation if needed

2. **PR description should include:**
   - What the change does
   - Why the change is needed
   - How it was tested
   - Any breaking changes

3. **Review process:**
   - PRs require at least one approval
   - Address all review comments
   - Keep changes focused and atomic

## Release Process

Releases follow semantic versioning (SemVer):

- **Major** (X.0.0): Breaking changes
- **Minor** (0.X.0): New features, backward compatible
- **Patch** (0.0.X): Bug fixes, backward compatible

### Creating a Release

1. Update changelog
2. Create a tag: `git tag v1.0.0`
3. Push the tag: `git push origin v1.0.0`
4. GitHub Actions will automatically build and publish

## Getting Help

- Check existing [issues](https://github.com/sahmadiut/half-tunnel/issues)
- Read the [documentation](docs/)
- Open a new issue for questions

## License

By contributing, you agree that your contributions will be licensed under the MIT License.

Thank you for contributing to Half-Tunnel!
