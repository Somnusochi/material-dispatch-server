# HTTP API

基址 `/api/v1`。成功 `{ "data": ..., "request_id": "..." }`；失败 `{ "error": { "code": "...", "message": "..." }, "request_id": "..." }`。创建返回201，查询/修改/状态动作返回200。

探针 `/healthz`、`/readyz`、`/api/v1/system/status` 无身份数据，可匿名访问。其他业务接口须登录；业务 POST/PUT 须带 `X-CSRF-Token`，从 login/me/refresh 返回取得。refresh 不依赖access/CSRF，必须带可信 `Origin` 和 `X-Refresh-Request: 1`。浏览器同源 Cookie 自动携带，不使用 localStorage token。

## 接口与权限

| 方法 / 路径                                            | 内容                                                                                 | 权限           |
| ------------------------------------------------------ | ------------------------------------------------------------------------------------ | -------------- |
| POST /auth/login                                       | username、password，返回会话及到期时间，设置双token Cookie                           | 匿名           |
| POST /auth/refresh                                     | 消费并轮换refresh Cookie，返回新会话和Cookie；非法/过期/重用401                      | 有效refresh    |
| GET /auth/me                                           | 当前用户和 CSRF token                                                                | 登录           |
| POST /auth/logout                                      | 撤销当前会话                                                                         | 登录           |
| POST /auth/password                                    | current_password、new_password，撤销全部会话                                         | 登录           |
| GET /contacts                                          | 启用用户的 id/name，用于选择负责人                                                   | 登录           |
| GET /users                                             | 用户分页，不含口令哈希                                                               | 管理员         |
| POST /users                                            | username/password/real_name/phone/role/status                                        | 管理员         |
| PUT /users/:id                                         | real_name/phone/role/status，password可选重置                                        | 管理员         |
| GET /locations                                         | 地点分页                                                                             | 登录           |
| GET /locations/:id/destinations                         | 按来源到目的地的最短驾车距离升序分页，未知置后                                          | 管理员、调度员 |
| POST /locations、PUT /locations/:id                    | name/code/loc_type/address/longitude/latitude/manager_id/contact_phone/status/remark | 管理员、调度员 |
| GET /material-specs                                    | 规格分页                                                                             | 登录           |
| POST /material-specs、PUT /material-specs/:id          | name/unit必填，image_url可选；兼容旧属性，详见下方约定                               | 管理员、调度员 |
| GET /map/locations                                     | 启用地点、物料预览与完整种类数；可筛spec_id，保留旧数量字段以兼容                    | 登录           |
| GET /map/locations/:id/materials                       | 指定启用地点的逐项库存，支持分页、keyword、spec_id                                   | 登录           |
| GET /map/summary                                       | 全量统计地点数、各状态物料种类；可按spec_id和location_id精确筛选                    | 登录           |
| GET /inventory                                         | 库存分页                                                                             | 登录           |
| POST /inventory/adjust                                 | location_id/spec_id/normal_qty/damaged_qty/version/reason                            | 管理员         |
| GET /dispatches、GET /dispatches/:id                   | 调度分页和详情                                                                       | 登录           |
| POST /dispatches                                       | from_location_id/to_location_id/spec_id/dispatch_qty/priority/remark                 | 管理员、调度员 |
| POST /dispatches/:id/depart                            | 执行调度，空JSON对象；driver_name/driver_phone/vehicle_plate已移除，提交返回400                | 管理员、调度员 |
| POST /dispatches/:id/cancel                            | 空JSON对象，只允许待调度                                                             | 管理员、调度员 |
| POST /dispatches/:id/receive                           | normal_qty/damaged_qty/description/image_urls                                        | 登录           |
| GET /damage-reports、GET /damage-reports/:id           | 损毁分页和详情                                                                       | 登录           |
| POST /damage-reports                                   | location_id/spec_id/damaged_qty/description，image_urls可选                           | 登录           |
| POST /damage-reports/:id/{confirm,reject,repair,scrap} | remark                                                                               | 管理员、调度员 |
| POST /uploads                                          | multipart 字段 file，返回 id/url/name/content_type/size                              | 登录           |
| GET /uploads/:id                                       | 私有图片，带正确 MIME 和 nosniff                                                     | 登录           |
| GET /operation-logs                                    | 审计分页、before_data/after_data                                                     | 管理员         |

## 参数约定

- ID 为正整数的十进制字符串；经纬度为 WGS84 数值。地点输入必须包含 longitude/latitude。
- role：admin / dispatcher / inspector；status：0 停用、1 启用。
- loc_type：warehouse / site / yard；priority：1 紧急、2 普通、3 低。
- dispatch status：pending / in_transit / completed / cancelled。
- damage status：pending / confirmed / repaired / scrapped / rejected。
- 损毁上报不要求等级，界面不展示等级；`damage_level` 仅兼容旧客户端（minor / moderate / scrapped）。省略、null或空串使用既有数据库兼容值moderate，不代表新的人工评级，也不参与库存和处置规则；读取继续保留旧字段，历史记录不变。显式非法等级仍返回400。
- 分页：page 默认1、page_size默认20且1–100；返回 items/total/page/page_size。keyword 搜索名称/编码或单号。调度、损毁支持 status/location_id/spec_id；库存支持 location_id/spec_id；地点支持 status；审计支持 module/action。
- 更新为表单 PUT，不提供通用 PATCH。物料规格仅要求name、unit；可选兼容字段省略时保留旧值。清空 manager_id 用空字符串；不提供硬删除历史数据接口。
- 物料规格：name（最多128字）和unit（最多16字）去除首尾空白后不能为空，不默认单位。最小请求`{"name":"钢管","unit":"根"}`；可选image_url为上传引用，省略时编辑保留旧图片，传空字符串移除图片。
- 规格兼容字段spec_code/category/unit_weight_kg/dimensions/remark均可省略。新建时自动生成MAT-前缀随机编码，分类默认general、单重默认0，尺寸/备注默认空；更新时省略或null保留原值。旧客户端可继续显式传入这些字段，校验原范围；响应仍保留旧字段。
- 图片仅接受上传服务生成的稳定引用。损毁最多6张，规格1张（前端限制）；签收存在损毁时 description 必填。
- 数量上限100000000，非负或正数按字段业务规则；单重0–99999999.99 kg。
- 时间返回 ISO timestamptz，客户端以 Asia/Shanghai 展示。

