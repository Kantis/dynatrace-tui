package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func runTUI(cmd *cobra.Command, args []string) error {
	return fmt.Errorf("TUI not implemented yet; use `dttui query \"<DQL>\"` for now")
}
