//go:build desktop

package desktop

import (
	"os/exec"
	"testing"
)

func TestInstallerQuitWaitsForIndependentWindows(t *testing.T) {
	desktop := &Desktop{installerQuitPending: true, children: map[string]*exec.Cmd{"child": {}}}
	if err := desktop.ConfirmQuit(); err != nil {
		t.Fatal(err)
	}
	if !desktop.installerQuitApproved || desktop.quitConfirmed || desktop.windowClosed {
		t.Fatal("installer exit must wait for children without hiding main")
	}
	if err := desktop.reserveDetachedStart("new"); err == nil {
		t.Fatal("allowed a new independent window during installation shutdown")
	}
	desktop.CancelQuit()
	if desktop.installerQuitPending || desktop.installerQuitApproved || desktop.quitPending {
		t.Fatal("cancel did not reset installer exit state")
	}
}
