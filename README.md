# material-dispatch-server

华正物料地图调度系统后端，Go 1.27.4 + Gin + pgx + PostgreSQL + Goose。提供本地账号、角色权限、地点与规格、库存、调度、损毁照片及不可变操作审计。前端为相邻 `material-dispatch-web`。

损毁上报按地点、规格、损毁数量和说明记录，照片可选，无需填写等级。确认后按数量转入损毁库存；历史等级字段仅保留兼容，不参与处置规则。

## 本机运行

本机已准备专用开发库 `material_dispatch`、测试库 `material_dispatch_test` 和忽略提交的 `.env`。数据库账号没有超级用户、建库或建角色权限。

```sh
make migrate-up
make build
# 创建你明确指定的管理员；替换用户名和姓名，密码通过终端隐藏输入。
python3 -c 'import getpass,sys; sys.stdout.write(getpass.getpass("管理员密码："))' | ./bin/admin --username YOUR_ADMIN --name 管理员姓名 --password-stdin
make dev
```

不会创建默认账号或默认密码。用户名为 3–64 位字母、数字、点、短横线或下划线，密码为 10–72 字节。管理员命令仅用于受信任的本地运维，数据库口令不应交给普通用户。

API 默认 `http://127.0.0.1:8080`，前端 `http://127.0.0.1:5176`。开发模式下，`APP_ORIGIN` 配置为 `127.0.0.1` 或 `localhost` 时，两种本机地址均可使用，但协议和端口必须一致；默认也可访问 `http://localhost:5176`。生产、测试或安全Cookie模式仅接受与 `APP_ORIGIN` 完全一致的来源，自定义非本机域名不会自动增加本机别名。环境变量优先于 `.env`，服务启动不会自动迁移。

## 新机器

运行基线为 Go 1.27.4、PostgreSQL 18。已有 Go 1.21+ 启动器时，Make会按go.mod自动下载并使用指定工具链，无需修改其他项目的全局版本。以有建库权限的本地用户执行：

```sh
createuser --pwprompt --no-superuser --no-createdb --no-createrole material_dispatch
createdb --owner=material_dispatch material_dispatch
createdb --owner=material_dispatch material_dispatch_test
cp .env.example .env
chmod 600 .env
# 填写 .env 的数据库连接口令
make migrate-up
make check
```

