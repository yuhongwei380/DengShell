# DevShell v26.10.0 · 构建 20261021137

Phoenix 分支（[yuhongwei380/DevShell](https://github.com/yuhongwei380/DevShell)）；版本号 `vX.Y.Z`，内部构建号 `YYYYMMDD` + 三位序号递增防回退。

## 本次变化

- 修复：macOS 本机提示符 `traceroute` 要等约 45 秒超时后才一次性显示结果——输出接管道时被 libc 全缓冲
  - macOS/Linux 改为在伪终端（openpty）中运行 traceroute，结果按行流式推送，逐跳实时出现；Windows 同样改为增量显示
- 新增：运行中的任何本机命令（`ping` 与 `ping -t`、traceroute、nslookup、dig、curl）按 Ctrl+C 可中途停止，本机进程／查询随即终止，界面显示 `^C 已停止`
- 新增：保存连接的三个入口（服务器管理新建／编辑、快速连接保存、链接栏固定）统一默认记住凭据；取消勾选仍为显式拒绝并清除已存凭据
- 新增：顶栏版本号悬停显示构建号（「当前版本 v26.10.0（构建 …）」）
- 调整：macOS 安装包、bundle id、数据目录（`~/Library/Application Support/DevShell`，首次启动自动迁移旧目录）与 DMG 使用说明全面改用 DevShell 名称
- 调整：README 截图全部换为当前界面；安装协议版权改为 DevShell；第三方开源组件许可证集中保存到 `LICENSES/` 目录，发行包同步附带

## 升级说明

- 首次安装选 `DevShell-Setup-x64.exe`；`up.*` 文件是内置自动更新载荷，无需手动下载。
- macOS 用户升级到本版本后，数据目录会自动改名为 `DevShell`；旧 DengShell 目录不会删除，可回退旧版本。