错误：400 VALIDATION_ERROR、401 UNAUTHORIZED/INVALID_CREDENTIALS、403 FORBIDDEN/ORIGIN_NOT_ALLOWED、404 NOT_FOUND、409 CONFLICT、429 RATE_LIMITED、500 INTERNAL_ERROR。`ORIGIN_NOT_ALLOWED` 表示浏览器访问来源未获允许，提示“当前访问地址未获允许，请使用配置的系统地址”；`FORBIDDEN` 保留角色或CSRF拒绝语义。失败响应不泄露数据库详情，调试按 request_id 查服务日志。

## 地图统计

`GET /map/locations`每地点返回`materials`（前3条）和`material_count`（完整物料种类数）。每项包含字符串`id/spec_id`、`spec_name/unit`及`normal_qty/damaged_qty/in_transit_qty`。仅包含任一状态有正数量、未删除规格的库存，按库存记录ID降序；无库存的启用地点返回空数组和0。选定规格时预览仅含该规格。不得把同单位的不同物料相加展示。

`GET /map/locations/:id/materials?page=1&page_size=3`返回标准`items/total/page/page_size`，与预览同口径、同顺序，可浏览超过100种物料。可选`keyword`按物料名称字面包含匹配，`spec_id`筛选具体规格；分页1–100，默认20。无效参数400；停用、删除或不存在地点404；未登录401。只读查询不改库存。地点还提供`normal_spec_count/damaged_spec_count/in_transit_spec_count`，各自按该状态正库存的规格ID计数；这些计数可重叠，表示含相应库存的种类，并不表示该规格全部损毁或全部在途。旧`stock_by_unit`与裸数量保留兼容。

`GET /map/summary`接受可选`spec_id`、`location_id`及`keyword`（地点名称/地址包含搜索，最多255字，大小写和`%`、`_`按字面匹配）。条件取交集，location_id按ID精确匹配，避免合并同名地点；非法地点ID返回400，不存在/停用/删除地点返回404。响应为`location_count`、`normal_spec_count`、`damaged_spec_count`、`in_transit_spec_count`、`normal_qty`、`damaged_qty`、`in_transit_qty`、`unit`。

未选规格时，地点数含启用空地点；物料种类按规格ID跨地点去重，只有对应状态数量大于0才计入；三个`*_qty`及`unit`为null，禁止跨物料混加。选定规格后返回该物料的三类数量和单位，地点数仅计任一状态有正库存的地点；无库存返回0及规格单位。统计排除停用/删除地点、删除规格，不受分页限制；无效ID返回400，不存在或已删除规格返回404。`GET /map/locations?spec_id=...`同样只返回该物料有库存的地点。

## 调配目的地距离

`GET /locations/:id/destinations?page=1&page_size=50&keyword=`。来源必须是启用且未删除地点，否则 404；参数格式错误 400、未登录 401、无调度权限 403。排除来源自身及停用/删除目的地；关键词对名称、地址按大小写不敏感字面包含匹配。按 `distance_m ASC NULLS LAST, id ASC` 排序后分页，范围同普通列表；距离按有向道路分别计算。

返回 `items/total/page/page_size/pending_count`，`pending_count` 为当前关键词下所有待计算目的地数量。每项包括 `id/name/address/loc_type/distance_m/calculated_at/route_status`，未知距离与时间为 null。`ready` 为有效缓存，`stale` 为到期或刷新失败但可参考的旧距离，`pending` 为后台补算中，`unavailable` 为当前未配置/失败/预算耗尽而无可用结果。局部无路线不会阻止调度；数据库和整体请求错误仍走统一错误 envelope。距离不参与调度写入校验，发车库存事务保持不变。

## 双 token 会话约定

login/me/refresh成功响应data包含user、csrf_token、access_expires_at和refresh_expires_at。期限为24小时/7天；轮换不延长最初7天，access最长不超过family到期。原始token不在JSON返回，只保存在HttpOnly、SameSite=Strict、生产Secure的Cookie：access名material_dispatch_session（Path=/api/v1），refresh名material_dispatch_refresh（Path=/api/v1/auth）。数据库只存token哈希；CSRF独立随机且随family稳定。角色和账户状态每次业务请求验证。

protected401先自动续签，成功后只重试一次；刷新401清理Cookie并回登录，网络/500不清理。登录INVALID_CREDENTIALS直接走普通错误toast。并发刷新由前端单次Promise及跨标签Web Locks合并。退出、密码变更/重置、禁用/角色变更立即撤销访问和续签；旧refresh重用撤销整个会话并同事务记录审计。

00003升级后旧8小时单token会话需重新登录一次；已有账号口令、业务和图片均保留。

### JSON 请求对象

业务 JSON 写请求只能包含单个对象（最大 1 MiB），不能发送根 `null`、数组、标量、多个值或未知字段。无字段的执行/取消调度动作发送 `{}`；非法请求返回 400 `VALIDATION_ERROR`，不改变单据、库存或审计。
