//go:build darwin

package updater

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

func prepareUpdaterProcess(cmd *exec.Cmd)       { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
func prepareRestartedApplication(cmd *exec.Cmd) { prepareUpdaterProcess(cmd) }

func waitForUpdateParent(pid int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); err == syscall.ESRCH {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("等待旧程序退出超时")
}

// darwinBundlePlan resolves the installed bundle from the running executable
// and proves the installation location is writable. It runs in the application
// before it quits and again in the helper, so a read-only DMG volume or a
// root-owned /Applications entry is reported while the old program is visible.
func darwinBundlePlan(p updatePlan) (string, error) {
	if p.PackageFormat != bundlePackageFormat {
		return "", errors.New("更新安装包格式与当前平台不匹配")
	}
	bundle, err := bundleFromExecutable(p.Target)
	if err != nil {
		return "", err
	}
	if err := directoryWritable(filepath.Dir(bundle)); err != nil {
		return "", err
	}
	return bundle, nil
}

func validatePlatformUpdate(p updatePlan) error {
	if _, err := darwinBundlePlan(p); err != nil {
		return err
	}
	if !looksLikeZipArchive(p.Staged) {
		return errors.New("下载的文件不是有效的应用压缩包")
	}
	return nil
}

func applyPlatformUpdate(p updatePlan) (string, error) {
	bundle, err := darwinBundlePlan(p)
	if err != nil {
		return "", err
	}
	// Staging stays next to the bundle so the final step is a same-volume
	// rename: the update is either the old bundle or the new one, never a
	// half-copied directory.
	stage, err := os.MkdirTemp(filepath.Dir(bundle), bundleRootName+bundleStagePrefix)
	if err != nil {
		return "", fmt.Errorf("无法创建临时安装目录：%w", err)
	}
	defer func() { _ = os.RemoveAll(stage) }()
	unpacked, err := unpackAppBundle(p.Staged, stage)
	if err != nil {
		return "", err
	}
	if err := verifyUnpackedBundle(unpacked, p.ExecutableSHA256); err != nil {
		return "", err
	}
	if err := swapAppBundle(bundle, unpacked); err != nil {
		return "", err
	}
	return bundleExecutableIn(bundle), nil
}

func rollbackPlatformUpdate(p updatePlan) error {
	bundle, err := bundleFromExecutable(p.Target)
	if err != nil {
		return err
	}
	return rollbackAppBundle(bundle)
}
