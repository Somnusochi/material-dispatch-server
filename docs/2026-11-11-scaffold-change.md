# 2026-11-11 后端骨架与数据库初始化

> 本文为骨架阶段历史记录。后续完整业务实现与最终验收见 [完整系统变更记录](2026-11-11-complete-system-change.md)。

## 背景与当前行为

本仓库原为空。用户要求 Gin 后端、本地 PostgreSQL，与相邻 React 前端配套；用户提供的 PDF《物料地图调度系统工程命名方案》确认用户、地点、规格、库存、调度、损毁照片和审计需求。

## 预期行为与验收标准

- Go + Gin，配置校验、pgx 连接池、JSON 日志、请求 ID、统一响应、HTTP 超时和优雅退出。
- `/healthz` 不依赖数据库；`/readyz` 与 `/api/v1/system/status` 限时检查连接，失败为 503，错误不泄漏凭据。
- 创建本地开发库与独立测试库，专用非超级用户，不影响已有库。
- Goose 显式事务迁移、会话锁、七张基础表及约束；无默认管理员、无业务写入 API，服务启动不自动迁移。
- 格式、静态检查、编译、全量竞态测试、真实数据库迁移测试及生产构建全部通过。

## 影响模块与接口

`cmd/server` 装配；`internal/config` 配置；`internal/app` 生命周期；`internal/httpapi` HTTP；`internal/platform/postgres` 连接池；`cmd/migrate` 和 `migrations` 数据库迁移。

成功响应 `{data,request_id}`；失败 `{error:{code,message},request_id}`。错误 HTTP 状态保留 404/405/500/503。不信任代理头，不开放宽泛 CORS。开发前端由 Vite 同源代理。

`material_dispatch` schema 包含 sys_users、locations、material_specs、material_inventories、dispatch_orders、damage_reports、operation_logs。详细契约与后续事务约束见 [架构文档](architecture.md)。

## RED / GREEN / REFACTOR 证据

1. 生产实现前执行 `go test ./internal/config ./internal/app`：Load、NewServer、Serve 未定义；连接测试 Open 未定义，HTTP 测试 NewRouter 未定义。新仓库的预期缺失行为均被测试捕获。
2. 建表前 `psql -X -d material_dispatch_test -f tests/schema.sql` 退出 3，报 `missing expected business table: material_dispatch.sys_users`。
3. 实现后配置、HTTP、连接失败脱敏、优雅退出、七表、非负库存、唯一性、坐标、枚举、照片类型、审计不可变、updated_at 和迁移回滚测试通过。
4. 真实重连发现账号与 schema 同名改变默认 search_path，新的迁移进程误读版本历史。先新增 `TestMigrationHistorySurvivesReconnect` 并确认 `migration history lost on reconnect` 失败，再将元数据固定为 `public.goose_db_version`。跨连接测试与 `make migrate-status` 均通过，版本 1 为 applied。
5. 重构统一 HTTP envelope、探针处理、错误返回及连接池生命周期；配置和凭据仅由入口注入，SQL 显式限定 schema。

## 最终验证（2026-11-11）

| 命令/验证                                  | 结果                                                                                  |
| ------------------------------------------ | ------------------------------------------------------------------------------------- |
| `make check`                               | 通过：gofmt、go vet、全包编译、全量 go test -race、强制数据库集成测试、两个生产二进制 |
| `make test-integration`（包含在 check 中） | TestSchemaLifecycle、TestMigrationHistorySurvivesReconnect 均通过                     |
| `make migrate-up`                          | 开发库版本 1 已应用                                                                   |
| `make migrate-status`                      | `1 applied`                                                                           |
| `psql ... -f tests/schema.sql`             | 七张预期表存在                                                                        |
| 本地角色核验                               | rolsuper / rolcreatedb / rolcreaterole 均 false                                       |
| 真实 HTTP 联调                             | `/readyz` 与前端代理 `/api/v1/system/status` 均 200，database=up                      |
| 本地凭据                                   | `.env` 权限 0600，Git 忽略有效                                                        |

GitHub CI 已配置但尚未推送，未宣称远程 CI 执行结果。依赖网络缓慢时使用临时镜像，未更改全局 Go 配置。

## 兼容、风险与回滚

全新接口，无历史兼容负担。开发库 `material_dispatch`、测试库 `material_dispatch_test`；测试仅允许 `_test` 库。迁移版本表固定在 public，业务表固定在 material_dispatch。Down 会删除七张业务表与数据，只能在明确可丢弃环境或备份后使用；没有 CASCADE，未知依赖不会被隐式删除。自动回滚仅在专用测试库演练。

PostGIS、用户登录/授权、库存并发事务、图片存储与真实业务 API 尚未实现。未来业务审计必须与数据变更同一事务写入，不能用访问日志替代。所有地点坐标统一 WGS84，API bigint ID 用字符串，数据库时间为 timestamptz。生产部署应分离迁移 owner 和运行时 DML 用户。
