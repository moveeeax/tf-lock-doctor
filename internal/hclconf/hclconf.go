// Package hclconf extracts the backend configuration from a Terraform module's
// terraform { backend "..." {} } block without running Terraform itself.
package hclconf

import (
	"encoding/json"
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

// ApplyBackendConfig merges -backend-config style overrides into cfg the way
// `terraform init` does: items are applied in order and a later value wins
// over an earlier one and over the backend block. An item containing "=" is a
// key=value pair; anything else is the path of a file of top-level attributes
// (a *.tfbackend file). A relative path is resolved against dir first, as
// `terraform -chdir=<dir> init` would, then against the current directory.
func ApplyBackendConfig(cfg Config, items []string, dir string) (Config, error) {
	merged := make(map[string]string, len(cfg.Attrs))
	for k, v := range cfg.Attrs {
		merged[k] = v
	}
	for _, item := range items {
		if eq := strings.Index(item, "="); eq >= 0 {
			name := strings.TrimSpace(item[:eq])
			if name == "" {
				return Config{}, fmt.Errorf("backend config %q: empty key", item)
			}
			merged[name] = item[eq+1:]
			continue
		}
		attrs, err := loadBackendConfigFile(resolvePath(item, dir))
		if err != nil {
			return Config{}, err
		}
		for k, v := range attrs {
			merged[k] = v
		}
	}
	cfg.Attrs = merged
	return cfg, nil
}

func resolvePath(p, dir string) string {
	if filepath.IsAbs(p) {
		return p
	}
	inDir := filepath.Join(dir, p)
	if _, err := os.Stat(inDir); err == nil {
		return inDir
	}
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return inDir
}

func loadBackendConfigFile(path string) (map[string]string, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read backend config file: %w", err)
	}
	file, diags := hclparse.NewParser().ParseHCL(src, path)
	if diags.HasErrors() {
		return nil, fmt.Errorf("parse %s: %s", path, diags.Error())
	}
	return stringAttrs(file.Body)
}

// initState is the part of .terraform/terraform.tfstate that `terraform init`
// writes to record the backend it configured, after merging the backend block
// with every -backend-config item.
type initState struct {
	Backend *struct {
		Type   string                     `json:"type"`
		Config map[string]json.RawMessage `json:"config"`
	} `json:"backend"`
}

// LoadInit reads the backend configuration that `terraform init` recorded in
// the module's data directory (.terraform, or $TF_DATA_DIR relative to dir).
// Only string values are kept, matching what Load extracts from HCL.
func LoadInit(dir string) (Config, error) {
	dataDir := os.Getenv("TF_DATA_DIR")
	if dataDir == "" {
		dataDir = ".terraform"
	}
	if !filepath.IsAbs(dataDir) {
		dataDir = filepath.Join(dir, dataDir)
	}
	path := filepath.Join(dataDir, "terraform.tfstate")
	src, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read init state (run terraform init first): %w", err)
	}
	var st initState
	if err := json.Unmarshal(src, &st); err != nil {
		return Config{}, fmt.Errorf("decode %s: %w", path, err)
	}
	if st.Backend == nil || st.Backend.Type == "" {
		return Config{}, fmt.Errorf("%s records no backend", path)
	}
	attrs := map[string]string{}
	for name, raw := range st.Backend.Config {
		var s string
		if err := json.Unmarshal(raw, &s); err == nil && s != "" {
			attrs[name] = s
		}
	}
	return Config{Type: st.Backend.Type, Attrs: attrs}, nil
}
