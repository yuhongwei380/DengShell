//go:build desktop && !windows

package desktop

func preparePlatformWindow()                  {}
func installPlatformWindowIcon()              {}
func platformWebviewDataPath(_ string) string { return "" }
