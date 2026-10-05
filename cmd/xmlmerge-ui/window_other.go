//go:build !windows && !linux

package main

import (
	"fmt"
	"xmlmerge/internal/ui"
)

func runWindow(*ui.Server) error {
	return fmt.Errorf("интерактивное приложение поддерживает Windows и Linux")
}
func showError(err error) { diagnostic(err) }
