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

# Exclude test files
recovercheck -test=false ./...
```

## Configuration
recovercheck uses go/analysis flags for configuration. Run `recovercheck -h` to see all available options.

## License

MIT
