package installguard

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestInstallerProcessFixture(t *testing.T) {
	mode := os.Getenv("DEVSHELL_INSTALL_PROCESS_FIXTURE")
	if mode == "" {
		return
	}
	if mode != "1" {
		runFixtureWindow(t, mode)
		return
	}
	time.Sleep(45 * time.Second)
}

func startProcessFixture(t *testing.T) (string, *exec.Cmd) {
	return startProcessFixtureMode(t, "1")
}

func startProcessFixtureMode(t *testing.T, mode string) (string, *exec.Cmd) {
	t.Helper()
	directory := t.TempDir()
	current, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "DevShell.exe")
	if err = copyFile(current, path); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(path, "-test.run=^TestInstallerProcessFixture$")
	command.Env = append(os.Environ(), "DEVSHELL_INSTALL_PROCESS_FIXTURE="+mode)
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { command.Process.Kill(); command.Wait() })
	return directory, command
}

func runFixtureWindow(t *testing.T, mode string) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	name := "DengShellTrayWindow"
	if mode == "legacy" {
		name = "DengShellWindow"
	}
	className, _ := windows.UTF16PtrFromString(name)
	kernel := windows.NewLazySystemDLL("kernel32.dll")
	module, _, _ := kernel.NewProc("GetModuleHandleW").Call(0)
	type windowClass struct {
		size, style                        uint32
		proc                               uintptr
		classExtra, windowExtra            int32
		instance, icon, cursor, background uintptr
		menu, name                         *uint16
		smallIcon                          uintptr
	}
	message := quitMessage()
	callback := windows.NewCallback(func(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
		if uintptr(msg) == message && mode != "legacy" {
			if wparam == QuitRequest && mode != "hold" {
				user32.NewProc("DestroyWindow").Call(hwnd)
			}
			if wparam == QuitCancel {
				executable, _ := os.Executable()
				os.WriteFile(filepath.Join(filepath.Dir(executable), "cancelled"), []byte("yes"), 0600)
			}
			return QuitAcknowledged
		}
		if msg == 0x10 {
			user32.NewProc("DestroyWindow").Call(hwnd)
			return 0
		}
		if msg == 2 {
			user32.NewProc("PostQuitMessage").Call(0)
			return 0
		}
		result, _, _ := user32.NewProc("DefWindowProcW").Call(hwnd, uintptr(msg), wparam, lparam)
		return result
	})
	class := windowClass{proc: callback, instance: module, name: className}
	class.size = uint32(unsafe.Sizeof(class))
	if atom, _, err := user32.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&class))); atom == 0 {
		t.Fatal(err)
	}
	if hwnd, _, err := user32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(className)), 0, 0, 0, 0, 0, 0, 0, 0, module, 0); hwnd == 0 {
		t.Fatal(err)
	}
	executable, _ := os.Executable()
	os.WriteFile(filepath.Join(filepath.Dir(executable), "ready"), []byte("yes"), 0600)
	var msg struct {
		window         uintptr
		message        uint32
		wparam, lparam uintptr
		time           uint32
		x, y           int32
		private        uint32
	}
	for {
		ok, _, _ := user32.NewProc("GetMessageW").Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(ok) <= 0 {
			return
		}
		user32.NewProc("TranslateMessage").Call(uintptr(unsafe.Pointer(&msg)))
		user32.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func waitForFixtureFile(t *testing.T, directory, name string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(directory, name)); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("fixture marker missing: %s", name)
}

func TestGracefulNewAndLegacyShutdown(t *testing.T) {
	for _, mode := range []string{"native", "legacy"} {
		t.Run(mode, func(t *testing.T) {
			directory, _ := startProcessFixtureMode(t, mode)
			waitForFixtureFile(t, directory, "ready")
			session, err := newResourceSession(directory)
			if err != nil {
				t.Fatal(err)
			}
			defer session.close()
			waitForResource(t, session)
			prompts := 0
			accept := func(uintptr, string, string, ...choice) (int32, error) { prompts++; return 100, nil }
			if err = closeRunning(session, 0, false, accept, 5*time.Second); err != nil {
				t.Fatal(err)
			}
			if prompts != 1 {
				t.Fatalf("extra prompts: %d", prompts)
			}
		})
	}
}

