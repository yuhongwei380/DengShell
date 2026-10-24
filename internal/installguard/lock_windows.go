package installguard

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

func directoryKey(directory string) (string, error) {
	directory, err := filepath.Abs(directory)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(directory); err == nil {
		directory = resolved
	}
	hash := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(directory))))
	return fmt.Sprintf(`Local\DevShell.Install.%x`, hash), nil
}

func acquireInstallLock(directory string) (func(), error) {
	name, err := directoryKey(directory)
	if err != nil {
		return nil, err
	}
	p, _ := windows.UTF16PtrFromString(name)
	handle, err := windows.CreateMutex(nil, false, p)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		windows.CloseHandle(handle)
		return nil, errors.New("此目录已有安装正在进行，请等待完成")
	}
	if err != nil {
		return nil, err
	}
	return func() { windows.CloseHandle(handle) }, nil
}

// Called before loading user configuration or starting a WebView/SSH backend.
func CheckLaunch() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	name, err := directoryKey(filepath.Dir(executable))
	if err != nil {
		return err
	}
	p, _ := windows.UTF16PtrFromString(name)
	handle, err := windows.OpenMutex(windows.SYNCHRONIZE, false, p)
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("无法确认安装状态：%w", err)
	}
	windows.CloseHandle(handle)
	return ErrInstalling
}
