package gore

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"text/tabwriter"
	"time"
	"unicode"

	"go/ast"
	"go/build"
	"go/parser"
	"go/types"

	"golang.org/x/tools/go/ast/astutil"
)

type command struct {
	name     commandName
	action   func(*Session, string) error
	complete func(*Session, string) []string
	arg      string
	document string
}

var commands []command

func init() {
	commands = []command{
		{
			name:     commandName("i[mport]"),
			action:   actionImport,
			complete: completeImport,
			arg:      "<package>",
			document: "import a package",
		},
		{
			name:     commandName("t[ype]"),
			action:   actionType,
			arg:      "<expr>",
			complete: completeDoc,
			document: "print the type of expression",
		},
		{
			name:     commandName("print"),
			action:   actionPrint,
			document: "print current source",
		},
		{
			name:     commandName("w[rite]"),
			action:   actionWrite,
			complete: nil, // TODO implement
			arg:      "[<file>]",
			document: "write out current source",
		},
		{
			name:     commandName("clear"),
			action:   actionClear,
			document: "clear the code",
		},
		{
			name:     commandName("d[oc]"),
			action:   actionDoc,
			complete: completeDoc,
			arg:      "<expr or pkg>",
			document: "show documentation",
		},
		{
			name:     commandName("h[elp]"),
			action:   actionHelp,
			document: "show this help",
		},
		{
			name:     commandName("q[uit]"),
			action:   actionQuit,
			document: "quit the session",
		},
		{
			name:     commandName("e[dit]"),
			action:   actionEdit,
			arg:      "[cmd]",
			document: "edit the current source",
		},
		{
			name:     commandName("r[un]"),
			action:   actionRun,
			document: "run the current code",
		},
	}
}

func actionImport(s *Session, arg string) error {
	if arg == "" {
		return fmt.Errorf("argument is required")
	}

	if strings.Contains(arg, " ") {
		for _, v := range strings.Fields(arg) {
			if v == "" {
				continue
			}
			if err := actionImport(s, v); err != nil {
				return err
			}
		}

		return nil
	}

	path := strings.Trim(arg, `"`)

	// check if the package specified by path is importable
	_, err := s.types.Importer.Import(path)
	if err != nil {
		return err
	}

	astutil.AddImport(s.fset, s.file, path)

	return nil
}

var gorootSrc = filepath.Join(filepath.Clean(runtime.GOROOT()), "src")

func completeImport(s *Session, prefix string) []string {
	result := []string{}
	seen := map[string]bool{}

	p := strings.LastIndexFunc(prefix, unicode.IsSpace) + 1

	d, fn := path.Split(prefix[p:])
	for _, srcDir := range build.Default.SrcDirs() {
		dir := filepath.Join(srcDir, d)

		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			if err != nil && !os.IsNotExist(err) {
				errorf("Stat %s: %s", dir, err)
			}
			continue
		}

		entries, err := ioutil.ReadDir(dir)
		if err != nil {
			errorf("ReadDir %s: %s", dir, err)
			continue
		}
		for _, fi := range entries {
			if !fi.IsDir() {
				continue
			}

			name := fi.Name()
			if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "testdata" {
				continue
			}

			if strings.HasPrefix(name, fn) {
				r := path.Join(d, name)
				if srcDir != gorootSrc {
					// append "/" if this directory is not a repository
					// e.g. does not have VCS directory such as .git or .hg
					// TODO: do not append "/" to subdirectories of repos
					var isRepo bool
					for _, vcsDir := range []string{".git", ".hg", ".svn", ".bzr"} {
						_, err := os.Stat(filepath.Join(srcDir, filepath.FromSlash(r), vcsDir))
						if err == nil {
							isRepo = true
							break
						}
					}
					if !isRepo {
						r = r + "/"
					}
				}

				if !seen[r] {
					result = append(result, prefix[:p]+r)
					seen[r] = true
				}
			}
		}
	}

	return result
}

func completeDoc(s *Session, prefix string) []string {
	pos, cands, err := s.completeCode(prefix, len(prefix), false)
	if err != nil {
		errorf("completeCode: %s", err)
		return nil
	}

	result := make([]string, 0, len(cands))
	for _, c := range cands {
		result = append(result, prefix[0:pos]+c)
	}

	return result
}

func actionPrint(s *Session, _ string) error {
	source, err := s.source(true)
	if err != nil {
		return err
	}

	fmt.Println(source)

	return nil
}

func actionType(s *Session, in string) error {
	if in == "" {
		return fmt.Errorf("argument is required")
	}

	s.clearQuickFix()

	s.storeCode()
	defer s.restoreCode()

	expr, err := s.evalExpr(in)
	if err != nil {
		return err
	}

	s.typeInfo = types.Info{
		Types:  make(map[ast.Expr]types.TypeAndValue),
		Uses:   make(map[*ast.Ident]types.Object),
		Defs:   make(map[*ast.Ident]types.Object),
		Scopes: make(map[ast.Node]*types.Scope),
	}
	_, err = s.types.Check("_tmp", s.fset, []*ast.File{s.file}, &s.typeInfo)
	if err != nil {
		debugf("typecheck error (ignored): %s", err)
	}

	typ := s.typeInfo.TypeOf(expr)
	if typ == nil {
		return fmt.Errorf("cannot get type: %v", expr)
	}
	if typ, ok := typ.(*types.Basic); ok && typ.Kind() == types.Invalid {
		return fmt.Errorf("cannot get type: %v", expr)
	}
	fmt.Fprintf(s.stdout, "%v\n", typ)
	return nil
}

