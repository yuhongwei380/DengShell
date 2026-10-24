package installguard

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFixture(t *testing.T, directory, relative, value string) string {
	t.Helper()
	path := filepath.Join(directory, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(value), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
func expectFile(t *testing.T, path, value string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil || string(data) != value {
		t.Fatalf("%s: %q / %v, want %q", path, data, err, value)
	}
}

func TestTransactionPreservesPersonalData(t *testing.T) {
	payload, target := t.TempDir(), t.TempDir()
	writeFixture(t, payload, "DevShell.exe", "new")
	writeFixture(t, payload, "data/docs/guide.md", "new guide")
	writeFixture(t, target, "DevShell.exe", "old")
	writeFixture(t, target, "data/docs/guide.md", "old guide")
	for _, relative := range []string{"data/config.json", "data/keys/private", "data/fonts/custom.ttf", "data/backgrounds/photo.png"} {
		writeFixture(t, target, relative, "personal")
	}
	transaction, err := prepareTransaction(payload, target)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(transaction.scratch)
	if err = transaction.commit(); err != nil {
		t.Fatal(err)
	}
	expectFile(t, filepath.Join(target, "DevShell.exe"), "new")
	expectFile(t, filepath.Join(target, "data/docs/guide.md"), "new guide")
	for _, relative := range []string{"data/config.json", "data/keys/private", "data/fonts/custom.ttf", "data/backgrounds/photo.png"} {
		expectFile(t, filepath.Join(target, relative), "personal")
	}
}

func TestTransactionRollbackAfterPartialReplacement(t *testing.T) {
	payload, target := t.TempDir(), t.TempDir()
	writeFixture(t, payload, "DevShell.exe", "new")
	writeFixture(t, payload, "data/docs/guide.md", "new guide")
	writeFixture(t, target, "DevShell.exe", "old")
	writeFixture(t, target, "data/docs/guide.md", "old guide")
	transaction, err := prepareTransaction(payload, target)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(transaction.scratch)
	// The executable is replaced first; fail the following managed file to
	// prove rollback restores files already replaced, not just the failing one.
	if err = os.Remove(filepath.Join(transaction.scratch, "new/data/docs/guide.md")); err != nil {
		t.Fatal(err)
	}
	if err = transaction.commit(); err == nil {
		t.Fatal("expected failure")
	}
	expectFile(t, filepath.Join(target, "DevShell.exe"), "old")
	expectFile(t, filepath.Join(target, "data/docs/guide.md"), "old guide")
}

func TestTransactionRejectsUserFilesAndRedirectedTargets(t *testing.T) {
	payload, target := t.TempDir(), t.TempDir()
	writeFixture(t, payload, "DevShell.exe", "new")
	writeFixture(t, payload, "data/config.json", "must not install")
	if transaction, err := prepareTransaction(payload, target); err == nil {
		os.RemoveAll(transaction.scratch)
		t.Fatal("accepted configuration in payload")
	}
	if err := os.Remove(filepath.Join(payload, "data/config.json")); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, payload, "data/docs/guide.md", "guide")
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(target, "data")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if transaction, err := prepareTransaction(payload, target); err == nil {
		os.RemoveAll(transaction.scratch)
		t.Fatal("accepted redirected data directory")
	}
}

func TestTransactionCancellationRestoresReplacedFiles(t *testing.T) {
	payload, target := t.TempDir(), t.TempDir()
	writeFixture(t, payload, "DevShell.exe", "new")
	writeFixture(t, payload, "data/docs/guide.md", "new guide")
	writeFixture(t, target, "DevShell.exe", "old")
	writeFixture(t, target, "data/docs/guide.md", "old guide")
	transaction, err := prepareTransaction(payload, target)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(transaction.scratch)
	checks := 0
	transaction.cancelled = func() bool { checks++; return checks == 2 }
	if err = transaction.commit(); err == nil {
		t.Fatal("expected cancellation")
	}
	expectFile(t, filepath.Join(target, "DevShell.exe"), "old")
	expectFile(t, filepath.Join(target, "data/docs/guide.md"), "old guide")
}
