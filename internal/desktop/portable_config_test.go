package desktop

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPortableConfigurationFollowsExecutableAndNeverOldUserDirectory(t *testing.T) {
	folder := t.TempDir()
	executable := filepath.Join(folder, "DevShell.exe")
	if err := os.WriteFile(executable, []byte("fixture"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "previous-config"))
	t.Setenv("APPDATA", filepath.Join(t.TempDir(), "previous-appdata"))
	folder, err := filepath.EvalSymlinks(folder) // Windows may expand an 8.3 temporary path.
	if err != nil {
		t.Fatal(err)
	}
	got, err := portableDirectoryForExecutable(executable)
	if err != nil || got != filepath.Join(folder, "data") {
		t.Fatal("portable config used old user directory", got, err)
	}
	if runtime.GOOS != "windows" {
		link := filepath.Join(t.TempDir(), "shortcut")
		if err := os.Symlink(executable, link); err != nil {
			t.Fatal(err)
		}
		got, err = portableDirectoryForExecutable(link)
		if err != nil || got != filepath.Join(folder, "data") {
			t.Fatal("symlink changed portable folder", got, err)
		}
	}
}

func TestMacOSBundleRecognition(t *testing.T) {
	for _, tc := range []struct{ executable, bundle string }{
		{"/Applications/DevShell.app/Contents/MacOS/DevShell", "/Applications/DevShell.app"},
		{"/Volumes/DevShell/DevShell.app/Contents/MacOS/DevShell", "/Volumes/DevShell/DevShell.app"},
		{"/tmp/ordinary/Contents/MacOS/DevShell", ""},
		{"/tmp/DevShell.app/DevShell", ""},
		{"/tmp/DevShell", ""},
	} {
		if got := macOSApplicationBundle(filepath.FromSlash(tc.executable)); got != filepath.FromSlash(tc.bundle) {
			t.Fatalf("bundle for %q = %q; want %q", tc.executable, got, tc.bundle)
		}
	}
}

func TestMacOSBundleConfigurationSurvivesMovingApplication(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS Application Support path")
	}
	t.Setenv("HOME", t.TempDir())
	root, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	for _, location := range []string{"Applications", "Read Only Disk Image"} {
		executable := filepath.Join(t.TempDir(), location, "DevShell.app", "Contents", "MacOS", "DevShell")
		if err := os.MkdirAll(filepath.Dir(executable), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(executable, []byte("fixture"), 0500); err != nil {
			t.Fatal(err)
		}
		got, err := portableDirectoryForExecutable(executable)
		if err != nil || got != filepath.Join(root, "DevShell") {
			t.Fatalf("bundle configuration = %q, %v", got, err)
		}
		if _, err := os.Stat(filepath.Join(filepath.Dir(executable), "data")); !os.IsNotExist(err) {
			t.Fatal("configuration was written into the application bundle")
		}
	}
}

func TestMacOSLegacyConfigurationDirectoryRenamedOnce(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS Application Support path")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	support := filepath.Join(home, "Library", "Application Support")
	legacy := filepath.Join(support, "DengShell")
	if err := os.MkdirAll(legacy, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "carried.txt"), []byte("configuration"), 0600); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(t.TempDir(), "Applications", "DevShell.app", "Contents", "MacOS", "DevShell")
	if err := os.MkdirAll(filepath.Dir(executable), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, []byte("fixture"), 0500); err != nil {
		t.Fatal(err)
	}
	got, err := portableDirectoryForExecutable(executable)
	if err != nil || got != filepath.Join(support, "DevShell") {
		t.Fatalf("bundle configuration = %q, %v", got, err)
	}
	if data, err := os.ReadFile(filepath.Join(got, "carried.txt")); err != nil || string(data) != "configuration" {
		t.Fatalf("legacy configuration not carried over: %v", err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatal("legacy directory left behind")
	}
}
