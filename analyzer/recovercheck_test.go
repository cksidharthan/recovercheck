package analyzer

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Test data for various scenarios
const (
	testCodeUnsafe = `package test
func UnsafeGoroutine() {
	go func() {
		panic("no recovery")
	}()
}`

	testCodeSafe = `package test
func SafeGoroutine() {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				// handle panic
			}
		}()
		panic("recovered")
	}()
}`

	testCodeNamedFunction = `package test
func recoverFunc() {
	if r := recover(); r != nil {
		// handle panic
	}
}

func SafeWithNamed() {
	go recoverFunc()
}`

	testCodeCrossPackage = `package test
import "pkg"

func SafeWithCrossPackage() {
	go pkg.SafeFunction()
}`

	testCodeMixed = `package test
func MixedScenarios() {
	// Unsafe goroutine
	go func() {
		panic("unsafe")
	}()
	
	// Safe goroutine with defer recover
	go func() {
		defer func() {
			recover()
		}()
		panic("safe")
	}()
	
	// Safe goroutine with named function
	go safeFunc()
}`
)

// Helper function to parse test code and create inspector
func parseTestCode(t testing.TB, code string) (*inspector.Inspector, *token.FileSet, *ast.File) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", code, parser.ParseComments)
	if err != nil {
		t.Fatalf("Failed to parse test code: %v", err)
	}

	insp := inspector.New([]*ast.File{file})
	return insp, fset, file
}

// Helper function to create a mock analysis.Pass
func createMockPass(t testing.TB, fset *token.FileSet, insp *inspector.Inspector) *analysis.Pass {
	t.Helper()

	return &analysis.Pass{
		Analyzer: New(),
		Fset:     fset,
		ResultOf: map[*analysis.Analyzer]interface{}{
			inspect.Analyzer: insp,
		},
		Report: func(d analysis.Diagnostic) {
			// Store diagnostics for testing
		},
	}
}

// TestCollectNodes tests the CollectNodes function
func TestCollectNodes(t *testing.T) {
	tests := []struct {
		name              string
		code              string
		expectedFuncCount int
		expectedGoCount   int
	}{
		{
			name:              "empty code",
			code:              "package test",
			expectedFuncCount: 0,
			expectedGoCount:   0,
		},
		{
			name:              "single function no goroutines",
			code:              "package test\nfunc Test() {}",
			expectedFuncCount: 1,
			expectedGoCount:   0,
		},
		{
			name:              "single goroutine",
			code:              testCodeUnsafe,
			expectedFuncCount: 1,
			expectedGoCount:   1,
		},
		{
			name:              "multiple goroutines",
			code:              testCodeMixed,
			expectedFuncCount: 1,
			expectedGoCount:   3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			insp, _, _ := parseTestCode(t, tt.code)
			collector := CollectNodes(insp)

			if len(collector.FunctionDecls) != tt.expectedFuncCount {
				t.Errorf("Expected %d functions, got %d", tt.expectedFuncCount, len(collector.FunctionDecls))
			}

			if len(collector.GoStatements) != tt.expectedGoCount {
				t.Errorf("Expected %d go statements, got %d", tt.expectedGoCount, len(collector.GoStatements))
			}
		})
	}
}

// TestRecoverAnalyzer_analyzeFunction tests the analyzeFunction method
func TestRecoverAnalyzer_analyzeFunction(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		funcName string
		expected bool
	}{
		{
			name: "function with recover",
			code: `package test
func TestFunc() {
	if r := recover(); r != nil {
		// handle
	}
}`,
			funcName: "TestFunc",
			expected: true,
		},
		{
			name: "function without recover",
			code: `package test
func TestFunc() {
	println("no recover")
}`,
			funcName: "TestFunc",
			expected: false,
		},
		{
			name: "function with deferred recover",
			code: `package test
func TestFunc() {
	defer func() {
		recover()
	}()
}`,
			funcName: "TestFunc",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			insp, fset, _ := parseTestCode(t, tt.code)
			pass := createMockPass(t, fset, insp)

			analyzer := &RecoverAnalyzer{
				pass:             pass,
				recoverFunctions: make(map[string]bool),
			}

			collector := CollectNodes(insp)
			analyzer.AnalyzeFunctions(collector.FunctionDecls)

			if hasRecover, exists := analyzer.recoverFunctions[tt.funcName]; !exists {
				t.Errorf("Function %s not found in analyzer results", tt.funcName)
			} else if hasRecover != tt.expected {
				t.Errorf("Expected function %s to have recover=%v, got %v", tt.funcName, tt.expected, hasRecover)
			}
		})
	}
}

