# 地点物料列表与部分损毁语义修正

## 背景与当前行为

全部物料模式的地点卡片及地图提示展示按单位合计的数量，不同物料即使用相同单位也会被混加。用户进一步明确地点卡片应逐项列出物料，不能仅显示种类数；部分库存损毁也不能把整种物料标为损毁。

## 预期行为与验收

- 每个地点增加 `normal_spec_count`、`damaged_spec_count`、`in_transit_spec_count`；按该地点的规格 ID 分别去重，只有对应状态数量大于 0 才计入。
- 同单位的不同物料分别计数；零库存、已删除规格不计入；启用空地点三个统计为 0，停用及删除地点不返回。
- 选择具体规格时继续返回该物料的数量和单位，既有库存、选点及调度行为不变。
- 超过 100 种物料也按数据库全量统计，不依赖库存分页。
- 地图每个地点增加 `materials`（按库存记录 ID 降序的前 3 条）和 `material_count`（有任意正库存的完整物料种类数），每条有字符串 `id/spec_id`、`spec_name/unit` 和三类数量，禁止跨规格相加。
- `GET /map/locations/:id/materials` 支持标准分页，使用与预览相同的有效地点、规格、正库存规则及稳定顺序；翻页可读取全部物料。
- 同一种物料可以同时有完好与损毁数量；状态计数表示“含该状态库存的物料种类”，不表示该物料全部处于该状态。

## 影响模块与契约

catalog 查询与 service、HTTP 只读路由、集成测试、API 与架构说明。保留 `stock_by_unit` 和原有数量字段兼容旧客户端；列表首页批量返回预览，只有地点卡片翻页时单独请求，避免首次载入每地点发起请求。无需数据库迁移或修改库存数据。

## RED / GREEN / REFACTOR

- RED：地点计数测试因缺字段失败（`/tmp/map-location-overview-backend-red.log`）；最终逐项物料与分页测试因`materials`缺失和新接口404按预期失败（`/tmp/map-material-list-backend-red.log`）。
- GREEN：`TestMapLocationMaterials`、`TestMapMaterialSummary`、`TestMapUnitTotals`全部通过（`/tmp/map-material-list-backend-green.log`），包含同单位分行、部分损毁、空地点、无效/未登录访问以及108种物料36页无遗漏重复。
- REFACTOR：预览、分页共享有效库存查询，保持既有单位字段与筛选行为。

## 最终验证

- 针对性集成：`go test -p 1 -race -count=1 -v -tags=integration ./tests/integration -run '^TestMap(LocationMaterials|MaterialSummary|UnitTotals)$'`，3组全部通过。
- `make check`：exit 0，格式、vet、编译、全量竞态测试、独立 `_test` 库迁移与集成测试、三个生产二进制构建通过。日志 `/tmp/map-material-list-backend-check.log`。
- 已重启本地开发后端，`GET /readyz` 为200；未登录访问新物料分页接口为401。开发库没有执行迁移或改写库存。
- 前端真实接口联调1项通过：15个模拟地点逐项显示物料、三类数量与单位，总量100t；只使用独立测试库。临时8081服务已停止，临时登录文件已清理。日志：`/tmp/map-material-list-live.log`。

## 兼容、风险与回滚

后端应先于前端更新。旧字段保留仅为兼容，不作为全部物料模式的展示依据。回滚程序无需回滚数据库；不得修改既有库存或迁移记录。
