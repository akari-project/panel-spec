<!-- SPDX-License-Identifier: Apache-2.0 -->
# 变更记录

格式遵循 spec/42 42.5：每个版本分为新增、变更、修复、安全四部分。1.0 之前的小版本允许破坏性变更，但必须在此说明迁移方法（ENG-04）。

## v0.6.0（M1-01 后续：运营模块开关 features 的语义）

相对 v0.5.0。依据 workspace spec/03 3.6、spec/13 OPS-08、spec/30 API-11，以及评审记录 `review/features-spec-debate-2026-09-25.md`。含语义变更（`features` 返回有效值、更新按键合并并拒绝开启未实现的模块），按 ENG-04 升小版本。最低契约版本为 v0.6.0。

### 新增
- 管理接口 `Settings.features`（响应）列出 `required`：五个模块键必有，与客户端接口 `SignedConfig.features` 一致。

### 变更
- 管理接口 `Settings.features` 与客户端接口 `SignedConfig.features` 返回有效值：存储为 `true` 且本版本控制面已实现该模块才为 `true`，缺省全部为 `false`（OPS-08）。存储中残留的 `true` 保留，升级到实现该模块的版本后恢复生效。
- `SettingsUpdate.features` 按键合并，只修改提交的模块（与 `min_version` 的整体替换不同）；`null`、非布尔值、未知模块名返回 400 `invalid_request`（`invalid_format`，`field` 如 `features.support`）；开启未实现的模块返回 400 `invalid_request`（`not_allowed`），提交 `false` 始终允许（CONV-16）。迁移：设置管理接口尚未实现（M1-09），无已有调用方；后台只提交要修改的模块，并按构建时的模块清单置灰未实现的模块。
- `SignedConfig.payload.issued_at`：只有 `features`、`registration_policy`、`min_version` 的有效值实际变化时更新，严格递增（`max(当前时刻, 上一次的值 + 1 秒)`，秒精度，API-11）；客户端仍按“不早于”防回滚，同一文档重复获取必须接受。

### 修复
- 管理接口 `getSettings`、`updateSettings` 的示例中 `features` 改为全部 `false`，与缺省值及 M1 的行为一致（此前示例开启了尚未实现的模块）。
- `features` 的描述写明是 CONV-10 布尔前缀的例外（管理接口此前只在客户端接口中说明）。
- 两份 OpenAPI 的 `info.version` 改为 0.6.0。

### 安全
- `issued_at` 严格递增：此前同一秒内的两次修改会得到相同的 `issued_at`，按“不早于”规则客户端会接受被替换的旧签名文档；副本时钟偏差也可能写入更早的值，使客户端长期拒绝新文档。

能否在线执行：可以，不涉及 proto 与数据库。控制面与管理后台、用户中心嵌入同一二进制，同时升级；回滚到旧二进制时，新版本开启的模块若旧版本未实现，按有效值规则下发 `false`。

## v0.5.0（M1-01 后续：重新验证范围与 /v1/config 的生成规则）

相对 v0.4.0。依据 workspace spec/10 AUTH-11、AUTH-23，spec/30 API-03、API-11，spec/02 CONV-30，spec/03 3.6，以及评审记录 `review/m1-01-followups-debate-2026-09-24.md`。含语义变更（开始绑定 TOTP 需要重新验证），按 ENG-04 升小版本。最低契约版本为 v0.5.0。

### 新增
- 管理接口 `Settings.min_version`（响应必有，默认 `{}`）与 `SettingsUpdate.min_version`（可选）：按平台的客户端最低版本，值为 `x.y.z`；新增 `ClientPlatform` 定义，与客户端接口相同。
- 客户端接口 `GET /v1/config` 的 200 响应列出 `Cache-Control: no-cache`。

### 变更（破坏性）
- `POST /v1/me/mfa/totp`（`startTotpEnrollment`）需要重新验证（AUTH-23），401 改为引用 `MfaRequired`；待确认的密钥绑定发起会话，确认必须来自同一会话链（AUTH-11）；`activateTotp` 写明没有待确认密钥、已作废或会话链不符时返回 409 `invalid_state`。迁移：客户端收到 401 `mfa_required` 时调用 `POST /v1/me/reauthentications` 后重试，与停用 TOTP 相同；以密码登录后 5 分钟内视为已重新验证，新用户登录后立即绑定不受影响。

