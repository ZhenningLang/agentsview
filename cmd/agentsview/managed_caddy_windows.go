//go:build windows

package main

import (
	"fmt"
	"os"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

type managedCaddyGuard interface {
	Close()
}

type windowsManagedCaddyGuard struct {
	handle windows.Handle
	once   sync.Once
}

func guardManagedCaddyProcess(proc *os.Process) (managedCaddyGuard, error) {
	if proc == nil || proc.Pid <= 0 {
		// A usable no-op guard, not a nil interface: the Unix half
		// returns one for this input, and a caller that closed what it
		// was handed would panic on Windows only.
		return &windowsManagedCaddyGuard{}, nil
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("creating job object: %w", err)
	}
	guard := &windowsManagedCaddyGuard{handle: job}

	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		guard.Close()
		return nil, fmt.Errorf("configuring job object: %w", err)
	}

	processHandle, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE,
		false,
		uint32(proc.Pid),
	)
	if err != nil {
		guard.Close()
		return nil, fmt.Errorf("opening process for job object: %w", err)
	}
	defer windows.CloseHandle(processHandle)
	if err := windows.AssignProcessToJobObject(job, processHandle); err != nil {
		guard.Close()
		return nil, fmt.Errorf("assigning process to job object: %w", err)
	}

	return guard, nil
}

func (g *windowsManagedCaddyGuard) Close() {
	g.once.Do(func() {
		if g.handle != 0 {
			_ = windows.CloseHandle(g.handle)
		}
	})
}
