package analyzer

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// RecoverAnalyzer holds the state and methods for analyzing recover patterns
type RecoverAnalyzer struct {
	pass             *analysis.Pass
	recoverFunctions map[string]bool // funcName -> hasRecover
}

// NodeCollector collects AST nodes for analysis
type NodeCollector struct {
	FunctionDecls []*ast.FuncDecl
	GoStatements  []*ast.GoStmt
}

// CollectNodes extracts relevant nodes from the AST for analysis
func CollectNodes(insp *inspector.Inspector) *NodeCollector {
	collector := &NodeCollector{}

	// Collect function declarations
	insp.Nodes([]ast.Node{(*ast.FuncDecl)(nil)}, func(node ast.Node, push bool) bool {
		if push {
			collector.FunctionDecls = append(collector.FunctionDecls, node.(*ast.FuncDecl))
		}
		return false
	})

	// Collect go statements
	insp.Nodes([]ast.Node{(*ast.GoStmt)(nil)}, func(node ast.Node, push bool) bool {
		if push {
			collector.GoStatements = append(collector.GoStatements, node.(*ast.GoStmt))
		}
		return false
	})

	return collector
}

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
	analyzer := &RecoverAnalyzer{
		pass:             pass,
		recoverFunctions: make(map[string]bool),
	}

	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	// Collect all relevant nodes
	nodes := CollectNodes(insp)

	// Analyze in a more testable way
	analyzer.AnalyzeFunctions(nodes.FunctionDecls)
	analyzer.AnalyzeGoroutines(nodes.GoStatements)

	return nil, nil
}

// AnalyzeFunctions processes all function declarations
func (r *RecoverAnalyzer) AnalyzeFunctions(functions []*ast.FuncDecl) {
	for _, funcDecl := range functions {
		r.analyzeFunction(funcDecl)
	}
}

// AnalyzeGoroutines processes all go statements
func (r *RecoverAnalyzer) AnalyzeGoroutines(goStmts []*ast.GoStmt) {
	for _, goStmt := range goStmts {
		r.analyzeGoroutine(goStmt)
	}
}

// analyzeFunction processes a single function declaration
func (r *RecoverAnalyzer) analyzeFunction(funcDecl *ast.FuncDecl) {
	if funcDecl.Name == nil || funcDecl.Body == nil {
		return
	}

	funcName := funcDecl.Name.Name
	hasRecover := r.containsRecover(funcDecl.Body)
	r.recoverFunctions[funcName] = hasRecover
}

// analyzeGoroutine processes a single go statement
func (r *RecoverAnalyzer) analyzeGoroutine(goStmt *ast.GoStmt) {
	if goStmt.Call == nil {
		r.pass.Reportf(goStmt.Pos(), "go statement without call expression")
		return
	}

	if !r.hasRecoveryLogic(goStmt.Call) {
		r.pass.Reportf(goStmt.Pos(), "goroutine created without panic recovery")
	}
}

// hasRecoveryLogic determines if a function call includes panic recovery
func (r *RecoverAnalyzer) hasRecoveryLogic(call *ast.CallExpr) bool {
	switch fun := call.Fun.(type) {
	case *ast.FuncLit:
		return r.containsRecover(fun.Body)
	case *ast.Ident:
		return r.isRecoveryFunction(fun.Name)
	case *ast.SelectorExpr:
		return r.isCrossPackageRecoveryFunction(fun)
	}
	return false
}

// isRecoveryFunction checks if a named function contains recovery logic
func (r *RecoverAnalyzer) isRecoveryFunction(funcName string) bool {
	if hasRecover, exists := r.recoverFunctions[funcName]; exists {
		return hasRecover
	}
	// Unknown functions are assumed unsafe
	return false
}

// isCrossPackageRecoveryFunction handles pkg.Function() calls
func (r *RecoverAnalyzer) isCrossPackageRecoveryFunction(sel *ast.SelectorExpr) bool {
	funcName := sel.Sel.Name

	// Check if we have explicit knowledge of this cross-package function
	if pkgIdent, ok := sel.X.(*ast.Ident); ok {
		key := pkgIdent.Name + "." + funcName
		if hasRecover, exists := r.recoverFunctions[key]; exists {
			return hasRecover
		}

		// Try to find the actual function definition in the imported package
		if r.analyzeCrossPackageFunction(pkgIdent.Name, funcName) {
			r.recoverFunctions[key] = true
			return true
		} else {
			r.recoverFunctions[key] = false
			return false
		}
	}

	return false
}

