# 损毁上报取消等级录入

## 背景与当前行为

用户指出损毁数量已经表达本次涉及的损毁库存，轻微/中等/严重等级没有必要。当前前端表单默认传 `moderate`，列表展示等级；后端不传等级会返回400，但库存和确认、驳回、修复、报废均不使用等级。

## 预期与验收

- 上报只需地点、规格、正整数损毁数量和损毁说明，照片可选；`damage_level` 不再必传，省略、null、空串都可提交。
- 数量、说明、图片引用、身份及角色校验继续执行；上报待审核不改变库存，确认时才按数量转入损毁库存。
- 不增加或删除数据库列，不改写历史记录或已应用迁移。旧字段仅作兼容：省略时使用既有默认值 `moderate`，旧客户端显式合法等级仍保留，非法值仍返回400。该兼容值不代表用户的新评级，不用于界面展示或业务决策。
- 确认、驳回、修复、报废、运输签收和同事务审计规则不变。

## 影响模块与契约

- `internal/damage/service.go`：等级改为兼容性可选输入。
- `tests/integration/`：无等级创建、数量处理、照片/审计、旧参数兼容与校验。
- `POST /api/v1/damage-reports` 必填字段文档同步；读取仍保留旧字段供兼容。
- 相邻前端移除表单、默认值和列表列，不再发送等级。

## RED / GREEN / REFACTOR

- RED：`go test -p 1 -race -count=1 -v -tags=integration ./tests/integration -run 'TestDamageReportWithoutLevel|TestInventoryDispatchDamageAndAudit'` 已失败，省略/null/空串均返回400“损毁等级无效”，说明/照片校验也被等级拦截；日志 `/tmp/damage-without-level-backend-red.log`。失败原因符合预期后才修改生产代码。
- GREEN：同一命令通过，包含无等级/空值创建、照片保存、事务审计、确认/修复、重复确认409、运输签收/报废闭环和旧参数兼容；日志 `/tmp/damage-without-level-backend-green.log`。服务端对缺省旧等级使用兼容值，保留原显式值校验。
- REFACTOR：确认等级没有参与签收或损毁处置，前端移除默认值及依赖，复用原表结构与事务，不增加重复接口。

## 最终验证

- 定向集成测试通过，命令与日志见GREEN。
- `make check` 退出0：gofmt、go vet、编译、全部竞态单元测试、迁移/路线/业务集成测试及三个生产命令构建通过；日志 `/tmp/damage-without-level-backend-check.log`。数据库测试仅使用 `material_dispatch_test`。
- 已重启本地开发API，`GET /readyz` 返回200且数据库up；匿名访问 `/api/v1/damage-reports` 仍返回401。本轮无需迁移。
- `make test-e2e-server` 配合前端 `pnpm exec playwright test --config playwright.live.config.ts tests/live/workflow.spec.ts --grep '真实图片损毁上报' --trace off` 通过1项真实流程（16.6秒）：照片上传、无等级上报返回201、确认损毁、报废及库存核对；日志 `/tmp/damage-without-level-live.log`。使用独立测试库，结束后关闭8081临时API并清理测试凭据文件。

## 兼容、风险与回滚

无需迁移或数据修复，历史等级和审计快照保留。先更新后端再使用新版前端；只回退后端会重新要求等级，需同时回退前端。测试不改开发库的库存或损毁记录。
