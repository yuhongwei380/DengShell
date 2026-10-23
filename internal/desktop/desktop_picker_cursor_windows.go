//go:build desktop && windows

package desktop

import (
	"sync"

	"golang.org/x/sys/windows"
)

var (
	pickerCursorStates sync.Map // OS thread ID -> *pickerCursorState
	pickerCursorHook   = windows.NewCallback(pickerCursorActivated)
)

type pickerCursorState struct{ increments int }

// A thread-local CBT hook runs after modal ownership attaches input queues.
// Normalizing before Show is too early: the owner's negative ShowCursor count
// can be inherited when the native dialog activates. Never install a global
// hook, move the mouse, or change the system's mouse preferences.
func installPickerCursorGuard() (func(), error) {
	thread := windows.GetCurrentThreadId()
	state := &pickerCursorState{}
	pickerCursorStates.Store(thread, state)
	hook, _, err := windowUser32.NewProc("SetWindowsHookExW").Call(5, pickerCursorHook, 0, uintptr(thread)) // WH_CBT
	if hook == 0 {
		pickerCursorStates.Delete(thread)
		return nil, err
	}
	return func() {
		windowUser32.NewProc("UnhookWindowsHookEx").Call(hook)
		pickerCursorStates.Delete(thread)
		// Balance only OUR increments, including cancel/error paths. Chromium
		// still owns and balances its own hide/show calls on the UI thread.
		for i := 0; i < state.increments; i++ {
			windowUser32.NewProc("ShowCursor").Call(0)
		}
	}, nil
}

func pickerCursorActivated(code int32, window, data uintptr) uintptr {
	if code == 5 { // HCBT_ACTIVATE, only windows on this picker's thread
		if value, ok := pickerCursorStates.Load(windows.GetCurrentThreadId()); ok {
			state := value.(*pickerCursorState)
			show := windowUser32.NewProc("ShowCursor")
			count, _, _ := show.Call(1)
			if int32(count) > 0 {
				show.Call(0) // already visible; don't alter its count
			} else {
				state.increments++
				for int32(count) < 0 && state.increments < 64 {
					count, _, _ = show.Call(1)
					state.increments++
				}
			}
		}
	}
	result, _, _ := windowUser32.NewProc("CallNextHookEx").Call(0, uintptr(code), window, data)
	return result
}
