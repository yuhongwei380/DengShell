# DevShell v26.10.0 · 构建 20261018135

Phoenix 分支（[yuhongwei380/DevShell](https://github.com/yuhongwei380/DevShell)）；版本号 `vX.Y.Z`，内部构建号 `YYYYMMDD` + 三位序号递增防回退。

## 本次变化

- 修复：断开连接的会话里本机提示符 `ping` 输出逐行向右阶梯缩进（xterm 把裸换行按“只下移不回行首”处理），多行输出现统一转为 CRLF，持续探测同样修复。

## 升级说明

- 首次安装选 `DevShell-Setup-x64.exe`；`up.*` 文件是内置自动更新载荷，无需手动下载。
- 构建 ≤20261018118 的旧版无法内置更新到新版，请手动安装一次最新 `DevShell-Setup-x64.exe`；连接、密钥等数据自动沿用。
- 构建 20261018128 的桌面窗口不可用（所有请求报「桌面接口尚未就绪」）且无法内置更新，请手动安装一次最新 `DevShell-Setup-x64.exe`；数据自动沿用。