// analyzeCrossPackageFunction analyzes a function from an imported package
func (r *RecoverAnalyzer) analyzeCrossPackageFunction(pkgName, funcName string) bool {
	// Get the package object from the type info
	if r.pass.TypesInfo == nil {
		return false
	}

	// Look for the package in the current scope
	for _, pkg := range r.pass.Pkg.Imports() {
		if pkg.Name() == pkgName {
			// Look for the function in the package scope
			if obj := pkg.Scope().Lookup(funcName); obj != nil {
				// Try to get the function declaration from the object
				if funcObj, ok := obj.(*types.Func); ok {
					// Get the position of the function to find its AST
					pos := funcObj.Pos()
					if pos.IsValid() {
						// Find the function declaration in the imported package's files
						return r.analyzeFunctionFromPosition(pkg, funcName, pos)
					}
				}
			}
		}
	}

	// If we can't find the package or function, assume it's unsafe
	return false
}

// analyzeFunctionFromPosition finds and analyzes a function from an imported package
func (r *RecoverAnalyzer) analyzeFunctionFromPosition(pkg *types.Package, funcName string, pos token.Pos) bool {
	// Get the file set from the analysis pass
	fset := r.pass.Fset
	
	// Get the position information
	position := fset.Position(pos)
	if !position.IsValid() {
		return false
	}

	// Parse the file containing the function
	file, err := parser.ParseFile(fset, position.Filename, nil, parser.ParseComments)
	if err != nil {
		// If we can't parse the file, assume it's unsafe
		return false
	}

	// Find the function declaration in the parsed file
	var funcDecl *ast.FuncDecl
	ast.Inspect(file, func(n ast.Node) bool {
		if fn, ok := n.(*ast.FuncDecl); ok {
			if fn.Name != nil && fn.Name.Name == funcName {
				funcDecl = fn
				return false // Stop searching
			}
		}
		return true
	})

	// If we found the function, analyze it for recovery logic
	if funcDecl != nil && funcDecl.Body != nil {
		return r.containsRecover(funcDecl.Body)
	}

	// If we can't find the function, assume it's unsafe
	return false
}


// containsRecover performs a deep search for recover() calls in any AST node
func (r *RecoverAnalyzer) containsRecover(node ast.Node) bool {
	return r.findRecoverCall(node)
}

// findRecoverCall recursively searches for recover() calls
func (r *RecoverAnalyzer) findRecoverCall(node ast.Node) bool {
	found := false

	ast.Inspect(node, func(n ast.Node) bool {
		if found {
			return false // Stop traversal once found
		}

		switch node := n.(type) {
		case *ast.CallExpr:
			if r.isRecoverCall(node) {
				found = true
				return false
			}
		case *ast.DeferStmt:
			if r.isDeferredRecovery(node) {
				found = true
				return false
			}
		case *ast.BlockStmt:
			// search for CallExpr and DeferStmt within the block statements
			for _, stmt := range node.List {
				if r.findRecoverCall(stmt) {
					found = true
					return false
				}
			}
		}
		return true
	})

	return found
}

// isRecoverCall checks if a call expression is a direct recover() call
func (r *RecoverAnalyzer) isRecoverCall(call *ast.CallExpr) bool {
	if ident, ok := call.Fun.(*ast.Ident); ok {
		return ident.Name == "recover"
	}
	return false
}

// isDeferredRecovery checks if a defer statement contains recovery logic
func (r *RecoverAnalyzer) isDeferredRecovery(deferStmt *ast.DeferStmt) bool {
	if deferStmt.Call == nil {
		return false
	}

	// Check for direct defer recover()
	if r.isRecoverCall(deferStmt.Call) {
		return true
	}

	// Check for defer func() { ... recover() ... }()
	if funcLit, ok := deferStmt.Call.Fun.(*ast.FuncLit); ok {
		return r.containsRecover(funcLit.Body)
	}

	// Check for defer someRecoveryFunc()
	if ident, ok := deferStmt.Call.Fun.(*ast.Ident); ok {
		return r.isRecoveryFunction(ident.Name)
	}

	// Check for defer pkg.RecoveryFunc()
	if sel, ok := deferStmt.Call.Fun.(*ast.SelectorExpr); ok {
		return r.isCrossPackageRecoveryFunction(sel)
	}

	return false
}