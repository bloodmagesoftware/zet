package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bloodmagesoftware/zet/internal/options"
	"github.com/bloodmagesoftware/zet/internal/project"
	"github.com/bloodmagesoftware/zet/internal/remote"
	"github.com/spf13/cobra"
)

var (
	initCmd = &cobra.Command{
		Use:   "init",
		Short: "Initialize a new zet project",
		RunE: func(cmd *cobra.Command, args []string) error {
			// ensure output directory exists
			if options.FlagOut != nil {
				if err := os.MkdirAll(*options.FlagOut, 0755); err != nil {
					if !os.IsExist(err) {
						return errors.Join(fmt.Errorf("failed to make directory %s", *options.FlagOut), err)
					}
				}
				if err := os.Chdir(*options.FlagOut); err != nil {
					return errors.Join(fmt.Errorf("failed to change directory into %s", *options.FlagOut), err)
				}
			}

			// Check if there already is a project
			if exists, err := project.Exists(); err != nil {
				return errors.Join(errors.New("failed to check if project file exists"), err)
			} else if exists && !options.FlagForce {
				return errors.New("project file already exists, use --force to overwrite")
			}

			p, err := project.NewInteractive()
			if err != nil {
				return errors.Join(errors.New("failed to create new project file interactively"), err)
			}

			r, err := remote.Connect(p)
			if err != nil {
				return errors.Join(errors.New("failed to connect to remote"), err)
			}
			defer r.Close()

			if empty, err := r.IsEmpty(); err != nil {
				return errors.Join(errors.New("failed to check if remote dir is empty"), err)
			} else if !empty && !options.FlagForce {
				return errors.New("remote directory is not empty, use --force to overwrite")
			}

			// save project config
			if err := project.Save(p); err != nil {
				return errors.Join(errors.New("failed to save project file"), err)
			}

			// done
			fmt.Println("initialization done")
			fmt.Printf("use `%s push` to upload your local files to the remote", filepath.Base(os.Args[0]))

			return nil
		},
	}
)

func init() {
	rootCmd.AddCommand(initCmd)
	options.FlagOut = initCmd.Flags().StringP("out", "o", ".", "Output directory")
}
