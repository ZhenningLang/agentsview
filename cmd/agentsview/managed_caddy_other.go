//go:build !windows

package main

import "os"

type managedCaddyGuard interface {
	Close()
}

type noopManagedCaddyGuard struct{}

func guardManagedCaddyProcess(*os.Process) (managedCaddyGuard, error) {
	return noopManagedCaddyGuard{}, nil
}

func (noopManagedCaddyGuard) Close() {}
