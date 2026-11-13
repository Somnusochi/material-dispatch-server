# 删除调度单废弃运输字段

## 背景与当前行为

用户要求后端删减字段时同步修改数据库。上一轮已移除前端司机姓名、司机电话和车牌号，但后端请求、SQL读写、响应以及调度表仍保留三个兼容字段。本次将这三个字段从当前业务契约与表结构中删除。

## 预期行为与验收标准

- 新增Goose 00005迁移，删除`material_dispatch.dispatch_orders`中的`driver_name`、`driver_phone`、`vehicle_plate`，不改写已应用的00001–00004。
- 后端不再接受、读取、写入或返回这三个字段；旧请求携带这些字段按既有严格JSON校验返回400。执行调度继续发送空对象`{}`。
- 创建、详情、列表、状态转换响应及新生成的调度审计快照均不含这些字段。已有不可变审计快照保留历史原貌。
- 升级不影响单据、数量、状态、库存、人员关联或审计记录；保存、执行、取消、签收及并发防超卖继续正常。
- 在独立`*_test`库验证v4带历史数据升级、回滚再升级及迁移幂等性；下行迁移恢复相同类型的可空列，但原字段值需从升级前备份恢复。
- 完成`make check`及真实接口验收后，备份本地开发库并显式执行迁移，核对版本和列结构，再启动新后端。启动过程不自动迁移。

## 影响模块与契约

`internal/dispatch/service.go`、`repository.go`、新增迁移及迁移测试、调度集成测试、API/架构/README。前端上一轮已不使用这些字段，无需修改前端业务实现。

## RED / GREEN / REFACTOR

- RED：实现修改前运行`go test -p 1 -race -count=1 -v -tags=integration ./migrations ./tests/integration -run '^(TestDispatchTransportColumnsMigration|TestMigrationHistorySurvivesReconnect|TestDispatchWithoutTransportInfoAndTerminology)$'`，退出1，确认数据库仍有三列、迁移版本仍为4、创建/详情/列表/执行响应和新审计仍含旧字段，三个废弃字段请求均错误返回200并扣库存。日志`/tmp/remove-dispatch-transport-red.log`；先修正测试夹具的自增ID后得到上述预期失败，未提前改动生产实现。
- GREEN：在上面的定向命令中加入`TestInventoryDispatchDamageAndAudit|TestConcurrentDepartDoesNotOversell`，迁移、重连历史、调度契约、库存/签收/审计闭环、并发防超卖全部通过（退出0）。旧字段请求返回400且不改状态、不扣库存、不生成执行审计；新响应和新审计快照无废弃字段。日志`/tmp/remove-dispatch-transport-green.log`。
- REFACTOR：同步移除请求类型、长度校验、查询投影、UPDATE列与参数，复用既有严格JSON校验、事务和状态流；不新增兼容字段或启动迁移逻辑。迁移采用单条ALTER和5秒锁等待上限，不使用CASCADE。

## 最终验证

- 定向测试通过，见GREEN记录。
- `make check`全部通过，退出0：Go格式、vet、编译、race单元测试、独立`*_test`数据库的迁移/路线/全量业务集成测试、服务与运维工具构建。日志`/tmp/remove-dispatch-transport-check.log`。
- 前端目录运行`pnpm exec playwright test --config playwright.live.config.ts tests/live/map-dispatch.spec.ts tests/live/workflow.spec.ts --trace off`，3/3通过（34.9秒）：在实际v5测试库验证地图当前页调度、管理页保存/调度/签收、库存/损毁/取消及审计闭环。日志`/tmp/remove-dispatch-transport-live.log`。
- 前端业务代码未变；仅给上一轮文档补充后续变更说明，`pnpm exec prettier --check docs/2026-11-13-dispatch-without-transport-change.md`通过。
- 本地开发库`material_dispatch`升级前为v4、三个旧列存在、调度单0条。短暂停止旧API后，通过`pg_dump --format=custom`备份，使用`pg_restore --file=/dev/null`完整读取验证归档；当前无开发上传目录需归档。
- 私有备份目录`.cache/database-backups/20261113-115531-before-00005-ei1kr0i7`（0700），`material_dispatch.dump`为0600、61,274字节。目录内包含迁移日志、前后快照摘要及SHA-256清单；不入Git。
- 显式执行`./bin/migrate up`成功应用1项迁移。后置脚本最初将SQL NULL当JSON解析而退出；改为列数量查询后完成只读核验，无重复迁移：版本5、废弃列0个，13张业务表的行数及排除这三列后的数据摘要全部一致。
- `./bin/migrate status`确认1–5均为applied；已启动新构建，`GET /readyz`返回200且数据库up。临时8081验收API已关闭，临时登录文件已删除。

## 兼容、风险与回滚

这三个废弃字段不再兼容旧请求；前端已使用空对象执行调度。旧后端SQL引用被删列，因此本地迁移时短暂停止旧服务；迁移完成后启动新构建。迁移在事务内执行，不使用CASCADE。升级前备份完整开发库，备份存于被Git忽略的私有目录。下行只恢复列结构，旧值需通过备份恢复；回退旧后端前先回滚00005。不可变历史审计不做清洗或修改。
