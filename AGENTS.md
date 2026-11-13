# material-dispatch-server 开发规范

- 使用中文交流，先阅读 README.md 和 docs/architecture.md。
- Go + Gin + PostgreSQL；固定依赖在 go.mod/go.sum，禁止自动迁移数据库或提交凭据。
- 所有业务行为变更（含修复）先在 docs/YYYY-MM-DD-<topic>-change.md 记录背景、当前/预期行为、验收标准、模块/接口、TDD 证据、最终验证与回滚。
- 严格 RED → GREEN → REFACTOR；先运行失败测试且确认原因，再写生产实现。没有可用测试方案时先询问用户。
- 完成前必须 make check 全部通过；集成测试只允许独立 *_test 数据库。
- 新业务按 internal/<domain> 组织。HTTP 处理参数与响应，service 负责状态规则和事务，repository 负责参数化 SQL；接口只在确有替换需求处建立。
- 库存流转和审计必须在同一事务内完成；不使用异步 HTTP 日志替代业务审计。
- 所有地图输入统一为 WGS84；时间存 timestamptz，展示时转换为 Asia/Shanghai。
- 不创建默认管理员或默认口令。身份认证与授权未接入前，不开放业务数据接口。
- 提交采用中文 Conventional Commits；未经用户要求不提交或推送。
