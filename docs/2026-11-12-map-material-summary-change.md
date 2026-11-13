# 地图按物料统计口径

## 背景与当前行为

地图只提供按单位汇总的数量。同单位的不同物料也被相加，无法用于“全部物料”的种类统计。用户已确认：全部物料显示地点数及各状态的物料种类数，选择一种物料后显示其库存数量。

## 预期与验收

- 新增登录可读的`GET /api/v1/map/summary`，支持`spec_id`和地点名称/地址`keyword`筛选。
- 总览返回地点数及完好、损毁、入库在途的去重物料种类数；同一规格在多个地点只计一次，某状态数量大于0才计入该状态种类数。不同规格即使同单位也分别计数。
- 总览不返回跨物料的数量合计，三个数量字段为null。选定规格时返回该规格的数量、单位及有库存（任一状态大于0）的地点数。
- 总览地点数包含启用的空地点；选定规格后地图和地点数均只包含该规格有库存的地点。停用/删除地点和删除规格不参与统计。
- 搜索与前端现有名称/地址包含匹配一致，大小写及`%`、`_`按字面匹配。统计覆盖全部记录，不受库存列表分页限制。
- 无库存时返回0及所选物料单位；非法规格ID、过长关键词返回400，不存在/已删除规格返回404。

## 模块与契约

`internal/catalog`增加参数化聚合查询与输入校验，`internal/httpapi`注册受保护路由。返回`location_count`、`normal_spec_count`、`damaged_spec_count`、`in_transit_spec_count`、`normal_qty`、`damaged_qty`、`in_transit_qty`、`unit`；数量及unit在总览为null。现有地图数组与`stock_by_unit`保持兼容，筛规格时排除无库存点。

## TDD

- RED：生产代码修改前，`make test-integration`发现新接口返回404。修正测试子用例的错误上报作用域后，独立运行`go test -p 1 -race -count=1 -v -tags=integration ./tests/integration -run '^TestMapMaterialSummary$'`，6个子用例均因接口不存在返回404而按预期失败；日志`/tmp/map-summary-server-red-focused.log`。
- GREEN：同一针对性集成命令6个子用例全部通过，验证跨地点去重、同单位不同规格、数量与地点一致、字面搜索、零库存单位、超过100种、权限和输入校验。日志`/tmp/map-summary-server-green.log`。
- REFACTOR：统计在参数化SQL中一次聚合，service负责输入校验；前端无需扫描库存分页。地图规格筛选与统计使用相同的正库存规则。保留旧地图响应字段，无schema变更。

## 验证与回滚

仅使用独立`material_dispatch_test`数据库验证多地点重复规格、同单位不同物料、空地点、零库存、停用/删除、超过100种物料、筛选与权限。最终运行`make check`，不迁移或修改开发数据。回滚路由、catalog查询及对应前端即可；无需schema变更。

- `make check`退出码0：gofmt、go vet、全包编译、全部普通/竞态测试、迁移生命周期、真实业务集成和3个生产二进制构建通过。日志`/tmp/map-summary-server-check.log`。
- 本地8080开发服务已使用新二进制重启，`/readyz`返回200，未登录访问新统计接口返回401。
- 已同步README、API契约及架构说明。前端真实浏览器联调1项通过：在独立测试库确认15地点总览1种完好物料，选定配重块显示100t，并打开地点库存抽屉。日志`/tmp/map-summary-live.log`。联调后停止8081测试服务并清理临时凭据，开发数据库不变。
