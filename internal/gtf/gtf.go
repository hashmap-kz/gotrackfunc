package gtf

import (
	"bufio"
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"golang.org/x/tools/go/packages"
)

func RunApp(args []string) {
	if args[0] == "summarize" {
		summarizeTrackLog()
		return
	}

	for _, arg := range args {
		var files []string
		var err error

		if arg == "./..." {
			files, err = expandDotDotDot()
		} else {
			files = []string{arg}
		}

		if err != nil {
			log.Fatalf("Failed to expand %s: %v", arg, err)
		}

		for _, f := range files {
			log.Printf("Processing %s", f)
			if err := processFile(f); err != nil {
				log.Printf("Error processing %s: %v", f, err)
			}
		}
	}
}

func expandDotDotDot() ([]string, error) {
	var files []string

	cfg := &packages.Config{
		Mode:  packages.NeedFiles,
		Tests: false,
	}
	pkgs, err := packages.Load(cfg, "./...")
	if err == nil && len(pkgs) > 0 {
		for _, pkg := range pkgs {
			files = append(files, pkg.GoFiles...)
		}
		return files, nil
	}

	log.Printf("Fallback: WalkDir for ./...")
	err = filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			base := filepath.Base(path)
			if skipDirs[base] {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	return files, err
}

var skipDirs = map[string]bool{
	".git":        true,
	".github":     true,
	".idea":       true,
	"vendor":      true,
	"gotrackfunc": true, // don't process the hook package itself!
}

func detectModulePath() (string, error) {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module ")), nil
		}
	}
	return "", fmt.Errorf("module path not found in go.mod")
}

func processFile(filename string) error {
	fset := token.NewFileSet()
	src, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}

	f, err := parser.ParseFile(fset, filename, src, parser.AllErrors)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	injectedAny := false
	pkgName := f.Name.Name

	ast.Inspect(f, func(n ast.Node) bool {
		fd, ok := n.(*ast.FuncDecl)
		if !ok || fd.Body == nil {
			return true
		}

		if shouldSkip(fd) {
			return true
		}

		if alreadyHasDefer(fd) {
			return true
		}

		injectTimingDefer(fd, pkgName)
		injectedAny = true
		return true
	})

	if injectedAny {
		ensureImport(f, "time")
		modulePath, err := detectModulePath()
		if err != nil {
			return fmt.Errorf("cannot detect module path: %w", err)
		}
		ensureImport(f, modulePath+"/gotrackfunc")
		fmt.Fprintln(os.Stderr, modulePath)
		ensureGotrackfuncPackage()
	}

	// Write result
	var buf bytes.Buffer
	printer.Fprint(&buf, fset, f)

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return fmt.Errorf("format.Source: %w", err)
	}

	err = os.WriteFile(filename, formatted, 0o644)
	if err != nil {
		return fmt.Errorf("write: %w", err)
	}

	return nil
}

func shouldSkip(fd *ast.FuncDecl) bool {
	if fd.Doc != nil {
		for _, c := range fd.Doc.List {
			if strings.Contains(c.Text, "notrack") {
				return true
			}
		}
	}
	return false
}

func injectTimingDefer(fd *ast.FuncDecl, pkgName string) {
	funcFullName := pkgName + "." + fd.Name.Name

	timingDefer := &ast.DeferStmt{
		Call: &ast.CallExpr{
			Fun: &ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   ast.NewIdent("gotrackfunc"),
					Sel: ast.NewIdent("Hook"),
				},
				Args: []ast.Expr{
					&ast.BasicLit{
						Kind:  token.STRING,
						Value: strconv.Quote(funcFullName),
					},
					&ast.CallExpr{
						Fun: &ast.SelectorExpr{
							X:   ast.NewIdent("time"),
							Sel: ast.NewIdent("Now"),
						},
					},
				},
			},
		},
	}

	fd.Body.List = append([]ast.Stmt{timingDefer}, fd.Body.List...)
}

