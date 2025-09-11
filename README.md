# recovercheck

recovercheck finds goroutines created without panic recovery logic in Go code.

For all goroutines created with the `go` statement, recovercheck ensures they have proper panic recovery mechanisms to prevent crashes. It detects both inline `defer recover()` patterns and calls to functions that contain recovery logic.

```go
// Bad: goroutine without recovery - will be flagged
go func() {
    panic("This will crash the program")
}()

// Good: goroutine with recovery - will not be flagged
go func() {
    defer func() {
        if r := recover(); r != nil {
            log.Println("Recovered from panic:", r)
        }
    }()
    panic("This will be recovered")
}()
```

Please note that recovercheck uses heuristics for cross-package function analysis. Functions with names containing "recover", "panic", "safe", "rescue", or "catch" are assumed to contain recovery logic.

## Install

```bash
go install github.com/cksidharthan/recovercheck@latest
```

recovercheck requires Go 1.18 or newer.

## Use

For basic usage, just give the package path of interest as the first argument:

```bash
recovercheck github.com/youruser/yourproject/pkg
```

To check all packages beneath the current directory:

```bash
recovercheck ./...
```

Or check all packages in your $GOPATH and $GOROOT:

```bash
recovercheck all
```

recovercheck also recognizes the following command-line options:

The `-test` flag enables checking test files. It takes no arguments.

```bash
recovercheck -test ./...
```

The `-json` flag enables JSON output for integration with other tools.

```bash
recovercheck -json ./...
```

### go/analysis

The package provides an `Analyzer` instance that can be used with [go/analysis](https://pkg.go.dev/golang.org/x/tools/go/analysis) API:

```go
import "github.com/cksidharthan/recovercheck/analyzer"

// Use in your analysis tool
analyzer.New()
```

## Detection Patterns

recovercheck detects several patterns of goroutine safety:

### Safe Patterns (Not Flagged)

**Inline defer recover:**
```go
go func() {
    defer func() {
        if r := recover(); r != nil {
            log.Printf("Recovered: %v", r)
        }
    }()
    // goroutine code that might panic
}()
```

**Named function with recovery:**
```go
func safeWorker() {
    defer func() {
        if r := recover(); r != nil {
            log.Printf("Worker recovered: %v", r)
        }
    }()
    // work that might panic
}

go safeWorker() // Not flagged - named function assumed safe
```

**Cross-package recovery function:**
```go
import "github.com/yourorg/pkg"

go func() {
    defer pkg.PanicRecover() // Not flagged - name suggests recovery
}()
```

### Unsafe Patterns (Flagged)

**No recovery mechanism:**
```go
go func() {
    panic("This will crash the program") // Flagged!
}()
```

**Defer without recovery:**
```go
go func() {
    defer fmt.Println("cleanup") // Flagged - no recovery!
    panic("unhandled panic")
}()
```

## Exit Codes

recovercheck returns 3 if any issues are found, 0 otherwise.

## Editor Integration

### VS Code

Add this to your VS Code settings to integrate with the Go extension:

```json
{
    "go.lintTool": "recovercheck",
    "go.lintFlags": ["./..."]
}
```

### Vim/Neovim

For vim-go users:

```vim
let g:go_metalinter_enabled = ['recovercheck']
```

### golangci-lint

Add recovercheck to your `.golangci.yml`:

```yaml
linters:
  enable:
    - recovercheck
```

## Examples

**Basic usage:**
```bash
$ recovercheck ./...
./main.go:15:2: goroutine created without panic recovery
./worker/unsafe.go:23:5: goroutine created without panic recovery
```

**With JSON output:**
```bash
$ recovercheck -json ./...
{
  "Issues": [
    {
      "FromLinter": "recovercheck",
      "Text": "goroutine created without panic recovery",
      "Pos": {
        "Filename": "./main.go",
        "Line": 15,
        "Column": 2
      }
    }
  ]
}
```

## Why Use recovercheck?

Unhandled panics in goroutines can crash your entire Go program. Unlike panics in the main goroutine, panics in background goroutines cannot be caught by a top-level recovery mechanism.

**Without recovery:**
```go
func main() {
    go func() {
        panic("background panic") // Crashes entire program!
    }()
    time.Sleep(1 * time.Second)
    fmt.Println("This will never print")
}
```

**With recovery:**
```go
func main() {
    go func() {
        defer func() {
            if r := recover(); r != nil {
                log.Printf("Recovered: %v", r)
            }
        }()
        panic("background panic") // Handled gracefully
    }()
    time.Sleep(1 * time.Second)
    fmt.Println("Program continues normally")
}
```

recovercheck helps you identify and fix these potential crash points before they reach production.

## License

MIT License - see [LICENSE](LICENSE) file for details.
            log.Println("Recovered from panic:", r)
        }
    }()
    panic("This will be recovered")
}()
