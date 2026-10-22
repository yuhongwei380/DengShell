# DevShell v26.10.0 · 构建 20261022140

Phoenix 分支（[yuhongwei380/DevShell](https://github.com/yuhongwei380/DevShell)）；版本号 `vX.Y.Z`，内部构建号 `YYYYMMDD` + 三位序号递增防回退。

## 本次变化

- 修复：macOS 点「检查更新」只提示「未知错误」，实际上什么都没发生
  - 原因：macOS 没有更新清单地址，客户端直接判定「该平台没有内置更新通道」，界面又把这句话显示成「未知错误」
- 新增：macOS「检查更新」可用——下载通用应用包，校验 SHA-256 与构建号，退出旧版本后原子替换 `.app` 并自动启动新版本
- 新增：更新失败自动恢复原版本；安装位置不可写（例如仍从挂载的 DMG 运行）时在退出前就提示「无法写入安装位置」，不会留下半更新的应用
- 新增：不提供内置更新的平台，提示改为「请从 GitHub Releases 下载对应安装包，原数据目录会保留」

## 升级说明

- macOS：本版本需要手动升级一次——用 DMG 或 ZIP 覆盖 `/Applications/DevShell.app`，`~/Library/Application Support/DevShell/` 原地保留；此后就能直接用「检查更新」自升级。升级后请从「应用程序」启动，不要从 DMG 卷直接运行，否则内置更新会拒绝替换。
- Windows：内置自动更新照旧；首次安装选 `DevShell-Setup-x64.exe`，`up.*` 文件是更新载荷。
- 更新清单新增 `up-macos-arm64.json` / `up-macos-amd64.json`，两者都指向 `DevShell-macos-universal.zip`，均已进入 `SHA256SUMS.txt`。
