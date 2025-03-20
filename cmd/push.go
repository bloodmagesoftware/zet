package cmd

import (
	"errors"
	"fmt"

	"github.com/bloodmagesoftware/zet/internal/project"
	"github.com/bloodmagesoftware/zet/internal/remote"
	"github.com/spf13/cobra"
)

var pushCmd = &cobra.Command{
	Use:     "push",
	Aliases: []string{"commit"},
	Short:   "Push local changes to remote",
	RunE: func(cmd *cobra.Command, args []string) error {
		p, err := project.Load()
		if err != nil {
			return errors.Join(fmt.Errorf("failed to open project file %s", project.ProjectFileName), err)
		}

		r, err := remote.Connect(p)
		if err != nil {
			return errors.Join(errors.New("failed to connect to remote"), err)
		}
		defer r.Close()

		if empty, err := r.IsEmpty(); err != nil {
			return errors.Join(errors.New("failed to check if remote directory is empty"), err)
		} else if empty {
			if err := r.InitialCommit(); err != nil {
				return errors.Join(errors.New("failed to push initial commit"), err)
			} else {
				return nil
			}
		} else {
			if err := r.CommitInteractive(); err != nil {
				return errors.Join(errors.New("failed to push to remote"), err)
			} else {
				return nil
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(pushCmd)
}
