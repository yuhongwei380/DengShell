//go:build desktop && windows

package desktop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/windows"
)

// Model the actual failure condition: the caller's UI thread has hidden the
// pointer after keyboard input. The picker must have an independent, visible
// cursor count without changing the caller's count, on success AND error.
func TestWindowsPickerCursorThreadIsolation(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	showCursor := windowUser32.NewProc("ShowCursor")
	cursorCount := func() int32 {
		showCursor.Call(1)
		count, _, _ := showCursor.Call(0)
		return int32(count)
	}
	for i := 0; i < 3; i++ {
		showCursor.Call(0)
		defer showCursor.Call(1)
	}
	hiddenCount := cursorCount()
	if hiddenCount >= 0 {
		t.Fatalf("测试前提：调用线程应隐藏指针，计数 %d", hiddenCount)
	}
	caller := windows.GetCurrentThreadId()
	failure := errors.New("picker failed")
	for _, expectedError := range []error{nil, failure} {
		value, err := runWindowsFileDialog(func(_ uintptr) (int, error) {
			if windows.GetCurrentThreadId() == caller {
				return 0, errors.New("选择框仍在调用线程上打开")
			}
			if count := cursorCount(); count < 0 {
				return 0, fmt.Errorf("选择框继承隐藏指针计数 %d", count)
			}
			return 42, expectedError
		})
		if value != 42 || !errors.Is(err, expectedError) {
			t.Fatalf("返回值/错误没有透传：%d / %v", value, err)
		}
		if count := cursorCount(); count != hiddenCount {
			t.Fatalf("选择框改变了调用线程的指针计数：%d -> %d", hiddenCount, count)
		}
	}
}

