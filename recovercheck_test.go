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

	return &analysis.Pass{
		Analyzer: recovercheck.New(),
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
	analyzer := recovercheck.New()

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

func TestSkipTestFiles(t *testing.T) {
	t.Run("flag_configuration", func(t *testing.T) {
		analyzer := recovercheck.New()
		
		// Test that the flag exists and can be set
		err := analyzer.Flags.Set("skip-test-files", "true")
		if err != nil {
			t.Fatalf("Failed to set skip-test-files flag: %v", err)
		}
		
		// Test that the flag can be set to false
		err = analyzer.Flags.Set("skip-test-files", "false")
		if err != nil {
			t.Fatalf("Failed to set skip-test-files flag to false: %v", err)
		}
	})
	
	t.Run("functional_test", func(t *testing.T) {
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
		
		// Count diagnostics without skip flag
		var diagnosticsWithoutSkip []analysis.Diagnostic
		passWithoutSkip := &analysis.Pass{
			Analyzer: recovercheck.New(),
			Fset:     fset,
			Files:    []*ast.File{file},
			ResultOf: map[*analysis.Analyzer]interface{}{
				inspect.Analyzer: insp,
			},
			Report: func(diag analysis.Diagnostic) {
				diagnosticsWithoutSkip = append(diagnosticsWithoutSkip, diag)
			},
		}

		// Run analyzer without skip flag (default config)
		_, err = recovercheck.New().Run(passWithoutSkip)
		if err != nil {
			t.Fatalf("Analyzer run failed: %v", err)
		}

		// Count diagnostics with skip flag enabled
		var diagnosticsWithSkip []analysis.Diagnostic
		analyzerWithSkip := recovercheck.New()
		err = analyzerWithSkip.Flags.Set("skip-test-files", "true")
		if err != nil {
			t.Fatalf("Failed to set skip-test-files flag: %v", err)
		}

		passWithSkip := &analysis.Pass{
			Analyzer: analyzerWithSkip,
			Fset:     fset,
			Files:    []*ast.File{file},
			ResultOf: map[*analysis.Analyzer]interface{}{
				inspect.Analyzer: insp,
			},
			Report: func(diag analysis.Diagnostic) {
				diagnosticsWithSkip = append(diagnosticsWithSkip, diag)
			},
		}

		// Run analyzer with skip flag
		_, err = analyzerWithSkip.Run(passWithSkip)
		if err != nil {
			t.Fatalf("Analyzer run with skip flag failed: %v", err)
		}

		// Verify results
		if len(diagnosticsWithoutSkip) == 0 {
			t.Error("Expected to find diagnostics in test file without skip flag, but found none")
		}

		if len(diagnosticsWithSkip) > 0 {
			t.Errorf("Expected no diagnostics in test file with skip flag, but found %d", len(diagnosticsWithSkip))
		}

		t.Logf("Without skip flag: %d diagnostics", len(diagnosticsWithoutSkip))
		t.Logf("With skip flag: %d diagnostics", len(diagnosticsWithSkip))
	})
}

func TestAll(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), recovercheck.New(), "recovercheck")
}
