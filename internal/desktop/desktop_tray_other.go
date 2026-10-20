//go:build desktop && !windows && !linux

package desktop

func platformInitializeTray(string) {}
func platformTrayAvailable() bool   { return false }
func platformTrayEvents() int       { return 0 }
func platformCloseTray()            {}
func platformRaiseWindow()          {}