### 修复
- `getConfig` 的描述写明：`key_id` 为十进制字符串（示例由 `cfg-2026-01` 改为 `1`，与 CONV-30 的 `PANEL_CONFIG_KEY` 一致）；签名确定、各副本 ETag 一致；客户端按 `issued_at` 防回滚；本接口永不返回 426。
- `payload.api_endpoints` 写明为接口根地址（与 DEP-04 的 `api_base_url` 相同，可以带路径前缀），第一项为主地址；`announcement_version` 写明语义；`min_version` 写明未列出的平台不限制。
- 文档总述改为只有 `createSession`、`createSessionNonce`、`createDeviceLink`、`getMyConfiguration` 返回 426，与 API-03 及这四个操作的响应一致；此前写“任何接口都可能返回 426”。
- `SignedConfig.key_id` 加约束（1–255 的十进制字符串）；`issued_at` 的描述补全；管理接口 `min_version` 的版本号不允许前导零，更新时整体替换。
- 两份 OpenAPI 的 `info.version` 改为 0.5.0。

### 安全
- 开始绑定 TOTP 需要重新验证，并绑定发起会话：被盗的会话不能为账号绑定攻击者的验证器而把所有者锁在外面。
- `/v1/config` 的防回滚规则写入契约：客户端不接受 `issued_at` 早于已接受值的旧签名文档。

能否在线执行：可以，不涉及 proto 与数据库。控制面与用户中心嵌入同一二进制，同时升级；部署需新增环境变量 `PANEL_CONFIG_KEY`（CONV-30），实现 `GET /v1/config` 的版本起 api 角色缺少时拒绝启动。

## v0.4.0（M1-01 后续：浏览器刷新响应、SMTP 加密模式、设备签名域分隔）

相对 v0.3.0。依据 workspace spec/10 AUTH-08、AUTH-10、AUTH-23、AUTH-24 与 spec/03 3.6，以及评审记录 `review/panel-spec-v0.4.0-debate-2026-09-24.md`。含语义变更，按 ENG-04 升小版本。最低契约版本为 v0.4.0。

### 新增
- 客户端接口 `CookieTokenRefresh`：浏览器刷新令牌的响应，只含 `token_type`、`expires_in`、`device_id`，令牌只以 Cookie 下发（AUTH-08）。
- 管理接口 `settings.smtp.tls`（`starttls`、`implicit`、`none`，缺省 `starttls`）：连接加密方式，支持端口 465 的隐式 TLS。端口 465 与 `starttls`、`none` 与 `username` 的组合在保存时返回 `not_allowed`。

### 变更（破坏性）
- `POST /v1/oauth/token` 的 200 响应改为 `oneOf [TokenPair, CookieTokenRefresh]`；`TokenPair` 本身不变。迁移：浏览器客户端改用 `CookieTokenRefresh`，不再读取响应体中的令牌。
- 设备证明 `DeviceInfo.device_proof.signature` 的签名对象由 nonce 改为 `akari-device-proof-v1|<device_id>|<nonce>`；扫码批准 `device_signature` 的签名对象由 `{id}.{check_digits}` 改为 `akari-device-link-approval-v1|<id>|<check_digits>`。两种签名以前缀做域分隔，UUID 用小写规范写法。迁移：自研客户端尚未发布，直接切换，不提供旧格式过渡。
- SMTP 缺省由“服务器支持时使用 STARTTLS”改为“要求 STARTTLS”。迁移：已有站点的 SMTP 服务器必须支持 STARTTLS，或把 `tls` 设为 `implicit`；开发环境的 Mailpit 显式设为 `none`。

### 修复
- `Problem.challenge_id` 与 `methods` 写明：登录第二步必附 `challenge_id`，重新验证场景不附，`methods` 可为空数组（AUTH-23）。
- 签名的编码写明为标准 base64（带填充）；扫码批准的示例签名改为 64 字节。
- `DeviceInfo.public_key` 写明同一账号未吊销设备的公钥不得重复，冲突时返回 `not_allowed`（AUTH-10）。
- 两份 OpenAPI 的 `info.version` 改为 0.4.0。

### 安全
- 设备密钥的两种签名（设备证明、扫码批准）以前缀做域分隔，同一签名不能在两种用途之间挪用。
- SMTP 缺省要求加密：验证码与找回密码链接等安全类邮件不再因 STARTTLS 被剥离而以明文发送。

