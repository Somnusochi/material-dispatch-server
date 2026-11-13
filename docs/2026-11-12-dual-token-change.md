# 双 token 登录与自动续签

## 背景和当前行为

用户要求短期 access token、长期 refresh token，改善反复登录体验，并明确 access 默认 **24 小时**。当前仅有 8 小时 session Cookie，失效后必须重新输入密码；普通错误与认证失效需分开处理。

## 预期行为及验收

- 登录签发独立随机 access（24 小时）与 refresh（7 天绝对有效期）token，仅通过 HttpOnly、SameSite=Strict Cookie 返回，生产 Secure；响应只包含用户、CSRF 和有效期。
- access 用于业务和私有图片；refresh 仅用于 `/api/v1/auth/refresh`，每次成功轮换。refresh 不得访问业务，access 不得续签。
- refresh 请求必须携带可信 Origin 与 `X-Refresh-Request: 1`。业务写操作仍需绑定会话的 CSRF，轮换时 CSRF 稳定。
- 数据库只存 token SHA-256 哈希；已使用 refresh 再次使用则撤销整个会话。退出、改密、重置、禁用及角色变更撤销关联 token。
- 前端收到认证 401 后合并续签，成功最多重试一次原请求；多标签页通过 Web Locks 串行并先检查是否已被其他标签续签。续签 401 才回登录；网络/服务失败保留登录状态并统一 toast。
- 初次未登录静默；密码错误、403、业务错误、网络错误仍走统一普通错误处理。移除服务状态卡片和额外探测，表单失败保留输入。

## 模块和契约

`internal/auth`、`internal/httpapi`、迁移 00003、集成测试；前端 request/business/AuthGate/provider 与页面统一错误展示。session 响应增加 `access_expires_at`、`refresh_expires_at`；增加 POST `/auth/refresh`。access Cookie `material_dispatch_session` 路径 `/api/v1`，refresh Cookie `material_dispatch_refresh` 路径 `/api/v1/auth`。响应及认证接口禁止缓存。

迁移新增 `session_families`（用户、稳定 CSRF、绝对到期）和 `refresh_tokens`（历史哈希及使用状态），sessions 增加可空 family_id 外键。新认证只接受具备有效 family 的 access；旧会话需重新登录一次。业务表及现有账号不改动。后端不自动迁移。

## TDD 证据

- RED：`make test-integration`（/tmp/material-dual-token-red.log）显示缺少第二Cookie、24h/7d到期字段、迁移表11张实际9张等预期失败；原业务/Origin/图片仍通过。
- GREEN：新增00003和双token服务、HTTP接口后，`make test-integration` 全部通过（/tmp/material-dual-token-green.log）。覆盖过期恢复、轮换、重用撤销、7天绝对到期、token不可互用、可信Origin/header、退出/改密/禁用/改角色/重置撤销。
- REFACTOR：issueTokens统一生成；用户→family固定锁序，与密码/角色操作同序；重用撤销先提交事务再返回401，避免回滚撤销。保留原业务和图片鉴权。

## 最终验证

- `make check` 已全部通过（/tmp/material-dual-token-check.log）：格式、vet、全包编译、race普通测试、独立测试库全量迁移/业务/鉴权集成测试、server/migrate/admin生产构建。
- 本地显式 `bin/migrate up` 已应用00003，版本1/2/3均applied；11张表，现有admin仍启用且角色admin。
- 已重启本地8080后端，现有管理员真实请求验证24h/7d Cookie、无access时401、refresh恢复、me成功、退出后refresh401；没有改动账号密码或业务记录。
- 备份说明：前置pg_dump尝试因连接参数格式不兼容失败，后续追加迁移已应用（仅新增认证表/列，没有改写业务数据）。已修正参数并完成、校验迁移后备份 `.cache/after-dual-token-2026-11-12.dump`（0600，Git忽略）；该文件不是迁移前备份。
- 前端 `pnpm check` 已通过：44项单元、59项开发浏览器、59项生产预览浏览器，格式/lint/类型/生产构建全部通过（/tmp/material-dual-token-web-check.log）。`pnpm test:integration` 3/3通过（/tmp/material-dual-token-live.log）：真实双Cookie期限/删除access后自动续签/轮换/退出撤销，地点→库存→发车→签收→损毁修复→审计，图片上报/报废/取消调度/用户维护。仅使用material_dispatch_test库临时账号。

## 兼容、风险与回滚

access 最长 24 小时，family 到期优先；refresh 轮换不延长最初 7 天。业务操作仍实时检查账户与会话，撤销即时生效。Cookie 名保留以兼容私有图片，但本次升级旧会话需登录一次。跨标签续签依赖安全上下文 Web Locks（本地 localhost/127.0.0.1、生产 HTTPS）；旧浏览器单页内仍合并，遇到真实重放需重新登录。

本地应用前先备份、仅显式 up；测试只使用 *_test 库。回滚程序与 00003 必须一起进行，会清除新会话记录、用户重新登录，业务数据保留；不得在生产盲目执行 down。

设计参考：[OWASP CSRF Prevention](https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html) 的自定义 header 与来源校验，以及 [RFC 9700](https://www.rfc-editor.org/rfc/rfc9700.html) 的 refresh 轮换及重用撤销思想；本系统不是 OAuth 服务。
