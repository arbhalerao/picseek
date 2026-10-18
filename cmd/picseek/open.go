package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"
)

func (a *app) openCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "open <number>...",
		Short: "Open results of the last search by their [number]",
		Example: `  picseek search visa
  picseek open 2
  picseek open 1 3`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return errors.New("give the [number] of a result, e.g. `picseek open 2`")
			}
			paths, err := loadLast()
			if errors.Is(err, os.ErrNotExist) {
				return errors.New("nothing to open yet; run `picseek search` first")
			}
			if err != nil {
				return err
			}
			targets := make([]string, len(args))
			for i, arg := range args {
				n, err := strconv.Atoi(arg)
				if err != nil || n < 1 {
					return fmt.Errorf("%q is not a result number", arg)
				}
				if n > len(paths) {
					return fmt.Errorf("no result [%d]; the last search had %d %s", n, len(paths), plural(len(paths), "result", "results"))
				}
				if _, err := os.Stat(paths[n-1]); err != nil {
					return fmt.Errorf("[%d] no longer exists: %s", n, paths[n-1])
				}
				targets[i] = paths[n-1]
			}
			for _, p := range targets {
				if err := openFile(p); err != nil {
					return fmt.Errorf("open %s: %w", p, err)
				}
			}
			return nil
		},
	}
}
