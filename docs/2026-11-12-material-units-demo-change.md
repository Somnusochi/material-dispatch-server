# 物料单位与金华15地点模拟数据

## 背景与当前行为

用户要求物料有单位，并以100t配重块在15个地点生成模拟数据测试。规格、库存、调度和损毁接口已有unit字段，但地图直接跨规格求和、未返回单位分组。当前本地material_dispatch库地点、规格、库存、调度均为0条。

## 预期与验收

- 地图新增stock_by_unit数组，每项包含unit、normal_qty、damaged_qty、in_transit_qty；按地点和单位分别汇总，支持spec_id筛选，无库存返回空数组。
- 保留原三个数量字段以兼容旧客户端；新前端只使用带单位分组，不将t与块直接相加，不隐含单位换算。
- 显式执行本地模拟数据脚本：1个配重块规格、单位t、每单位1000kg；15个启用地点覆盖金华9区县市，完好总量100t，损毁和在途为0。地点名称及备注明确标注模拟，坐标为已有WGS84边界中的人工模拟点，不表示真实仓库。
- 使用DEMO-JH-100T编码前缀，与现有数据隔离；整批写入与审计处于同一事务。重复执行不重复创建，也不覆盖已经发生调拨或调整的库存。
- 自动化集成测试仅使用独立*_test库，验证15点与100t、调拨数量守恒、单位和重复执行保护；验证后才显式写入本地开发库。

## 影响范围

catalog地图SQL与API文档；scripts/seed-jinhua-demo.sql；集成测试及开发说明。无数据库迁移，不改变现有整数数量精度、认证与授权。

## RED / GREEN / REFACTOR

- RED：make test-integration确认TestMapUnitTotals因缺少stock_by_unit失败，TestJinhuaDemoSeed因脚本不存在失败（/tmp/material-units-server-red.log）。
- GREEN：补充repository按单位聚合及原子、可重放的显式脚本；make test-integration全部通过，包括新单位分组、15点100t、3t调拨守恒、重放不覆盖库存的用例（/tmp/material-units-server-green.log）。
- REFACTOR：保持查询在repository，模拟数据只通过显式脚本写入，不随服务启动或迁移运行。
- 检查现有CI配置时发现PostgreSQL以容器运行；测试库连接在服务端看到的是网桥地址。补充隔离事务测试覆盖测试库网桥地址、本地开发库与其他数据库保护，先验证现有地址限制的预期失败，再兼容专用测试库。
- 容器兼容RED：TestDemoSeedDatabaseGuard/container_test明确失败“allowed database was blocked”（/tmp/material-units-guard-red.log）。仅放行专用material_dispatch_test的容器网络，开发库仍要求本机地址，其他数据库仍拒绝；所有环境探测替身均在测试库回滚事务内，不修改真实连接配置。
- 容器兼容GREEN：独立执行`go test -race -tags=integration ./tests/integration -run '^TestDemoSeedDatabaseGuard$' -count=1`通过，四种环境的允许/拒绝结果均正确（/tmp/material-units-guard-green.log）。这验证了地址分支，不代表已在线运行GitHub CI。

## 验证与落库记录

- 已校验15个模拟坐标都位于对应区县WGS84边界内，覆盖9区县市。
- 本地material_dispatch显式落库完成：1个配重规格，15地点，完好100t、损毁0t、在途0t；已有业务数据未被覆盖。数据带DEMO-JH-100T前缀，写入demo/seed审计。
- 最终`make check`退出码0：Go格式、vet、全包编译、普通测试与竞态检查、强制独立库集成测试、server/migrate/admin生产构建全部通过（/tmp/material-units-server-verified-check.log）。
- 前端`pnpm check`全部通过，另有`pnpm test:integration`真实接口4/4通过，验证15点100t地图、库存调拨与签收、损毁处置、图片和用户维护。联调完成后停止8081测试服务并清理临时登录文件，保留正常8080开发服务。
- 最终只读核对本地material_dispatch：15地点、9区县市、单位t、完好100、损毁0、在途0、整批seed审计1条。测试库中的3t调拨不影响本地开发库初始数据。

| 地区 | 地点数 | 完好库存 |
| ---- | -----: | -------: |
| 婺城 |      2 |      20t |
| 金东 |      2 |      17t |
| 兰溪 |      2 |      14t |
| 义乌 |      2 |      14t |
| 东阳 |      2 |      12t |
| 永康 |      2 |      10t |
| 浦江 |      1 |       5t |
| 武义 |      1 |       4t |
| 磐安 |      1 |       4t |
| 合计 |     15 |     100t |

## 兼容、风险与回滚

原地图字段保留；前后端一起更新即可。模拟数据只允许本地material_dispatch或专用material_dispatch_test数据库（测试库兼容CI容器网络），拒绝其他库。已有数据不删除、不重置；需清理模拟数据时按编码识别，通过盘点归零与地点停用保留审计，不以回滚迁移删除业务数据。未创建默认账号或口令。
