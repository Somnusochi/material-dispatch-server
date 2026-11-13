# 工程架构

## 范围

依据用户提供 PDF《物料地图调度系统工程命名方案》和追加的金华天地图搜索取点要求，当前实现用户、地点、物料规格、库存、调度、损毁图片和审计模块。按地点和规格汇总数量，不引入独立物料序列号、车载轨迹或第三方供应商系统。

## 工具链与构建

Go 1.27.4为当前构建基线，go.mod统一声明版本；Make据此设置精确的GOTOOLCHAIN，编译、测试和GOROOT内的gofmt使用同一工具链，缓存隔离在项目.cache中。CI从go.mod安装相同版本并执行make check，不依赖机器PATH中gofmt的版本。业务JSON继续使用encoding/json的原有API及兼容语义，采用Go 1.27默认实现，不设置nojsonv2。

## 分层

```text
cmd/server                 配置、依赖装配、启动与优雅停止
cmd/migrate                显式 up/down/status
cmd/admin                  受信任本地管理员创建命令
internal/httpapi           参数、响应、会话/CSRF/角色校验、路由
internal/auth              密码、会话、用户及权限数据
internal/catalog           地点、负责人、规格、地图库存汇总
internal/inventory         盘点、库存锁定与数量更新
internal/dispatch          调度状态与签收事务
internal/damage            损毁审核、修复、报废事务
internal/upload            图片内容校验、私有文件、元数据
internal/audit             同事务业务审计
internal/<domain>/repository.go  固定参数化 SQL
internal/platform/business  错误、分页、事务和 pool/tx 共用 SQL 执行
internal/platform/postgres  连接池与启动 Ping
migrations                 嵌入式 Goose 迁移、数据库会话锁
```

`httpapi/application.go` 只组装依赖和路由；各领域在 `*_handlers.go` 注册路由与处理参数，认证/请求策略在 `middleware.go`，JSON 输入在 `request.go`，分页在 `pagination.go`，业务响应在 `response.go`，登录限流独立。`auth` 按 users/login/session/password 拆文件，`catalog` 按 locations/specs/map/validation 拆文件；同一领域仍使用同一个包。

服务层组织状态规则和事务，repository 保管 SQL；pool 和事务只通过实际需要替换的 Queryer 交互。不使用 ORM 自动迁移。HTTP 请求有超时、请求 ID、恢复和访问日志，日志不包含查询参数、请求体、口令或 Cookie。业务 JSON 请求限制 1 MiB，只允许单个对象；根 `null`、数组/标量、尾随值及未知字段返回统一 400，解析失败不会进入业务服务。允许无字段的动作仍应提交 `{}`。

## 数据与一致性

所有业务表位于 `material_dispatch` schema，迁移元数据固定 `public.goose_db_version`，不依赖 search_path。版本1七张核心表、版本2的sessions/uploads和版本3的session_families/refresh_tokens共十一表；版本4新增两张路线表，当前共十三表；版本5删除调度单三个废弃运输列。不改写已应用版本。所有 bigint API ID 以字符串传输，分页按数值 ID 降序。

| 表                   | 约束和用途                                                   |
| -------------------- | ------------------------------------------------------------ |
| sys_users            | 唯一用户名、bcrypt 哈希、三类角色、启停、登录失败锁定        |
| locations            | 仓库/工地/堆场、WGS84、用户负责人、电话、启停                |
| material_specs       | 名称、单位、参考照片；自动唯一编码，兼容保留分类/单重/尺寸   |
| material_inventories | 地点+规格唯一；完好、损毁、入库在途非负；版本号              |
| dispatch_orders      | 起终点不同、正数量、四种状态、创建/调度/签收人员与时间；无司机车辆字段 |
| damage_reports       | 正损毁量、说明、图片数组、五种状态、审核信息；旧等级仅作兼容 |
| operation_logs       | 只增不改，触发器拒绝 UPDATE/DELETE/TRUNCATE，保留用户快照    |
| sessions             | access token的SHA-256哈希、用户、family及24小时有效期        |
| session_families     | 登录会话、用户、稳定CSRF、7天绝对有效期                      |
| refresh_tokens       | refresh哈希、所属family和使用时间，用于轮换/重用检测         |
| uploads              | 随机 ID、所有者、私有存储路径、验证后的 MIME/大小            |
| route_distances      | 来源+目的地有向唯一，坐标快照、米数、更新时间与下一次计算时间 |
| route_daily_usage    | 北京时间日期唯一，所有实例共享每日调用预算                  |

执行调度时锁调度单和库存行，库存按稳定地点顺序锁定，避免相向调拨死锁。签收和损毁处置均先校验状态，再同事务更新库存、单据、审计。失败整体回滚，重复状态操作返回 409；不做异步 HTTP 日志代替业务审计。

前端统一使用“待调度”“执行调度”，不再采集或展示司机车辆信息。底层仍使用pending/in_transit状态及depart接口，执行请求为`{}`；00005删除司机姓名、电话、车牌三列，服务请求和SQL读写同步移除，携带废弃字段按严格JSON校验返回400。新响应及审计快照不再包含这些字段，已有不可变审计保留原貌。升级需停旧服务、备份并显式迁移后启新服务；下行仅重建可空列，历史列值需从备份恢复。

物料规格最小写入契约为名称和单位，图片可选。新建自动生成内部编码；单重等旧字段无需录入。兼容属性用可选输入区分省略和显式零值/空串，更新省略字段时保留旧值。保留既有数据库列与库存关联，不改写已应用迁移。

