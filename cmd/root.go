package cmd

import "github.com/spf13/cobra"

var (
	configPath string
	envName    string
)

func Root() *cobra.Command {
	root := &cobra.Command{
		Use:   "dttui",
		Short: "Dynatrace logs CLI/TUI",
		Long:  "Search Dynatrace logs via DQL. With no subcommand, launches the TUI.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTUI(cmd, args)
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&configPath, "config", "", "path to config file (default ~/.config/dynatrace-tui/config.yaml)")
	root.PersistentFlags().StringVarP(&envName, "env", "e", "", "name of the environment to use (overrides `default:` in config)")

	root.AddCommand(queryCmd())
	return root
}
