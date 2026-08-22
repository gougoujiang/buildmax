//go:build windows

package proc

import (
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// sysProcAttr detaches the child from this console's Ctrl+C group; tree
// termination is the Job Object's, not the console's.
func sysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

// treeKiller owns a Job Object holding the child and its descendants.
// KILL_ON_JOB_CLOSE makes the OS reap the tree even if this process dies
// before calling close. Windows has no polite signal, so graceful and
// forced termination are the same operation.
//
// Known limit: the child can spawn a grandchild before assignment lands, and
// that grandchild is outside the job. Closing the race needs CREATE_SUSPENDED
// plus a manual resume; deferred until evidence shows it matters.
type treeKiller struct {
	job windows.Handle
}

func newTreeKiller(p *os.Process) (*treeKiller, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	if _, err := windows.SetInformationJobObject(
		job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)),
	); err != nil {
		_ = windows.CloseHandle(job)
		return nil, err
	}
	proc, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(p.Pid),
	)
	if err != nil {
		_ = windows.CloseHandle(job)
		return nil, err
	}
	defer func() { _ = windows.CloseHandle(proc) }()
	if err := windows.AssignProcessToJobObject(job, proc); err != nil {
		_ = windows.CloseHandle(job)
		return nil, err
	}
	return &treeKiller{job: job}, nil
}

func (k *treeKiller) signal(bool) error {
	err := windows.TerminateJobObject(k.job, 1)
	if err == windows.ERROR_ACCESS_DENIED {
		// Already terminated.
		return nil
	}
	return err
}

func (k *treeKiller) close() { _ = windows.CloseHandle(k.job) }
