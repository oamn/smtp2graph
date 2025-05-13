# Project Context

- This is a Go application that integrates with Microsoft Graph and SMTP.
- Key dependencies include:
  - `emersion/go-smtp` and `emersion/go-sasl` for SMTP server/client logic
  - `Azure/azure-sdk-for-go/sdk/azidentity` for Microsoft authentication
  - `getsentry/sentry-go` for error reporting
- The code should be production-ready, maintainable, and easy to test.

# General Principles

- Avoid apologizing or making conciliatory statements.
- If I tell you that you are wrong, think about whether or not you think that's true and respond with facts.
- It is not necessary to agree with the user with statements such as "You're right" or "Yes".
- Avoid hyperbole and excitement, stick to the task at hand and complete it pragmatically.
- Write idiomatic, concise, and robust Go code.
- Use clear, descriptive naming conventions following Go best practices.
- Prefer explicit error handling; avoid panics unless absolutely necessary.
- Use Go standard library features when possible.
- Write small, focused, and testable functions.
- Add comments to all exported functions, types, and packages.
- Avoid unnecessary abstractions; keep code simple and readable.
- Use `context.Context` for long-running or cancellable operations, especially for network and API calls.
- Prefer slices over arrays, and use maps for key-value data.
- Use struct embedding for composition, not inheritance.
- Format code according to `gofmt` standards.

# Code Style

- Use camelCase for variables and functions, PascalCase for exported names.
- Group related code into packages with clear responsibilities.
- Keep package-level variables to a minimum.
- Use interfaces to decouple code, but avoid over-abstraction.
- When working with external APIs (SMTP, Microsoft Graph), encapsulate logic in dedicated files.

# Error Handling

- Always check and handle errors explicitly.
- Return errors as the last return value.
- Use `errors.Is` and `errors.As` for error inspection.
- Wrap errors with context using `fmt.Errorf` or `errors.Join` when needed.
- Integrate Sentry error reporting for critical failures.

# Documentation

- Document all exported symbols with Go-style comments.
- Use examples in comments to clarify usage when appropriate.
- Document configuration, authentication, and environment variable requirements in code and in the README.

# Testing

- Write table-driven tests using Go's `testing` package.
- Use `t.Helper()` in helper functions.
- Prefer subtests for related test cases.
- Mock external dependencies (SMTP, Microsoft Graph) in tests.

# Project-Specific Guidance

- When implementing SMTP handlers, follow the patterns in the codebase and use the `go-smtp` library idiomatically.
- For Microsoft Graph API calls, use direct HTTP requests with the `net/http` package and handle authentication manually using Microsoft Graph's OAuth2 flow.
- Ensure all network operations are cancellable via `context.Context`.
- Log and report errors to Sentry where appropriate.
- Keep the codebase easy to navigate and maintain by following the established file/module structure.
