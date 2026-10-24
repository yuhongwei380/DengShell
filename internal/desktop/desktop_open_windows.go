//go:build desktop && windows

package desktop

import (
	"context"
	"runtime"
	"unsafe"

	"github.com/go-ole/go-ole"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/windows"
)

const (
	openPickFolders   = 0x20
	openMultiSelect   = 0x200
	openFileMustExist = 0x1000
	openShowHidden    = 0x10000000
)

func platformOpenFileDialog(_ context.Context, options wailsruntime.OpenDialogOptions) (string, error) {
	return singleWindowsOpenDialog(options, false)
}

func platformOpenDirectoryDialog(_ context.Context, options wailsruntime.OpenDialogOptions) (string, error) {
	return singleWindowsOpenDialog(options, true)
}

func singleWindowsOpenDialog(options wailsruntime.OpenDialogOptions, folder bool) (string, error) {
	paths, err := runWindowsFileDialog(func(owner uintptr) ([]string, error) {
		return showWindowsOpenDialog(owner, options, false, folder)
	})
	if err != nil || len(paths) == 0 {
		return "", err
	}
	return paths[0], nil
}

func platformOpenMultipleFilesDialog(_ context.Context, options wailsruntime.OpenDialogOptions) ([]string, error) {
	return runWindowsFileDialog(func(owner uintptr) ([]string, error) {
		return showWindowsOpenDialog(owner, options, true, false)
	})
}

func newWindowsOpenDialog(options wailsruntime.OpenDialogOptions, multi, folder bool) (_ *shellCOMObject, err error) {
	unknown, err := ole.CreateInstance(
		ole.NewGUID("{DC1C5A9C-E88A-4DDE-A5A1-60F82A20AEF7}"), // FileOpenDialog
		ole.NewGUID("{D57C7288-D4AD-4768-BE02-9D969532D960}")) // IFileOpenDialog
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
	flags |= saveForceFilesystem | savePathMustExist | openFileMustExist | 0x8 // FOS_NOCHANGEDIR
	if multi {
		flags |= openMultiSelect
	}
	if folder {
		flags |= openPickFolders
	}
	if options.ShowHiddenFiles {
		flags |= openShowHidden
	}
	if err = dialog.call(9, uintptr(flags)); err != nil { // SetOptions
		return nil, err
	}
	if err = dialog.setText(17, options.Title); err != nil {
		return nil, err
	}
	if err = dialog.setText(15, options.DefaultFilename); err != nil {
		return nil, err
	}
	if options.DefaultDirectory != "" {
		if err = dialog.setFolder(options.DefaultDirectory); err != nil {
			return nil, err
		}
	}
	if !folder {
		if err = dialog.setFilters(options.Filters); err != nil {
			return nil, err
		}
	}
	return dialog, nil
}

func (dialog *shellCOMObject) setFilters(filters []wailsruntime.FileFilter) error {
	if len(filters) == 0 {
		return nil
	}
	type filterSpec struct{ name, pattern *uint16 }
	specs := make([]filterSpec, len(filters))
	for i, filter := range filters {
		name, err := windows.UTF16PtrFromString(filter.DisplayName)
		if err != nil {
			return err
		}
		pattern, err := windows.UTF16PtrFromString(filter.Pattern)
		if err != nil {
			return err
		}
		specs[i] = filterSpec{name, pattern}
	}
	err := dialog.call(4, uintptr(len(specs)), uintptr(unsafe.Pointer(&specs[0]))) // SetFileTypes
	runtime.KeepAlive(specs)
	return err
}

func showWindowsOpenDialog(owner uintptr, options wailsruntime.OpenDialogOptions, multi, folder bool) ([]string, error) {
	dialog, err := newWindowsOpenDialog(options, multi, folder)
	if err != nil {
		return nil, err
	}
	defer dialog.release()
	if err = dialog.call(3, owner); err != nil { // Show
		if status, ok := err.(*ole.OleError); ok && status.Code() == saveCancelled {
			return nil, nil
		}
		return nil, err
	}
	var items *shellCOMObject
	if err = dialog.call(27, uintptr(unsafe.Pointer(&items))); err != nil { // IFileOpenDialog.GetResults
		return nil, err
	}
	defer items.release()
	var count uint32
	if err = items.call(7, uintptr(unsafe.Pointer(&count))); err != nil { // IShellItemArray.GetCount
		return nil, err
	}
	paths := make([]string, 0, count)
	for i := uint32(0); i < count; i++ {
		var item *shellCOMObject
		if err = items.call(8, uintptr(i), uintptr(unsafe.Pointer(&item))); err != nil { // GetItemAt
			return nil, err
		}
		path, err := item.filesystemPath()
		item.release()
		if err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, nil
}
