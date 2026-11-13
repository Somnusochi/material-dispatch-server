# 本地登录来源校验修复

## 背景与当前行为

管理员账号已创建且启用，使用 `http://localhost:5176` 登录时却提示“没有此操作权限”。后端默认 `APP_ORIGIN=http://127.0.0.1:5176`，登录前来源校验只接受完全相同的 Origin，把两个常用本机入口当作不同的、不受信任的来源。使用不存在账号探针时，127.0.0.1来源进入认证并返回401，localhost来源直接返回403，说明失败位于来源校验而非管理员角色。

## 预期与验收

- 保留 `APP_ORIGIN` 作为明确可信来源。仅 `APP_ENV=development` 且未使用安全Cookie时，配置主机为 `127.0.0.1` 或 `localhost`，额外接受同协议、同端口的另一种精确拼写。
- 生产/测试模式及安全Cookie模式不增加别名；开发模式配置非loopback域名时不自动扩展。
- 错误端口、协议、`localhost.evil`、外部主机、`null`及含路径/查询的来源仍拒绝；不以请求Host推断信任，不增加宽泛CORS。
- 来源拒绝返回403 `ORIGIN_NOT_ALLOWED` 与“当前访问地址未获允许，请使用配置的系统地址”；角色或CSRF拒绝继续使用已有权限错误。
- 无Origin的非浏览器客户端行为不变；两个本机入口登录后均必须通过会话及CSRF校验，退出可撤销会话。
- 不重置账号、密码、角色、库或业务数据；无需数据库迁移。

## 影响范围

后端HTTP入口可信Origin构建、`cmd/server`环境装配、HTTP单元与独立测试库集成测试、README及架构说明。Cookie仍为HttpOnly/SameSite=Strict，生产Secure，前端继续同源代理API。

## RED / GREEN / REFACTOR

- RED：`go test ./internal/httpapi -run TestLoginOriginPolicy -count=1` 实际失败，开发双向别名预期400（通过来源后停在无效JSON）却得到403；其他拒绝分支仍返回误导性的 `FORBIDDEN`。
- RED：加载忽略提交的 `.env` 后，`go test -tags=integration ./tests/integration -run TestDevelopmentLoginOriginsRequireCSRF -count=1` 在独立 `_test` 库实际失败：localhost登录预期200却返回403；非法来源预期 `ORIGIN_NOT_ALLOWED` 却返回 `FORBIDDEN`。
- GREEN：上述两个定向命令均通过（HTTP单测18个场景，集成测试包含两个来源真实登录、会话读取、缺失/错误CSRF拒绝、非法来源拒绝、有效退出及会话撤销）。`Options.Development` 在首轮编译测试后仅增加类型字段以便运行行为RED；来源白名单实现与服务器环境装配均在上述RED之后编写。
- REFACTOR：来源策略独立为 `trustedOrigins`，路由创建时构建一次精确白名单；无数据库或认证服务改动。

## 最终验证

- `make check`：通过，退出码0。包括gofmt、go vet、全包编译、全量普通测试及竞态、独立 `_test` 库迁移/全部业务集成测试及竞态、server/migrate/admin生产构建。集成测试总耗时57.394秒。
- 双入口集成测试额外验证缺失Origin时仍不能绕过CSRF；原有权限、库存并发防超卖、审计、私有图片、严格JSON检查全部通过。
- 独立只读复核通过，未发现开发来源放宽影响生产、请求Host信任或CSRF绕过的问题。
- 主任务已重启8080 API与5176前端，以现有管理员在无头浏览器分别访问 `http://localhost:5176/` 与 `http://127.0.0.1:5176/`：登录200、会话读取200、管理员角色正确、用户管理200、退出200、退出后会话读取401。只关闭验证会话，未修改账号、密码或角色。

## 兼容、风险与回滚

开发别名白名单仅来自已经配置的本机Origin，匹配完整字符串，端口和协议不扩展。生产默认精确来源策略不变。回滚HTTP来源构建与环境装配可恢复旧行为，使用配置中的完整地址仍可登录；不涉及库迁移。
