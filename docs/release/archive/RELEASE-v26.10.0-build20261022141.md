# DevShell v26.10.0 · 构建 20261022141

Phoenix 分支（[yuhongwei380/DevShell](https://github.com/yuhongwei380/DevShell)）；版本号 `vX.Y.Z`，内部构建号 `YYYYMMDD` + 三位序号递增防回退。

## 本次变化

- 修复：终端里输入 `rz` 直接报错并刷出 `**B0100000023be50e…` 乱码
  - 本程序没有 ZModem 对端，lrzsz 的握手广告被退格覆盖打印后无人应答，只能刷屏到超时
- 新增：`rz` / `sz` 桥接到 SFTP——先中止 ZModem 握手，`rz` 打开本机文件选择上传到终端当前目录，`sz 文件名…` 逐个直接下载；无法确认命令来源时只中止握手并提示改用文件面板 / scp
- 新增：无连接（无会话或会话已结束）时，命令条 ↑ / ↓ 调出本机最近 50 条输入，不占按钮，加密保存且重启保留；已连接时仍只用该服务器的历史命令
- 调整：SFTP 文件区底部不再常驻 下载 / 重命名 / 删除，文件命令全部收进右键菜单
  - 单个文件补上 **下载**（保留原文件名，不打包）；目录与多选仍用 **打包并下载**；键盘 **Delete** 仍删除所选
- 修复：本机 traceroute 不再在 45 秒被一刀切，截止时间按工具自身的跳数预算推导（macOS / Linux 140 秒、Windows 80 秒）
- 文档：README、应用内帮助跟到本构建；README 新增「终端里的 rz / sz（lrzsz）」小节

## 升级说明

- Windows：内置自动更新照旧；首次安装选 `DevShell-Setup-x64.exe`，`up.*` 文件是更新载荷。
- macOS：自构建 20261022140 起可直接「检查更新」自升级（需从「应用程序」启动，仍挂在 DMG 卷上会拒绝替换）；否则用 DMG / ZIP 覆盖 `/Applications/DevShell.app`，数据目录原地保留。
- 更新清单：`up-macos-arm64.json` / `up-macos-amd64.json` 都指向同一个 `DevShell-macos-universal.zip`，均已进入 `SHA256SUMS.txt`。
