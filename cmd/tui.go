package cmd

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/kantis/dynatrace-tui/internal/auth"
	"github.com/kantis/dynatrace-tui/internal/config"
	"github.com/kantis/dynatrace-tui/internal/grail"
	"github.com/kantis/dynatrace-tui/internal/tui"
)

func runTUI(cmd *cobra.Command, args []string) error {
	loaded, err := config.Load(configPath, envName)
	if err != nil {
		return err
	}
	if len(loaded.Names) == 0 {
		return fmt.Errorf("no environments configured in %s", loaded.Path)
	}

	makeClient := func(name string) (*grail.Client, error) {
		cfg, err := loaded.Config(name)
		if err != nil {
			return nil, err
		}
		var tokens grail.TokenProvider
		if cfg.PlatformToken != "" {
			tokens = auth.Static(cfg.PlatformToken)
		} else {
			tokens = auth.New(cfg.ClientID, cfg.ClientSecret, cfg.Scopes)
		}
		return grail.New(cfg.EnvironmentID, tokens), nil
	}

	client, err := makeClient(loaded.Selected)
	if err != nil {
		return err
	}

	prog := tea.NewProgram(
		tui.New(client, loaded.Selected, loaded.Names, makeClient, loaded.VimMode, loaded.TimePickerFrom, loaded.TimePickerTo),
		tea.WithAltScreen(),
	)
	_, err = prog.Run()
	return err
}
