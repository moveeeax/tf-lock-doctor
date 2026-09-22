package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/moveeeax/tf-lock-doctor/internal/audit"
	"github.com/moveeeax/tf-lock-doctor/internal/backend"
	"github.com/moveeeax/tf-lock-doctor/internal/liveness"
	"github.com/moveeeax/tf-lock-doctor/internal/lock"
)

func newReleaseCmd(opts *globalOpts) *cobra.Command {
	var (
		dryRun     bool
		forceAfter time.Duration
		auditPath  string
		yes        bool
	)
	cmd := &cobra.Command{
		Use:   "release",
		Short: "Release the state lock, but only when the holding run is confirmed gone",
		Long: "release refuses to unlock while it cannot confirm the holding run has ended.\n" +
			"Use --force-after to authorise releasing a lock older than a given age, which\n" +
			"covers genuinely abandoned locks left behind by a crashed apply.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := context.Background()
			b, _, err := resolveBackend(ctx, opts)
			if err != nil {
				return err
			}
			info, err := b.Read(ctx)
			if errors.Is(err, backend.ErrNoLock) {
				fmt.Fprintln(cmd.OutOrStdout(), "State is not locked; nothing to release.")
				return nil
			}
			if err != nil {
				return err
			}

			checker := liveness.NewAgeChecker(forceAfter)
			verdict := checker.Check(ctx, info)

			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Lock %s held by %s (%s), age %s\n",
				info.ID, info.Who, nonEmpty(info.Operation), info.Age(time.Now()).Round(time.Second))
			fmt.Fprintf(w, "Holder liveness: %s — %s\n", verdict.State, verdict.Reason)

			if verdict.State != liveness.Dead {
				return fmt.Errorf("refusing to unlock: holder not confirmed gone. " +
					"Pass --force-after <dur> once you are sure the run is abandoned")
			}

			if dryRun {
				fmt.Fprintf(w, "\n[dry-run] would run: terraform force-unlock %s\n", info.ID)
				return writeAudit(auditPath, b.Name(), info, verdict.Reason, true)
			}

			if !yes {
				fmt.Fprintf(w, "\nRelease this lock? Re-run with --yes to confirm.\n")
				return errors.New("confirmation required (--yes)")
			}

			if err := b.Release(ctx, info.ID); err != nil {
				return fmt.Errorf("release lock: %w", err)
			}
			fmt.Fprintf(w, "\nLock %s released.\n", info.ID)
			return writeAudit(auditPath, b.Name(), info, verdict.Reason, false)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show the force-unlock that would run, without doing it")
	cmd.Flags().DurationVar(&forceAfter, "force-after", 0, "treat a lock older than this as abandoned and eligible for release (e.g. 2h)")
	cmd.Flags().StringVar(&auditPath, "audit-log", defaultAuditPath(), "append a JSON audit line for every release to this file")
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm the release (required for a real unlock)")
	return cmd
}

func writeAudit(path, backendName string, info lock.Info, reason string, dryRun bool) error {
	if path == "" {
		return nil
	}
	w, f, err := audit.OpenFile(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return w.Record(audit.DefaultActor(), backendName, info, reason, dryRun)
}

func defaultAuditPath() string {
	if v := os.Getenv("TF_LOCK_DOCTOR_AUDIT_LOG"); v != "" {
		return v
	}
	return "tf-lock-doctor-audit.log"
}
