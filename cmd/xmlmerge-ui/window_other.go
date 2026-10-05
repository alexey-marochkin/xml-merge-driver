//go:build !windows

package main

import (
	"fmt"
	"xmlmerge/internal/ui"
)

func runWindow(*ui.Server) error {
	return fmt.Errorf("интерактивное приложение поддерживает Windows")
}
func showError(err error) { diagnostic(err) }
