package analyzer

import (
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// New returns new recovercheck analyzer.
func New() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "recovercheck",
		Doc:      "Checks that goroutines have panic recovery logic",
		Run:      run,
		Requires: []*analysis.Analyzer{inspect.Analyzer},
	}
}

func run(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	// Collect all function declarations that contain recover() calls
	// Key format: "packageName.FunctionName" for cross-package, "FunctionName" for same package
	recoverFunctions := make(map[string]bool)
	
	// For cross-package recovery detection, we'll use a heuristic approach
	// Functions with names containing "recover", "panic", "safe", etc. are likely recovery functions
	
	// Also analyze functions in the current package
	insp.Nodes([]ast.Node{(*ast.FuncDecl)(nil)}, func(node ast.Node, push bool) bool {
		if !push {
			return false
		}
		
		funcDecl := node.(*ast.FuncDecl)
		if funcDecl.Name != nil && funcDecl.Body != nil {
			if containsRecoverAnywhere(funcDecl.Body) {
				recoverFunctions[funcDecl.Name.Name] = true
			}
		}
		return false
	})

	insp.Nodes([]ast.Node{(*ast.GoStmt)(nil)}, func(node ast.Node, push bool) bool {
		if !push {
			return false
		}

		goStmt := node.(*ast.GoStmt)
		if goStmt.Call == nil {
			pass.Reportf(goStmt.Pos(), "go statement without call expression")
			return false
		}

		// Check if the goroutine has recover logic
		if !hasRecoverLogic(goStmt.Call, recoverFunctions) {
			pass.Reportf(goStmt.Pos(), "goroutine created without panic recovery")
		}

		return false
	})

	return nil, nil
}

// hasRecoverLogic checks if a function call or function literal contains defer recover() logic
func hasRecoverLogic(call *ast.CallExpr, recoverFunctions map[string]bool) bool {
	// Check if it's a function literal (anonymous function)
	if funcLit, ok := call.Fun.(*ast.FuncLit); ok {
		return hasRecoverInFunction(funcLit.Body, recoverFunctions)
	}

	// For function calls, we can't analyze the function body without more complex analysis
	// For now, we'll assume named functions might have recovery logic
	// This is a limitation but keeps the implementation simple as requested
	if _, ok := call.Fun.(*ast.Ident); ok {
		return true // Assume named functions handle recovery properly
	}

	// For selector expressions (e.g., obj.method()), assume they handle recovery
	if _, ok := call.Fun.(*ast.SelectorExpr); ok {
		return true
	}

	return false
}

// hasRecoverInFunction checks if a function body contains defer recover() logic
func hasRecoverInFunction(body *ast.BlockStmt, recoverFunctions map[string]bool) bool {
	if body == nil {
		return false
	}

	// Walk through all statements in the function body
	for _, stmt := range body.List {
		if deferStmt, ok := stmt.(*ast.DeferStmt); ok {
			if hasRecoverInCall(deferStmt.Call, recoverFunctions) {
				return true
			}
		}
	}
	return false
}

// hasRecoverInCall recursively checks if a call contains recover()
func hasRecoverInCall(call *ast.CallExpr, recoverFunctions map[string]bool) bool {
	if call == nil {
		return false
	}

	// Direct recover() call
	if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "recover" {
		return true
	}

	// Check if it's a call to a function that contains recover()
	if ident, ok := call.Fun.(*ast.Ident); ok {
		if recoverFunctions[ident.Name] {
			return true
		}
	}

	// Check for cross-package function calls (e.g., pkg.PanicRecover)
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		funcName := sel.Sel.Name
		// Use heuristic: functions with "recover", "panic", "safe" in name are likely recovery functions
		if isLikelyRecoveryFunction(funcName) {
			return true
		}
		// Also check if we have explicit knowledge of this function
		if pkgIdent, ok := sel.X.(*ast.Ident); ok {
			key := pkgIdent.Name + "." + funcName
			if recoverFunctions[key] {
				return true
			}
		}
	}

	// Function literal that might contain recover - this is the key pattern
	if funcLit, ok := call.Fun.(*ast.FuncLit); ok {
		return containsRecoverAnywhere(funcLit.Body)
	}

	return false
}

// isLikelyRecoveryFunction uses heuristics to determine if a function name suggests recovery logic
func isLikelyRecoveryFunction(funcName string) bool {
	lowerName := strings.ToLower(funcName)
	return strings.Contains(lowerName, "recover") ||
		strings.Contains(lowerName, "panic") ||
		strings.Contains(lowerName, "safe") ||
		strings.Contains(lowerName, "rescue") ||
		strings.Contains(lowerName, "catch")
}

// containsRecoverAnywhere does a deep search for recover() calls in any context
func containsRecoverAnywhere(node ast.Node) bool {
	found := false

	ast.Inspect(node, func(n ast.Node) bool {
		if found {
			return false // Stop traversal if already found
		}

		if call, ok := n.(*ast.CallExpr); ok {
			if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "recover" {
				found = true
				return false
			}
		}
		return true
	})

	return found
}

// hasRecoverInFunctionBody recursively searches for recover() in function body
func hasRecoverInFunctionBody(body *ast.BlockStmt) bool {
	if body == nil {
		return false
	}

	for _, stmt := range body.List {
		if hasRecoverInStatement(stmt) {
			return true
		}
	}
	return false
}

// hasRecoverInStatement checks if a statement contains recover()
func hasRecoverInStatement(stmt ast.Stmt) bool {
	switch s := stmt.(type) {
	case *ast.ExprStmt:
		return hasRecoverInExpression(s.X)
	case *ast.AssignStmt:
		for _, expr := range s.Rhs {
			if hasRecoverInExpression(expr) {
				return true
			}
		}
	case *ast.IfStmt:
		if s.Cond != nil && hasRecoverInExpression(s.Cond) {
			return true
		}
		if hasRecoverInFunctionBody(s.Body) {
			return true
		}
		if s.Else != nil && hasRecoverInStatement(s.Else) {
			return true
		}
	case *ast.BlockStmt:
		return hasRecoverInFunctionBody(s)
	}
	return false
}

// hasRecoverInExpression checks if an expression contains recover()
func hasRecoverInExpression(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.CallExpr:
		if ident, ok := e.Fun.(*ast.Ident); ok && ident.Name == "recover" {
			return true
		}
		// Check function arguments
		for _, arg := range e.Args {
			if hasRecoverInExpression(arg) {
				return true
			}
		}
	case *ast.BinaryExpr:
		return hasRecoverInExpression(e.X) || hasRecoverInExpression(e.Y)
	case *ast.UnaryExpr:
		return hasRecoverInExpression(e.X)
	}
	return false
}
