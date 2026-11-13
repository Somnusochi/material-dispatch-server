# 地图地点下拉选择与精确统计

## 背景、当前与预期

用户要求将地图页的已建地点搜索框改为可搜索的下拉选择。当前统计只接受名称/地址关键词，不能区分同名地点；前端已有完整地图地点集合，可以复用生成选项。

选定地点后必须按地点ID精确筛选列表、地图和统计，避免将同名地点库存一起计入。全部地点继续沿用原统计；原keyword参数保持兼容。

## 验收与接口

- `GET /api/v1/map/summary` 新增可选 `location_id`，与 `spec_id`、`keyword` 取交集。
- 精确地点的总览按物料种类统计；选定规格后按该地点的数量和单位统计，空地点返回零库存。
- 非法ID返回400；不存在、停用或删除地点返回404；认证要求不变。
- 不增加数据库表或迁移，不修改库存、审计或地图交互规则。
- 前端选项使用当前物料下已有的完整地图点位，不增加100条的截断。

## 影响模块

`internal/catalog/service.go`、`repository.go`、`internal/httpapi/application.go`，统计集成测试和API文档。

## RED / GREEN / REFACTOR

- RED：`go test -p 1 -race -count=1 -v -tags=integration ./tests/integration -run TestMapMaterialSummary` 预期失败：选择单个地点仍得到全部50个地点，非法ID仍200；日志 `/tmp/map-location-select-backend-red.log`。确认后才修改生产代码。
- GREEN：同一定向命令通过所有9组统计子测试，包含同名地点隔离、空地点、非法/不可见地点以及旧keyword兼容；日志 `/tmp/map-location-select-backend-green.log`。
- REFACTOR：复用启用地点查询及现有聚合，不新增并行统计实现。

## 最终验证

- 定向统计集成测试通过，日志见GREEN。
- `make check` 退出0：格式、go vet、编译、全部竞态单元/集成测试、迁移检查、生产构建通过；日志 `/tmp/map-location-select-backend-check.log`。仅使用独立 `material_dispatch_test` 数据库。
- 本地API已重启，`GET /readyz` 返回200且数据库up，无需迁移。
- 真实前后端验收2项通过：15个模拟地点/100t的物料筛选、地点下拉精确统计与清空，以及地图当前页发车、来源库存/目的地在途和审计；前端执行 `pnpm exec playwright test --config playwright.live.config.ts tests/live/material-map.spec.ts tests/live/map-dispatch.spec.ts --trace off`，日志 `/tmp/map-location-select-live.log`。仅连接独立测试库，未修改开发库库存。
- 验收后已停止本轮8081测试API，临时登录文件随进程退出清理，开发API继续运行。

## 兼容与回滚

新参数可选，旧客户端及keyword保持原语义。需先更新后端再使用前端精确地点过滤；回退时同时回退前端，避免旧接口忽略location_id。无数据迁移或回滚操作。
