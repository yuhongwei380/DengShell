package updater

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeFileMode(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func readOrEmpty(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

func sha256Text(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// writeBundleArchive packs a DevShell.app tree the way ditto --keepParent does,
// including the metadata entries that must never reach the installed bundle.
func writeBundleArchive(t *testing.T, path string, entries map[string]string, executableMode os.FileMode) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	writer := zip.NewWriter(file)
	add := func(name, content string, mode os.FileMode) {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetMode(mode)
		entry, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		_, err = entry.Write([]byte(content))
		if err != nil {
			t.Fatal(err)
		}
	}
	add("DevShell.app/", "", os.ModeDir|0755)
	for name, content := range entries {
		mode := os.FileMode(0644)
		if strings.HasSuffix(name, "MacOS/DevShell") {
			mode = executableMode
		}
		add(name, content, mode)
	}
	add("__MACOSX/._DevShell.app", "resource fork", 0644)
	add("DevShell.app/Contents/Resources/.DS_Store", "finder cache", 0644)
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
}

func installedBundle(t *testing.T, root string) string {
	t.Helper()
	bundle := filepath.Join(root, "Applications", "DevShell.app")
	writeFileMode(t, filepath.Join(bundle, "Contents", "Info.plist"), "old plist", 0644)
	writeFileMode(t, bundleExecutableIn(bundle), "old binary", 0755)
	return bundle
}

func TestUnpackAppBundleRestoresExecutableAndSkipsMetadata(t *testing.T) {
	root := t.TempDir()
	archive := filepath.Join(root, "DevShell-macos-universal.zip")
	writeBundleArchive(t, archive, map[string]string{
		"DevShell.app/Contents/Info.plist":               "new plist",
		"DevShell.app/Contents/MacOS/DevShell":           "new binary",
		"DevShell.app/Contents/Resources/app/web/app.js": "console.log(1)",
	}, 0755)
	stage := filepath.Join(root, "stage")
	if err := os.MkdirAll(stage, 0755); err != nil {
		t.Fatal(err)
	}
	bundle, err := unpackAppBundle(archive, stage)
	if err != nil {
		t.Fatal(err)
	}
	if got := readOrEmpty(t, filepath.Join(bundle, "Contents", "Info.plist")); got != "new plist" {
		t.Fatalf("plist: %q", got)
	}
	if got := readOrEmpty(t, bundleExecutableIn(bundle)); got != "new binary" {
		t.Fatalf("executable: %q", got)
	}
	info, err := os.Stat(bundleExecutableIn(bundle))
	if err != nil {
		t.Fatal(err)
	}
	// Windows has no executable permission bits, so only POSIX hosts can prove
	// the archive mode survived the round trip.
	if runtime.GOOS != "windows" && info.Mode().Perm()&0111 == 0 {
		t.Fatalf("executable lost its mode bits: %v", info.Mode())
	}
	if _, err := os.Stat(filepath.Join(stage, "__MACOSX")); !os.IsNotExist(err) {
		t.Fatal("archive metadata reached the bundle")
	}
	if _, err := os.Stat(filepath.Join(bundle, "Contents", "Resources", ".DS_Store")); !os.IsNotExist(err) {
		t.Fatal("finder metadata reached the bundle")
	}
	if err = verifyUnpackedBundle(bundle, sha256Text("new binary")); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyUnpackedBundleRejectsUnusablePayload(t *testing.T) {
	root := t.TempDir()
	archive := filepath.Join(root, "payload.zip")
	writeBundleArchive(t, archive, map[string]string{
		"DevShell.app/Contents/Info.plist":     "new plist",
		"DevShell.app/Contents/MacOS/DevShell": "new binary",
	}, 0755)
	bundle, err := unpackAppBundle(archive, root)
	if err != nil {
		t.Fatal(err)
	}
	if err = verifyUnpackedBundle(bundle, sha256Text("someone else's binary")); err == nil || !strings.Contains(err.Error(), "更新描述不一致") {
		t.Fatalf("mismatched executable accepted: %v", err)
	}
	if err = os.Remove(filepath.Join(bundle, "Contents", "Info.plist")); err != nil {
		t.Fatal(err)
	}
	if err = verifyUnpackedBundle(bundle, sha256Text("new binary")); err == nil || !strings.Contains(err.Error(), "Info.plist") {
		t.Fatalf("bundle without Info.plist accepted: %v", err)
	}

	second := filepath.Join(root, "noexec")
	if err := os.MkdirAll(second, 0755); err != nil {
		t.Fatal(err)
	}
	archive2 := filepath.Join(root, "noexec.zip")
	writeBundleArchive(t, archive2, map[string]string{
		"DevShell.app/Contents/Info.plist":     "new plist",
		"DevShell.app/Contents/MacOS/DevShell": "new binary",
	}, 0644)
	unpacked, err := unpackAppBundle(archive2, second)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		// The mode check below is a POSIX-only property; the hash checks above
		// still ran on every platform.
		return
	}
	if err = verifyUnpackedBundle(unpacked, sha256Text("new binary")); err == nil || !strings.Contains(err.Error(), "可执行程序") {
		t.Fatalf("non-executable payload accepted: %v", err)
	}
}

func TestUnpackAppBundleRejectsHostileArchives(t *testing.T) {
	for _, tc := range []struct{ name, entry string }{
		{"wrong root", "Other.app/Contents/MacOS/DevShell"},
		{"parent traversal", "DevShell.app/Contents/../../evil"},
		{"bare parent", "DevShell.app/.."},
	} {
		root := t.TempDir()
		archive := filepath.Join(root, "payload.zip")
		writeBundleArchive(t, archive, map[string]string{tc.entry: "payload"}, 0755)
		if _, err := unpackAppBundle(archive, filepath.Join(root, "stage")); err == nil {
			t.Fatalf("%s archive accepted", tc.name)
		}
	}
	root := t.TempDir()
	archive := filepath.Join(root, "empty.zip")
	if err := os.WriteFile(archive, []byte("PK\x03\x04 not an archive"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := unpackAppBundle(archive, filepath.Join(root, "stage")); err == nil {
		t.Fatal("a non-archive was accepted")
	}
}

func TestSwapAppBundleReplacesAndRollbackRestores(t *testing.T) {
	root := t.TempDir()
	installed := installedBundle(t, root)
	archive := filepath.Join(root, "payload.zip")
	writeBundleArchive(t, archive, map[string]string{
		"DevShell.app/Contents/Info.plist":     "new plist",
		"DevShell.app/Contents/MacOS/DevShell": "new binary",
	}, 0755)
	stage := filepath.Join(root, "stage")
	if err := os.MkdirAll(stage, 0755); err != nil {
		t.Fatal(err)
	}
	unpacked, err := unpackAppBundle(archive, stage)
	if err != nil {
		t.Fatal(err)
	}
	if err = swapAppBundle(installed, unpacked); err != nil {
		t.Fatal(err)
	}
	if got := readOrEmpty(t, bundleExecutableIn(installed)); got != "new binary" {
		t.Fatalf("installed bundle: %q", got)
	}
	if _, err := os.Stat(installed + bundleBackupSuffix); !os.IsNotExist(err) {
		t.Fatal("the previous bundle backup survived a successful update")
	}
	if _, err := os.Stat(unpacked); !os.IsNotExist(err) {
		t.Fatal("the staging bundle survived the swap")
	}

	// A failure after the first rename leaves exactly this shape on disk.
	writeFileMode(t, bundleExecutableIn(installed), "broken binary", 0755)
	if err = os.Rename(installed, installed+bundleBackupSuffix); err != nil {
		t.Fatal(err)
	}
	if err = rollbackAppBundle(installed); err != nil {
		t.Fatal(err)
	}
	if got := readOrEmpty(t, bundleExecutableIn(installed)); got != "broken binary" {
		t.Fatalf("rollback: %q", got)
	}
	if err = rollbackAppBundle(installed); err == nil {
		t.Fatal("rollback without a backup succeeded")
	}
}

func TestBundleFromExecutableRejectsForeignLayouts(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "Applications", "DevShell.app", "Contents", "MacOS", bundleExecutableName)
	for _, tc := range []struct {
		name, target string
		want         string
	}{
		{"installed bundle", inside, filepath.Join(root, "Applications", "DevShell.app")},
		{"renamed bundle", filepath.Join(root, "Applications", "DevShell 2.app", "Contents", "MacOS", bundleExecutableName), filepath.Join(root, "Applications", "DevShell 2.app")},
		{"bare binary", filepath.Join(root, "dengshell", bundleExecutableName), ""},
		{"missing MacOS", filepath.Join(root, "DevShell.app", "Contents", bundleExecutableName), ""},
		{"not an app bundle", filepath.Join(root, "DevShell", "Contents", "MacOS", bundleExecutableName), ""},
	} {
		bundle, err := bundleFromExecutable(tc.target)
		if tc.want == "" {
			if err == nil {
				t.Fatalf("%s accepted as a bundle", tc.name)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if bundle != filepath.Clean(tc.want) {
			t.Fatalf("%s: %q != %q", tc.name, bundle, tc.want)
		}
	}
}

func TestUpdatePayloadShapeChecks(t *testing.T) {
	root := t.TempDir()
	if err := directoryWritable(root); err != nil {
		t.Fatal(err)
	}
	if err := directoryWritable(filepath.Join(root, "missing")); err == nil {
		t.Fatal("a missing directory reported as writable")
	}
	archive := filepath.Join(root, "payload.zip")
	writeBundleArchive(t, archive, map[string]string{"DevShell.app/Contents/MacOS/DevShell": "binary"}, 0755)
	if !looksLikeZipArchive(archive) {
		t.Fatal("a published archive was rejected")
	}
	text := filepath.Join(root, "notes.md")
	if err := os.WriteFile(text, []byte("# 本次变化\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if looksLikeZipArchive(text) || looksLikeZipArchive(filepath.Join(root, "missing.zip")) {
		t.Fatal("a non-archive passed the shape check")
	}
}
