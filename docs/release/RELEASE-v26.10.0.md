# DevShell v26.10.0 · 构建 20261018136

Phoenix 分支（[yuhongwei380/DevShell](https://github.com/yuhongwei380/DevShell)）；版本号 `vX.Y.Z`，内部构建号 `YYYYMMDD` + 三位序号递增防回退。

## 本次变化

- 新增：未连接服务器时本机提示符支持 `traceroute 主机`、`nslookup 主机`、`curl [-I] 网址`
  - curl 仅支持 http/https、不跟随重定向；traceroute 调用系统工具，参数固定、目标严格校验，绝不执行用户 shell
- 新增：标题栏明暗切换按钮左侧显示大版本号（如 v26.10.0，悬停可见完整版本）
- 修复：链接栏点击已有会话的连接无任何响应
  - 现与服务器列表双击同语义，每次点击都新开标签建立独立会话
- 修复：断开连接的会话里本机提示符 `ping` 输出逐行向右阶梯缩进
  - xterm 把裸换行按「只下移不回行首」处理，多行输出现统一转为 CRLF，持续探测同样修复

## 升级说明

- 首次安装选 `DevShell-Setup-x64.exe`；`up.*` 文件是内置自动更新载荷，无需手动下载。