// TestRecoverAnalyzer_hasRecoveryLogic tests the hasRecoveryLogic method
func TestRecoverAnalyzer_hasRecoveryLogic(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected bool
	}{
		{
			name:     "function literal with recover",
			code:     testCodeSafe,
			expected: true,
		},
		{
			name:     "function literal without recover",
			code:     testCodeUnsafe,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			insp, fset, _ := parseTestCode(t, tt.code)
			pass := createMockPass(t, fset, insp)

			analyzer := &RecoverAnalyzer{
				pass:             pass,
				recoverFunctions: make(map[string]bool),
			}

			collector := CollectNodes(insp)
			if len(collector.GoStatements) == 0 {
				t.Fatal("No go statements found in test code")
			}

			goStmt := collector.GoStatements[0]
			result := analyzer.hasRecoveryLogic(goStmt.Call)

			if result != tt.expected {
				t.Errorf("Expected hasRecoveryLogic to return %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestRecoverAnalyzer_isRecoverCall tests the isRecoverCall method
func TestRecoverAnalyzer_isRecoverCall(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected bool
	}{
		{
			name: "direct recover call",
			code: `package test
func Test() {
	recover()
}`,
			expected: true,
		},
		{
			name: "other function call",
			code: `package test
func Test() {
	println("test")
}`,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			insp, fset, _ := parseTestCode(t, tt.code)
			pass := createMockPass(t, fset, insp)

			analyzer := &RecoverAnalyzer{
				pass:             pass,
				recoverFunctions: make(map[string]bool),
			}

			// Find the call expression in the AST
			var callExpr *ast.CallExpr
			_, _, file := parseTestCode(t, tt.code)
			ast.Inspect(file, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok {
					callExpr = call
					return false
				}
				return true
			})

			if callExpr == nil {
				if tt.expected {
					t.Fatal("Expected to find a call expression")
				}
				return
			}

			result := analyzer.isRecoverCall(callExpr)
			if result != tt.expected {
				t.Errorf("Expected isRecoverCall to return %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestRecoverAnalyzer_hasRecoveryNaming tests the hasRecoveryNaming method
func TestRecoverAnalyzer_hasRecoveryNaming(t *testing.T) {
	tests := []struct {
		name     string
		funcName string
		expected bool
	}{
		{"recover in name", "recoverFromPanic", true},
		{"panic in name", "handlePanic", true},
		{"safe in name", "safeExecute", true},
		{"rescue in name", "rescueOperation", true},
		{"catch in name", "catchError", true},
		{"uppercase recover", "RecoverHandler", true},
		{"mixed case", "PanicRecover", true},
		{"no recovery keywords", "normalFunction", false},
		{"partial match", "coverage", false}, // "recover" is substring but not recovery-related
	}

	analyzer := &RecoverAnalyzer{
		recoverFunctions: make(map[string]bool),
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := analyzer.hasRecoveryNaming(tt.funcName)
			if result != tt.expected {
				t.Errorf("Expected hasRecoveryNaming(%s) to return %v, got %v", tt.funcName, tt.expected, result)
			}
		})
	}
}

// TestRecoverAnalyzer_containsRecover tests the containsRecover method
func TestRecoverAnalyzer_containsRecover(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected bool
	}{
		{
			name: "direct recover call",
			code: `package test
func Test() {
	recover()
}`,
			expected: true,
		},
		{
			name: "recover in if statement",
			code: `package test
func Test() {
	if r := recover(); r != nil {
		// handle
	}
}`,
			expected: true,
		},
		{
			name: "recover in defer",
			code: `package test
func Test() {
	defer func() {
		recover()
	}()
}`,
			expected: true,
		},
		{
			name: "no recover",
			code: `package test
func Test() {
	println("no recover")
}`,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			insp, fset, _ := parseTestCode(t, tt.code)
			pass := createMockPass(t, fset, insp)

			analyzer := &RecoverAnalyzer{
				pass:             pass,
				recoverFunctions: make(map[string]bool),
			}

			collector := CollectNodes(insp)
			if len(collector.FunctionDecls) == 0 {
				t.Fatal("No function declarations found")
			}

			funcDecl := collector.FunctionDecls[0]
			result := analyzer.containsRecover(funcDecl.Body)

			if result != tt.expected {
				t.Errorf("Expected containsRecover to return %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestRecoverAnalyzer_isDeferredRecovery tests the isDeferredRecovery method
func TestRecoverAnalyzer_isDeferredRecovery(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected bool
	}{
		{
			name: "defer recover()",
			code: `package test
func Test() {
	defer recover()
}`,
			expected: true,
		},
		{
			name: "defer func with recover",
			code: `package test
func Test() {
	defer func() {
		recover()
	}()
}`,
			expected: true,
		},
		{
			name: "defer without recover",
			code: `package test
func Test() {
	defer println("cleanup")
}`,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			insp, fset, _ := parseTestCode(t, tt.code)
			pass := createMockPass(t, fset, insp)

			analyzer := &RecoverAnalyzer{
				pass:             pass,
				recoverFunctions: make(map[string]bool),
			}

			// Find the defer statement in the AST
			var deferStmt *ast.DeferStmt
			_, _, file := parseTestCode(t, tt.code)
			ast.Inspect(file, func(n ast.Node) bool {
				if defer_, ok := n.(*ast.DeferStmt); ok {
					deferStmt = defer_
					return false
				}
				return true
			})

			if deferStmt == nil {
				t.Fatal("No defer statement found")
			}

			result := analyzer.isDeferredRecovery(deferStmt)
			if result != tt.expected {
				t.Errorf("Expected isDeferredRecovery to return %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestNew tests the analyzer creation
func TestNew(t *testing.T) {
	analyzer := New()

	if analyzer.Name != "recovercheck" {
		t.Errorf("Expected analyzer name to be 'recovercheck', got '%s'", analyzer.Name)
	}

	if analyzer.Doc == "" {
		t.Error("Expected analyzer to have documentation")
	}

	if analyzer.Run == nil {
		t.Error("Expected analyzer to have a Run function")
	}

	if len(analyzer.Requires) != 1 || analyzer.Requires[0] != inspect.Analyzer {
		t.Error("Expected analyzer to require inspect.Analyzer")
	}
}

// TestEdgeCases tests various edge cases and error conditions
func TestEdgeCases(t *testing.T) {
	tests := []struct {
		name string
		code string
		desc string
	}{
		{
			name: "nil function name",
			code: `package test
var _ = func() {}`, // Anonymous function assigned to blank identifier
			desc: "Should handle functions without names gracefully",
		},
		{
			name: "empty function body",
			code: `package test
func EmptyFunc()`, // Function declaration without body (interface method)
			desc: "Should handle functions without bodies",
		},
		{
			name: "nested goroutines",
			code: `package test
func NestedGoroutines() {
	go func() {
		go func() {
			panic("nested")
		}()
	}()
}`,
			desc: "Should handle nested goroutines",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// These tests mainly ensure no panics occur during analysis
			insp, fset, _ := parseTestCode(t, tt.code)
			pass := createMockPass(t, fset, insp)

			analyzer := &RecoverAnalyzer{
				pass:             pass,
				recoverFunctions: make(map[string]bool),
			}

			collector := CollectNodes(insp)

			// Should not panic
			analyzer.AnalyzeFunctions(collector.FunctionDecls)
			analyzer.AnalyzeGoroutines(collector.GoStatements)
		})
	}
}

// Integration test using analysistest package
func TestIntegration(t *testing.T) {
	// This would typically use analysistest.Run with testdata directory
	// For now, we'll create a simple integration test

	testCode := `package testpkg

func UnsafeGoroutine() {
	go func() { // want "goroutine created without panic recovery"
		panic("unsafe")
	}()
}

func SafeGoroutine() {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				// recovered
			}
		}()
		panic("safe")
	}()
}
`

	// Create a temporary test environment
	insp, fset, _ := parseTestCode(t, testCode)

	var diagnostics []analysis.Diagnostic
	pass := &analysis.Pass{
		Analyzer: New(),
		Fset:     fset,
		ResultOf: map[*analysis.Analyzer]interface{}{
			inspect.Analyzer: insp,
		},
		Report: func(d analysis.Diagnostic) {
			diagnostics = append(diagnostics, d)
		},
	}

	// Run the analyzer
	_, err := run(pass)
	if err != nil {
		t.Fatalf("Analyzer run failed: %v", err)
	}

	// Check that we got the expected diagnostic for unsafe goroutine
	if len(diagnostics) != 1 {
		t.Errorf("Expected 1 diagnostic, got %d", len(diagnostics))
		for i, d := range diagnostics {
			t.Logf("Diagnostic %d: %s", i, d.Message)
		}
	} else {
		expectedMsg := "goroutine created without panic recovery"
		if !strings.Contains(diagnostics[0].Message, expectedMsg) {
			t.Errorf("Expected diagnostic message to contain '%s', got '%s'", expectedMsg, diagnostics[0].Message)
		}
	}
}

// Benchmark tests for performance
func BenchmarkCollectNodes(b *testing.B) {
	insp, _, _ := parseTestCode(b, testCodeMixed)

	for b.Loop() {
		CollectNodes(insp)
	}
}

func BenchmarkAnalyzeGoroutines(b *testing.B) {
	insp, fset, _ := parseTestCode(b, testCodeMixed)
	pass := createMockPass(b, fset, insp)

	analyzer := &RecoverAnalyzer{
		pass:             pass,
		recoverFunctions: make(map[string]bool),
	}

	collector := CollectNodes(insp)

	for b.Loop() {
		analyzer.AnalyzeGoroutines(collector.GoStatements)
	}
}
