# 通用物料规格录入

## 背景与当前行为

用户指出当前物料录入偏向配重块，要求只保留名称、单位和规格图片。现有接口要求规格编码，缺省分类为counterweight、单位为块；更新时缺失的旧字段会被默认值覆盖。

## 预期行为与验收

- 名称和单位必填，图片可选。仅传name、unit及可选image_url即可创建任意物料。
- 未提供spec_code时由服务端生成唯一内部编码；新建不再默认配重块分类，兼容分类默认general。
- 单重、尺寸、分类、备注、手工编码作为兼容字段保留但不要求前端录入。已有ID和库存关联不改变，不迁移或清空历史数据。
- 更新时，未提供的兼容字段和图片保留原值；显式image_url空字符串可移除图片。仍校验已提供字段范围及上传图片引用。
- 旧客户端完整请求继续可用；名称和单位空白应返回统一400，不偷偷填入“块”。修改与审计同一事务。

## 影响范围

catalog.SpecInput、SaveSpec、repository SQL、API文档和集成测试。使用已有数据库结构，无新依赖或迁移。前端为同名变更文档。

## RED / GREEN / REFACTOR

- RED：`go test -race -tags=integration ./tests/integration -run '^TestSimpleSpec' -count=1`失败：最小创建和更新均返回400“规格编码不能为空”；缺失单位反而返回201并默认块。验证了编码强制和单位默认值问题，日志`/tmp/material-spec-simple-server-red.log`。
- GREEN：相同命令通过3项集成测试（含6个无效输入子用例）。仅name/unit可创建、图片可选、编码不重复、旧字段和图片省略时保留、显式空图片可移除。
- REFACTOR：参数验证留在service，字段保留由参数化SQL处理，保持事务审计。

## 最终验证

- `make check`通过：gofmt、go vet、全包编译、全量竞态测试、迁移及真实PostgreSQL集成测试、三个生产可执行文件构建。日志`/tmp/material-spec-simple-server-check.log`。
- 本地开发API已用本次构建重启，8080健康检查成功；没有迁移或重新写入模拟数据。
- 只读核对开发库`material_dispatch`：`DEMO-JH-100T-CW`原编码、单位t、单重1000保留，15个库存地点，完好100、损毁0、在途0，模拟批次审计仍为1条。
- 前端`pnpm test:integration`真实浏览器联调4项全部通过：最小字段新建物料后完成库存、发车、签收、损毁处置与审计；15地点100t地图和双token会话回归通过。使用独立`material_dispatch_test`与8081临时服务，日志`/tmp/material-spec-simple-live.log`。

## 兼容与回滚

保留原字段与编码，不改现有库存、调度、损毁外键。新旧版本均可读取新记录；旧版本表单仍会展示兼容默认值。回滚服务和前端无需回滚数据库。新契约要求显式单位，旧调用方若依赖默认块需补传unit。
