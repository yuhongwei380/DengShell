package installguard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type processSession interface {
	processes() ([]runningProcess, error)
}
type prompt func(uintptr, string, string, ...choice) (int32, error)

func Install(payload, directory string, owner uintptr, silent bool) error {
	return installWithPrompt(payload, directory, owner, silent, showChoices)
}

func installWithPrompt(payload, directory string, owner uintptr, silent bool, show prompt) error {
	if !installerAlive(owner) {
		return ErrCancelled
	}
	// Resolve the root before deriving its mutex name, including first installs.
	if err := os.MkdirAll(directory, 0755); err != nil {
		return err
	}
	release, err := acquireInstallLock(directory)
	if err != nil {
		return err
	}
	defer release()
	t, err := prepareTransaction(payload, directory)
	if err != nil {
		return err
	}
	defer func() {
		if !t.retain {
			os.RemoveAll(t.scratch)
		}
	}()
	t.cancelled = func() bool { return !installerAlive(owner) }
	session, err := newResourceSession(t.directory)
	if err != nil {
		return fmt.Errorf("无法检查运行中的程序：%w", err)
	}
	defer session.close()
	if err = closeRunning(session, owner, silent, show, 15*time.Second); err != nil {
		return err
	}
	// Recheck immediately before committing: old releases cannot observe our
	// installation gate and could have been started during the prompt.
	if processes, err := session.processes(); err != nil {
		return err
	} else if len(processes) > 0 {
		return fmt.Errorf("DevShell 再次启动，安装已停止。请退出后重新安装")
	}
	return t.commit()
}

func closeRunning(session processSession, owner uintptr, silent bool, show prompt, timeout time.Duration) (err error) {
	processes, err := session.processes()
	if err != nil || len(processes) == 0 {
		return err
	}
	if silent {
		return fmt.Errorf("DevShell 正在运行；静默安装不会自动关闭程序，请退出后再安装")
	}
	selected, err := show(owner, "关闭 DevShell 以继续安装", "安装器将关闭此目录下的 DevShell 窗口，退出后自动继续。\n\n请先保存编辑并完成传输，SSH 会话将断开。", choice{100, "关闭并继续"}, choice{2, "取消安装"})
	if err != nil {
		return err
	}
	if selected != 100 {
		return ErrCancelled
	}
	requested := map[uniqueProcess]runningProcess{}
	defer func() {
		if err != nil {
			for _, p := range requested {
				p.requestClose(true)
			}
		}
	}()
	deadline := time.Now().Add(timeout)
	for {
		if !installerAlive(owner) {
			return ErrCancelled
		}
		processes, err = session.processes()
		if err != nil {
			return err
		}
		if len(processes) == 0 {
			return nil
		}
		for _, p := range processes {
			id := uniqueProcess{p.pid, p.started}
			if _, ok := requested[id]; !ok {
				if p.requestClose(false) {
					requested[id] = p
				}
			}
		}
		if time.Now().Before(deadline) {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		var names []string
		for _, p := range processes {
			names = append(names, fmt.Sprintf("%s · PID %d", filepath.Base(p.path), p.pid))
		}
		selected, err = show(owner, "正在等待 DevShell 退出", "请返回 DevShell，处理未保存的编辑或传输。\n\n"+strings.Join(names, "\n"), choice{100, "继续等待"}, choice{101, "强制关闭…"}, choice{2, "取消安装"})
		if err != nil {
			return err
		}
		if selected == 2 {
			return ErrCancelled
		}
		if selected == 101 {
			selected, err = show(owner, "确认强制关闭？", "强制关闭会断开 SSH 会话、中止传输，并丢失尚未保存的编辑。", choice{101, "强制关闭"}, choice{2, "返回"})
			if err != nil {
				return err
			}
			if selected == 101 {
				if !installerAlive(owner) {
					return ErrCancelled
				}
				for _, p := range processes {
					if err = p.terminate(); err != nil {
						return fmt.Errorf("无法关闭 DevShell（PID %d）：%w", p.pid, err)
					}
				}
			}
		} else if selected != 100 {
			return ErrCancelled
		}
		// A user may have cancelled an application's close confirmation to save
		// work. An explicit Continue Waiting requests normal shutdown again.
		requested = map[uniqueProcess]runningProcess{}
		deadline = time.Now().Add(timeout)
	}
}

func installerAlive(owner uintptr) bool {
	if owner == 0 {
		return true
	} // standalone helper/test invocation
	alive, _, _ := user32.NewProc("IsWindow").Call(owner)
	return alive != 0
}
