package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"go/ast"
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
	"golang.org/x/tools/imports"
)

func main() {
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		log.Fatal("Usage: gotrackfunc ./... or gotrackfunc file.go")
	}

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
	".git":    true,
	".github": true,
	".idea":   true,
	"vendor":  true,
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
		ensureImport(f, "fmt")
		ensureImport(f, "os")
	}

	var buf bytes.Buffer
	cfg := &printer.Config{Mode: printer.UseSpaces | printer.TabIndent, Tabwidth: 8}

	err = cfg.Fprint(&buf, fset, f)
	if err != nil {
		return fmt.Errorf("print: %w", err)
	}

	out, err := imports.Process(filename, buf.Bytes(), nil)
	if err != nil {
		return fmt.Errorf("imports.Process: %w", err)
	}

	err = os.WriteFile(filename, out, 0o644)
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
			Fun: &ast.FuncLit{
				Type: &ast.FuncType{
					Params: &ast.FieldList{
						List: []*ast.Field{
							{
								Names: []*ast.Ident{ast.NewIdent("start")},
								Type: &ast.SelectorExpr{
									X:   ast.NewIdent("time"),
									Sel: ast.NewIdent("Time"),
								},
							},
						},
					},
				},
				Body: &ast.BlockStmt{
					List: []ast.Stmt{
						&ast.AssignStmt{
							Lhs: []ast.Expr{ast.NewIdent("elapsed")},
							Tok: token.DEFINE,
							Rhs: []ast.Expr{
								&ast.CallExpr{
									Fun: &ast.SelectorExpr{
										X:   ast.NewIdent("time"),
										Sel: ast.NewIdent("Since"),
									},
									Args: []ast.Expr{
										ast.NewIdent("start"),
									},
								},
							},
						},
						&ast.AssignStmt{
							Lhs: []ast.Expr{ast.NewIdent("f"), ast.NewIdent("_")},
							Tok: token.DEFINE,
							Rhs: []ast.Expr{
								&ast.CallExpr{
									Fun: &ast.SelectorExpr{
										X:   ast.NewIdent("os"),
										Sel: ast.NewIdent("OpenFile"),
									},
									Args: []ast.Expr{
										&ast.BasicLit{
											Kind:  token.STRING,
											Value: strconv.Quote("gotrackfunc.log"),
										},
										&ast.BinaryExpr{
											X: &ast.SelectorExpr{
												X:   ast.NewIdent("os"),
												Sel: ast.NewIdent("O_APPEND"),
											},
											Op: token.OR,
											Y: &ast.BinaryExpr{
												X: &ast.SelectorExpr{
													X:   ast.NewIdent("os"),
													Sel: ast.NewIdent("O_CREATE"),
												},
												Op: token.OR,
												Y: &ast.SelectorExpr{
													X:   ast.NewIdent("os"),
													Sel: ast.NewIdent("O_WRONLY"),
												},
											},
										},
										&ast.BasicLit{
											Kind:  token.INT,
											Value: "0644",
										},
									},
								},
							},
						},
						&ast.IfStmt{
							Cond: &ast.BinaryExpr{
								X:  ast.NewIdent("f"),
								Op: token.NEQ,
								Y:  ast.NewIdent("nil"),
							},
							Body: &ast.BlockStmt{
								List: []ast.Stmt{
									&ast.ExprStmt{
										X: &ast.CallExpr{
											Fun: &ast.SelectorExpr{
												X:   ast.NewIdent("fmt"),
												Sel: ast.NewIdent("Fprintf"),
											},
											Args: []ast.Expr{
												ast.NewIdent("f"),
												&ast.BasicLit{
													Kind:  token.STRING,
													Value: strconv.Quote("%s 1 %d\n"),
												},
												&ast.BasicLit{
													Kind:  token.STRING,
													Value: strconv.Quote(funcFullName),
												},
												&ast.CallExpr{
													Fun: &ast.SelectorExpr{
														X:   ast.NewIdent("elapsed"),
														Sel: ast.NewIdent("Nanoseconds"),
													},
													Args: []ast.Expr{},
												},
											},
										},
									},
									&ast.ExprStmt{
										X: &ast.CallExpr{
											Fun: &ast.SelectorExpr{
												X:   ast.NewIdent("f"),
												Sel: ast.NewIdent("Close"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			Args: []ast.Expr{
				&ast.CallExpr{
					Fun: &ast.SelectorExpr{
						X:   ast.NewIdent("time"),
						Sel: ast.NewIdent("Now"),
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

	// Check if defer contains "start" param (means it's our injected one)
	callExpr, ok := first.Call.Fun.(*ast.FuncLit)
	if !ok || callExpr.Type == nil || callExpr.Type.Params == nil {
		return false
	}
	for _, param := range callExpr.Type.Params.List {
		if len(param.Names) > 0 && param.Names[0].Name == "start" {
			return true
		}
	}
	return false
}

func ensureImport(f *ast.File, pkg string) {
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
	f.Imports = append(f.Imports, newImp)
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

	// Prepare slice of names for sorting
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

	// Sort by sum DESC
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].sum > entries[j].sum
	})

	// Output with tabwriter
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "FUNCTION\tCALLS\tTOTAL_NS\tTOTAL_SEC")
	fmt.Fprintln(w, "--------\t-----\t--------\t---------")

	for _, e := range entries {
		fmt.Fprintf(w, "%s\t%d\t%d\t%.2f\n", e.name, e.count, e.sum, float64(e.sum)/1e9)
	}

	w.Flush()
}
