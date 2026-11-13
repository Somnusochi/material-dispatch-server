# 后端代码评审与职责拆分

## 背景与问题

按用户要求完成 Go 最佳实践评审及重构，同时完成 Go 1.27.4 升级验收。评审使用 GitNexus 1.6.12 的 context、impact、query，并结合源码和现有集成测试；图谱不能替代动态路由、SQL 和事务验证。

## 当前行为与评审发现

- `internal/httpapi/application.go` 的 250 行构造函数同时负责组装依赖、认证、全部业务路由和匿名处理器；整个文件 484 行。
- `auth/service.go` 同时包含用户维护、密码、登录与轮换会话；catalog 同时包含地点、规格和地图查询。已有领域目录及 repository SQL 分离合理，保留它们。
- 请求解析直接 Decode 到结构体，疑似接受根 JSON `null`；对于允许空对象的调度动作可能产生实际库存和审计写入。先运行自动化测试确认。
- 库存/调度/损毁通过同一数据库事务写业务与审计，保留现有并发锁、权限及状态机。

## 预期行为与验收

1. 构造函数仅组装服务与路由；按领域分离 HTTP handlers，认证、请求解析、分页、响应和限流各归属明确。
2. auth/catalog 按用例拆文件，保持同一 Go 包；不为了分层创建无替换需求的接口。
3. 业务 JSON 写接口只接受单个对象；`null`、数组、标量、多对象、未知字段及超限请求返回 400，不能更改单据、库存或审计。合法 `{}` 动作继续可用。
4. URL、角色、响应 envelope、双 token 时效及数据库 schema v5 均保持兼容，无迁移需求。
5. `make check` 全通过，并通过前端真实 API 联调回归。

## 影响模块 / 接口

`internal/httpapi`、`internal/auth`、`internal/catalog`；全部 `/api/v1` 路由注册方式，业务请求契约仅收紧非法根 JSON 输入。`dispatch`、`inventory`、`damage` 的事务业务规则不重写。

## RED / GREEN / REFACTOR

- 升级基线：Go 1.27.4 `make check` 全部通过，日志 `/tmp/go-1274-baseline-check.log`。
- RED：`TestBindRequiresSingleJSONObject/null` 得到 true/HTTP 200；独立测试库 `TestNullDispatchActionHasNoSideEffects` 的 depart/cancel 均得到 200，实际改变状态。日志 `/tmp/server-review-refactor-red.log`、`/tmp/server-review-refactor-integration-red.log`。
- GREEN：先保留 1 MiB 限制，再要求根 JSON 对象，继续严格解码未知字段与尾随值。初始 14 个解析场景与两种动作零写入测试通过，合法空对象动作继续通过。日志 `/tmp/server-review-refactor-green.log`、`/tmp/server-review-refactor-integration-green.log`；最终补充空白边界后共 16 个解析场景。
- REFACTOR：HTTP 分离按领域的 route registration/handlers、middleware、request/response、分页与限流；auth 分 users/login/session/password，catalog 分 locations/specs/map/validation。事务、锁序和领域 SQL 未改。完整后端回归已通过。

## 最终验证

- `make check`：通过，Go 1.27.4；包含格式、vet、编译、竞态单元测试、专用 *_test 库迁移/业务集成与 3 个二进制构建。日志 `/tmp/server-review-refactor-check.log`。
- `go version -m bin/server bin/admin bin/migrate`：确认三个产物均为 Go 1.27.4。全部验证后，本地 8080 已按原配置重启为最终构建；`GET /readyz` 返回 ready / database up，未执行开发库迁移。
- GitNexus 1.6.12 CLI 与 MCP 均安装验证；已验证握手、17 个工具和两个仓库 context 资源。按 `--index-only` 保留现有 AGENTS.md，索引不会提交。
- 语法树函数比对：原有 111 个函数仅 NewApplication（路由拆分）与 bind（已测修复）有内容变化；没有移除函数。另确认 7 个非 auth 领域 repository 文件逐字节不变，auth 只移入用户扫描/列清单，SQL 未重写。比对日志 `/tmp/server-review-refactor-function-comparison.json`。
- GitNexus 最终重建成功：938 节点、2708 关系。`detect-changes --scope all` 因本仓库尚无首次提交/HEAD 无法计算 Git diff；未为工具创建提交，使用源码快照比对、调用关系查询和完整测试验收。图谱存在调用解析与流程截断限制，不将缺失关系视为代码不可达。
- 前端 `pnpm check`：通过，包含 44 项单元测试、开发环境 131 项与生产预览 131 项浏览器测试、生产构建。日志 `/tmp/web-review-refactor-check.log`。
- `pnpm test:integration`：真实 API 的 5 项浏览器回归全部通过，覆盖续签、地图直接调度、库存签收、损毁/图片和用户管理，日志 `/tmp/review-refactor-live.log`。测试 API 已停止，临时凭据已清理。

## 兼容性与回滚

合法客户端不受影响；依赖 `null` 作为动作请求体的非规范客户端应改为 `{}`。无数据库变更。重构前源码保存在忽略目录 `.cache/review-refactor-before`，回滚需同时还原相关测试及文档，避免覆盖用户并行改动。没有提交或推送。

### 解析空白边界复核

最终评审发现通用 TrimSpace 会接受 JSON 不允许的 NBSP。先增加 non_json_leading_space / non_json_trailing_space 两个失败用例，均复现返回 true/HTTP 200（`/tmp/server-review-json-whitespace-red.log`）。改为只去除 JSON 合法的空格、制表、回车、换行；16 个解析用例通过（`/tmp/server-review-json-whitespace-green.log`），随后最终 `make check` 和真实 API 浏览器回归全部通过。
