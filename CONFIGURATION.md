# Configuration Options

The recovercheck linter supports several configuration options to customize its behavior.

## Skip Test Files

Use the `-skip-test-files` flag to skip analysis of `*_test.go` files.

### Usage

```bash
# Skip analysis of test files
recovercheck -skip-test-files=true ./...

# Analyze all files including test files (default behavior)
recovercheck -skip-test-files=false ./...
```

### Examples

```bash
# Analyze only production code, skip test files
recovercheck -skip-test-files=true ./src/...

# Analyze everything including test files
recovercheck ./...
```

## Why Skip Test Files?

Test files often contain intentional unsafe goroutines for testing error conditions or may use testing frameworks that handle panics differently. Skipping test files allows you to focus on production code while avoiding false positives in test scenarios.

## Implementation Details

- The flag checks the filename suffix `_test.go` to determine if a file should be skipped
- Skipping is applied at the goroutine and errgroup analysis level
- The default behavior is to analyze all files (skip-test-files=false)
