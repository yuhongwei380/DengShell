//go:build desktop && windows

package desktop

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"github.com/go-ole/go-ole"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/windows"
)

func TestWindowsSaveDialogConfiguration(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		if status, ok := err.(*ole.OleError); !ok || status.Code() != 1 {
			t.Fatal(err)
		}
	}
	defer ole.CoUninitialize()
	directory := t.TempDir()
	dialog, err := newWindowsSaveDialog(wailsruntime.SaveDialogOptions{
		Title: "下载文件", DefaultFilename: "测试 file.yml", DefaultDirectory: directory,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer dialog.release()
	var flags uint32
	if err := dialog.call(10, uintptr(unsafe.Pointer(&flags))); err != nil {
		t.Fatal(err)
	}
	for _, required := range []uint32{saveNoTestCreate, saveOverwritePrompt, savePathMustExist, saveForceFilesystem} {
		if flags&required == 0 {
			t.Fatalf("保存框缺少选项 %#x（当前 %#x）", required, flags)
		}
	}
	if err := dialog.setFolder(filepath.Join(directory, "missing")); err == nil {
		t.Fatal("不存在的目录必须返回错误")
	}
	if err := dialog.setText(15, "bad\x00name"); err == nil {
		t.Fatal("非法文件名必须返回错误")
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 0 {
		t.Fatalf("选择文件前不应试写目录：%v / %v", entries, err)
	}
}

// Opt-in UI test: operates only on this test process's uniquely titled native
// picker. It never drives an existing DevShell window or writes a user file.
func TestWindowsSaveDialogNative(t *testing.T) {
	if os.Getenv("DENGSHELL_NATIVE_DIALOG_TEST") != "1" {
		t.Skip("set DENGSHELL_NATIVE_DIALOG_TEST=1 for native picker integration")
	}
	for _, accept := range []bool{false, true} {
		t.Run(fmt.Sprintf("accept=%v", accept), func(t *testing.T) {
			directory := t.TempDir()
			title := fmt.Sprintf("DevShell native save QA %d %v", os.Getpid(), accept)
			done := make(chan struct{})
			defer close(done)
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
			selected, err := platformSaveFileDialog(context.Background(), wailsruntime.SaveDialogOptions{
				Title: title, DefaultFilename: "测试 file.yml", DefaultDirectory: directory,
			})
			if err != nil {
				t.Fatal(err)
			}
			want := ""
			if accept {
				want = filepath.Join(directory, "测试 file.yml")
			}
			if selected != want {
				t.Fatalf("选择结果 %q，期望 %q", selected, want)
			}
			entries, err := os.ReadDir(directory)
			if err != nil || len(entries) != 0 {
				t.Fatalf("保存框不应创建文件：%v / %v", entries, err)
			}
		})
	}
}
