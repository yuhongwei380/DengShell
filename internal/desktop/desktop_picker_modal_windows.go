//go:build desktop && windows

package desktop

import (
	"sync"

	"golang.org/x/sys/windows"
)

var (
	pickerModalStates sync.Map // OS thread ID -> *pickerModalState
	pickerModalHook   = windows.NewCallback(pickerModalEvent)
)

type pickerModalState struct {
	main, dialog      uintptr
	reenable, refocus bool
}

func (state *pickerModalState) enableMain() {
	if state.reenable {
		windowUser32.NewProc("EnableWindow").Call(state.main, 1)
		state.reenable = false
	}
}

// Implement application modality without an HWND owner relationship. Giving a
// dialog the WebView HWND implicitly attaches their input queues, sharing both
// ShowCursor counts and cursor handles even across otherwise independent STAs.
// A one-time cursor reset cannot protect against later Chromium hide calls.
func installPickerModalGuard(main uintptr) (func(), error) {
	thread := windows.GetCurrentThreadId()
	state := &pickerModalState{main: main}
	pickerModalStates.Store(thread, state)
	hook, _, err := windowUser32.NewProc("SetWindowsHookExW").Call(5, pickerModalHook, 0, uintptr(thread)) // WH_CBT
	if hook == 0 {
		pickerModalStates.Delete(thread)
		return nil, err
	}
	if main != 0 {
		// Preserve a window that was already disabled by another modal operation.
		previouslyDisabled, _, _ := windowUser32.NewProc("EnableWindow").Call(main, 0)
		state.reenable = previouslyDisabled == 0
	}
	return func() {
		state.enableMain() // also covers COM/configuration failures before Show
		windowUser32.NewProc("UnhookWindowsHookEx").Call(hook)
		pickerModalStates.Delete(thread)
		if state.refocus {
			windowUser32.NewProc("SetForegroundWindow").Call(main)
		}
	}, nil
}

func pickerModalEvent(code int32, window, data uintptr) uintptr {
	if value, ok := pickerModalStates.Load(windows.GetCurrentThreadId()); ok {
		state := value.(*pickerModalState)
		if code == 5 && state.dialog == 0 { // HCBT_ACTIVATE
			owner, _, _ := windowUser32.NewProc("GetWindow").Call(window, 4) // GW_OWNER
			if owner == 0 {
				state.dialog = window
			}
		}
		if code == 4 && window == state.dialog { // HCBT_DESTROYWND
			foreground, _, _ := windowUser32.NewProc("GetForegroundWindow").Call()
			foregroundThread, _, _ := windowUser32.NewProc("GetWindowThreadProcessId").Call(foreground, 0)
			// Do not steal focus if the user switched to another application.
			state.refocus = state.reenable && uint32(foregroundThread) == windows.GetCurrentThreadId()
			// Re-enable BEFORE the dialog is destroyed so Windows can activate main.
			state.enableMain()
		}
	}
	result, _, _ := windowUser32.NewProc("CallNextHookEx").Call(0, uintptr(code), window, data)
	return result
}
