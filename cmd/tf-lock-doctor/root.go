package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/moveeeax/tf-lock-doctor/internal/backend"
	"github.com/moveeeax/tf-lock-doctor/internal/hclconf"
)

type globalOpts struct {
	chdir     string
	workspace string
}

func newRootCmd() *cobra.Command {
	opts := &globalOpts{}
	cmd := &cobra.Command{
		Use:   "tf-lock-doctor",
		Short: "Diagnose and safely release a stuck Terraform state lock",
		Long: "tf-lock-doctor inspects the lock a Terraform backend holds, shows who took it and\n" +
			"how long ago, and releases it only after confirming the holding run is really gone.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version,
	}
	cmd.PersistentFlags().StringVar(&opts.chdir, "chdir", ".", "Terraform module directory to read the backend config from")
	cmd.PersistentFlags().StringVar(&opts.workspace, "workspace", "default", "Terraform workspace whose lock to act on")

	cmd.AddCommand(newInspectCmd(opts))
	cmd.AddCommand(newReleaseCmd(opts))
	return cmd
}

// resolveBackend parses the module's backend block and constructs the matching
// Backend. Only backends tf-lock-doctor implements are accepted; others fail
// with a clear message rather than pretending to work.
func resolveBackend(ctx context.Context, opts *globalOpts) (backend.Backend, hclconf.Config, error) {
	cfg, err := hclconf.Load(opts.chdir)
	if err != nil {
		return nil, hclconf.Config{}, err
	}
	switch cfg.Type {
	case "s3":
		s3cfg := backend.FromAttrs(cfg.Attrs, opts.workspace)
		b, err := backend.NewS3(ctx, s3cfg)
		return b, cfg, err
	default:
		return nil, cfg, fmt.Errorf("backend %q is not supported yet; tf-lock-doctor currently implements: s3", cfg.Type)
	}
}
