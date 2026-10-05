package main

import (
	"fmt"
	"os"

	"xmlmerge/internal/interactive"
	"xmlmerge/internal/ui"
)

func main() { os.Exit(run()) }
func run() int {
	o, err := interactive.Parse(os.Args[1:], os.Stderr)
	if err != nil {
		showError(err)
		return interactive.Failure
	}
	session, err := interactive.New(o)
	if err != nil {
		showError(err)
		return interactive.Failure
	}
	server, err := ui.Start(session)
	if err != nil {
		showError(err)
		return interactive.Failure
	}
	defer server.Close()
	if err := runWindow(server); err != nil {
		showError(err)
		return interactive.Failure
	}
	return server.Close()
}
func diagnostic(err error) { fmt.Fprintln(os.Stderr, err) }
