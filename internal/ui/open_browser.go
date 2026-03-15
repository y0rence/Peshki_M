package ui

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
)

// OpenBrowser opens the given URL in the default browser.
func OpenBrowser(url string) error {
	return openBrowser(context.Background(), url)
}

func openBrowser(ctx context.Context, url string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("automatic browser launch is not implemented for %s", runtime.GOOS)
	}

	command := exec.CommandContext(ctx, "open", url)
	return command.Start()
}
