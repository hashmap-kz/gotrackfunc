package gtf

import (
	"bytes"
	"go/ast"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/require"
)

func TestDetectModulePath(t *testing.T) {
	tmpDir := t.TempDir()

	//nolint:goconst
	goModContent := "module github.com/example/testmod\n"
	err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goModContent), 0o600)
	require.NoError(t, err)

	oldWd, err := os.Getwd()
	require.NoError(t, err)
	defer func(dir string) {
		err := os.Chdir(dir)
		if err != nil {
			t.Log(err)
		}
	}(oldWd)
	require.NoError(t, os.Chdir(tmpDir))

	modulePath, err := detectModulePath()
	require.NoError(t, err)
	assert.Equal(t, "github.com/example/testmod", modulePath)
}

func TestShouldSkip(t *testing.T) {
	fd := &ast.FuncDecl{
		Doc: &ast.CommentGroup{
			List: []*ast.Comment{
				{Text: "// notrack"},
			},
		},
	}
	assert.True(t, shouldSkip(fd))

	fd2 := &ast.FuncDecl{
		Doc: &ast.CommentGroup{
			List: []*ast.Comment{
				{Text: "// some other comment"},
			},
		},
	}
	assert.False(t, shouldSkip(fd2))
}

func TestAlreadyHasDefer(t *testing.T) {
	fd := &ast.FuncDecl{
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				&ast.DeferStmt{
					Call: &ast.CallExpr{
						Fun: &ast.CallExpr{
							Fun: &ast.SelectorExpr{
								X:   ast.NewIdent("gotrackfunc"),
								Sel: ast.NewIdent("Hook"),
							},
						},
					},
				},
			},
		},
	}

	assert.True(t, alreadyHasDefer(fd))

	fd2 := &ast.FuncDecl{
		Body: &ast.BlockStmt{
			List: []ast.Stmt{},
		},
	}

	assert.False(t, alreadyHasDefer(fd2))
}

func TestProcessFile_SimpleInject(t *testing.T) {
	tmpDir := t.TempDir()

	input := `package foo

func Bar() {}
`

	inputPath := filepath.Join(tmpDir, "input.go")
	err := os.WriteFile(inputPath, []byte(input), 0o600)
	require.NoError(t, err)

	goModContent := "module github.com/example/testmod\n"
	err = os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goModContent), 0o600)
	require.NoError(t, err)

	oldWd, err := os.Getwd()
	require.NoError(t, err)
	defer func(dir string) {
		err := os.Chdir(dir)
		if err != nil {
			t.Log(err)
		}
	}(oldWd)
	require.NoError(t, os.Chdir(tmpDir))

	err = processFile(inputPath)
	require.NoError(t, err)

	output, err := os.ReadFile(inputPath)
	require.NoError(t, err)

	expectedContains := `defer gotrackfunc.Hook("foo.Bar", time.Now())()`
	assert.Contains(t, string(output), expectedContains)
}

func TestSummarizeTrackLog(t *testing.T) {
	tmpDir := t.TempDir()

	logContent := `foo.Bar 1 1000000
foo.Bar 1 2000000
baz.Qux 1 3000000
`

	logPath := filepath.Join(tmpDir, "gotrackfunc.log")
	err := os.WriteFile(logPath, []byte(logContent), 0o600)
	require.NoError(t, err)

	oldWd, err := os.Getwd()
	require.NoError(t, err)
	defer func(dir string) {
		err := os.Chdir(dir)
		if err != nil {
			t.Log(err)
		}
	}(oldWd)
	require.NoError(t, os.Chdir(tmpDir))

	// Redirect stdout temporarily to capture summarizeTrackLog output
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	summarizeTrackLog()

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "foo.Bar")
	assert.Contains(t, output, "baz.Qux")
	assert.Contains(t, output, "TOTAL_SEC")
}

func TestExpandDotDotDot(t *testing.T) {
	tmpDir := t.TempDir()

	// Create go.mod so that packages.Load does not fail
	goModContent := "module github.com/example/testmod\n"
	err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goModContent), 0o600)
	require.NoError(t, err)

	// Create a couple of .go files
	file1 := `package foo
func A() {}
`
	file2 := `package foo
func B() {}
`

	err = os.WriteFile(filepath.Join(tmpDir, "file1.go"), []byte(file1), 0o600)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(tmpDir, "file2.go"), []byte(file2), 0o600)
	require.NoError(t, err)

	// Also create a _test.go file which should NOT be included
	testFile := `package foo
func TestA() {}
`

	err = os.WriteFile(filepath.Join(tmpDir, "file_test.go"), []byte(testFile), 0o600)
	require.NoError(t, err)

	// Switch to temp dir so ./... works as intended
	oldWd, err := os.Getwd()
	require.NoError(t, err)
	defer func(dir string) {
		err := os.Chdir(dir)
		if err != nil {
			t.Log(err)
		}
	}(oldWd)
	require.NoError(t, os.Chdir(tmpDir))

	files, err := expandDotDotDot()
	require.NoError(t, err)

	// Sanity check: should contain file1.go and file2.go, but not file_test.go
	assert.Contains(t, files, filepath.Join(tmpDir, "file1.go"))
	assert.Contains(t, files, filepath.Join(tmpDir, "file2.go"))

	for _, f := range files {
		assert.False(t, strings.HasSuffix(f, "_test.go"), "should not include test files")
	}
}