func TestCancelAfterRequestClearsApplicationState(t *testing.T) {
	directory, _ := startProcessFixtureMode(t, "hold")
	waitForFixtureFile(t, directory, "ready")
	session, err := newResourceSession(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer session.close()
	waitForResource(t, session)
	count := 0
	cancel := func(uintptr, string, string, ...choice) (int32, error) {
		count++
		if count == 1 {
			return 100, nil
		}
		return 2, nil
	}
	if err = closeRunning(session, 0, false, cancel, time.Millisecond); !errors.Is(err, ErrCancelled) {
		t.Fatal(err)
	}
	waitForFixtureFile(t, directory, "cancelled")
	if len(waitForResource(t, session)) != 1 {
		t.Fatal("cancel unexpectedly exited application")
	}
}

func waitForResource(t *testing.T, session *resourceSession) []runningProcess {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		items, err := session.processes()
		if err != nil {
			t.Fatal(err)
		}
		if len(items) > 0 {
			return items
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("Restart Manager did not find fixture image")
	return nil
}

func TestResourceScopeCancelAndForce(t *testing.T) {
	if unsafe.Sizeof(processInfo{}) != 668 {
		t.Fatalf("RM_PROCESS_INFO layout: %d", unsafe.Sizeof(processInfo{}))
	}
	directory, command := startProcessFixture(t)
	otherDirectory, other := startProcessFixture(t)
	t.Logf("target PID=%d other PID=%d", command.Process.Pid, other.Process.Pid)
	session, err := newResourceSession(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer session.close()
	items := waitForResource(t, session)
	if len(items) != 1 || items[0].pid != uint32(command.Process.Pid) {
		t.Fatalf("wrong processes: %+v", items)
	}
	bad := items[0]
	bad.started.LowDateTime++
	if err = bad.terminate(); err != nil {
		t.Fatal(err)
	} // stale PID identity must be ignored
	cancel := func(uintptr, string, string, ...choice) (int32, error) { return 2, nil }
	if err = closeRunning(session, 0, false, cancel, time.Millisecond); !errors.Is(err, ErrCancelled) {
		t.Fatalf("cancel: %v", err)
	}
	if len(waitForResource(t, session)) != 1 {
		t.Fatal("cancel killed process")
	}
	if err = closeRunning(session, 0, true, cancel, time.Millisecond); err == nil {
		t.Fatal("silent install should reject running process")
	}
	answers := []int32{100, 101, 101}
	calls := 0
	force := func(_ uintptr, _, _ string, _ ...choice) (int32, error) {
		if calls >= len(answers) {
			t.Fatal("unexpected extra prompt")
		}
		answer := answers[calls]
		calls++
		return answer, nil
	}
	if err = closeRunning(session, 0, false, force, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Fatalf("force confirmation count: %d", calls)
	}
	otherSession, err := newResourceSession(otherDirectory)
	if err != nil {
		t.Fatal(err)
	}
	defer otherSession.close()
	remaining := waitForResource(t, otherSession)
	if len(remaining) != 1 || remaining[0].pid != uint32(other.Process.Pid) {
		t.Fatal("other installation affected")
	}
}

func TestInstallationLaunchGate(t *testing.T) {
	executable, _ := os.Executable()
	directory := filepath.Dir(executable)
	release, err := acquireInstallLock(directory)
	if err != nil {
		t.Fatal(err)
	}
	if err = CheckLaunch(); err == nil {
		release()
		t.Fatal("startup was not blocked")
	}
	if another, err := acquireInstallLock(directory); err == nil {
		another()
		release()
		t.Fatal("concurrent installer accepted")
	}
	release()
	if err = CheckLaunch(); err != nil {
		t.Fatal(err)
	}
}

func TestInstallerTaskDialogNative(t *testing.T) {
	if os.Getenv("DEVSHELL_NATIVE_INSTALLER_TEST") != "1" {
		t.Skip("set DEVSHELL_NATIVE_INSTALLER_TEST=1 for native installer prompt")
	}
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		caption, _ := windows.UTF16PtrFromString("DevShell 安装")
		for {
			select {
			case <-stop:
				return
			case <-time.After(100 * time.Millisecond):
			}
			var hwnd uintptr
			for {
				hwnd, _, _ = user32.NewProc("FindWindowExW").Call(0, hwnd, 0, uintptr(unsafe.Pointer(caption)))
				if hwnd == 0 {
					break
				}
				var pid uint32
				user32.NewProc("GetWindowThreadProcessId").Call(hwnd, uintptr(unsafe.Pointer(&pid)))
				if pid == uint32(os.Getpid()) {
					user32.NewProc("PostMessageW").Call(hwnd, 0x400+102, 100, 0)
				}
			}
		}
	}()
	selected, err := showChoices(0, "安装提示回归", "仅用于项目测试的安装提示，不会关闭正在运行的应用。", choice{100, "关闭 DevShell 并继续安装"}, choice{2, "取消安装"})
	if err != nil || selected != 100 {
		t.Fatalf("native prompt: %d %v", selected, err)
	}
}

func TestInstallWithoutRunningProcess(t *testing.T) {
	payload, target := t.TempDir(), t.TempDir()
	writeFixture(t, payload, "DevShell.exe", "new")
	writeFixture(t, target, "DevShell.exe", "old")
	if err := Install(payload, target, 0, true); err != nil {
		t.Fatal(err)
	}
	expectFile(t, filepath.Join(target, "DevShell.exe"), "new")
	items, err := os.ReadDir(target)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if strings.HasPrefix(item.Name(), ".devshell-install-") {
			t.Fatal("transaction left behind")
		}
	}
}

func TestLockedManagedFileRollsBackExecutable(t *testing.T) {
	payload, target := t.TempDir(), t.TempDir()
	writeFixture(t, payload, "DevShell.exe", "new")
	writeFixture(t, payload, "data/docs/guide.md", "new guide")
	writeFixture(t, target, "DevShell.exe", "old")
	locked := writeFixture(t, target, "data/docs/guide.md", "old guide")
	transaction, err := prepareTransaction(payload, target)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(transaction.scratch)
	name, _ := windows.UTF16PtrFromString(locked)
	handle, err := windows.CreateFile(name, windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	err = transaction.commit()
	windows.CloseHandle(handle)
	if err == nil {
		t.Fatal("locked target unexpectedly replaced")
	}
	expectFile(t, filepath.Join(target, "DevShell.exe"), "old")
	expectFile(t, locked, "old guide")
}

func TestInstallRunningApplicationEndToEnd(t *testing.T) {
	directory, _ := startProcessFixtureMode(t, "native")
	waitForFixtureFile(t, directory, "ready")
	payload := t.TempDir()
	writeFixture(t, payload, "DevShell.exe", "new executable")
	writeFixture(t, payload, "data/docs/guide.md", "new guide")
	writeFixture(t, directory, "data/config.json", "personal settings")
	count := 0
	accept := func(uintptr, string, string, ...choice) (int32, error) { count++; return 100, nil }
	if err := installWithPrompt(payload, directory, 0, false, accept); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("wrong shutdown prompt count: %d", count)
	}
	expectFile(t, filepath.Join(directory, "DevShell.exe"), "new executable")
	expectFile(t, filepath.Join(directory, "data/config.json"), "personal settings")
}