func TestWindowsOpenDialogConfiguration(t *testing.T) {
	for _, kind := range []string{"file", "multi", "folder"} {
		t.Run(kind, func(t *testing.T) {
			directory := t.TempDir()
			_, err := runWindowsFileDialog(func(_ uintptr) (bool, error) {
				dialog, err := newWindowsOpenDialog(wailsruntime.OpenDialogOptions{
					Title: "选择文件", DefaultDirectory: directory, ShowHiddenFiles: true,
					Filters: []wailsruntime.FileFilter{{DisplayName: "字体", Pattern: "*.ttf;*.otf"}},
				}, kind == "multi", kind == "folder")
				if err != nil {
					return false, err
				}
				defer dialog.release()
				var flags uint32
				if err := dialog.call(10, uintptr(unsafe.Pointer(&flags))); err != nil {
					return false, err
				}
				required := uint32(saveForceFilesystem | savePathMustExist | openFileMustExist | openShowHidden)
				if flags&required != required || (flags&openMultiSelect != 0) != (kind == "multi") ||
					(flags&openPickFolders != 0) != (kind == "folder") {
					return false, fmt.Errorf("%s 选项不匹配：%#x", kind, flags)
				}
				if err := dialog.setFilters([]wailsruntime.FileFilter{{DisplayName: "bad\x00name", Pattern: "*.*"}}); err == nil {
					return false, errors.New("非法过滤器必须返回错误")
				}
				return true, nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

// Like the save-picker test, only operate on this test process's unique dialog.
// Cover repeated cancel/accept, Unicode paths, multi-selection and directories.
func TestWindowsOpenDialogNative(t *testing.T) {
	if os.Getenv("DENGSHELL_NATIVE_DIALOG_TEST") != "1" {
		t.Skip("set DENGSHELL_NATIVE_DIALOG_TEST=1 for native picker integration")
	}
	for _, kind := range []string{"file", "multi", "folder"} {
		for _, accept := range []bool{false, true, false, true} {
			t.Run(fmt.Sprintf("%s/accept=%v", kind, accept), func(t *testing.T) {
				directory := t.TempDir()
				names := []string{"测试 file.txt", "另一个.txt"}
				for _, name := range names {
					if err := os.WriteFile(filepath.Join(directory, name), []byte("picker QA"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				title := fmt.Sprintf("DevShell native open QA %d %d", os.Getpid(), time.Now().UnixNano())
				stop := driveTestOpenPicker(title, accept)
				defer stop()
				options := wailsruntime.OpenDialogOptions{Title: title, DefaultDirectory: directory}
				var selected []string
				var err error
				switch kind {
				case "multi":
					options.DefaultFilename = `"` + strings.Join(names, `" "`) + `"`
					selected, err = platformOpenMultipleFilesDialog(context.Background(), options)
				case "file":
					options.DefaultFilename = names[0]
					var path string
					path, err = platformOpenFileDialog(context.Background(), options)
					if path != "" {
						selected = []string{path}
					}
				case "folder":
					var path string
					path, err = platformOpenDirectoryDialog(context.Background(), options)
					if path != "" {
						selected = []string{path}
					}
				}
				if err != nil {
					t.Fatal(err)
				}
				var want []string
				if accept {
					want = []string{filepath.Join(directory, names[0])}
					if kind == "multi" {
						want = append(want, filepath.Join(directory, names[1]))
					} else if kind == "folder" {
						want = []string{directory}
					}
				}
				if !reflect.DeepEqual(selected, want) {
					t.Fatalf("选择结果 %q，期望 %q", selected, want)
				}
			})
		}
	}
}

// Reproduce a keyboard-hidden pointer on a real modal owner. Check the picker
// thread's count at activation, AFTER Windows attaches the owner, without
// moving the user's mouse or changing system pointer settings.
func TestWindowsPickerHiddenOwnerNative(t *testing.T) {
	if os.Getenv("DENGSHELL_NATIVE_DIALOG_TEST") != "1" {
		t.Skip("set DENGSHELL_NATIVE_DIALOG_TEST=1 for native picker integration")
	}
	owner, stopOwner := testHiddenPickerOwner(t)
	defer stopOwner()
	for _, kind := range []string{"open", "save"} {
		for _, accept := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/accept=%v", kind, accept), func(t *testing.T) {
				checkHiddenOwnerPicker(t, owner, kind, accept)
			})
		}
	}
}

func checkHiddenOwnerPicker(t *testing.T, owner uintptr, kind string, accept bool) {
	t.Helper()
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "source.txt"), []byte("QA"), 0600); err != nil {
		t.Fatal(err)
	}
	title := fmt.Sprintf("DevShell hidden owner QA %d %s %v", os.Getpid(), kind, accept)
	stopPicker := driveTestOpenPicker(title, accept)
	defer stopPicker()
	_, err := runWindowsFileDialog(func(actualOwner uintptr) (bool, error) {
		if actualOwner != owner {
			return false, fmt.Errorf("选择框所属窗口 %x，期望 %x", actualOwner, owner)
		}
		checked := false
		var checkError error
		callback := syscall.NewCallback(func(code int32, window, data uintptr) uintptr {
			// The production hook was registered first. Let it restore visibility
			// before checking the activated dialog's state.
			result, _, _ := windowUser32.NewProc("CallNextHookEx").Call(0, uintptr(code), window, data)
			if code == 5 { // HCBT_ACTIVATE
				parent, _, _ := windowUser32.NewProc("GetWindow").Call(window, 4) // GW_OWNER
				if parent == owner {
					checked = true
					ownerThread, _, _ := windowUser32.NewProc("GetWindowThreadProcessId").Call(owner, 0)
					windowUser32.NewProc("ShowCursor").Call(1)
					count, _, _ := windowUser32.NewProc("ShowCursor").Call(0)
					if uint32(ownerThread) == windows.GetCurrentThreadId() || int32(count) < 0 {
						checkError = fmt.Errorf("模态窗口未隔离或指针仍隐藏：cursor=%d", int32(count))
					}
				}
			}
			return result
		})
		hook, _, err := windowUser32.NewProc("SetWindowsHookExW").Call(5, callback, 0, uintptr(windows.GetCurrentThreadId()))
		if hook == 0 {
			return false, err
		}
		defer windowUser32.NewProc("UnhookWindowsHookEx").Call(hook)
		var selected bool
		if kind == "save" {
			var path string
			path, err = showWindowsSaveDialog(owner, wailsruntime.SaveDialogOptions{
				Title: title, DefaultDirectory: directory, DefaultFilename: "download.txt",
			})
			selected = path != ""
		} else {
			var paths []string
			paths, err = showWindowsOpenDialog(owner, wailsruntime.OpenDialogOptions{
				Title: title, DefaultDirectory: directory, DefaultFilename: "source.txt",
			}, true, false)
			selected = len(paths) != 0
		}
		if err != nil {
			return false, err
		}
		if !checked {
			return false, errors.New("没有检查到模态选择框激活")
		}
		if selected != accept {
			return false, fmt.Errorf("选择状态 %v，期望 %v", selected, accept)
		}
		return true, checkError
	})
	if err != nil {
		t.Fatal(err)
	}
	count, _, _ := windowUser32.NewProc("SendMessageW").Call(owner, 0x8000, 0, 0)
	if int32(count) != -3 {
		t.Fatalf("选择框未平衡指针计数，父窗口剩余 %d，期望 -3", int32(count))
	}
}

func testHiddenPickerOwner(t *testing.T) (uintptr, func()) {
	t.Helper()
	type result struct {
		handle uintptr
		err    error
	}
	ready := make(chan result, 1)
	stopped := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(stopped)
		className, _ := windows.UTF16PtrFromString("DengShellWindow")
		module, _, _ := windowKernel32.NewProc("GetModuleHandleW").Call(0)
		type windowClass struct {
			size, style          uint32
			proc                 uintptr
			classExtra, winExtra int32
			instance, icon       uintptr
			cursor, background   uintptr
			menu, name           *uint16
			smallIcon            uintptr
		}
		class := windowClass{instance: module, name: className}
		class.size = uint32(unsafe.Sizeof(class))
		class.proc = syscall.NewCallback(func(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
			if msg == 0x8000 { // query this thread's cursor count without changing it
				windowUser32.NewProc("ShowCursor").Call(1)
				count, _, _ := windowUser32.NewProc("ShowCursor").Call(0)
				return count
			}
			if msg == 2 { // WM_DESTROY
				windowUser32.NewProc("PostQuitMessage").Call(0)
			}
			result, _, _ := windowUser32.NewProc("DefWindowProcW").Call(hwnd, uintptr(msg), wparam, lparam)
			return result
		})
		atom, _, err := windowUser32.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&class)))
		if atom == 0 {
			ready <- result{err: err}
			return
		}
		defer windowUser32.NewProc("UnregisterClassW").Call(uintptr(unsafe.Pointer(className)), module)
		hwnd, _, err := windowUser32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(className)), 0, 0xcf0000,
			0, 0, 300, 200, 0, 0, module, 0)
		if hwnd == 0 {
			ready <- result{err: err}
			return
		}
		for i := 0; i < 3; i++ {
			windowUser32.NewProc("ShowCursor").Call(0)
			defer windowUser32.NewProc("ShowCursor").Call(1)
		}
		ready <- result{handle: hwnd}
		var message struct {
			hwnd           uintptr
			kind           uint32
			wparam, lparam uintptr
			time           uint32
			x, y           int32
			private        uint32
		}
		for {
			got, _, _ := windowUser32.NewProc("GetMessageW").Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
			if int32(got) <= 0 {
				return
			}
			windowUser32.NewProc("TranslateMessage").Call(uintptr(unsafe.Pointer(&message)))
			windowUser32.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&message)))
		}
	}()
	r := <-ready
	if r.err != nil {
		t.Fatal(r.err)
	}
	return r.handle, func() {
		windowUser32.NewProc("PostMessageW").Call(r.handle, 0x10, 0, 0) // WM_CLOSE
		select {
		case <-stopped:
		case <-time.After(5 * time.Second):
			t.Error("测试所属窗口未关闭")
		}
	}
}

func driveTestOpenPicker(title string, accept bool) func() {
	done := make(chan struct{})
	go func() {
		caption, _ := windows.UTF16PtrFromString(title)
		deadline := time.Now().Add(10 * time.Second)
		for {
			select {
			case <-done:
				return
			case <-time.After(100 * time.Millisecond):
			}
			handle, _, _ := windowUser32.NewProc("FindWindowW").Call(0, uintptr(unsafe.Pointer(caption)))
			if handle == 0 {
				continue
			}
			var processID uint32
			windowUser32.NewProc("GetWindowThreadProcessId").Call(handle, uintptr(unsafe.Pointer(&processID)))
			if processID != uint32(os.Getpid()) {
				continue
			}
			command := uintptr(2) // IDCANCEL
			if accept && time.Now().Before(deadline) {
				command = 1 // IDOK
			}
			windowUser32.NewProc("PostMessageW").Call(handle, 0x111, command, 0)
		}
	}()
	return func() { close(done) }
}
