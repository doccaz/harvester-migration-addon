// Command crd-contract writes the field paths of the VM Import Controller's custom resources, read from
// the Go types of a vm-import-controller checkout. The controller creates its CRDs at runtime from those
// types, so they are the contract this UI has to stay inside.
//
//	git clone --depth 1 --branch v1.8.2 https://github.com/harvester/vm-import-controller reference/vm-import-controller
//	go run ./hack/crd-contract reference/vm-import-controller v1.8.2 > ui-backend/internal/vmic/testdata/upstream-contract.json
//
// Only the standard library is used, so it never needs the controller's own (awkward) go.mod.
package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

// External types the controller embeds. Anything not listed is treated as a leaf (a scalar, a Quantity, a
// time, ...), so a path below it cannot be checked; those are reported separately.
var external = map[string][]string{
	"corev1.SecretReference": {"name", "namespace"},
	"corev1.ObjectReference": {"kind", "namespace", "name", "uid", "apiVersion", "resourceVersion", "fieldPath"},
}

var kinds = []string{"VmwareSource", "OvaSource", "OpenstackSource", "VirtualMachineImport"}

type structs map[string]*ast.StructType

func load(dir, prefix string, into structs) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool {
		n := fi.Name()
		return !strings.HasSuffix(n, "_test.go") && !strings.HasPrefix(n, "zz_generated")
	}, 0)
	if err != nil {
		fatal(err)
	}
	for _, p := range pkgs {
		for _, f := range p.Files {
			for _, d := range f.Decls {
				g, ok := d.(*ast.GenDecl)
				if !ok {
					continue
				}
				for _, s := range g.Specs {
					if ts, ok := s.(*ast.TypeSpec); ok {
						if st, ok := ts.Type.(*ast.StructType); ok {
							into[prefix+ts.Name.Name] = st
						}
					}
				}
			}
		}
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

// typeName turns a field type expression into "Name", "pkg.Name", with the slice marker split off.
func typeName(e ast.Expr) (name string, slice bool) {
	switch t := e.(type) {
	case *ast.StarExpr:
		return typeName(t.X)
	case *ast.ArrayType:
		n, _ := typeName(t.Elt)
		return n, true
	case *ast.MapType:
		return "map", false
	case *ast.Ident:
		return t.Name, false
	case *ast.SelectorExpr:
		return fmt.Sprintf("%s.%s", t.X.(*ast.Ident).Name, t.Sel.Name), false
	}
	return "?", false
}

type result struct {
	paths, required, opaque map[string]bool
}

func jsonTag(f *ast.Field) (name string, omit, inline bool) {
	if f.Tag == nil {
		return "", false, false
	}
	tag := reflect.StructTag(strings.Trim(f.Tag.Value, "`")).Get("json")
	parts := strings.Split(tag, ",")
	name = parts[0]
	for _, p := range parts[1:] {
		switch p {
		case "omitempty":
			omit = true
		case "inline":
			inline = true
		}
	}
	return name, omit, inline
}

func (r *result) walk(all structs, st *ast.StructType, prefix string, depth int) {
	if depth > 8 {
		return
	}
	for _, f := range st.Fields.List {
		name, omit, inline := jsonTag(f)
		tn, slice := typeName(f.Type)
		if inline {
			if inner, ok := all[tn]; ok {
				r.walk(all, inner, prefix, depth+1)
			}
			continue
		}
		if name == "" || name == "-" {
			continue
		}
		path := prefix + name
		if slice {
			path += "[]"
		}
		r.paths[path] = true
		_, pointer := f.Type.(*ast.StarExpr)
		if !omit && !pointer {
			r.required[path] = true
		}
		child := path
		if slice {
			child = path + "."
		} else {
			child = path + "."
		}
		switch {
		case all[tn] != nil:
			r.walk(all, all[tn], child, depth+1)
		case external[tn] != nil:
			for _, sub := range external[tn] {
				r.paths[child+sub] = true
			}
		case strings.Contains(tn, "."), tn == "map", tn == "?":
			r.opaque[path] = true // an external type or a map: its inner fields are not enumerated
		}
	}
}

func list(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func main() {
	if len(os.Args) != 3 {
		fatal(fmt.Errorf("usage: crd-contract <vm-import-controller checkout> <version label>"))
	}
	root, version := os.Args[1], os.Args[2]
	all := structs{}
	load(filepath.Join(root, "pkg/apis/migration.harvesterhci.io/v1beta1"), "", all)
	load(filepath.Join(root, "pkg/apis/common"), "common.", all)

	out := map[string]interface{}{
		"source":  "github.com/harvester/vm-import-controller pkg/apis (the CRDs are generated from these types at runtime)",
		"version": version,
	}
	ks := map[string]interface{}{}
	for _, k := range kinds {
		top, ok := all[k]
		if !ok {
			fatal(fmt.Errorf("kind %s not found under %s", k, root))
		}
		r := &result{paths: map[string]bool{}, required: map[string]bool{}, opaque: map[string]bool{}}
		for _, f := range top.Fields.List {
			name, _, _ := jsonTag(f)
			if name != "spec" && name != "status" {
				continue
			}
			tn, _ := typeName(f.Type)
			r.paths[name] = true
			if inner, ok := all[tn]; ok {
				r.walk(all, inner, name+".", 0)
			}
		}
		ks[k] = map[string]interface{}{"paths": list(r.paths), "required": list(r.required), "opaque": list(r.opaque)}
	}
	out["kinds"] = ks
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		fatal(err)
	}
}