能否在线执行：可以，不涉及 proto 与数据库。需要注意两点：
- 浏览器刷新响应与签名格式的变化，要求控制面与用户中心同时升级（二者嵌入同一二进制）。
- SMTP 缺省值变化后，服务器不支持 STARTTLS 的站点在把 `tls` 改为 `implicit` 或 `none` 之前无法投递邮件，应在升级前完成设置。

## v0.3.0（M1-01 开工前的契约修订）

相对 v0.2.1。依据 workspace 的 M1-01 规格缺口评审（`review/m1-01-spec-gaps-2026-09-24.md`）与 spec/30、spec/10 AUTH-08。只新增可选字段，非破坏性；按 spec-change 的约定，新增字段升小版本。最低契约版本仍为 v0.2.0。

迁移：无。不涉及 proto 与数据库，控制面与用户中心可以按任意顺序升级，可以在线执行（spec/40 DEP-12）。

### 新增
- 客户端接口 `GET /v1/config` 的 `payload.registration_policy`（可选，`open`、`invite_only`、`closed`，spec/10 AUTH-02）：用户中心据此隐藏注册入口或显示邀请码输入框；缺省时按 `open` 处理。

### 修复
- 客户端接口 `cookie` 认证方式的描述与 AUTH-08 一致：`__Host-` Cookie 必须为 Path=/，刷新令牌 Cookie 随每个同源请求发送，服务端只在 `POST /v1/oauth/token` 读取它（原描述称“只随 `POST /v1/oauth/token` 发送”，浏览器无法做到）。
- 两份 OpenAPI 的 `info.version` 改为 `0.3.0`，与发布版本一致（此前停留在 `0.1.0`）。管理接口本版只有描述变化，版本号随仓库发布版本统一。
- 两份 OpenAPI 的 `cookie` 认证描述写明 Path=/；`SignedConfig.signature` 写明按原始 `payload`（含未知字段）规范化后验签。

## v0.2.1（M0-07 澄清与 LICENSE）

相对 v0.2.0。依据 workspace spec/20 的 M0-07 澄清（akari-project/workspace#10）。只有注释与仓库文件变化，proto 字段、编号与线上编码不变，OpenAPI 与 JSON Schema 不变，非破坏性，按补丁版本发布；最低契约版本仍为 v0.2.0。

### 新增
- 根目录 `LICENSE`：Apache-2.0 全文，与 `LICENSES/Apache-2.0.txt` 相同。go-licenses 只从模块根目录的许可证文件识别许可证，此前无法识别本模块；`make licenses` 不再忽略本模块。

### 变更
- 注释修订，字段与线上编码不变（依据 workspace spec/20 M0-07 澄清）：
  - `Frame.seq`：每帧必须恰好比上一帧大 1，重复、不递增或跳号均为协议错误，以 WebSocket 关闭码 1002 关闭（NODE-12）。
  - `messages.proto` 文件头与 `SyncDelta`：`SyncDelta` 单列应用条件，`to_version` 不大于本地只确认，`from_version` 等于本地才应用，其余请求全量同步（NODE-23）；“等于本地 + 1”只适用于 `CredUpsert`、`CredRemove`、`InboundApply`、`RoutesApply`。

## v0.2.0（M0-04 审计后的修订）

相对 v0.1.0。依据 node-agent FORK_PLAN（M0-04）第 6 节与 workspace spec/21 AGT-15、spec/23。含破坏性变更，按 ENG-04 升为小版本。

### 新增
- `Credential.ss2022_key_16`（字段 7）与 `Credential.ss2022_key_32`（字段 8）：Shadowsocks 2022 用户密钥的原始字节，由控制面从 `secret` 经 HKDF-SHA256 生成，Agent 只做 base64 编码。
- `testdata/node-v1-vectors.json` 新增 `credential`：`secret` 的 uuid_text、两把 SS2022 用户密钥（原始字节与 base64），以及 SIP022 客户端密码的完整示例（`ss2022_client`：`inbound_key + ":" + base64(用户密钥)`）。
- mKCP 的 `mkcp.finalmask`（必填）：`header`（可选，`header-srtp`、`header-utp`、`header-wechat`、`header-dtls`、`header-wireguard`）与 `obfs`（必填，`mkcp-original`，或 `mkcp-aes128gcm` 加 `key`）。Agent 按 `[header, obfs]` 的顺序写入 Xray-core 的 `streamSettings.finalmask.udp`，对照用例见 `testdata/mkcp-finalmask.json`（`tools/checkschema` 测试）。

