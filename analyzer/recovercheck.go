package analyzer

import (
	"go/ast"
	"slices"

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
		if !hasRecoverLogic(goStmt.Call) {
			pass.Reportf(goStmt.Pos(), "goroutine created without panic recovery")
		}

		return false
	})

	return nil, nil
}

// hasRecoverLogic checks if a function call or function literal contains defer recover() logic
func hasRecoverLogic(call *ast.CallExpr) bool {
	// Check if it's a function literal (anonymous function)
	if funcLit, ok := call.Fun.(*ast.FuncLit); ok {
		return hasRecoverInFunction(funcLit.Body)
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
func hasRecoverInFunction(body *ast.BlockStmt) bool {
	if body == nil {
		return false
	}

	// Walk through all statements in the function body
	for _, stmt := range body.List {
		if deferStmt, ok := stmt.(*ast.DeferStmt); ok {
			if hasRecoverInCall(deferStmt.Call) {
				return true
			}
		}
	}
	return false
}

// hasRecoverInCall recursively checks if a call contains recover()
func hasRecoverInCall(call *ast.CallExpr) bool {
	if call == nil {
		return false
	}

	// Direct recover() call
	if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "recover" {
		return true
	}

	// Function literal that might contain recover - this is the key pattern
	if funcLit, ok := call.Fun.(*ast.FuncLit); ok {
		return containsRecoverAnywhere(funcLit.Body)
	}

	return false
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

	return slices.ContainsFunc(body.List, hasRecoverInStatement)
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
		if slices.ContainsFunc(e.Args, hasRecoverInExpression) {
			return true
		}
	case *ast.BinaryExpr:
		return hasRecoverInExpression(e.X) || hasRecoverInExpression(e.Y)
	case *ast.UnaryExpr:
		return hasRecoverInExpression(e.X)
	}
	return false
}
