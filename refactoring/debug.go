// Copyright 2014 Auburn University. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file defines a "debug" refactoring, which is not really a refactoring
// at all.  It does not change any files; rather, it is invoked to print
// information about the Go refactoring engine and its internals.  For example,
// it can display the AST for a file, output a GraphViz DOT file with a file's
// control flow graphs, display what package(s) are loaded, or display what
// identifiers resolve to what objects.

package refactoring

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/printer"
	"io"
	"os"
	"sort"
	"strings"

	"golang.org/x/tools/go/loader"
	"golang.org/x/tools/go/types"

	"github.com/godoctor/godoctor/analysis/cfg"
	"github.com/godoctor/godoctor/analysis/dataflow"
	"github.com/godoctor/godoctor/analysis/names"
	"github.com/godoctor/godoctor/text"
)

const usage = `Usage: debug <options>
where <options> can be any or all of:
    fmt               Format the node enclosing the selection using go/printer
    showaffected      Show names affected if the selected identifier is renamed
    showast           Show the abstract syntax tree for the selected file
    showflow          Show GraphViz DOT flow graphs for the selected file
    showidentifiers   Show name references (ast.Object) in initial packages
    showpackages      List all packages loaded (due to --scope)
    showreferences    Show all direct references to the selected identifier`

type Debug struct {
	refactoringBase
}

func (r *Debug) Description() *Description {
	return &Description{
		Name:      "Debug Refactoring",
		Synopsis:  "Provides assorted debugging outputs",
		Usage:     "<command>",
		Multifile: false,
		Params: []Parameter{Parameter{
			Label:        "Command",
			Prompt:       "Command",
			DefaultValue: "",
		}},
		Hidden: true,
	}
}

func (r *Debug) Run(config *Config) *Result {
	r.refactoringBase.Run(config)

	r.Log.ChangeInitialErrorsToWarnings()
	if r.Log.ContainsErrors() {
		return &r.Result
	}

	if len(config.Args) == 0 {
		r.Log.Error(usage)
		return &r.Result
	}

	if !validateArgs(config, r.Description(), r.Log) {
		return &r.Result
	}
	command := strings.ToLower(strings.TrimSpace(config.Args[0].(string)))

	var b bytes.Buffer
	switch command {
	case "fmt":
		r.fmt()
	case "showaffected":
		r.showAffected(&b)
	case "showast":
		r.showAST(&b)
	case "showflow":
		r.showCFG(&b)
	case "showidentifiers":
		r.showIdentifiers(&b)
	case "showpackages":
		r.showLoadedPackagesAndFiles(&b)
	case "showreferences":
		r.showReferences(&b)
	default:
		r.Log.Errorf("Unknown option %s", command)
		return &r.Result
	}
	if !r.Log.ContainsErrors() && command != "fmt" {
		insert := &text.Extent{0, 0}
		r.Edits[r.filename].Add(insert, b.String())
	}
	return &r.Result
}

func (r *Debug) fmt() {
	// Find the smallest formattable node enclosing the selection
	_, nodes, _ := r.program.PathEnclosingInterval(r.selectionStart, r.selectionEnd)
	for _, node := range nodes {
		if canFormat(node) {
			cnode := &printer.CommentedNode{
				Node:     node,
				Comments: r.file.Comments}
			printConfig := &printer.Config{
				Mode:     printer.UseSpaces | printer.TabIndent,
				Tabwidth: 8}
			var b bytes.Buffer
			err := printConfig.Fprint(&b, r.program.Fset, cnode)
			if err != nil {
				r.Log.Error(err)
				return
			}

			r.Edits[r.filename].Add(r.extent(node), b.String())
			return
		}
	}
}

func canFormat(node interface{}) bool {
	switch node.(type) {
	case ast.Expr:
		return true
	case ast.Stmt:
		return true
	case ast.Spec:
		return true
	case ast.Decl:
		return true
	case *ast.File:
		return true
	default:
		return false
	}

}

func (r *Debug) showAffected(out io.Writer) {
	errorMsg := "Please select an identifier for showaffected"

	if r.selectedNode == nil {
		r.Log.Error(errorMsg)
		r.Log.AssociatePos(r.selectionStart, r.selectionEnd)
		return
	}
	switch id := r.selectedNode.(type) {
	case *ast.Ident:
		fmt.Fprintf(out, "Affected Declarations:\n")
		obj := r.selectedNodePkg.ObjectOf(id)
		searchResult := names.FindDeclarationsAcrossInterfaces(obj, r.program)
		result := []string{}
		for obj := range searchResult {
			p := r.program.Fset.Position(obj.Pos())
			result = append(result,
				fmt.Sprintf("  %s - %s, Line %d\n",
					obj.Name(), p.Filename, p.Line))
		}
		sort.Strings(result)
		for _, line := range result {
			fmt.Fprintf(out, line)
		}
	default:
		r.Log.Error(errorMsg)
		r.Log.AssociatePos(r.selectionStart, r.selectionEnd)
		return
	}
}

