//go:build desktop

package desktop

import (
	"errors"
	"path/filepath"
	"testing"
)

// Two native dialogs at once means two nested modal loops on the UI thread, and
// Windows answers that with a folder view that snaps back and an invisible
// pointer. A second request while one dialog is open must therefore be refused.
func TestNativePickersAreSerialised(t *testing.T) {
	desktop := &Desktop{}
	if !desktop.beginPicker() {
		t.Fatal("第一个选择框应获准打开")
	}
	if desktop.beginPicker() {
		t.Fatal("已有选择框打开时不得再开一个")
	}
	result, err := desktop.openDialog("download", func(string) (string, error) {
		t.Fatal("被拒绝的请求不应真正打开对话框")
		return "", nil
	})
	if result != "" || err != nil {
		t.Fatalf("被拒绝的请求应直接返回空，got %q / %v", result, err)
	}
	results, err := desktop.openDialogMulti("upload", func(string) ([]string, error) {
		t.Fatal("被拒绝的请求不应真正打开对话框")
		return nil, nil
	})
	if len(results) != 0 || err != nil {
		t.Fatalf("被拒绝的多选请求应直接返回空，got %v / %v", results, err)
	}
	desktop.endPicker()
	if !desktop.beginPicker() {
		t.Fatal("关闭后可以再次打开")
	}
	desktop.endPicker()

	// A failing dialog still releases the lock, otherwise every later picker is
	// blocked forever.
	if _, err = desktop.openDialog("download", func(string) (string, error) { return "", errors.New("boom") }); err == nil {
		t.Fatal("对话框错误应向上抛出")
	}
	if !desktop.beginPicker() {
		t.Fatal("出错后选择框状态未释放")
	}
	desktop.endPicker()
}

// Without a starting folder the OS restores whatever it persisted for the
// dialog role, which reads as jumping back to a folder from an earlier attempt.
func TestNativePickersReopenInTheirLastFolder(t *testing.T) {
	directory := t.TempDir()
	other := t.TempDir()
	desktop := &Desktop{}

	var opened []string
	if _, err := desktop.openDialogMulti("upload", func(start string) ([]string, error) {
		opened = append(opened, start)
		return []string{filepath.Join(directory, "a.txt")}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := desktop.openDialogMulti("upload", func(start string) ([]string, error) {
		opened = append(opened, start)
		return nil, nil
	}); err != nil {
		t.Fatal(err)
	}
	if opened[0] != "" || opened[1] != directory {
		t.Fatalf("第二次应从上次目录打开，got %q 然后 %q", opened[0], opened[1])
	}

	if _, err := desktop.openFolderDialog("upload-folder", func(start string) (string, error) {
		opened = append(opened, start)
		return other, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := desktop.openFolderDialog("upload-folder", func(start string) (string, error) {
		opened = append(opened, start)
		return "", nil
	}); err != nil {
		t.Fatal(err)
	}
	if opened[2] != "" || opened[3] != other {
		t.Fatalf("目录选择框应记住所选目录本身，got %q 然后 %q", opened[2], opened[3])
	}

	// Different kinds do not share a folder, and a vanished folder is dropped.
	if got := desktop.rememberedDir("download"); got != "" {
		t.Fatalf("未用过的类型不应继承目录，got %q", got)
	}
	desktop.rememberFolder("download", filepath.Join(directory, "已删除"))
	if got := desktop.rememberedDir("download"); got != "" {
		t.Fatalf("不存在的目录应忽略，got %q", got)
	}
}
