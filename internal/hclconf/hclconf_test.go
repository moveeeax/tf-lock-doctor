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
