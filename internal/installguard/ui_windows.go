package installguard

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

type choice struct {
	id   int32
	text string
}

// TASKDIALOGCONFIG and TASKDIALOG_BUTTON use pack(1), including on x64.
// Keep their byte layout explicit instead of relying on Go struct padding.
func showChoices(owner uintptr, heading, content string, choices ...choice) (int32, error) {
	return showChoicesWithIcon(owner, heading, content, 0, choices...)
}

func showChoicesWithIcon(owner uintptr, heading, content string, systemIcon uint64, choices ...choice) (int32, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if unsafe.Sizeof(uintptr(0)) != 8 {
		return 0, fmt.Errorf("安装助手需要 Windows x64")
	}
	deactivate, err := activateControls()
	if err != nil {
		return 0, err
	}
	defer deactivate()
	var keep []*uint16
	stringPointer := func(text string) uint64 {
		p, _ := windows.UTF16PtrFromString(text)
		keep = append(keep, p)
		return uint64(uintptr(unsafe.Pointer(p)))
	}
	var config [160]byte
	put := func(offset int, value uint64) { binary.LittleEndian.PutUint64(config[offset:], value) }
	binary.LittleEndian.PutUint32(config[:], uint32(len(config)))
	put(4, uint64(owner))
	flags := uint32(0x0008 | 0x0100) // allow cancellation, position relative to parent
	put(28, stringPointer("DevShell 安装"))
	if systemIcon != 0 {
		put(36, systemIcon)
	} else if appIcon := installerIcon(owner); appIcon != 0 {
		flags |= 0x0002 // TDF_USE_HICON_MAIN; the owner retains ownership of its icon.
		put(36, uint64(appIcon))
	}
	binary.LittleEndian.PutUint32(config[20:], flags)
	put(44, stringPointer(heading))
	put(52, stringPointer(content))
	buttons := make([]byte, len(choices)*12)
	for i, button := range choices {
		binary.LittleEndian.PutUint32(buttons[i*12:], uint32(button.id))
		binary.LittleEndian.PutUint64(buttons[i*12+4:], stringPointer(button.text))
	}
	binary.LittleEndian.PutUint32(config[60:], uint32(len(choices)))
	put(64, uint64(uintptr(unsafe.Pointer(&buttons[0]))))
	binary.LittleEndian.PutUint32(config[72:], uint32(choices[len(choices)-1].id)) // safest choice gets focus
	binary.LittleEndian.PutUint32(config[156:], 236)                               // dialog units; Windows scales the layout with DPI
	var selected int32
	hr, _, _ := windows.NewLazySystemDLL("comctl32.dll").NewProc("TaskDialogIndirect").Call(uintptr(unsafe.Pointer(&config[0])), uintptr(unsafe.Pointer(&selected)), 0, 0)
	runtime.KeepAlive(keep)
	runtime.KeepAlive(buttons)
	if int32(hr) < 0 {
		return 0, fmt.Errorf("无法显示安装提示：0x%x", hr)
	}
	return selected, nil
}

func installerIcon(owner uintptr) uintptr {
	if owner == 0 {
		return 0
	}
	// NSIS waits for this helper in another process. Never block indefinitely
	// asking that window for its icon, and never destroy its shared icon handle.
	var icon uintptr
	user32.NewProc("SendMessageTimeoutW").Call(owner, 0x007f, 1, 0, 0x0002, 100, uintptr(unsafe.Pointer(&icon))) // WM_GETICON, ICON_BIG
	if icon == 0 {
		icon, _, _ = user32.NewProc("GetClassLongPtrW").Call(owner, ^uintptr(13)) // GCLP_HICON (-14)
	}
	return icon
}

func activateControls() (func(), error) {
	directory, err := os.MkdirTemp("", "devshell-controls-")
	if err != nil {
		return nil, err
	}
	path := filepath.Join(directory, "controls.manifest")
	manifest := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0"><assemblyIdentity name="devshell.install.control" version="1.0.0.0" type="win32" processorArchitecture="amd64"/><dependency><dependentAssembly><assemblyIdentity name="Microsoft.Windows.Common-Controls" version="6.0.0.0" type="win32" processorArchitecture="amd64" publicKeyToken="6595b64144ccf1df" language="*"/></dependentAssembly></dependency></assembly>`
	if err = os.WriteFile(path, []byte(manifest), 0600); err != nil {
		os.RemoveAll(directory)
		return nil, err
	}
	source, _ := windows.UTF16PtrFromString(path)
	type actctx struct {
		size, flags                     uint32
		source                          *uint16
		arch, language                  uint16
		assembly, resource, application *uint16
		module                          uintptr
	}
	context := actctx{source: source}
	context.size = uint32(unsafe.Sizeof(context))
	kernel := windows.NewLazySystemDLL("kernel32.dll")
	handle, _, err := kernel.NewProc("CreateActCtxW").Call(uintptr(unsafe.Pointer(&context)))
	if handle == ^uintptr(0) {
		os.RemoveAll(directory)
		return nil, err
	}
	var cookie uintptr
	ok, _, err := kernel.NewProc("ActivateActCtx").Call(handle, uintptr(unsafe.Pointer(&cookie)))
	if ok == 0 {
		kernel.NewProc("ReleaseActCtx").Call(handle)
		os.RemoveAll(directory)
		return nil, err
	}
	return func() {
		kernel.NewProc("DeactivateActCtx").Call(0, cookie)
		kernel.NewProc("ReleaseActCtx").Call(handle)
		os.RemoveAll(directory)
	}, nil
}

func ShowError(owner uintptr, err error) {
	_, showError := showChoicesWithIcon(owner, "安装未完成", err.Error(), 0xfffe, choice{2, "返回安装器"}) // TD_ERROR_ICON
	if showError != nil {
		text, _ := windows.UTF16PtrFromString(err.Error())
		title, _ := windows.UTF16PtrFromString("DevShell 安装")
		user32.NewProc("MessageBoxW").Call(owner, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), 0x10)
	}
}