Go工具链、模块和构建缓存位于 `.cache/`。go.mod是版本来源，Make从中设置精确的GOTOOLCHAIN，gofmt也取自同一工具链的GOROOT；CI通过go-version-file读取同一版本。`make toolchain`可核对实际版本。所有 Make 命令默认使用 [Goproxy.cn](https://goproxy.cn/) 镜像 `https://goproxy.cn,direct`，保留默认 `sum.golang.org` 完整性校验；项目不改全局 Go 配置。

```sh
make deps
make toolchain
# 镜像不可用或需要官方源时，可覆盖单次命令
make deps GOPROXY=https://proxy.golang.org,direct
# 直接执行 go 命令不读取 Makefile，需要显式指定
GOPROXY=https://goproxy.cn,direct go mod download
```

环境变量 `GOPROXY` 也可覆盖项目默认值。镜像返回404/410时会尝试 direct；网络错误时按上面的命令切换源。

## 配置

| 变量              | 默认 / 要求                                                  |
| ----------------- | ------------------------------------------------------------ |
| APP_ENV           | development / test / production                              |
| HTTP_ADDR         | 127.0.0.1:8080                                               |
| APP_ORIGIN        | http://127.0.0.1:5176；生产必须显式指定 HTTPS origin，无路径 |
| DATABASE_URL      | 必填，PostgreSQL URL；生产按数据库要求启用 TLS               |
| TEST_DATABASE_URL | `make check` 必填，独立且名称以 `_test` 结尾                 |
| DB_MAX_CONNS      | 10                                                           |
| LOG_LEVEL         | info                                                         |
| UPLOAD_DIR        | .data/uploads；业务图片私有存储，需持久化与备份              |
| TIANDITU_SERVER_KEY | 天地图服务端应用 Key，可选；为空时不计算路线，浏览器 Key 不可混用 |
| ROUTE_REFRESH_HOUR | 2；Asia/Shanghai 每日更新已请求的路线，范围 0–23 |
| ROUTE_DAILY_LIMIT | 1000；项目每日外部请求预算，包括失败，不代表天地图账号配额 |

登录使用24小时access token和7天refresh token，HttpOnly Cookie承载、数据库保存哈希；access过期自动续签，refresh轮换且保持7天绝对期限。退出/改密/禁用/角色变更即时撤销。升级迁移00003后旧单token会话需重新登录一次。

生产通过 HTTPS 同源反向代理访问前端与 `/api`；Cookie 在生产为 Secure。不开宽泛 CORS。不要把数据库口令、Cookie、服务器存储密钥放入前端环境变量。

## 命令与验证

目的地距离使用天地图最短驾车路线（`style=1`），由后端缓存并按米数排序。第一次选择来源时登记路线任务，后端串行、间隔 500ms 补算，浏览器每 3 秒读取计算进度；已知距离立即返回。缓存仅覆盖实际请求过的来源，不执行全地点两两扫描。每日北京时间 02:00 刷新已有路线，停机后会补算；机器休眠或服务停止期间不执行。移动地点立即使关联旧距离失效。普通错误退避 15 分钟，鉴权/配额拒绝暂停待算任务 1 小时，达到本项目预算推迟到次日刷新。路线仅是普通驾车距离，不包含货车车型和载重限行。新增 00004 迁移需显式执行，启动不迁移。

| 命令                             | 作用                                                 |
| -------------------------------- | ---------------------------------------------------- |
| make deps                        | 通过镜像下载模块并执行 go mod verify                 |
| make toolchain                   | 打印并校验go.mod指定的实际Go工具链                   |
| make dev                         | 启动 API                                             |
| make fmt / fmt-check             | Go 格式化 / 检查                                     |
| make lint                        | go vet                                               |
| make typecheck                   | 全包编译                                             |
| make test                        | 全量普通测试和竞态检测                               |
| make test-integration            | 串行执行迁移与真实业务库测试、并发防超卖测试         |
| make check                       | 工具链、格式、lint、类型、全量测试、强制集成测试、生产构建 |
| make build                       | bin/server、bin/migrate、bin/admin                   |
| make migrate-up / migrate-status | 显式迁移 / 查看版本                                  |
| make migrate-down                | 回滚一版，可能删除数据；仅在备份和回滚计划明确后使用 |

集成测试会在专用测试库中回滚并重建本项目 schema；不能与其他测试进程共用同一个库。默认 `make test` 没有测试库时明确跳过数据库测试，`make check` 不允许跳过。

真实浏览器业务验收：先运行 `make test-integration`，随后运行 `make test-e2e-server`（仅能连接 `_test` 库，生成随机临时账号并监听 8081）；在前端目录运行 `pnpm test:integration`。测试账号文件 `.cache/e2e-login.json` 不入 Git，服务退出时删除。执行完停止测试服务，再运行会重建 schema 的检查。

## 业务规则

物料录入仅要求名称和单位，规格图片可选；服务端自动生成内部编码，不再要求填写分类、单重或尺寸。旧属性保留兼容，精简表单编辑不会清空历史属性或改变库存关联。接口约定见[API](docs/api.md)。

物料规格的`unit`是库存、调拨与损毁数量的计量单位。地图地点的`materials`逐项返回名称、单位、完好/损毁/在途数量，每地点预览前3条；`material_count`为有库存的完整种类数，`GET /map/locations/:id/materials`提供后续分页。同单位的不同物料也不合并，部分损毁时同一条物料同时展示完好与损毁数量。`stock_by_unit`及裸数量字段仅保留兼容旧客户端。当前数量沿用整数存储，不自动进行kg与t换算。

统计面板使用`GET /map/summary`：全部物料展示地点数与完好/损毁/在途各自去重的物料种类数；选定规格后才返回数量和单位。物料种类按规格ID跨地点去重，不因同单位而合并；总览数量字段为null。支持地点名称/地址字面搜索，统计覆盖全部库存且排除停用/删除地点与删除规格。指定规格时，地图与统计仅包含该物料有库存的地点。

本地演示可显式执行`scripts/seed-jinhua-demo.sql`（使用本机PostgreSQL账号连接`material_dispatch`）。脚本创建金华9区县市内15个带“模拟”标识的地点、1个单位为t的配重块规格和总计100t完好库存；全批次与审计原子写入，重复执行不覆盖已有变更。开发库限定本机`material_dispatch`，专用`material_dispatch_test`允许CI容器连接，拒绝其他库。

```sh
psql -h 127.0.0.1 -U material_dispatch -d material_dispatch -v ON_ERROR_STOP=1 -f scripts/seed-jinhua-demo.sql
```

该脚本不随启动或迁移运行，详情与验收见[单位与模拟数据记录](docs/2026-11-12-material-units-demo-change.md)。

- 管理员管理用户、盘点库存、查审计，并拥有全部业务权限。
- 调度员维护地点/规格、创建和执行调度、签收、审核损毁。
- 巡检员查阅业务数据、签收和上报损毁。
- 保存待调度不改变库存；执行调度原子扣来源完好库存、增目的地入库在途，只允许待调度取消。执行接口接收空JSON对象。司机姓名、司机电话和车牌号已从请求、响应及调度表删除；携带废弃字段返回400。升级前备份并显式执行00005，旧后端需先停止；历史不可变审计保留原始快照。迁移回滚仅恢复列结构，旧值需通过备份恢复，见[字段删除记录](docs/2026-11-13-remove-dispatch-transport-columns-change.md)。
- 一次性签收，完好 + 损毁 = 调拨数量；运输损毁直接生成已确认报告。
- 独立损毁审核时从完好转损毁；修复归还完好，报废扣减损毁。
- 盘点用版本号防止覆盖并发操作，损毁库存不能低于待处置报告量。
- 有库存或未结束调度的地点不能停用。规格保留历史关联，不提供破坏性删除。
- 业务变更与审计同事务，地图坐标为 WGS84，时间存 timestamptz。

详见 [架构](docs/architecture.md)、[API](docs/api.md)、[完整开发与验证记录](docs/2026-11-11-complete-system-change.md)。

## 本地代码索引

GitNexus 1.6.12 已安装，可运行 `gitnexus analyze --index-only --workers 2` 更新索引，再用 `context` / `impact` / `query` 核对调用关系。`.gitnexus/` 是忽略的本地缓存，`--index-only` 不改写开发规范。动态注册和 SQL 的行为仍以源码及集成测试为准。
