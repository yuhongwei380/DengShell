//go:build desktop && !linux && !windows && !darwin

package desktop

func platformSystemTheme() string { return "" }