### 变更
- `Credential` 注释写明一条凭据施加于节点的全部入站，凭据消息不带入站 tag（spec/21 21.3、AGT-15）。
- `ReportStatus.kernel_error` 注释写明凭据校验失败的格式 `credential <id>: invalid length`（多条以分号分隔）与清空条件。
- mKCP 的 `read_buffer_mb`、`write_buffer_mb` 说明中写明单位为 MiB（spec/02 CONV-33）。

### 变更（破坏性）

proto（线上编码与字段编号不变，`buf breaking` 通过；破坏性在于语义收窄）：
- `Credential.secret`（字段 3）限定为 16 字节原始 UUIDv4（原注释为“按协议解释：UUID、密码等”），并作为 SS2022 用户密钥派生的输入。各协议使用的形式（uuid_text、SS2022 用户密钥）写在 `Credential` 注释中，两个内核一致。`secret` 长度不是 16 字节，或 `ss2022_key_16`、`ss2022_key_32` 为空或长度错误时，Agent 不把该凭据加入任何入站，并在 `ReportStatus.kernel_error` 中报告凭据 ID。
- 迁移：
  - 控制面：改为保存和下发 16 字节原始值；已有 `proxy_credentials.secret_enc` 若为 36 字符 UUID 文本，读取时解析为 16 字节，或用一次性任务重新加密。当前只有 `panel admin create` 生成的共用凭据受影响（workspace backlog M1-03 已登记后续任务）。
  - Agent：按 `Credential` 注释实现编码与长度校验。没有已发布的旧 Agent。
- 能否在线执行：可以。解析兼容两种形式，不改表结构；重新加密为逐行 UPDATE（spec/40 DEP-12）。

schema：
- `vless-mkcp`、`vmess-mkcp` 删除 `mkcp.header_type` 与 `mkcp.seed`；`mkcp` 与 `mkcp.finalmask.obfs` 改为必填，没有隐式默认值。当前 Xray-core（本组织 fork `infra/conf/transport_internet.go`）在配置中出现 mKCP 的 `header` 或 `seed` 时拒绝加载，按旧字段下发的入站无法启动。
- 迁移：
  - `header_type`：`none` 改为省略 `finalmask.header`；`srtp`、`utp`、`dtls`、`wireguard` 改为 `header-` 加原值；`wechat-video` 改为 `header-wechat`；
  - `seed`：有值时改为 `finalmask.obfs = {"type": "mkcp-aes128gcm", "key": <原 seed>}`，没有值时改为 `{"type": "mkcp-original"}`（与旧版默认行为相同）；
- 能否在线执行：可以。控制面尚无已保存的 mKCP 入站，不需要数据迁移；若有，按上述规则逐行改写 `inbounds.settings`。

### 兼容性
- 不新增能力位，理由：
  - 没有已发布的 v0.1.0 Agent，不存在需要区分对待的旧节点；
  - 新增字段是纯数据，控制面总是填写，proto3 接收方忽略未知字段，不改变任何消息的收发条件（ENG-01 的“新行为配能力位”针对可选行为，本版没有可选行为）；
  - Agent 的最低契约版本为 v0.2.0。
- 最低契约版本（ENG-03）：控制面与 Agent 都必须基于 v0.2.0 或更高版本实现；`panel` 与 `node-agent` README 的兼容矩阵同步写明，v0.1.0 标记为不受支持。

## v0.1.0（M0-03 契约定稿）

相对起步包种子版本。

### 新增
- 节点协议：
  - `HelloReject` 与拒绝原因枚举 `HelloRejectReason`（spec/20 NODE-22）；
  - `HelloAck.server_capabilities_raw`（`ControlPlaneCapabilities`）与 `HelloAck.last_report_seq`（spec/22 ACC-03）；
  - `Snapshot`，以及 `OfflinePolicy`（spec/22 ACC-19）；
  - `Transport` 枚举、`Inbound.transport`、`KernelSupport` 的传输字段；
  - `LeaseRequest.is_release`、`AgentUpgrade.key_id`、`Credential.max_sources`、`SourceSet`、`ReportStatus.running_kernel` 与 `kernel_error`；
  - `Capabilities.supports_source_limit`。
