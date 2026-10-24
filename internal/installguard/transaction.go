package installguard

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type replacement struct {
	relative          string
	hadOld, installed bool
}
type transaction struct {
	directory, scratch string
	files              []replacement
	retain             bool
	cancelled          func() bool
}

// Only distribution-owned files can enter a transaction. User configuration,
// keys, fonts and backgrounds are never copied, deleted or rolled back.
func managedFile(relative string) bool {
	relative = filepath.ToSlash(relative)
	return relative == "DevShell.exe" || strings.HasPrefix(relative, "data/docs/") || strings.HasPrefix(relative, "data/licenses/")
}

func prepareTransaction(payload, directory string) (_ *transaction, err error) {
	directory, err = filepath.Abs(directory)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(directory, 0755); err != nil {
		return nil, err
	}
	// Resolve the installation root once; reject links below it so a managed
	// file cannot redirect a write into personal files or another installation.
	directory, err = filepath.EvalSymlinks(directory)
	if err != nil {
		return nil, err
	}
	scratch, err := os.MkdirTemp(directory, ".devshell-install-")
	if err != nil {
		return nil, err
	}
	t := &transaction{directory: directory, scratch: scratch}
	defer func() {
		if err != nil {
			os.RemoveAll(scratch)
		}
	}()
	err = filepath.WalkDir(payload, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("安装文件不允许使用链接：%s", path)
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("安装包包含非普通文件：%s", path)
		}
		relative, e := filepath.Rel(payload, path)
		if e != nil {
			return e
		}
		if !managedFile(relative) {
			return fmt.Errorf("安装包包含非程序文件：%s", relative)
		}
		if e = safeTarget(directory, relative); e != nil {
			return e
		}
		if e = copyFile(path, filepath.Join(scratch, "new", relative)); e != nil {
			return e
		}
		t.files = append(t.files, replacement{relative: relative})
		return nil
	})
	if err != nil {
		return nil, err
	}
	if _, err = os.Stat(filepath.Join(scratch, "new", "DevShell.exe")); err != nil {
		return nil, errors.New("安装包缺少 DevShell.exe")
	}
	return t, nil
}

func safeTarget(root, relative string) error {
	if filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("安装文件路径越界")
	}
	current := root
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("安装目标包含链接，请改用普通目录：%s", current)
		}
		resolved, err := filepath.EvalSymlinks(current)
		if err != nil {
			return err
		}
		if !strings.EqualFold(filepath.Clean(resolved), filepath.Clean(current)) {
			return fmt.Errorf("安装目标被重定向：%s", current)
		}
	}
	return nil
}

func copyFile(source, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if err == nil {
		err = out.Sync()
	}
	return errors.Join(err, out.Close())
}

func (t *transaction) commit() error {
	for i := range t.files {
		if t.cancelled != nil && t.cancelled() {
			return t.failed(ErrCancelled)
		}
		file := &t.files[i]
		if err := safeTarget(t.directory, file.relative); err != nil {
			return t.failed(err)
		}
		target := filepath.Join(t.directory, file.relative)
		old := filepath.Join(t.scratch, "old", file.relative)
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return t.failed(err)
		}
		if _, err := os.Stat(target); err == nil {
			if info, e := os.Stat(target); e != nil || !info.Mode().IsRegular() {
				return t.failed(fmt.Errorf("安装目标不是普通文件：%s", target))
			}
			if err = os.MkdirAll(filepath.Dir(old), 0755); err != nil {
				return t.failed(err)
			}
			if err = os.Rename(target, old); err != nil {
				return t.failed(err)
			}
			file.hadOld = true
		} else if !os.IsNotExist(err) {
			return t.failed(err)
		}
		if err := os.Rename(filepath.Join(t.scratch, "new", file.relative), target); err != nil {
			return t.failed(err)
		}
		file.installed = true
	}
	if t.cancelled != nil && t.cancelled() {
		return t.failed(ErrCancelled)
	}
	return nil
}

func (t *transaction) failed(cause error) error {
	var rollback error
	for i := len(t.files) - 1; i >= 0; i-- {
		file := t.files[i]
		target := filepath.Join(t.directory, file.relative)
		if file.installed {
			rollback = errors.Join(rollback, os.Remove(target))
		}
		if file.hadOld {
			rollback = errors.Join(rollback, os.Rename(filepath.Join(t.scratch, "old", file.relative), target))
		}
	}
	if rollback != nil {
		t.retain = true
		// Keep the only recoverable backups on disk rather than deleting them.
		return fmt.Errorf("安装失败：%v；自动恢复未完成，旧文件保留在 %s：%w", cause, t.scratch, rollback)
	}
	return fmt.Errorf("安装失败，已恢复原有程序：%w", cause)
}
