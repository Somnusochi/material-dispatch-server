# 调配目的地路线距离与定时更新

## 背景与当前行为

目的地目前按地点 ID 排序，没有来源相关的道路距离。用户要求显示最近路线距离，最近目的地优先，并考虑每天更新。

## 预期行为与验收

- 增加 `GET /api/v1/locations/:id/destinations`，沿用登录和调度角色权限、分页及统一错误响应。
- 查询全部启用目的地后按道路距离数值升序分页；来源、停用、删除地点排除。未知距离置后，禁止以直线距离冒充道路距离。
- 天地图驾车 `style=1`（最短路线）；方向分别缓存，WGS84 输入，内部统一米。密钥只保存在后端环境变量。
- 初次选择来源按需登记该来源到各目的地的计算任务；数据库缓存立即返回，后台限速补算。每天北京时间 02:00 更新已使用路线，停机后启动补算到期任务。新增地点在下次选择来源时加入，坐标变化立即失效并重新计算。
- 外部失败保留同坐标的旧距离并标记待更新；无旧值显示暂不可用。失败退避、每日调用预算与数据库锁避免重试风暴及多实例重复计算。
- 不改变调度创建、库存流转、审计与地图交互。

## 模块与契约

- `internal/routing`：天地图客户端、目的地查询、缓存与后台任务；`internal/config`、`cmd/server` 装配。
- 新增 Goose 00004：有向路线缓存与每日调用计数；不改已有迁移，不自动执行迁移。
- 目的地响应 `items/total/page/page_size/pending_count`；每项 `id/name/address/loc_type/distance_m/route_status/calculated_at`，ID 为字符串，未知距离为 null。
- `route_status`：ready、stale、pending、unavailable；返回成功查询中的局部路线状态，数据库/参数/鉴权错误仍走标准错误接口。
- 配置 `TIANDITU_SERVER_KEY`、`ROUTE_REFRESH_HOUR`（2）、`ROUTE_DAILY_LIMIT`（1000，项目预算，非天地图账户配额承诺）。

## 外部核实

- 官方参考：https://lbs.tianditu.gov.cn/server/drive.html 。2026-11-12 文档站返回 WAF 拦截，当前无法核实账户具体日配额。
- 同日对官方 HTTPS `/drive` 进行一次接口兼容检查，现有浏览器 Key 返回 HTTP 403、301012，说明需要服务端类型 Key；未更改或输出 Key。
- 用户提供服务端 Key 后，仅写入被忽略且 0600 的本地 `.env`。官方 HTTPS `/drive` 实测成功：`style=1`，顶层 distance=33.6，各路段 streetDistance 合计 33596 米，验证公里到米的换算；不保存密钥或完整响应到测试夹具。
- 仅保存距离与更新时间，不保存路网、路线几何或完整响应。此距离是普通驾车规划结果，不包含货车车型、载重限行约束。

## RED / GREEN / REFACTOR

- RED：`/tmp/route-backend-red.log` 确认目的地接口 404；`/tmp/route-migration-red.log` 确认仅 11 表而非 13 表；`/tmp/route-unit-red.log`、`/tmp/route-worker-red.log` 确认客户端、配置与任务模块尚不存在。
- GREEN：`make test` 通过；`/tmp/route-integration-green.log` 迁移上下行、缓存排序/分页、坐标失效、方向隔离、失败保留旧值、预算和 HTTP 端点测试通过。
- REFACTOR：HTTP 装配与 `routing` 仓储/服务/供应商客户端分离；后台按数据库到期时间统一处理每日刷新、首次补算与失败重试。增加算路期间改坐标、多实例互斥、停机补算测试。外部拒绝退避发现当前失败路线仅退避 15 分钟，先以 `/tmp/route-denial-red.log` 复现，再统一修复为 1 小时。

## 最终验证

`make check` 完整通过（`/tmp/route-backend-check-final.log`，普通及竞态测试、迁移上下行与重连、路线任务和全量集成测试、生产构建）。开发库迁移前备份 `.cache/backups/before-route-distance-20261112-170226.dump`（0600），手工 `bin/migrate up` 新增1版迁移后通过真实供应商检查（`/tmp/route-live-provider.log`）：模拟婺城仓库到其余14点全部取得距离，正确排序，最近9830米。

新后端已在原8080启动，readyz正常，匿名访问新接口401。2026-11-13只读核实14条路线已在02:14自动补算，下一次到期时间为2026-11-14 02:00；每日调用计数12日14次、13日14次。真实浏览器独立测试库地图发车验收通过（`/tmp/route-gold-live.log`），调度/库存/审计未受影响。

## 兼容与回滚

新增接口和表，旧接口兼容。无服务端 Key 时仍能选择目的地，距离显示暂不可用；不发外部请求。停用刷新任务或回退应用即可停止计算；确认不需要缓存后才能手工回退 00004。生产与开发迁移前备份，测试仅使用 `*_test` 数据库。
