package cmd

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/kantis/dynatrace-tui/internal/auth"
	"github.com/kantis/dynatrace-tui/internal/config"
	"github.com/kantis/dynatrace-tui/internal/grail"
	"github.com/kantis/dynatrace-tui/internal/tui"
)

func runTUI(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	var tokens grail.TokenProvider
	if cfg.PlatformToken != "" {
		tokens = auth.Static(cfg.PlatformToken)
	} else {
		tokens = auth.New(cfg.ClientID, cfg.ClientSecret, cfg.Scopes)
	}
	client := grail.New(cfg.EnvironmentID, tokens)

	prog := tea.NewProgram(tui.New(client), tea.WithAltScreen())
	_, err = prog.Run()
	return err
}
