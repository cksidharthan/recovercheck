package recovercheck

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// RecovercheckSettings holds configuration options for the analyzer
type RecovercheckSettings struct {
	// to be used in future.
}

// Analyzer holds the state and methods for analyzing recover patterns
type Analyzer struct {
	Pass             *analysis.Pass
	RecoverFunctions map[string]bool // funcName -> hasRecover
	Settings         *RecovercheckSettings
}

// NodeCollector collects AST nodes for analysis
type NodeCollector struct {
	FunctionDecls []*ast.FuncDecl
	GoStatements  []*ast.GoStmt
	ErrgroupCalls []*ast.CallExpr // errgroup.Group.Go() calls
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

	// Collect errgroup calls (method calls that might be errgroup.Group.Go())
	insp.Nodes([]ast.Node{(*ast.CallExpr)(nil)}, func(node ast.Node, push bool) bool {
		if push {
			call := node.(*ast.CallExpr)
			if isErrgroupGoCall(call) {
				collector.ErrgroupCalls = append(collector.ErrgroupCalls, call)
			}
		}
		return false
	})

	return collector
}

// isErrgroupGoCall checks if a call expression is an errgroup.Group.Go() call
func isErrgroupGoCall(call *ast.CallExpr) bool {
	// Look for method calls like g.Go() where g might be an errgroup.Group
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		// Check if the method name is "Go"
		if sel.Sel.Name == "Go" {
			// We'll do more sophisticated type checking later
			// For now, assume any .Go() call might be errgroup
			return true
		}
	}
	return false
}

// New returns new recovercheck analyzer.
func New(settings *RecovercheckSettings) *analysis.Analyzer {
	analyzer := &analysis.Analyzer{
		Name:     "recovercheck",
		Doc:      "Checks that goroutines have panic recovery logic",
		Requires: []*analysis.Analyzer{inspect.Analyzer},
	}

	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		return run(pass, settings)
	}

	return analyzer
}

func run(pass *analysis.Pass, config *RecovercheckSettings) (any, error) {
	analyzer := &Analyzer{
		Pass:             pass,
		RecoverFunctions: make(map[string]bool),
		Settings:         config,
	}

	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	// Collect all relevant nodes
	nodes := CollectNodes(insp)

	analyzer.AnalyzeFunctions(nodes.FunctionDecls)
	analyzer.AnalyzeGoroutines(nodes.GoStatements)
	analyzer.AnalyzeErrgroupCalls(nodes.ErrgroupCalls)

	return nil, nil
}

// AnalyzeFunctions processes all function declarations
func (r *Analyzer) AnalyzeFunctions(functions []*ast.FuncDecl) {
	for _, funcDecl := range functions {
		r.analyzeFunction(funcDecl)
	}
}

// AnalyzeGoroutines processes all go statements
func (r *Analyzer) AnalyzeGoroutines(goStmts []*ast.GoStmt) {
	for _, goStmt := range goStmts {
		r.analyzeGoroutine(goStmt)
	}
}

// AnalyzeErrgroupCalls processes all errgroup.Group.Go() calls
func (r *Analyzer) AnalyzeErrgroupCalls(calls []*ast.CallExpr) {
	for _, call := range calls {
		r.analyzeErrgroupCall(call)
	}
}

// analyzeFunction processes a single function declaration
func (r *Analyzer) analyzeFunction(funcDecl *ast.FuncDecl) {
	if funcDecl.Name == nil || funcDecl.Body == nil {
		return
	}

	funcName := funcDecl.Name.Name
	hasRecover := r.containsRecover(funcDecl.Body)
	r.RecoverFunctions[funcName] = hasRecover
}

// analyzeGoroutine processes a single go statement
func (r *Analyzer) analyzeGoroutine(goStmt *ast.GoStmt) {
	if goStmt.Call == nil {
		r.Pass.Reportf(goStmt.Pos(), "go statement without call expression")
		return
	}

	if !r.hasRecoveryLogic(goStmt.Call) {
		r.Pass.Reportf(goStmt.Pos(), "goroutine created without panic recovery")
	}
}

