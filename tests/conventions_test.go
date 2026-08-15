package tests

// 用 AST 扫描源码，禁止 dto 使用 gin binding tag，以及 dto 实现 validate.Validatable。

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// repoRoot 返回仓库根目录。
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("获取工作目录失败: %v", err)
	}
	return filepath.Dir(wd) // tests/ 的上一级
}

// walkGoFiles 遍历 dirs 下的非测试 .go 文件并解析 AST。
func walkGoFiles(t *testing.T, root string, dirs []string, fn func(fset *token.FileSet, file *ast.File, path string)) {
	t.Helper()
	fset := token.NewFileSet()

	for _, dir := range dirs {
		full := filepath.Join(root, dir)
		if _, err := os.Stat(full); os.IsNotExist(err) {
			continue // internal/example 之外，业务方可能还没建这个目录，跳过不算错误
		}

		err := filepath.WalkDir(full, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "vendor" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}

			file, perr := parser.ParseFile(fset, path, nil, parser.ParseComments)
			if perr != nil {
				return perr
			}
			fn(fset, file, path)
			return nil
		})
		if err != nil {
			t.Fatalf("遍历 %s 失败: %v", full, err)
		}
	}
}

// TestNoBindingTag 验证源码不使用 gin 的 binding tag，应使用 validate tag。
func TestNoBindingTag(t *testing.T) {
	root := repoRoot(t)
	var violations []string

	walkGoFiles(t, root, []string{"internal", "pkg"}, func(fset *token.FileSet, file *ast.File, path string) {
		ast.Inspect(file, func(n ast.Node) bool {
			field, ok := n.(*ast.Field)
			if !ok || field.Tag == nil {
				return true
			}
			raw, err := strconv.Unquote(field.Tag.Value)
			if err != nil {
				return true
			}
			if strings.Contains(raw, `binding:"`) {
				pos := fset.Position(field.Tag.Pos())
				rel, _ := filepath.Rel(root, path)
				violations = append(violations, pos.String()+" ("+rel+"): "+raw)
			}
			return true
		})
	})

	if len(violations) > 0 {
		t.Fatalf(
			"发现禁止使用的 gin `binding:` tag，请改成 `validate:` tag（见 docs/api-conventions.md 第 2 节）：\n%s",
			strings.Join(violations, "\n"),
		)
	}
}

// TestDTOPackagesDoNotImplementValidatable 验证 dto 包不实现 Validate() error。
func TestDTOPackagesDoNotImplementValidatable(t *testing.T) {
	root := repoRoot(t)
	var violations []string

	walkGoFiles(t, root, []string{"internal"}, func(fset *token.FileSet, file *ast.File, path string) {
		rel, _ := filepath.Rel(root, path)
		if !strings.Contains(filepath.ToSlash(rel), "/dto/") {
			return
		}

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Name.Name != "Validate" {
				continue
			}
			if fn.Type.Params != nil && len(fn.Type.Params.List) > 0 {
				continue // 有参数就不是 Validatable 要求的 Validate() 签名
			}
			if fn.Type.Results == nil || len(fn.Type.Results.List) != 1 {
				continue
			}
			ident, ok := fn.Type.Results.List[0].Type.(*ast.Ident)
			if !ok || ident.Name != "error" {
				continue
			}
			pos := fset.Position(fn.Pos())
			violations = append(violations, pos.String()+" ("+rel+")")
		}
	})

	if len(violations) > 0 {
		t.Fatalf(
			"dto 不应该实现 validate.Validatable（`func (x T) Validate() error`），"+
				"跨字段/业务规则请放进 validation 包并由 handler 显式调用（见 docs/api-conventions.md 第 2.1 节）：\n%s",
			strings.Join(violations, "\n"),
		)
	}
}
