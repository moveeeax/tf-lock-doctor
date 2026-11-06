package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigFromInitWithOverrides(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".terraform"), 0o755); err != nil {
		t.Fatal(err)
	}
	state := `{"backend":{"type":"s3","config":{"bucket":"b","key":"k","dynamodb_table":"t"}}}`
	if err := os.WriteFile(filepath.Join(dir, ".terraform", "terraform.tfstate"), []byte(state), 0o644); err != nil {
		t.Fatal(err)
	}
	// No *.tf file at all: --from-init must not need the backend block.
	cfg, err := loadConfig(&globalOpts{chdir: dir, fromInit: true, backendConfigs: []string{"key=other"}})
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.Type != "s3" || cfg.Attrs["bucket"] != "b" || cfg.Attrs["key"] != "other" {
		t.Errorf("cfg = %+v", cfg)
	}

	if _, err := loadConfig(&globalOpts{chdir: dir}); err == nil {
		t.Error("without --from-init a module with no backend block should fail")
	}
}

func TestBackendConfigFlagIsRepeatable(t *testing.T) {
	cmd := newRootCmd()
	if err := cmd.PersistentFlags().Parse([]string{
		"--backend-config", "prod.s3.tfbackend", "--backend-config", "key=a,b",
	}); err != nil {
		t.Fatal(err)
	}
	got, err := cmd.PersistentFlags().GetStringArray("backend-config")
	if err != nil {
		t.Fatal(err)
	}
	// StringArray, not StringSlice: a comma inside a value must not split it.
	if len(got) != 2 || got[1] != "key=a,b" {
		t.Errorf("backend-config = %q", got)
	}
}
