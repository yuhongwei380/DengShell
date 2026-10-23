// Package installguard coordinates Windows installer shutdown and file replacement.
package installguard

import "errors"

const (
	QuitMessage      = "DevShell.Installer.Quit.v1"
	QuitRequest      = 1
	QuitCancel       = 2
	QuitAcknowledged = 0x44534931
)

var ErrCancelled = errors.New("安装已取消，程序文件未修改")
var ErrInstalling = errors.New("DevShell 正在安装或升级，请等待安装完成后再启动")
