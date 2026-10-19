# DengShell 更新器约定

当前协议统一维护于 [在线更新协议](ONLINE-UPDATE-DESIGN.md)，签名细节见 [发行签名](SIGNED-UPDATES.md)，避免两份协议出现不一致。

- Windows：固定 `/up.exe.json` 与 `/up.exe`。
- Debian/Ubuntu：固定 `/up.deb.json` 与 `/up.deb`。
- Arch pacman（R28 起）：固定 `/up.pkg.tar.zst.json` 与 `/up.pkg.tar.zst`，签名中的平台为 `linux-amd64-pacman`。
- 来源固定为 `https://github.com/yuhongwei380/DevShell` 的 Releases 下载（仅允许 GitHub 官方资产主机 `objects.githubusercontent.com` 与 `release-assets.githubusercontent.com` 跳转），不接受其他跨域更新下载或描述文件指定的命令。
- 描述文件可不带签名（信任锚为 GitHub Releases 与 HTTPS）；一旦携带任何签名字段，则必须通过 Ed25519 验证。
- 版本号规则 `vX.Y.Z`：X 为年份后两位，Y 为月份（不补零），Z 为当月第几个版本（从 0 起）；`build` 为 `YYYYMMDD` + 三位序号，每次发布递增，禁止回退。
- schema 2；新版强制验证 SHA-256、大小、版本与构建高水位，描述文件携带签名字段时额外强制 Ed25519 验证与有效期；Arch 独立验证包格式、名称、架构和 pkgrel。
- 启动检查，用户确认后下载和安装；Linux 使用系统授权和对应包管理器。安装后继续使用原配置目录，重启程序，SSH 会话断开。
- 旧 Arch 版须先手动安装 R28 或更新版；不添加 pacman 软件仓库。GitHub 发布不会自动部署官网。