func actionWrite(s *Session, filename string) error {
	source, err := s.source(false)
	if err != nil {
		return err
	}

	if filename == "" {
		filename = fmt.Sprintf("gore_session_%s.go", time.Now().Format("20060102_150405"))
	}

	err = ioutil.WriteFile(filename, []byte(source), 0644)
	if err != nil {
		return err
	}

	infof("Source wrote to %s", filename)

	return nil
}

func actionClear(s *Session, _ string) error {
	return s.init()
}

func actionDoc(s *Session, in string) error {
	if in == "" {
		return fmt.Errorf("argument is required")
	}

	s.clearQuickFix()

	s.storeCode()
	defer s.restoreCode()

	expr, err := s.evalExpr(in)
	if err != nil {
		return err
	}

	s.typeInfo = types.Info{
		Types:  make(map[ast.Expr]types.TypeAndValue),
		Uses:   make(map[*ast.Ident]types.Object),
		Defs:   make(map[*ast.Ident]types.Object),
		Scopes: make(map[ast.Node]*types.Scope),
	}
	_, err = s.types.Check("_tmp", s.fset, []*ast.File{s.file}, &s.typeInfo)
	if err != nil {
		debugf("typecheck error (ignored): %s", err)
	}

	// :doc patterns:
	// - "json" -> "encoding/json" (package name)
	// - "json.Encoder" -> "encoding/json", "Encoder" (package member)
	// - "json.NewEncoder(nil).Encode" -> "encoding/json", "Decode" (package type member)
	var docObj types.Object
	if sel, ok := expr.(*ast.SelectorExpr); ok {
		// package member, package type member
		docObj = s.typeInfo.ObjectOf(sel.Sel)
	} else if t := s.typeInfo.TypeOf(expr); t != nil && t != types.Typ[types.Invalid] {
		for {
			if pt, ok := t.(*types.Pointer); ok {
				t = pt.Elem()
			} else {
				break
			}
		}
		switch t := t.(type) {
		case *types.Named:
			docObj = t.Obj()
		case *types.Basic:
			// builtin types
			docObj = types.Universe.Lookup(t.Name())
		}
	} else if ident, ok := expr.(*ast.Ident); ok {
		// package name
		mainScope := s.typeInfo.Scopes[s.mainFunc().Type]
		_, docObj = mainScope.LookupParent(ident.Name, ident.NamePos)
	}

	if docObj == nil {
		return fmt.Errorf("cannot determine the document location")
	}

	debugf("doc :: obj=%#v", docObj)

	var pkgPath, objName string
	if pkgName, ok := docObj.(*types.PkgName); ok {
		pkgPath = pkgName.Imported().Path()
	} else {
		if pkg := docObj.Pkg(); pkg != nil {
			pkgPath = pkg.Path()
		} else {
			pkgPath = "builtin"
		}
		objName = docObj.Name()
	}

	debugf("doc :: %q %q", pkgPath, objName)

	args := []string{"doc", pkgPath}
	if objName != "" {
		args = append(args, objName)
	}

	godoc := exec.Command("go", args...)
	godoc.Stderr = s.stderr

	// TODO just use PAGER?
	if pagerCmd := os.Getenv("GORE_PAGER"); pagerCmd != "" {
		r, err := godoc.StdoutPipe()
		if err != nil {
			return err
		}

		pager := exec.Command(pagerCmd)
		pager.Stdin = r
		pager.Stdout = s.stdout
		pager.Stderr = s.stderr

		err = pager.Start()
		if err != nil {
			return err
		}

		err = godoc.Run()
		if err != nil {
			return err
		}

		return pager.Wait()
	}
	godoc.Stdout = s.stdout
	return godoc.Run()
}

func actionHelp(s *Session, _ string) error {
	w := tabwriter.NewWriter(s.stdout, 0, 8, 4, ' ', 0)
	for _, command := range commands {
		cmd := fmt.Sprintf(":%s", command.name)
		if command.arg != "" {
			cmd = cmd + " " + command.arg
		}
		w.Write([]byte("    " + cmd + "\t" + command.document + "\n"))
	}
	w.Flush()

	return nil
}

func actionQuit(s *Session, _ string) error {
	return ErrQuit
}

func actionEdit(s *Session, arg string) error {
	var (
		editor, bin string
		args        []string
		err         error
	)

	filename := filepath.Join(s.tempDir, "edit.go")
	if len(arg) > 0 {
		bin = "/bin/sh"
		args = []string{"-c", fmt.Sprintf("%s %s", arg, filename)}
	} else {
		args = []string{filename}

		if editor = os.Getenv("VISUAL"); editor == "" {
			if editor = os.Getenv("EDITOR"); editor == "" {
				editor = "vi"
			}
		}

		bin, err = exec.LookPath(editor)
		if err != nil {
			return fmt.Errorf("Couldn't find editor: %v", err)
		}
	}

	src, err := s.source(false)
	if err != nil {
		return fmt.Errorf("Couldn't get source: %v", err)
	}

	ioutil.WriteFile(filename, []byte(src), 0644)
	if err != nil {
		return fmt.Errorf("Error writing source: %v", err)
	}

	cmd := exec.Command(bin, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err != nil {
		return err
	}

	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("Error reading back file: %v", err)
	}

	s.file, err = parser.ParseFile(s.fset, s.tempFilePath, data, parser.Mode(0))
	if err != nil {
		return err
	}

	s.mainBody = s.mainFunc().Body

	s.lastStmts = nil
	s.lastDecls = nil

	return s.Run()
}

func actionRun(s *Session, _ string) error {
	return s.Run()
}