func alreadyHasDefer(fd *ast.FuncDecl) bool {
	if len(fd.Body.List) == 0 {
		return false
	}

	first, ok := fd.Body.List[0].(*ast.DeferStmt)
	if !ok {
		return false
	}

	callExpr := first.Call

	nestedCallExpr, ok := callExpr.Fun.(*ast.CallExpr)
	if !ok {
		return false
	}

	sel, ok := nestedCallExpr.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	pkgIdent, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}

	return pkgIdent.Name == "gotrackfunc" && sel.Sel.Name == "Hook"
}

func ensureImport(f *ast.File, pkg string) {
	// Already present?
	for _, imp := range f.Imports {
		if strings.Trim(imp.Path.Value, `"`) == pkg {
			return
		}
	}

	newImp := &ast.ImportSpec{
		Path: &ast.BasicLit{
			Kind:  token.STRING,
			Value: strconv.Quote(pkg),
		},
	}

	// Try to find an existing import decl
	for _, decl := range f.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.IMPORT {
			continue
		}

		// Append to existing import block
		genDecl.Specs = append(genDecl.Specs, newImp)
		return
	}

	// No import block — create one
	newGenDecl := &ast.GenDecl{
		Tok: token.IMPORT,
		Specs: []ast.Spec{
			newImp,
		},
	}

	// Prepend import block to Decls
	f.Decls = append([]ast.Decl{newGenDecl}, f.Decls...)
}

func ensureGotrackfuncPackage() {
	path := filepath.Join("gotrackfunc", "gotrack.go")

	if _, err := os.Stat(path); err == nil {
		// Already exists, nothing to do
		return
	}

	log.Printf("Creating %s", path)

	err := os.MkdirAll(filepath.Dir(path), 0o755)
	if err != nil {
		log.Fatalf("failed to create gotrackfunc dir: %v", err)
	}

	const source = `package gotrackfunc

import (
    "fmt"
    "os"
    "sync"
    "time"
)

type trackEntry struct {
    funcName string
    elapsed  time.Duration
}

var (
    mu      sync.Mutex
    ch      chan trackEntry
    started bool
)

func Hook(funcName string, start time.Time) func() {
    ensureStarted()

    return func() {
        elapsed := time.Since(start)
        ch <- trackEntry{funcName, elapsed}
    }
}

func ensureStarted() {
    mu.Lock()
    defer mu.Unlock()

    if started {
        return
    }

    ch = make(chan trackEntry, 1000) // buffered
    go writer()
    started = true
}

func writer() {
    f, err := os.OpenFile("gotrackfunc.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
    if err != nil {
        // If cannot open -> disable logging
        fmt.Fprintf(os.Stderr, "gotrackfunc: failed to open log file: %v\n", err)
        return
    }
    defer f.Close()

    for entry := range ch {
        _, _ = fmt.Fprintf(f, "%s 1 %d\n", entry.funcName, entry.elapsed.Nanoseconds())
    }
}
`

	err = os.WriteFile(path, []byte(source), 0o644)
	if err != nil {
		log.Fatalf("failed to write gotrack.go: %v", err)
	}
}

// subcommands

func summarizeTrackLog() {
	f, err := os.Open("gotrackfunc.log")
	if err != nil {
		log.Fatalf("cannot open log: %v", err)
	}
	defer f.Close()

	counts := map[string]int{}
	sums := map[string]int64{}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var name string
		var count int
		var elapsed int64
		fmt.Sscanf(scanner.Text(), "%s %d %d", &name, &count, &elapsed)
		counts[name] += count
		sums[name] += elapsed
	}
	if err := scanner.Err(); err != nil {
		log.Fatalf("scan error: %v", err)
	}

	type entry struct {
		name  string
		count int
		sum   int64
	}
	var entries []entry
	for name := range counts {
		entries = append(entries, entry{
			name:  name,
			count: counts[name],
			sum:   sums[name],
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].sum > entries[j].sum
	})

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "FUNCTION\tCALLS\tTOTAL_NS\tTOTAL_SEC")
	fmt.Fprintln(w, "--------\t-----\t--------\t---------")

	for _, e := range entries {
		fmt.Fprintf(w, "%s\t%d\t%d\t%.2f\n", e.name, e.count, e.sum, float64(e.sum)/1e9)
	}

	w.Flush()
}
