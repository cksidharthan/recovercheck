package recovercheck_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/cksidharthan/recovercheck"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
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
	t.Helper()

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

	recovercheckSettings := &recovercheck.RecovercheckSettings{
		SkipTestFiles: false,
	}

	return &analysis.Pass{
		Analyzer: recovercheck.New(recovercheckSettings),
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
			collector := recovercheck.CollectNodes(insp)

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

			testAnalyzer := &recovercheck.Analyzer{
				Pass:             pass,
				RecoverFunctions: make(map[string]bool),
			}

			collector := recovercheck.CollectNodes(insp)
			testAnalyzer.AnalyzeFunctions(collector.FunctionDecls)

			if hasRecover, exists := testAnalyzer.RecoverFunctions[tt.funcName]; !exists {
				t.Errorf("Function %s not found in analyzer results", tt.funcName)
			} else if hasRecover != tt.expected {
				t.Errorf("Expected function %s to have recover=%v, got %v", tt.funcName, tt.expected, hasRecover)
			}
		})
	}
}

// TestNew tests the analyzer creation
func TestNew(t *testing.T) {
	recovercheckSettings := &recovercheck.RecovercheckSettings{
		SkipTestFiles: false,
	}

	analyzer := recovercheck.New(recovercheckSettings)

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

			testAnalyzer := &recovercheck.Analyzer{
				Pass:             pass,
				RecoverFunctions: make(map[string]bool),
			}

			collector := recovercheck.CollectNodes(insp)

			// Should not panic
			testAnalyzer.AnalyzeFunctions(collector.FunctionDecls)
			testAnalyzer.AnalyzeGoroutines(collector.GoStatements)
		})
	}
}

// Benchmark tests for performance
func BenchmarkCollectNodes(b *testing.B) {
	insp, _, _ := parseTestCode(b, testCodeMixed)

	for b.Loop() {
		recovercheck.CollectNodes(insp)
	}
}

func BenchmarkAnalyzeGoroutines(b *testing.B) {
	insp, fset, _ := parseTestCode(b, testCodeMixed)
	pass := createMockPass(b, fset, insp)

	testAnalyzer := &recovercheck.Analyzer{
		Pass:             pass,
		RecoverFunctions: make(map[string]bool),
	}

	collector := recovercheck.CollectNodes(insp)

	for b.Loop() {
		testAnalyzer.AnalyzeGoroutines(collector.GoStatements)
	}
}

func TestSkipTestFilesFunctional(t *testing.T) {
	// Test code for a test file with unsafe goroutines
	testCode := `package test
import "testing"

func TestUnsafeGoroutine(t *testing.T) {
	go func() {
		panic("test panic")
	}()
}`

	// Parse the test code
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test_test.go", testCode, parser.ParseComments)
	if err != nil {
		t.Fatalf("Failed to parse test code: %v", err)
	}

	insp := inspector.New([]*ast.File{file})

	t.Run("without_skip_flag", func(t *testing.T) {
		// Count diagnostics without skip flag
		var diagnostics []analysis.Diagnostic

		recovercheckSettings := &recovercheck.RecovercheckSettings{}

		pass := &analysis.Pass{
			Analyzer: recovercheck.New(recovercheckSettings),
			Fset:     fset,
			Files:    []*ast.File{file},
			ResultOf: map[*analysis.Analyzer]any{
				inspect.Analyzer: insp,
			},
			Report: func(diag analysis.Diagnostic) {
				diagnostics = append(diagnostics, diag)
			},
		}

		// Run analyzer without skip flag (default config)
		_, err = recovercheck.New(recovercheckSettings).Run(pass)
		if err != nil {
			t.Fatalf("Analyzer run failed: %v", err)
		}

		// Verify results
		if len(diagnostics) == 0 {
			t.Error("Expected to find diagnostics in test file without skip flag, but found none")
		}

		t.Logf("Found %d diagnostics without skip flag", len(diagnostics))
	})

	t.Run("with_skip_flag", func(t *testing.T) {
		// Count diagnostics with skip flag enabled
		var diagnostics []analysis.Diagnostic

		recovercheckSettings := &recovercheck.RecovercheckSettings{
			SkipTestFiles: true,
		}

		analyzer := recovercheck.New(recovercheckSettings)

		pass := &analysis.Pass{
			Analyzer: analyzer,
			Fset:     fset,
			Files:    []*ast.File{file},
			ResultOf: map[*analysis.Analyzer]any{
				inspect.Analyzer: insp,
			},
			Report: func(diag analysis.Diagnostic) {
				diagnostics = append(diagnostics, diag)
			},
		}

		// Run analyzer with skip flag
		_, err = analyzer.Run(pass)
		if err != nil {
			t.Fatalf("Analyzer run with skip flag failed: %v", err)
		}

		// Verify results
		if len(diagnostics) > 0 {
			t.Errorf("Expected no diagnostics in test file with skip flag, but found %d", len(diagnostics))
		}

		t.Logf("Found %d diagnostics with skip flag", len(diagnostics))
	})
}

func TestAll(t *testing.T) {
	recovercheckSettings := &recovercheck.RecovercheckSettings{}
	analysistest.Run(t, analysistest.TestData(), recovercheck.New(recovercheckSettings), "recovercheck")
}
