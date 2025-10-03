# recovercheck

A Go static analysis tool that finds goroutines created without panic recovery logic.

## Why?

Unhandled panics in goroutines crash your goroutines. recovercheck helps you catch these before they reach production.

```go
// ❌ Bad - will crash the program
go func() {
    panic("oops")
}()

// ✅ Good - panic is recovered
go func() {
    defer func() {
        if r := recover(); r != nil {
            log.Printf("recovered: %v", r)
        }
    }()
    panic("oops")
}()
```

## Install

Option 1: Install using go install
```bash
go install github.com/cksidharthan/recovercheck/cmd/recovercheck@latest
```

## Usage

```bash
# Check current directory
recovercheck ./...

# JSON output
recovercheck -json ./...

# Include test files
recovercheck -test ./...
```

## Configuration
recovercheck uses go/analysis flags for configuration. Run `recovercheck -h` to see all available options.

In addition to the default flags, recovercheck supports the following custom flag:
- `-skip-test-files`: Skip analysis of `*_test.go` files. Default is `false`.

```bash
recovercheck -skip-test-files=true ./...
```

For More details, see [CONFIGURATION.md](CONFIGURATION.md).

## License

MIT
