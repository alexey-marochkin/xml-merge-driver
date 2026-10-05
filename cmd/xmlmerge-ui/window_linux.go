//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"

	"xmlmerge/internal/ui"
)

// Linux uses the same loopback UI in the user's browser. The Close button in
// the application ends the session; Ctrl+C also stops the process.
func runWindow(server *ui.Server) error {
	url := server.URL + "/#" + server.Token
	fmt.Fprintf(os.Stderr, "XML Merge: %s\nЗакройте приложение кнопкой «Закрыть» или нажмите Ctrl+C.\n", url)
	cmd := exec.Command("xdg-open", url)
	if err := cmd.Start(); err == nil {
		go func() { _ = cmd.Wait() }()
	} else {
		fmt.Fprintf(os.Stderr, "Не удалось открыть браузер автоматически: %v\nОткройте адрес выше вручную.\n", err)
	}
	<-server.Done
	return nil
}

func showError(err error) { diagnostic(err) }
