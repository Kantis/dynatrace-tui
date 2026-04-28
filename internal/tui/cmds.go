package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kantis/dynatrace-tui/internal/grail"
)

type executeMsg struct {
	resp *grail.Response
	err  error
}

type pollMsg struct {
	resp *grail.Response
	err  error
}

type cancelDoneMsg struct{}

func executeCmd(ctx context.Context, c *grail.Client, dql string) tea.Cmd {
	return func() tea.Msg {
		resp, err := c.Execute(ctx, grail.ExecuteRequest{Query: dql})
		return executeMsg{resp: resp, err: err}
	}
}

func pollCmd(ctx context.Context, c *grail.Client, token string) tea.Cmd {
	return func() tea.Msg {
		resp, err := c.PollUntilDone(ctx, token)
		return pollMsg{resp: resp, err: err}
	}
}

func cancelCmd(c *grail.Client, token string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), apiCancelTimeout)
		defer cancel()
		_ = c.Cancel(ctx, token)
		return cancelDoneMsg{}
	}
}
