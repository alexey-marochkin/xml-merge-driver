// Developer-only browser preview of the same assets used by the native window.
package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"

	"xmlmerge/internal/interactive"
	"xmlmerge/internal/ui"
)

func main() {
	o, err := interactive.Parse(os.Args[1:], os.Stderr)
	if err != nil {
		panic(err)
	}
	s, err := interactive.New(o)
	if err != nil {
		panic(err)
	}
	server, err := ui.StartWithSettings(s, filepath.Join(filepath.Dir(o.Rules), "settings.xml"))
	if err != nil {
		panic(err)
	}
	defer server.Close()
	fmt.Println(server.URL + "/#" + server.Token)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	defer signal.Stop(sig)
	select {
	case <-server.Done:
	case <-sig:
	}
}
