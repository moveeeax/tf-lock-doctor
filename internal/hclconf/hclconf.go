// Package hclconf extracts the backend configuration from a Terraform module's
// terraform { backend "..." {} } block without running Terraform itself.
package hclconf

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclparse"
)

// Config is the resolved backend selection for a module directory.
type Config struct {
	// Type is the backend name, e.g. "s3", "gcs", "azurerm", "consul".
	Type string
	// Attrs holds the literal string attributes declared in the backend block.
	Attrs map[string]string
}

// backend "s3" {} where s3 is a label and the body is arbitrary attributes.
type rootSchema struct {
	Terraform []terraformBlock `hcl:"terraform,block"`
	Remain    hcl.Body         `hcl:",remain"`
}

type terraformBlock struct {
	Backend []backendBlock `hcl:"backend,block"`
	Remain  hcl.Body       `hcl:",remain"`
}

type backendBlock struct {
	Type string   `hcl:"type,label"`
	Body hcl.Body `hcl:",remain"`
}

// Load scans every *.tf file in dir for a single backend block and returns it.
// It reports an error if no backend block is found or if more than one is
// declared, since Terraform itself rejects both cases.
func Load(dir string) (Config, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return Config{}, fmt.Errorf("read module dir: %w", err)
	}

	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(e.Name(), ".tf") {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(files)

	parser := hclparse.NewParser()
	var found *backendBlock
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			return Config{}, fmt.Errorf("read %s: %w", f, err)
		}
		file, diags := parser.ParseHCL(src, f)
		if diags.HasErrors() {
			return Config{}, fmt.Errorf("parse %s: %s", f, diags.Error())
		}
		var root rootSchema
		// Only the terraform/backend blocks are decoded; everything else is
		// left in Remain and ignored, so provider/resource blocks never error.
		if diags := gohcl.DecodeBody(file.Body, nil, &root); diags.HasErrors() {
			// A partial decode is expected here because the module contains far
			// more than the terraform block; only hard structural errors matter.
			if hasFatal(diags) {
				return Config{}, fmt.Errorf("decode %s: %s", f, diags.Error())
			}
		}
		for i := range root.Terraform {
			for j := range root.Terraform[i].Backend {
				if found != nil {
					return Config{}, fmt.Errorf("multiple backend blocks found; Terraform allows only one")
				}
				b := root.Terraform[i].Backend[j]
				found = &b
			}
		}
	}

	if found == nil {
		return Config{}, fmt.Errorf("no backend block found in %s", dir)
	}

	attrs, err := stringAttrs(found.Body)
	if err != nil {
		return Config{}, err
	}
	return Config{Type: found.Type, Attrs: attrs}, nil
}

// stringAttrs pulls the literal string attributes out of a backend body. Values
// that are not simple string literals (references, functions) are skipped
// rather than failing, because tf-lock-doctor only needs the addressing keys
// (bucket, key, region, table) which are conventionally literals.
func stringAttrs(body hcl.Body) (map[string]string, error) {
	attrs := map[string]string{}
	content, _, diags := body.PartialContent(&hcl.BodySchema{})
	_ = content
	if diags.HasErrors() && hasFatal(diags) {
		return nil, fmt.Errorf("backend body: %s", diags.Error())
	}
	justAttrs, diags := body.JustAttributes()
	if diags.HasErrors() {
		// Blocks inside a backend body are unusual; ignore non-fatal cases.
		if hasFatal(diags) {
			return nil, fmt.Errorf("backend attributes: %s", diags.Error())
		}
	}
	for name, attr := range justAttrs {
		val, diags := attr.Expr.Value(nil)
		if diags.HasErrors() || val.IsNull() || !val.Type().IsPrimitiveType() {
			continue
		}
		if val.Type().FriendlyName() == "string" {
			attrs[name] = val.AsString()
		}
	}
	return attrs, nil
}

func hasFatal(diags hcl.Diagnostics) bool {
	for _, d := range diags {
		if d.Severity == hcl.DiagError && !strings.Contains(d.Summary, "Unsupported") {
			return true
		}
	}
	return false
}
