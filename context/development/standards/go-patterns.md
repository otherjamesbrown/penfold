# Go Patterns

**Write lint-compliant code from the start. These patterns are enforced by CI.**

## Code Quality Requirements

- **TDD Required**: Tests first, ensure they fail, then implement
- **Go conventions**: Follow standard Go formatting (`gofmt`)
- **Zero warnings** from `go vet` and `staticcheck`
- **80% test coverage** minimum for core packages

## Lint-Compliant Patterns

| Pattern | Wrong | Correct |
|---------|-------|---------|
| HTTP response close | `defer resp.Body.Close()` | `defer func() { _ = resp.Body.Close() }()` |
| JSON encode in handlers | `json.NewEncoder(w).Encode(x)` | `_ = json.NewEncoder(w).Encode(x)` |
| Env vars in tests | `os.Setenv("K", "v")` | `t.Setenv("K", "v")` or `_ = os.Setenv("K", "v")` |
| Error strings | `fmt.Errorf("Error msg")` | `fmt.Errorf("error msg")` (lowercase) |
| Ignored errors | `fn()` | `_ = fn()` (explicit ignore) |

## Key Lint Rules

- **errcheck**: Always handle or explicitly ignore error returns with `_ = `
- **ST1005**: Error strings should not be capitalized or end with punctuation
- **SA9003**: Avoid empty if branches - handle errors meaningfully
- **ineffassign**: Don't assign to variables that are never read
- **unused**: Don't declare unused variables, functions, or types
- **S1016**: Use type conversion instead of struct literal when types match

## HTTP Client Pattern

```go
resp, err := client.Do(req)
if err != nil {
    return err
}
defer func() { _ = resp.Body.Close() }()
```

## Test HTTP Handler Pattern

```go
server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    _ = json.NewEncoder(w).Encode(response)
}))
defer server.Close()
```

## Error Handling

```go
// Good - lowercase, no punctuation
return fmt.Errorf("failed to connect to database: %w", err)

// Bad - capitalized, punctuation
return fmt.Errorf("Failed to connect to database: %w.", err)
```

## Running Checks

```bash
# Format code
gofmt -w .

# Vet for issues
go vet ./...

# Static analysis
staticcheck ./...

# Run tests with coverage
go test ./... -coverprofile=coverage.out
go tool cover -func=coverage.out
```