独立损毁上报不立即减库存，确认时转入损毁；运输签收损毁已经直接入损毁库存，不能再次确认。盘点调整要求当前版本，新增余额使用版本 0，不能篡改在途量或侵占已确认损毁报告占用的数量。

损毁上报不再要求或展示等级，按损毁数量、说明和照片记录。保留旧数据库列和响应字段；省略等级时沿用moderate兼容值，显式合法旧值保留，均不参与业务决策，不改写历史记录或审计。

## 身份和图片

同源双token：access默认24小时，refresh默认7天绝对到期，均使用HttpOnly / SameSite=Strict Cookie，生产Secure。access用于业务和私有图片，refresh仅用于续签且每次轮换；已使用refresh重用撤销整个family。access到期不能晚于family到期。写接口要求与family绑定且轮换时稳定的CSRF header；refresh接口必须有可信Origin与X-Refresh-Request: 1，不依赖已过期access。前端通过Web Locks串行续签与认证Cookie变更，锁内先GET me避免多标签重复消费refresh。角色由服务器会话查询获得，禁用、改角色、重置或修改密码立即撤销会话。登录按 IP 限流，连续5次错误锁定15分钟；管理员可通过用户维护解除锁定。不会预置管理员或口令。

Origin 默认为精确匹配 `APP_ORIGIN`。仅开发模式且未使用安全Cookie时，为配置中的 `127.0.0.1` / `localhost` 增加同协议、同端口的另一种本机别名；生产、测试、安全Cookie模式及非本机域名不扩展。可信来源仅从配置构建，不信任请求Host，不增加CORS。来源不合法返回403 `ORIGIN_NOT_ALLOWED`，与角色/CSRF错误区分；缺失Origin的非浏览器客户端保留既有处理，写业务接口仍需会话和CSRF。

JPG/PNG/WebP 最大5 MiB，解码验证内容、限制20兆像素，每单最多6张；前端上传完成后保存稳定 `/api/v1/uploads/:id` 引用。读取同样需要登录。图片以随机文件名、0700目录/0600文件保存在 UPLOAD_DIR，数据库写入失败会清理本次文件。文件和库需同时持久化备份；生产按组织的保留期清理未关联图片，不暴露整个目录为公开静态资源。

## 地图

浏览器直接使用天地图浏览器 Key 和官方 v2/search；金华视野为 `[[28.50,119.20],[29.72,120.82]]`，是初始视野包围盒，不是行政边界。搜索结果过滤无效和区域外坐标，支持结果选择、地图点击与拖拽微调；使用 WGS84 数值存储，Leaflet 顺序为纬度、经度。地图从数据库批量读取各地点的逐项物料预览，可筛规格并从地点发起调度；旧单位汇总仅用于兼容。前端不内置模拟点位；本地演示由显式脚本seed-jinhua-demo.sql创建带模拟标识的15地点100t配重块数据，不随启动自动写入。市界与9个区县市边界复用 front-dashboard 的 CTAmap v2.0 WGS84 / CC BY 4.0 数据，县级面合并出整市 MultiPolygon；独立非交互图层叠加于天地图，保留作者、许可和可下载数据。详见前端 public/data/README.md。市界不用于自动判断业务地点归属。

地图统计由`GET /map/summary`在catalog仓储用单次参数化查询全量聚合。全部物料按状态计算`count(distinct spec_id)`（该状态数量>0），不返回跨物料数量总和；选定规格时返回数量和单位。同一规格可同时含完好、损毁和在途库存，状态种类数不互斥。地点计数与地图的规格过滤都采用正库存规则；总览地点数包含空的启用地点。关键词与浏览器的名称/地址包含搜索保持一致，排除停用/删除地点和删除规格，不通过库存分页推导统计。

地点卡片使用`materials/material_count`逐项展示库存，地图查询一次性带上各地点前3条，按库存记录ID降序截取，避免首次载入的每地点查询或无界物料数组。`GET /map/locations/:id/materials`在翻页时按同一有效库存条件分页；全部物料和具体规格都按名称、单位及三类数量逐行表达，旧单位汇总仅作兼容。

已建地点选择复用当前物料下完整地图点位，名称/地址只搜索下拉候选。选定后前端按ID过滤地图和列表，统计接口通过可选location_id按同一地点精确聚合，服务端先验证其为启用且未删除地点。旧keyword过滤保持兼容，location_id与其他筛选取交集。

## 部署与回滚

`internal/routing` 独立承载路线计算。HTTP 请求先校验来源和角色，按需登记该来源到所有启用目的地的路线，按全量距离排序后分页。查询和工作进程都验证坐标快照，算路期间发生坐标修改时丢弃旧结果。Go 后台任务使用数据库 advisory session lock 单实例执行、原子调用预算，外部 HTTP 调用不占数据库事务；进程退出取消 HTTP 并释放连接。成功把下一次计算时间写到下一个北京时间 02:00；失败不清空同坐标旧距离，使用退避时间，重启按到期时间补算。服务端 Key 只在环境中，外部错误不传播可能包含 tk 的 URL/响应。

前端静态文件 + HTTPS 同源反代 + Gin + PostgreSQL；Nginx 模板允许6 MiB请求，以容纳照片及 multipart 开销。开发只用本地 PostgreSQL。生产建议将迁移 owner 与运行时 DML 账户分离，运行时审计表只授予 INSERT/SELECT。

启动不迁移。先备份库和上传目录，再显式运行 bin/migrate up。下行迁移会删除相应版本数据，不能作为已发货或已签收业务的撤销方式。回退程序时确认 schema 兼容；物理流转更正使用明确的盘点或逆向调拨并保留审计。