- `enrollment.proto`：接入接口的请求与响应（spec/20 NODE-18）。
- `testdata/node-v1-vectors.json`：握手、会话加密、DNS 凭据加密、快照校验和与 Agent 升级签名（`agent_upgrade`）的测试向量，由 `tools/vectors` 生成。
- `openapi/client/v1.yaml`：按 spec/30 补齐的客户端接口（64 个操作）。
- `openapi/console/v1.yaml`：管理接口（spec/31）。
- `schemas/inbound/`：入站 settings 的 JSON Schema（spec/21 AGT-13、AGT-14）。
- 管理接口的请求头约定：
  - `If-Match`：修改或删除 CONV-13 所列资源时必须携带，缺少返回 428 `precondition_required`，不一致返回 409 `conflict`（CONV-28）；
  - `Mfa-Assertion`：敏感操作必须携带，由 `POST /v1/staff/me/step-up` 取得，5 分钟内有效，缺少或过期返回 401 `mfa_required`（spec/10 AUTH-19）；
  - `Audit-Reason`：不带请求体的敏感 DELETE 与恢复操作用它传递原因，缺少返回 400 `invalid_request`。
- `errors[].code` 新增 `incorrect`（密码或二次验证码不正确，AUTH-23）与 `taken`（取值已被占用，只用于管理接口）。
- 生成的 TypeScript 类型 `gen/ts/client.d.ts`、`gen/ts/console.d.ts`。
- 工具链：`Makefile`、`tools/checkapi`、`tools/checkschema`、`tools/schemagen`、REUSE、CI。

### 变更（破坏性）
- `Hello.capabilities`（字段 6，消息类型）改为 `Hello.capabilities_raw`（字段 9，`bytes`，序列化后的 `Capabilities`），字段 6 保留。迁移：发送方先序列化 `Capabilities` 再填入；MAC 改为覆盖这段原始字节。
- `SyncFull` 改为携带 `snapshot`（字段 7，序列化后的 `Snapshot`）与 `checksum`（字段 5，改为 SHA-256(snapshot)）。原字段 1、2、3、4、6 保留。迁移：接收方先校验 `checksum`，再解析 `snapshot`。
- 删除 `CredRemove.close_sessions`（字段 3 保留）。移除凭据一律关闭连接（spec/20 NODE-24）。
- MAC 与密钥派生的标签改为 `akari-` 前缀：`akari-node-hello-v1`、`akari-node-hello-ack-v1`、`akari-node-session-v1`、`akari-dns-secret-v1`、`akari-agent-upgrade-v1`。编码约定见 `envelope.proto` 文件头。
- 编码约定区分两种拼接：`|` 为带长度前缀的拼接（字符串与字节串前置 4 字节大端长度，整数为大端定长），只用于 MAC 与签名的输入；`||` 为不加前缀的原始拼接，用于 HKDF info、附加数据与密文布局。迁移：实现方按 `envelope.proto` 文件头重写 MAC、签名与密钥派生的输入构造，并用 `testdata/node-v1-vectors.json` 核对。
- 删除 `ControlPlaneCapabilities.supports_source_set`（字段 3 与字段名保留）。迁移：Agent 不再读取该字段；是否处理 `SourceSet` 只取决于收到的消息本身。
- `HelloAck.mac` 的输入增加 `SHA-256(server_capabilities_raw)` 与 `last_report_seq`。
- 客户端接口：
  - `Location.traffic_multiplier` 改名为 `usage_multiplier`（spec/30 API-01）；
  - 注册改为返回 202；
  - 自动续费开关从 `PATCH /v1/me/entitlements/current` 移到 `PATCH /v1/me`；
  - 余额流水从 `GET /v1/me/credits` 拆到分页的 `GET /v1/me/credit-entries`；
  - `QuoteRequest.use_credit` 改名为 `is_credit_applied`；
  - `Attachment.size_bytes` 改名为 `bytes_size`（CONV-07），并新增必填的 `filename`（上传时 multipart 分段的文件名，已去除路径与控制字符，下载时用于 `Content-Disposition`）。迁移：客户端改读 `bytes_size`；服务端保存上传文件名并在响应中返回。
