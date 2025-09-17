# recovercheck

A Go static analysis tool that finds goroutines created without panic recovery logic.

## Why?

Unhandled panics in goroutines crash your entire program. recovercheck helps you catch these before they reach production.

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

```bash
go install github.com/cksidharthan/recovercheck@latest
```

## Usage

```bash
# Check current directory
recovercheck ./...

# Check specific package
recovercheck github.com/user/project/pkg

# JSON output
recovercheck -json ./...

# Include test files
recovercheck -test ./...
```

### go/analysis
```go
import "github.com/cksidharthan/recovercheck/analyzer"

analyzer.New() // Use in your analysis tool
```

## License

MIT
