# Half-Tunnel Test Fixtures

This directory contains test fixtures, integration tests, and end-to-end tests.

## Structure

- `integration/` - Integration tests for client-server interaction
- `e2e/` - End-to-end tests for complete split-path data flow

## Running Tests

### Unit Tests

Unit tests are located in each package's `*_test.go` files:

```bash
# Run all unit tests
go test -v -short ./...

# Run with coverage
go test -v -race -coverprofile=coverage.out ./...
```

### Integration Tests

Integration tests verify client-server communication:

```bash
# Run integration tests
go test -v ./test/integration/...
```

### End-to-End Tests

E2E tests verify complete data flow through the tunnel:

```bash
# Run e2e tests
go test -v ./test/e2e/...
```

### All Tests

```bash
# Run all tests (unit, integration, e2e)
make test
```

## Test Levels

| Level       | Location            | Scope                                    |
|-------------|---------------------|------------------------------------------|
| Unit        | `*_test.go`         | Packet marshal/unmarshal, session logic  |
| Integration | `test/integration/` | Client ↔ Server over localhost WebSockets|
| E2E         | `test/e2e/`         | Full split-path with real data transfer  |

## Writing Tests

- Skip long-running tests in short mode: `if testing.Short() { t.Skip(...) }`
- Use unique ports to avoid conflicts between parallel tests
- Clean up resources with defer statements
- Use timeouts with contexts to prevent hung tests
