package hclconf

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTF(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadS3Backend(t *testing.T) {
	dir := t.TempDir()
	writeTF(t, dir, "backend.tf", `
terraform {
  backend "s3" {
    bucket         = "my-bucket"
    key            = "prod/terraform.tfstate"
    region         = "eu-central-1"
    dynamodb_table = "tf-locks"
  }
}
`)
	// A second file with resources must not confuse the parser.
	writeTF(t, dir, "main.tf", `
resource "aws_s3_bucket" "b" {
  bucket = "unrelated"
}
variable "x" { default = "y" }
`)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Type != "s3" {
		t.Fatalf("Type = %q, want s3", cfg.Type)
	}
	if cfg.Attrs["bucket"] != "my-bucket" || cfg.Attrs["dynamodb_table"] != "tf-locks" {
		t.Fatalf("unexpected attrs: %+v", cfg.Attrs)
	}
}

func TestLoadNoBackend(t *testing.T) {
	dir := t.TempDir()
	writeTF(t, dir, "main.tf", `resource "null_resource" "n" {}`)
	if _, err := Load(dir); err == nil {
		t.Fatal("expected error when no backend block is present")
	}
}

func TestLoadMultipleBackends(t *testing.T) {
	dir := t.TempDir()
	writeTF(t, dir, "a.tf", `terraform { backend "s3" { bucket = "a" } }`)
	writeTF(t, dir, "b.tf", `terraform { backend "gcs" { bucket = "b" } }`)
	if _, err := Load(dir); err == nil {
		t.Fatal("expected error for multiple backend blocks")
	}
}

func TestApplyBackendConfigMergesInOrder(t *testing.T) {
	dir := t.TempDir()
	writeTF(t, dir, "backend.tf", `
terraform {
  backend "s3" {
    region = "us-east-1"
  }
}
`)
	writeTF(t, dir, "prod.s3.tfbackend", `
bucket               = "prod-state"
key                  = "prod/terraform.tfstate"
region               = "eu-central-1"
dynamodb_table       = "tf-locks"
workspace_key_prefix = "workspaces"
`)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// The file is relative to the module dir, as with terraform -chdir; the
	// key=value after it wins over both the file and the block.
	cfg, err = ApplyBackendConfig(cfg, []string{"prod.s3.tfbackend", "key=override/terraform.tfstate"}, dir)
	if err != nil {
		t.Fatalf("ApplyBackendConfig: %v", err)
	}
	want := map[string]string{
		"bucket":               "prod-state",
		"key":                  "override/terraform.tfstate",
		"region":               "eu-central-1",
		"dynamodb_table":       "tf-locks",
		"workspace_key_prefix": "workspaces",
	}
	for k, v := range want {
		if cfg.Attrs[k] != v {
			t.Errorf("%s = %q, want %q", k, cfg.Attrs[k], v)
		}
	}
}

func TestApplyBackendConfigKeepsTheBlockUntouched(t *testing.T) {
	base := Config{Type: "s3", Attrs: map[string]string{"bucket": "a"}}
	got, err := ApplyBackendConfig(base, []string{"bucket=b", "token=x=y"}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got.Attrs["bucket"] != "b" || got.Attrs["token"] != "x=y" {
		t.Errorf("attrs = %+v", got.Attrs)
	}
	if base.Attrs["bucket"] != "a" {
		t.Errorf("the input config was mutated: %+v", base.Attrs)
	}
}

func TestApplyBackendConfigErrors(t *testing.T) {
	base := Config{Type: "s3", Attrs: map[string]string{}}
	if _, err := ApplyBackendConfig(base, []string{"missing.tfbackend"}, t.TempDir()); err == nil {
		t.Error("a missing file should fail")
	}
	if _, err := ApplyBackendConfig(base, []string{"=value"}, t.TempDir()); err == nil {
		t.Error("an empty key should fail")
	}
}

func TestLoadInitReadsRecordedBackend(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".terraform"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTF(t, filepath.Join(dir, ".terraform"), "terraform.tfstate", `{
  "version": 3,
  "backend": {
    "type": "s3",
    "config": {
      "bucket": "prod-state",
      "key": "prod/terraform.tfstate",
      "dynamodb_table": "tf-locks",
      "workspace_key_prefix": null,
      "encrypt": true
    },
    "hash": 1
  }
}`)
	cfg, err := LoadInit(dir)
	if err != nil {
		t.Fatalf("LoadInit: %v", err)
	}
	if cfg.Type != "s3" || cfg.Attrs["bucket"] != "prod-state" || cfg.Attrs["dynamodb_table"] != "tf-locks" {
		t.Errorf("cfg = %+v", cfg)
	}
	if _, ok := cfg.Attrs["workspace_key_prefix"]; ok {
		t.Error("a null value should be dropped")
	}
	if _, ok := cfg.Attrs["encrypt"]; ok {
		t.Error("a non-string value should be dropped")
	}
}

func TestLoadInitHonoursTFDataDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTF(t, filepath.Join(dir, "data"), "terraform.tfstate", `{"backend":{"type":"s3","config":{"bucket":"b"}}}`)
	t.Setenv("TF_DATA_DIR", "data")
	cfg, err := LoadInit(dir)
	if err != nil || cfg.Attrs["bucket"] != "b" {
		t.Fatalf("cfg = %+v, err = %v", cfg, err)
	}
}

func TestLoadInitWithoutInit(t *testing.T) {
	if _, err := LoadInit(t.TempDir()); err == nil {
		t.Error("a module that was never initialised should fail")
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".terraform"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTF(t, filepath.Join(dir, ".terraform"), "terraform.tfstate", `{"version":3}`)
	if _, err := LoadInit(dir); err == nil {
		t.Error("an init state without a backend should fail")
	}
}
