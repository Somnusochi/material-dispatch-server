# 2026-11-11 依赖下载镜像

## 背景与当前配置

用户指出依赖下载缓慢时可配置镜像。Go 默认代理为 `https://proxy.golang.org,direct`，前端 registry 为 `https://registry.npmjs.org/`；此前仅临时为部分 Go 命令指定镜像，工程内没有固定默认值。

## 预期与影响

- 后端 Make 命令默认 `GOPROXY=https://goproxy.cn,direct`，可通过环境变量或 Make 参数覆盖；增加 `make deps` 下载并校验模块。
- 前端 `pnpm-workspace.yaml` 默认使用 `https://registry.npmmirror.com/`，可用命令行 `--registry` 覆盖。
- 保持依赖版本、锁文件、TLS 和完整性校验；不更改全局工具配置或业务运行行为。Go 的 `GOSUMDB` 继续使用默认校验库。
- 更新两端 README 的安装与切回官方源命令。直接运行 `go` 不读取 Makefile，需要显式传入 GOPROXY。

## 验证与 TDD 范围

本次为依赖维护和构建配置，不涉及业务行为，按全局开发规范的依赖维护例外无需新增业务 RED/GREEN 用例。不新增仅重复配置内容的测试。

- 配置前实测：npmmirror 的 leaflet 1.9.4 元数据、goproxy.cn 的 Gin v1.12.0 元数据均返回 HTTP 200。
- `pnpm config get registry` 返回镜像地址；加 `--registry=https://registry.npmjs.org/` 返回官方地址；`pnpm view leaflet@1.9.4 dist.tarball` 返回 npmmirror 的包下载地址。
- 在 Make 环境查询 `go env GOPROXY GOSUMDB` 得到 `https://goproxy.cn,direct` 和 `sum.golang.org`；环境变量覆盖后得到官方代理。直接运行 `go env GOPROXY` 仍为原官方默认值，未修改全局配置。
- `make deps` 通过，输出 `all modules verified`。
- `pnpm install --frozen-lockfile --ignore-scripts` 通过：558项锁文件供应链策略校验通过，锁文件无需重解。沿用已安装依赖，本次不重跑安装脚本。
- SHA-256 对比确认 `go.mod`、`go.sum`、`package.json`、`pnpm-lock.yaml` 全部未变化。
- `make -n` 仍为 `go run ./cmd/server`，新增 deps 未改变原默认目标。
- 后端 `make check` 全部通过，含2项迁移测试、7项真实数据库业务测试（49.028s）、race 检查与生产构建。
- 前端 `pnpm check` 全部通过，含15项单元测试、13项开发浏览器、13项生产预览浏览器（43.7s）、格式、lint、类型和生产构建。

## 兼容与回滚

删除项目默认镜像设置，或按 README 对单次命令指定官方源即可回退。未使用 `go env -w` 或全局 pnpm 配置。`GOPROXY` 的逗号分隔项只在前一代理返回404/410时尝试下一项；网络错误时可显式覆盖，不关闭校验绕过下载问题。

## 配置依据

- [pnpm 项目配置](https://pnpm.io/settings)
- [npmmirror 使用说明](https://npmmirror.com/)
- [Goproxy.cn](https://goproxy.cn/)
- [Go 模块代理协议](https://go.dev/ref/mod#goproxy-protocol)
