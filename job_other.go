//go:build !windows

package main

// killChildrenOnExit is a no-op outside Windows: when the proxy dies, the
// agent sees EOF on its stdin and is expected to exit.
func killChildrenOnExit() error { return nil }
