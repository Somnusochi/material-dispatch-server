# Go 1.27.4 工具链升级

## 背景与当前行为

用户明确要求直接升级到Go 1.27.4，同步相关配置并完成全部测试。当前go.mod声明1.26.0，本机引导工具链为1.26.4；CI从go.mod选择版本，Makefile中的gofmt直接来自系统PATH。

## 预期行为与验收标准

- go.mod、Make构建/测试/格式化、CI和使用文档统一到Go 1.27.4，实际产出的server、migrate、admin二进制由go1.27.4构建。
- 使用项目内缓存和现有Go镜像下载并校验工具链；不修改其他项目使用的全局Go配置。
- 沿用encoding/json的现有API和业务JSON契约，使用Go 1.27的默认JSON实现；不设置nojsonv2规避验证，不批量改写业务代码或升级无关依赖。
- 完整后端检查、前端完整检查和真实API浏览器回归通过。数据库测试仅使用独立`*_test`库，不变更开发库schema和数据。
- 全部验证完成后重启本地后端并核对运行程序的构建版本和就绪状态。

## 影响模块与契约

go.mod、Makefile、.github/workflows/ci.yml、README及架构说明。依赖保持现有锁定版本，按实际工具链输出核对go.sum。无新增HTTP接口或数据库迁移。

## 升级前后验证 / RED、GREEN、REFACTOR

本次为工具链和构建配置版本升级，业务契约保持不变，按项目全局规范的版本升级例外，不增加仅比较版本常量的业务单元测试。升级前先自动核验目标工具链，确认当前版本不满足要求；已有完整测试用于检验兼容性。若发现需要改变业务实现的兼容问题，再按RED → GREEN → REFACTOR补充对应回归测试。

- 升级前：自动执行目标版本断言，输出`Expected: go1.27.4 / Actual: go1.26.4`，退出1，确认尚未使用目标工具链。随后通过Goproxy.cn下载并校验项目缓存中的go1.27.4，下载日志`/tmp/go-1274-download.log`。
- 升级后：`make toolchain` 和最终 `make check` 均实际使用 `go1.27.4 darwin/arm64`；`go version -m bin/server bin/admin bin/migrate` 确认三个产物均由 go1.27.4 构建，无额外 GOEXPERIMENT 覆盖。
- REFACTOR：go.mod作为版本来源，Make和CI从中选择1.27.4。新增make toolchain验证实际版本，格式化调用所选GOROOT下的gofmt并传播格式化失败；完整check先验证工具链。避免多个地方独立维护版本号。

## 最终验证

- 后端 `make check`：通过，包含工具链核对、格式、go vet、全包编译、竞态单元测试、专用测试库迁移与业务集成、三个生产二进制构建。升级基线日志为 `/tmp/go-1274-baseline-check.log`，评审修复后的最终日志为 `/tmp/server-review-refactor-check.log`。
- 前端 `pnpm check`：通过；格式、lint、类型、44 项单元测试、131 项开发环境浏览器测试、生产构建与 131 项生产预览测试全部通过，日志 `/tmp/web-review-refactor-check.log`。
- 前端 `pnpm test:integration`：5/5 通过，覆盖真实 API 认证续签、地图、调度签收、损毁处置、图片及用户管理，日志 `/tmp/review-refactor-live.log`。使用独立 `material_dispatch_test`，测试服务已停止，临时凭据文件已清理。
- 全部测试后已用最终 `bin/server` 重启本地 8080；`GET /readyz` 返回服务 ready、数据库 up。未修改全局 Go 安装，未执行开发数据库迁移。
- 评审中发现并按 RED → GREEN 修复非法 JSON 根值和空白字符边界；证据见 [后端评审记录](2026-11-13-review-refactor-change.md)。没有修改合法请求契约。

## 兼容、风险与回滚

Go 1.27更换了encoding/json底层实现，但保留v1兼容语义；需重点回归严格JSON输入、认证、库存调度事务、图片与地图接口。本次不更改数据结构。回滚时恢复上一版工具链配置并重新构建、重启服务，无需回滚数据库迁移。历史变更文档中的旧版本保留为当时记录。
