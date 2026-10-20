# DevShell v26.10.0 · 构建 20261018128

Phoenix 分支（[yuhongwei380/DevShell](https://github.com/yuhongwei380/DevShell)）；版本号 `vX.Y.Z`，内部构建号 `YYYYMMDD` + 三位序号递增防回退。

## 近期变化

- 仓库结构整理：前端按 `web/js`、`web/css` 归位，脚本按 `scripts/build`、`scripts/tests`、`scripts/tools` 归位，发版文档移入 `docs/release`，Go 源码按功能归入 `internal/desktop`、`internal/updater`；程序行为不变。
- 发版策略调整：只提供 Windows 安装包与 macOS 通用 DMG/ZIP；不再发布 Windows 绿色 ZIP 与 Linux DEB/RPM/pacman/tar.gz。已安装的 Linux 版可继续使用，更新检查会明确提示 Linux 已停止发布。
- 新增「串口终端」连接类型：一键检测本机串口，可配置波特率、数据位、校验位、停止位与流控，与 SSH、RDP 一样保存、分组、固定，双击即在终端标签中打开。
- 链接栏固定项通过 × 关闭按钮取消固定，点击后弹窗二次确认再移除，防止误删；不再支持右键取消固定。
- SFTP「跟随终端目录」默认开启：Bash / Zsh / Fish 集成就绪后文件面板自动跟随终端当前目录；PowerShell / cmd 等无集成的 Shell 保持关闭。
- 启用全新 logo：「Dev❯_」标志（深空蓝黑底、绿→青蓝渐变描边），窗口 / 任务栏 / 托盘 / 安装器 / 界面品牌按钮全部更新。
- 正常 `exit` / `logout`、关闭标签与退出应用不再写入断开日志，只有网络中断等异常断开才记录；断开后终端不再保留闪烁光标，重新连接自动恢复。
- 修复「更新文件与本次检查结果不一致」：手动检查后可直接下载更新，无需重启程序。
- 修复「检查更新失败：网络不可用或无法访问更新源」：适配 GitHub 新的资产下载域名。
- 链接栏：固定在顶部一行，标签常显，最多显示 7 个固定项，超出收入下拉。
- 头部布局：系统信息侧栏贯通到顶，窗口按钮固定右上角，快速连接「+」与链接栏左对齐。
- 会话 `exit` / `logout` 后落回本机提示符：`ssh 用户@主机`、`reconnect`、`ping 主机 [-t]`、`exit`、`help`。
- 正常退出不再误报异常断开；系统信息新增独显显存 / 占用与真实网卡地址。

## 升级说明

- 首次安装选 `DevShell-Setup-x64.exe`；`up.*` 文件是内置自动更新载荷，无需手动下载。
- 构建 ≤20261018118 的旧版无法内置更新到新版，请手动安装一次最新 `DevShell-Setup-x64.exe`；连接、密钥等数据自动沿用。