func (r *Debug) showAST(out io.Writer) {
	ast.Fprint(out, r.program.Fset, r.file, nil)
}

func (r *Debug) showCFG(out io.Writer) {
	ast.Inspect(r.file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncDecl:
			if x.Name != nil {
				fmt.Fprintf(out, "// %s\n", x.Name.Name)
			} else {
				fmt.Fprintf(out, "// (anonymous)\n")
			}
			cfg := cfg.FromFunc(x)
			cfg.PrintDot(out, r.program.Fset, r.describeDefsUses)
		}
		return true
	})
}

func (r *Debug) describeDefsUses(stmt ast.Stmt) string {
	var buf bytes.Buffer
	defs, uses := dataflow.ReferencedVars([]ast.Stmt{stmt}, r.selectedNodePkg)
	if len(defs) > 0 {
		fmt.Fprintf(&buf, "Defs: %s\n", listNames(defs))
	}
	if len(uses) > 0 {
		fmt.Fprintf(&buf, "Uses: %s\n", listNames(uses))
	}
	return strings.TrimSuffix(buf.String(), "\n")
}

func listNames(vars map[*types.Var]struct{}) string {
	names := []string{}
	for v, _ := range vars {
		names = append(names, v.Name())
	}
	sort.Strings(names)

	var buf bytes.Buffer
	for _, name := range names {
		fmt.Fprintf(&buf, ", %s", name)
	}
	return strings.TrimPrefix(buf.String(), ", ")
}

func (r *Debug) showIdentifiers(out io.Writer) {
	for _, pkgInfo := range r.program.InitialPackages() {
		for _, file := range pkgInfo.Files {
			r.showIdentifiersInFile(pkgInfo, file, out)
		}
	}
}

func (r *Debug) showIdentifiersInFile(pkgInfo *loader.PackageInfo, file *ast.File, out io.Writer) {
	fmt.Fprintf(out, "=====%s=====\n",
		r.program.Fset.Position(file.Pos()).Filename)
	ast.Inspect(file, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok {
			return true
		}

		position := r.program.Fset.Position(id.Pos())
		fmt.Fprintf(out, "%s\t(Line %d)", id.Name, position.Line)
		if obj := pkgInfo.ObjectOf(id); obj == nil {
			fmt.Fprintf(out, " does not reference an object\n")
		} else {
			fmt.Fprintf(out, " is a reference to %s (%s)\n",
				obj.Id(), r.program.Fset.Position(obj.Pos()))
		}
		return true
	})
}

func (r *Debug) showLoadedPackagesAndFiles(out io.Writer) {
	fmt.Fprintf(out, "GOPATH is %s\n", os.Getenv("GOPATH"))
	cwd, _ := os.Getwd()

	fmt.Fprintf(out, "Working directory is %s\n", cwd)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Packages/files loaded:")
	for _, pkgInfo := range r.program.AllPackages {
		fmt.Fprintf(out, "\t%s\n", pkgInfo.Pkg.Name())
		for _, file := range pkgInfo.Files {
			filename := r.program.Fset.Position(file.Pos()).Filename
			fmt.Fprintf(out, "\t\t%s\n", filename)
		}
	}
}

func (r *Debug) showReferences(out io.Writer) {
	errorMsg := "Please select an identifier for showreferences"

	if r.selectedNode == nil {
		r.Log.Error(errorMsg)
		r.Log.AssociatePos(r.selectionStart, r.selectionEnd)
		return
	}
	switch id := r.selectedNode.(type) {
	case *ast.Ident:
		fmt.Fprintf(out, "References to %s:\n", id.Name)
		ids := names.FindOccurrences(r.selectedNodePkg.ObjectOf(id), r.program)
		strs := []string{}
		for id, _ := range ids {
			description := fmt.Sprintf("  %s: %s",
				r.program.Fset.Position(id.Pos()).Filename,
				r.extent(id).String())
			strs = append(strs, description)
		}
		sort.Strings(strs)
		for _, s := range strs {
			fmt.Fprintln(out, s)
		}
	default:
		r.Log.Error(errorMsg)
		r.Log.AssociatePos(r.selectionStart, r.selectionEnd)
		return
	}
}
