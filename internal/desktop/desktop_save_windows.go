//go:build desktop && windows

package desktop

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/go-ole/go-ole"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/windows"
)

const (
	saveOverwritePrompt = 0x2
	saveForceFilesystem = 0x40
	savePathMustExist   = 0x800
	saveNoTestCreate    = 0x10000
	saveCancelled       = 0x800704c7
)

// DownloadTo writes a temporary file and renames it after the transfer finishes.
// The shell's preliminary create/delete of the destination is redundant and can
// reject a path the actual writer can use. Wails doesn't expose
// FOS_NOTESTFILECREATE, so use IFileSaveDialog directly, retaining path validation
// and the overwrite prompt. Real write failures still propagate from DownloadTo.
// https://learn.microsoft.com/windows/win32/api/shobjidl_core/ne-shobjidl_core-_fileopendialogoptions
func platformSaveFileDialog(_ context.Context, options wailsruntime.SaveDialogOptions) (string, error) {
	return runWindowsFileDialog(func(owner uintptr) (string, error) {
		return showWindowsSaveDialog(owner, options)
	})
}

// Keep ALL file pickers off WebView's UI thread. Chromium's
// HideCursorWhileTyping can leave that thread's ShowCursor count negative when
// a keyboard-triggered modal disables its parent, which then cannot receive the
// mouse event that would show it again. Use an unowned dialog on a separate STA
// and manually disable main: an HWND owner would attach their input queues and
// let later Chromium cursor changes leak back into the picker.
// https://github.com/electron/electron/issues/54630
// Do not change the user's system-wide "Hide pointer while typing" setting.
func runWindowsFileDialog[T any](show func(owner uintptr) (T, error)) (T, error) {
	type result struct {
		value T
		err   error
	}
	done := make(chan result, 1)
	owner := platformWindowHandle()
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		r := func() result {
			err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED|ole.COINIT_DISABLE_OLE1DDE)
			if err != nil {
				// S_FALSE also requires CoUninitialize: this thread already had an STA.
				if status, ok := err.(*ole.OleError); !ok || status.Code() != 1 {
					return result{err: err}
				}
			}
			defer ole.CoUninitialize()
			restore, err := installPickerModalGuard(owner)
			if err != nil {
				return result{err: err}
			}
			defer restore()
			selected, err := show(0)
			return result{selected, err}
		}()
		done <- r
	}()
	r := <-done
	return r.value, r.err
}

// COM objects contain only their native vtable here. Never store Go state (such
// as the owner HWND) in memory immediately after that pointer: it belongs to COM.
type shellCOMObject struct{ vtable *[29]uintptr }

//go:uintptrescapes
func (object *shellCOMObject) call(method int, args ...uintptr) error {
	params := append([]uintptr{uintptr(unsafe.Pointer(object))}, args...)
	hr, _, _ := syscall.SyscallN(object.vtable[method], params...)
	runtime.KeepAlive(object)
	if int32(hr) < 0 {
		return ole.NewError(uintptr(uint32(hr)))
	}
	return nil
}

func (object *shellCOMObject) release() {
	syscall.SyscallN(object.vtable[2], uintptr(unsafe.Pointer(object)))
}

func (object *shellCOMObject) setText(method int, text string) error {
	value, err := windows.UTF16PtrFromString(text)
	if err != nil {
		return err
	}
	err = object.call(method, uintptr(unsafe.Pointer(value)))
	runtime.KeepAlive(value)
	return err
}

func newWindowsSaveDialog(options wailsruntime.SaveDialogOptions) (_ *shellCOMObject, err error) {
	unknown, err := ole.CreateInstance(
		ole.NewGUID("{C0B4E2F3-BA21-4773-8DBA-335EC946EB8B}"),
		ole.NewGUID("{84BCCD23-5FDE-4CDB-AEA4-AF64B83D78AB}"))
	if err != nil {
		return nil, err
	}
	dialog := (*shellCOMObject)(unsafe.Pointer(unknown))
	defer func() {
		if err != nil {
			dialog.release()
		}
	}()
	var flags uint32
	if err = dialog.call(10, uintptr(unsafe.Pointer(&flags))); err != nil { // GetOptions
		return nil, err
	}
	flags |= saveOverwritePrompt | saveForceFilesystem | savePathMustExist | saveNoTestCreate
	if err = dialog.call(9, uintptr(flags)); err != nil { // SetOptions
		return nil, err
	}
	if err = dialog.setText(17, options.Title); err != nil { // SetTitle
		return nil, err
	}
	if err = dialog.setText(15, options.DefaultFilename); err != nil { // SetFileName
		return nil, err
	}
	if options.DefaultDirectory != "" {
		if err = dialog.setFolder(options.DefaultDirectory); err != nil {
			return nil, err
		}
	}
	return dialog, nil
}

func (dialog *shellCOMObject) setFolder(directory string) error {
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("保存位置不是目录：%s", absolute)
	}
	value, err := windows.UTF16PtrFromString(absolute)
	if err != nil {
		return err
	}
	iid := ole.NewGUID("{43826D1E-E718-42EE-BC55-A1E261C37BFE}") // IShellItem
	var folder *shellCOMObject
	hr, _, _ := windowShell32.NewProc("SHCreateItemFromParsingName").Call(
		uintptr(unsafe.Pointer(value)), 0, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&folder)))
	runtime.KeepAlive(value)
	runtime.KeepAlive(iid)
	if int32(hr) < 0 {
		return ole.NewError(uintptr(uint32(hr)))
	}
	defer folder.release()
	return dialog.call(12, uintptr(unsafe.Pointer(folder))) // SetFolder
}

func showWindowsSaveDialog(owner uintptr, options wailsruntime.SaveDialogOptions) (string, error) {
	dialog, err := newWindowsSaveDialog(options)
	if err != nil {
		return "", err
	}
	defer dialog.release()
	if err = dialog.call(3, owner); err != nil { // Show
		if status, ok := err.(*ole.OleError); ok && status.Code() == saveCancelled {
			return "", nil
		}
		return "", err
	}
	var item *shellCOMObject
	if err = dialog.call(20, uintptr(unsafe.Pointer(&item))); err != nil { // GetResult
		return "", err
	}
	defer item.release()
	return item.filesystemPath()
}

func (item *shellCOMObject) filesystemPath() (string, error) {
	var filename *uint16
	if err := item.call(5, 0x80058000, uintptr(unsafe.Pointer(&filename))); err != nil { // SIGDN_FILESYSPATH
		return "", err
	}
	defer ole.CoTaskMemFree(uintptr(unsafe.Pointer(filename)))
	return windows.UTF16PtrToString(filename), nil
}
