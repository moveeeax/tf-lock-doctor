package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/moveeeax/tf-lock-doctor/internal/backend"
	"github.com/moveeeax/tf-lock-doctor/internal/lock"
)

func newInspectCmd(opts *globalOpts) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "inspect",
		Short: "Show the current state lock, or report that the state is unlocked",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := context.Background()
			b, _, err := resolveBackend(ctx, opts)
			if err != nil {
				return err
			}
			info, err := b.Read(ctx)
			if errors.Is(err, backend.ErrNoLock) {
				if asJSON {
					fmt.Fprintln(cmd.OutOrStdout(), `{"locked":false}`)
				} else {
					fmt.Fprintln(cmd.OutOrStdout(), "State is not locked.")
				}
				return nil
			}
			if err != nil {
				return err
			}
			if asJSON {
				return printJSON(cmd, info)
			}
			printHuman(cmd, b.Name(), info)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit machine-readable JSON")
	return cmd
}

func printJSON(cmd *cobra.Command, info lock.Info) error {
	out := struct {
		Locked  bool      `json:"locked"`
		ID      string    `json:"id"`
		Who     string    `json:"who"`
		User    string    `json:"user"`
		Host    string    `json:"host"`
		Op      string    `json:"operation"`
		Created time.Time `json:"created"`
		AgeSecs int64     `json:"age_seconds"`
		Info    string    `json:"info"`
		Version string    `json:"terraform_version"`
	}{
		Locked:  true,
		ID:      info.ID,
		Who:     info.Who,
		User:    info.User(),
		Host:    info.Host(),
		Op:      info.Operation,
		Created: info.Created,
		AgeSecs: int64(info.Age(time.Now()).Seconds()),
		Info:    info.Info,
		Version: info.Version,
	}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func printHuman(cmd *cobra.Command, backendName string, info lock.Info) {
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "State is LOCKED (%s backend)\n", backendName)
	fmt.Fprintf(w, "  Lock ID:   %s\n", info.ID)
	fmt.Fprintf(w, "  Held by:   %s\n", info.Who)
	fmt.Fprintf(w, "  Operation: %s\n", nonEmpty(info.Operation))
	if info.Info != "" {
		fmt.Fprintf(w, "  Info:      %s\n", info.Info)
	}
	if !info.Created.IsZero() {
		age := info.Age(time.Now()).Round(time.Second)
		fmt.Fprintf(w, "  Created:   %s (%s ago)\n", info.Created.Format(time.RFC3339), age)
	}
	if info.Version != "" {
		fmt.Fprintf(w, "  TF ver:    %s\n", info.Version)
	}
}

func nonEmpty(s string) string {
	if s == "" {
		return "(unknown)"
	}
	return s
}
