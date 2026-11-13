# 无司机车辆信息的调度契约与文案

> 后续变更：用户进一步要求同步删除数据库字段，已转入[00005字段删除变更](2026-11-13-remove-dispatch-transport-columns-change.md)。以下是上一阶段的实施记录，其中旧字段兼容与无需迁移的结论已被后续变更取代。

## 背景、当前与预期

用户要求移除司机姓名、电话和车牌号，并将“发车”统一展示为“调度”。现有depart接口已接受空JSON对象，司机车辆字段是可选兼容属性；两处状态冲突错误仍使用“发车”。

## 验收与接口

- 新客户端使用 `POST /dispatches/:id/depart` 携带 `{}` 执行调度，不需要运输信息。
- 创建pending单据不扣库存；执行调度原子扣减来源完好库存、增加目的地在途，状态为in_transit，审计同事务。
- 重复调度409提示“当前单据无法调度”；非pending取消409提示“只能取消待调度的单据”。
- 旧客户端显式传入司机车辆字段仍可用，历史数据不删除；字段、路径、状态值、权限和并发保护均不变。

## 影响模块

`internal/dispatch/service.go` 的冲突提示、集成测试、README及接口/架构说明；不修改数据库结构或执行迁移。

## RED / GREEN / REFACTOR

- RED：生产修改前执行 `go test -p 1 -race -count=1 -v -tags=integration ./tests/integration -run '^TestDispatchWithoutTransportInfoAndTerminology$'`，两个冲突文案子用例按预期失败，分别仍返回“当前调度单不能发车”“只能取消待发车调度单”。空参数调度、库存/审计及旧字段兼容断言正常。仅连接独立测试库，日志 `/tmp/dispatch-without-transport-backend-red.log`，退出1。
- GREEN：同一定向集成命令通过，覆盖空JSON调度、保存不扣库存、执行只扣一次、目的地在途、同事务审计、两处409冲突文案及旧字段兼容。日志 `/tmp/dispatch-without-transport-backend-green.log`，退出0。
- REFACTOR：复用既有状态事务，不建立新的调度流程。

## 最终验证

- 定向集成测试已通过，见GREEN记录。
- `make check`全部通过（退出0）：Go格式、vet、编译、race单元测试、独立`*_test`数据库集成测试、服务及迁移/管理员工具构建。日志 `/tmp/dispatch-without-transport-backend-check.log`。
- 已重启本地开发API，`GET /readyz`返回200，数据库连接正常。
- 前端目录运行 `pnpm exec playwright test --config playwright.live.config.ts tests/live/map-dispatch.spec.ts tests/live/workflow.spec.ts --trace off`，3/3通过（32.9秒）。仅连接独立`*_test`数据库，验证无运输资料的地图直接调度、管理页保存后执行、签收、库存守恒和审计；日志 `/tmp/dispatch-without-transport-live.log`。
- 验收后已关闭临时测试API并清理临时登录文件，本地开发API继续运行；没有改写开发数据库的业务数据。

## 兼容与回滚

兼容全部既有请求字段及状态值，仅调整错误提示文字；无数据迁移或历史记录更新，回退代码即可恢复旧文案。