// analyzeErrgroupCall processes a single errgroup.Group.Go() call
func (r *Analyzer) analyzeErrgroupCall(call *ast.CallExpr) {
	// Errgroup.Go() calls take a function as their first argument
	if len(call.Args) == 0 {
		return
	}

	// The first argument should be a function literal that will be executed in a goroutine
	if funcLit, ok := call.Args[0].(*ast.FuncLit); ok {
		if !r.containsRecover(funcLit.Body) {
			r.Pass.Reportf(call.Pos(), "errgroup goroutine created without panic recovery")
		}
	} else {
		// If it's not a function literal, it might be a function reference
		// We need to check if that function has recovery logic
		if !r.hasRecoveryLogic(&ast.CallExpr{Fun: call.Args[0]}) {
			r.Pass.Reportf(call.Pos(), "errgroup goroutine created without panic recovery")
		}
	}
}

// hasRecoveryLogic determines if a function call includes panic recovery
func (r *Analyzer) hasRecoveryLogic(call *ast.CallExpr) bool {
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
func (r *Analyzer) isRecoveryFunction(funcName string) bool {
	if hasRecover, exists := r.RecoverFunctions[funcName]; exists {
		return hasRecover
	}
	// Unknown functions are assumed unsafe
	return false
}

// isCrossPackageRecoveryFunction handles pkg.Function() calls
func (r *Analyzer) isCrossPackageRecoveryFunction(sel *ast.SelectorExpr) bool {
	funcName := sel.Sel.Name

	// Check if we have explicit knowledge of this cross-package function
	if pkgIdent, ok := sel.X.(*ast.Ident); ok {
		key := pkgIdent.Name + "." + funcName
		if hasRecover, exists := r.RecoverFunctions[key]; exists {
			return hasRecover
		}

		// Use type information to resolve the actual package (handles both regular and aliased imports)
		var hasRecovery bool
		if r.Pass.TypesInfo != nil {
			if obj, ok := r.Pass.TypesInfo.Uses[pkgIdent]; ok {
				if pkgName, ok := obj.(*types.PkgName); ok {
					hasRecovery = r.analyzeCrossPackageFunction(pkgName.Imported(), funcName)
				}
			}
		}

		r.RecoverFunctions[key] = hasRecovery
		return hasRecovery
	}

	return false
}

// analyzeCrossPackageFunction analyzes a function from an imported package
func (r *Analyzer) analyzeCrossPackageFunction(pkg *types.Package, funcName string) bool {
	// Look for the function in the package scope
	if obj := pkg.Scope().Lookup(funcName); obj != nil {
		// Try to get the function declaration from the object
		if funcObj, ok := obj.(*types.Func); ok {
			// Get the position of the function to find its AST
			pos := funcObj.Pos()
			if pos.IsValid() {
				// Find the function declaration in the imported package's files
				return r.analyzeFunctionFromPosition(funcName, pos)
			}
		}
	}
	return false
}

// analyzeFunctionFromPosition finds and analyzes a function from an imported package
func (r *Analyzer) analyzeFunctionFromPosition(funcName string, pos token.Pos) bool {
	// Get the file set from the analysis pass
	fset := r.Pass.Fset

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
func (r *Analyzer) containsRecover(node ast.Node) bool {
	return r.findRecoverCall(node)
}

// findRecoverCall recursively searches for recover() calls
func (r *Analyzer) findRecoverCall(node ast.Node) bool {
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
func (r *Analyzer) isRecoverCall(call *ast.CallExpr) bool {
	if ident, ok := call.Fun.(*ast.Ident); ok {
		return ident.Name == "recover"
	}
	return false
}

// isDeferredRecovery checks if a defer statement contains recovery logic
func (r *Analyzer) isDeferredRecovery(deferStmt *ast.DeferStmt) bool {
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
