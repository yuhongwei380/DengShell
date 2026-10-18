# DengShell v26.10.0 · 构建 20261018001

Phoenix 分支（[yuhongwei380/DengShell](https://github.com/yuhongwei380/DengShell)）首个按新版本号规则发布的版本。

## 版本号规则

- 版本号 `vX.Y.Z`：X 为年份后两位，Y 为月份（不补零），Z 为当月第几个版本（从 0 起）。
- 内部构建号 `ApplicationBuild`：`YYYYMMDD` + 三位序号，每次发布递增，更新防回退依赖它。

## 本轮变化

- 内置更新检查与下载切换为 GitHub Releases；描述文件默认不带签名，保留 SHA-256、大小、版本与构建防回退校验。
- 界面字体为阿里巴巴普惠体 3.0 55 Regular（免费商用），替换 IBM Plex Sans SC；旧界面字体设置自动迁移。
- 界面展示版本为 `ver.Phoenix · v26.10.0`。

## 升级说明

- 本轮提供 Windows 安装版、绿色 ZIP，Linux DEB、RPM、pacman 与通用安装包，以及源码。
- 由本分支此前发行包安装的 Windows、DEB、pacman 客户端可通过内置更新获取后续版本；上游旧客户端仍检查原官网，不会收到本分支更新，需手动安装一次本分支发行包。
- 连接、密钥、字体及背景沿用原配置目录。
