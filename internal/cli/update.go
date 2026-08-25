package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/fvmoraes/kubepeep/internal/buildinfo"
	"github.com/fvmoraes/kubepeep/internal/updater"
	"github.com/spf13/cobra"
)

func newUpdateCommand(dependencies Dependencies) *cobra.Command {
	var targetVersion string
	command := &cobra.Command{
		Use:   "update --version X.Y.Z",
		Short: "Install one explicit, checksum-verified release",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if strings.TrimSpace(targetVersion) == "" {
				return &ExitError{Code: ExitInvalid, Err: updater.ErrInvalidVersion}
			}
			layout, err := dependencies.ResolveLayout()
			if err != nil {
				return &ExitError{Code: ExitOperational, Err: err}
			}
			_, active, err := dependencies.Controller.Status(command.Context(), layout.RuntimeDir)
			if err != nil {
				return &ExitError{Code: ExitOperational, Err: err}
			}
			if active {
				return &ExitError{Code: ExitOperational, Err: errors.New("kubePeep is running; run kubePeep stop before updating")}
			}
			result, err := dependencies.Updater.Update(command.Context(), updater.Request{
				CurrentVersion: buildinfo.Version,
				TargetVersion:  targetVersion,
			})
			if err != nil {
				code := ExitOperational
				if errors.Is(err, updater.ErrInvalidVersion) || errors.Is(err, updater.ErrUnsupported) {
					code = ExitInvalid
				}
				return &ExitError{Code: code, Err: err}
			}
			if result.AlreadyCurrent {
				_, err = fmt.Fprintf(command.OutOrStdout(), "kubePeep %s is already installed\n", result.TargetVersion)
				return err
			}
			if result.Scheduled {
				_, err = fmt.Fprintf(command.OutOrStdout(), "Verified kubePeep %s -> %s; Windows replacement is scheduled after this command exits\n", result.CurrentVersion, result.TargetVersion)
				return err
			}
			_, err = fmt.Fprintf(command.OutOrStdout(), "Updated kubePeep %s -> %s\n", result.CurrentVersion, result.TargetVersion)
			return err
		},
	}
	command.Flags().StringVar(&targetVersion, "version", "", "exact release version (X.Y.Z)")
	return command
}