- 管理接口（相对本版本开发过程中的草稿）：
  - 浏览器认证改用 `__Host-console_access_token`、`__Host-console_refresh_token` 两个 Cookie（HttpOnly、Secure、SameSite=Strict、Path=/），登录与刷新的响应体不再返回令牌，只返回 `access_expires_at`、`refresh_expires_at` 等会话信息；刷新接口不再接受表单中的 `refresh_token`。迁移：前端删除读取与保存令牌的代码，请求时依赖浏览器自动携带 Cookie，用响应中的过期时间安排刷新。
  - Cookie 名从草稿中与用户中心相同的 `__Host-access_token`、`__Host-refresh_token` 改为 `__Host-console_` 前缀：两个应用可以部署在同一主机、只以路径前缀区分（spec/31 CON-01、spec/40 DEP-02），而 `__Host-` Cookie 必须为 Path=/，同名会互相覆盖（spec/10 AUTH-08）。迁移：管理后台前端与控制面的 console 鉴权中间件改用新的 Cookie 名；已按草稿登录的管理会话需要重新登录。
  - 敏感的 DELETE 操作改由请求头 `Audit-Reason` 传递原因，请求体中不再带 `reason`。迁移：调用方把原来请求体中的 `reason` 移到 `Audit-Reason` 请求头，并去掉 DELETE 的请求体。
  - `KernelReport` 的传输字段拆为 `stable_transports` 与 `experimental_transports`；删除 `KernelSupport.host_reports`。迁移：读取方把原 `transports` 按稳定性分别读取两个字段；各主机上报的能力改从 `Host.reported_kernels` 读取。
  - 入站删除顶层的 `reality_public_key`、`reality_short_ids`，改用 `settings.reality.*`；私钥放在只写的 `secrets` 中，读取时不返回（CONV-19）。迁移：调用方改读写 `settings.reality` 下的对应字段，创建或轮换私钥时通过 `secrets` 提交。
  - `PaymentNotification.total_amount`（字符串金额）改为 `amount_minor`（整数，最小货币单位）加 `currency`（CONV-05）。迁移：读取方改用整数金额，不再解析小数字符串；支付宝通知原文中的 `total_amount` 不受影响。
  - `createHost`、`createEnrollmentToken`、`createRedeemCodeBatch`、`createCoupon` 的响应含只返回一次的秘密值，不再接受 `Idempotency-Key`（CONV-12）。迁移：调用方不再发送该请求头；响应丢失时，接入令牌重新签发，兑换码停用该批次后重新生成，优惠码按码值已占用（`taken`）的结果处理。
  - `RefundCreate.currency` 改为必填。迁移：调用方提交退款时带上 `currency`，取值必须等于订单币种（即站点结算货币，CONV-08）。
  - 入站端口冲突的 `errors[].code` 从 `not_allowed` 改为 `taken`。迁移：管理后台按 `taken` 显示“端口已被占用”的文案。
  - `createAccount` 邮箱重复从 409 `conflict` 改为 400 `invalid_request`，`errors[].code` 为 `taken`。迁移：管理后台按字段错误显示，不再把 409 当作邮箱重复。
- 代码生成改用本地 `protoc-gen-go`（版本由 `go.mod` 锁定），`go_package` 前缀改为 `github.com/akari-project/panel-spec/gen/go`。

### 修复
- 无。

### 安全
- 会话密钥分方向派生，nonce 每帧随机生成，不由序号派生（spec/20 NODE-11）。
- 握手 MAC 覆盖能力位的原始字节，避免新增字段导致重新序列化结果不一致（spec/20 20.3）。
- Agent 升级签名同时绑定版本号与制品摘要（spec/40 DEP-09）。

### 已知问题与后续
以下复审意见留到之后的版本处理：
- N7：安装命令的参数（如 `--server`）改为中性名称（spec/30 API-01），需要与 node-agent 一起修改。
- N9：错误码枚举（`code` 与 `errors[].code`）在 1.0 之前决定是否改为开放枚举，以便新增取值不构成破坏性变更。
- N11：`tools/checkapi` 增加两项检查：敏感操作必须带 `Mfa-Assertion` 与原因（请求体 `reason` 或 `Audit-Reason` 请求头）；响应含秘密值的操作不接受 `Idempotency-Key`（CONV-12）。
