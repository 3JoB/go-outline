package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"

	"github.com/3JoB/ulib/json"
	"golang.org/x/tools/go/buildutil"
)

type Declaration struct {
	Label        string        `json:"label"`
	Type         string        `json:"type"`
	ReceiverType string        `json:"receiverType,omitempty"`
	Start        token.Pos     `json:"start"`
	End          token.Pos     `json:"end"`
	Children     []Declaration `json:"children,omitempty"`
}

var (
	file        = flag.String("f", "", "the path to the file to outline")
	importsOnly = flag.Bool("imports-only", false, "parse imports only")
	modified    = flag.Bool("modified", false, "read an archive of the modified file from standard input")
)

func main() {
	flag.Parse()
	fset := token.NewFileSet()
	parserMode := parser.ParseComments
	if *importsOnly {
		parserMode = parser.ImportsOnly
	}

	var fileAst *ast.File
	var err error

	if *modified {
		archive, err := buildutil.ParseOverlayArchive(os.Stdin)
		if err != nil {
			reportError(fmt.Errorf("failed to parse -modified archive: %v", err))
		}
		fc, ok := archive[*file]
		if !ok {
			reportError(fmt.Errorf("couldn't find %s in archive", *file))
		}
		fileAst, err = parser.ParseFile(fset, *file, fc, parserMode)
	} else {
		fileAst, err = parser.ParseFile(fset, *file, nil, parserMode)
	}

	if err != nil {
		reportError(fmt.Errorf("could not parse file %s", *file))
	}

	declarations := []Declaration{}

	for _, decl := range fileAst.Decls {
		switch decl := decl.(type) {
		case *ast.FuncDecl:
			receiverType, err := getReceiverType(fset, decl)
			if err != nil {
				reportError(fmt.Errorf("failed to parse receiver type: %v", err))
			}
			declarations = append(declarations, Declaration{
				Label:        decl.Name.String(),
				Type:         "function",
				ReceiverType: receiverType,
				Start:        decl.Pos(),
				End:          decl.End(),
				Children:     []Declaration{},
			})
		case *ast.GenDecl:
			for _, spec := range decl.Specs {
				switch spec := spec.(type) {
				case *ast.ImportSpec:
					declarations = append(declarations, Declaration{
						Label:        spec.Path.Value,
						Type:         "import",
						ReceiverType: "",
						Start:        spec.Pos(),
						End:          spec.End(),
						Children:     []Declaration{},
					})
				case *ast.TypeSpec:
					// TODO: Members if it's a struct or interface type?
					declarations = append(declarations, Declaration{
						Label:        spec.Name.String(),
						Type:         "type",
						ReceiverType: "",
						Start:        spec.Pos(),
						End:          spec.End(),
						Children:     []Declaration{},
					})
				case *ast.ValueSpec:
					for _, id := range spec.Names {
						varOrConst := "variable"
						if decl.Tok == token.CONST {
							varOrConst = "constant"
						}
						declarations = append(declarations, Declaration{
							Label:        id.Name,
							Type:         varOrConst,
							ReceiverType: "",
							Start:        id.Pos(),
							End:          id.End(),
							Children:     []Declaration{},
						})
					}
				default:
					reportError(fmt.Errorf("unknown token type: %s", decl.Tok))
				}
			}
		default:
			reportError(fmt.Errorf("unknown declaration @ %v", decl.Pos()))
		}
	}

	pkg := []*Declaration{{
		Label:        fileAst.Name.String(),
		Type:         "package",
		ReceiverType: "",
		Start:        fileAst.Pos(),
		End:          fileAst.End(),
		Children:     declarations,
	}}

	fmt.Println(json.Marshal(pkg).String())
}

func getReceiverType(fset *token.FileSet, decl *ast.FuncDecl) (string, error) {
	if decl.Recv == nil {
		return "", nil
	}

	buf := &bytes.Buffer{}
	if err := format.Node(buf, fset, decl.Recv.List[0].Type); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func reportError(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
}
