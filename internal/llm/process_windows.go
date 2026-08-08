// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
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

// KillOrphanLlamaServers 清理上一次进程崩溃残留的孤儿 llama-server 进程。
//
// P2.5 修复：默认只清理"本应用 runtime 目录"下的 llama-server.exe。
// 此前按进程名（llama-server.exe）无差别清理，会误杀用户/其他工具手动启动的
// 同名进程。传入空字符串表示按进程名清理（保持旧行为，供无路径场景回退）。
//
// 实现：先枚举进程快照，对每个 llama-server.exe 查询其可执行文件完整路径，
// 若路径位于 runtimeDir 下（前缀匹配）才执行 taskkill。
// 生活类比：停车场保安只清理"自己公司车位"上乱停的车（runtime 目录下的进程），
// 不会去动隔壁公司停在自家小区里的车。
func KillOrphanLlamaServers(runtimeDir string) {
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
		if exeName == "llama-server.exe" && orphanBelongsToApp(entry.ProcessID, runtimeDir) {
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

// orphanBelongsToApp 判断给定 PID 的 llama-server 是否属于本应用。
// runtimeDir 为空时按进程名清理（保持旧行为）；非空时要求可执行文件路径位于其下。
func orphanBelongsToApp(pid uint32, runtimeDir string) bool {
	if runtimeDir == "" {
		return true
	}
	exePath, err := processExePath(pid)
	if err != nil {
		// 无法获取路径（权限/已退出）时保守不杀，避免误杀其他进程
		log.Debug().Uint32("pid", pid).Err(err).Msg("skip orphan cleanup: cannot get exe path")
		return false
	}
	return pathWithinDir(runtimeDir, exePath)
}

// pathWithinDir 判断 exePath 是否位于 dir 目录下。
// 用 filepath.Rel 判断：能算出相对路径且不以 ".." 开头，说明 exe 在 dir 内。
// 避免纯字符串前缀匹配误判 `...\runtime_evil\...` 这类路径。
func pathWithinDir(dir, exePath string) bool {
	rel, err := filepath.Rel(dir, exePath)
	if err != nil {
		return false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return false
	}
	return true
}

// processExePath 通过 QueryFullProcessImageNameW 获取进程可执行文件完整路径。
// PROCESS_QUERY_LIMITED_INFORMATION = 0x1000（Windows 7+，允许低权限查询进程路径）。
func processExePath(pid uint32) (string, error) {
	h, err := syscall.OpenProcess(0x1000, false, pid)
	if err != nil {
		return "", err
	}
	defer func() { _ = syscall.CloseHandle(h) }()

	procQueryImageName := kernel32.NewProc("QueryFullProcessImageNameW")
	buf := make([]uint16, 1024)
	bufSize := uint32(len(buf))
	ret, _, callErr := procQueryImageName.Call(
		uintptr(h),
		0,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&bufSize)),
	)
	if ret == 0 {
		return "", callErr
	}
	return syscall.UTF16ToString(buf[:bufSize]), nil
}
