package main

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// killChildrenOnExit puts this process in a job object that kills every
// process in it when its last handle closes, that is when this process
// exits. Children inherit the job, so the agent and its descendants die
// with the proxy however it exits, including TerminateProcess:
// Windows does not otherwise stop children when their parent dies.
func killChildrenOnExit() error {
	// Not inheritable: a handle held by a child would keep the job open.
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			// Only children created with CREATE_BREAKAWAY_FROM_JOB, daemons meant to outlive us, may leave.
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE | windows.JOB_OBJECT_LIMIT_BREAKAWAY_OK,
		},
	}
	_, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))
	if err == nil {
		err = windows.AssignProcessToJobObject(job, windows.CurrentProcess())
	}
	if err != nil {
		windows.CloseHandle(job)
		return err
	}
	// The handle is left open on purpose: closing it would kill us.
	return nil
}
