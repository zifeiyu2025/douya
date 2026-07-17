// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import (
	"fmt"
	"os/exec"
	"syscall"
	"unsafe"

	"github.com/rs/zerolog/log"
)

var (
	kernel32                     = syscall.NewLazyDLL("kernel32.dll")
	procCreateJobObject          = kernel32.NewProc("CreateJobObjectW")
	procSetInformationJobObject  = kernel32.NewProc("SetInformationJobObject")
	procAssignProcessToJobObject = kernel32.NewProc("AssignProcessToJobObject")
	procCloseHandle              = kernel32.NewProc("CloseHandle")
)

const (
	JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE = 0x2000
	JobObjectExtendedLimitInformation  = 9

	PROCESS_SET_QUOTA = 0x0100
	PROCESS_TERMINATE = 0x0001
)

type JOBOBJECT_BASIC_LIMIT_INFORMATION struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}

type IO_COUNTERS struct {
	ReadOperationCount  uint64
	WriteOperationCount uint64
	OtherOperationCount uint64
	ReadTransferCount   uint64
	WriteTransferCount  uint64
	OtherTransferCount  uint64
}

type JOBOBJECT_EXTENDED_LIMIT_INFORMATION struct {
	BasicLimitInformation JOBOBJECT_BASIC_LIMIT_INFORMATION
	IoInfo                IO_COUNTERS
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

type JobObject struct {
	handle syscall.Handle
}

func CreateJobObject() (*JobObject, error) {
	handle, _, err := procCreateJobObject.Call(0, 0, 0)
	if handle == 0 {
		return nil, err
	}

	info := JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE

	// L-4：unsafe.Pointer 用于 Windows API (SetInformationJobObject) 调用，
	// 传递 JOBOBJECT_EXTENDED_LIMIT_INFORMATION 结构体指针。无内存安全替代方案，
	// 参数类型和大小已校验（unsafe.Sizeof 确保结构体大小正确传递）。
	_, _, err = procSetInformationJobObject.Call(
		handle,
		JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		unsafe.Sizeof(info),
	)
	if err != nil && err != syscall.Errno(0) {
		_, _, _ = procCloseHandle.Call(handle)
		return nil, err
	}

	return &JobObject{handle: syscall.Handle(handle)}, nil
}

func (j *JobObject) AssignProcess(pid int) error {
	processHandle, err := syscall.OpenProcess(PROCESS_SET_QUOTA|PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return err
	}
	defer func() { _ = syscall.CloseHandle(processHandle) }()

	ret, _, err := procAssignProcessToJobObject.Call(
		uintptr(j.handle),
		uintptr(processHandle),
	)
	if ret == 0 {
		return err
	}
	return nil
}

func (j *JobObject) Close() {
	if j.handle != 0 {
		_, _, _ = procCloseHandle.Call(uintptr(j.handle))
		j.handle = 0
	}
}

func KillOrphanLlamaServers() {
	snapshot, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return
	}
	defer func() { _ = syscall.CloseHandle(snapshot) }()

	var entry syscall.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))

	if err := syscall.Process32First(snapshot, &entry); err != nil {
		return
	}

	killed := 0
	for {
		exeName := syscall.UTF16ToString(entry.ExeFile[:])
		if exeName == "llama-server.exe" {
			cmd := exec.Command("taskkill", "/PID", fmt.Sprintf("%d", entry.ProcessID), "/F", "/T")
			cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
			if err := cmd.Run(); err == nil {
				log.Info().Int("pid", int(entry.ProcessID)).Msg("killed orphan llama-server process")
				killed++
			}
		}
		if err := syscall.Process32Next(snapshot, &entry); err != nil {
			break
		}
	}

	if killed > 0 {
		log.Info().Int("count", killed).Msg("cleaned up orphan llama-server process(es)")
	}
}
