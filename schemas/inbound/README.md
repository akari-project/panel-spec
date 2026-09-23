<!-- SPDX-License-Identifier: Apache-2.0 -->
<!-- 生成文件：由 tools/schemagen 生成，请勿手改，修改后运行 make gen。 -->

# 入站 settings 的 JSON Schema

`Inbound.settings_json` 的结构按“协议 + 传输”分别定义（spec/21 AGT-13），JSON Schema draft 2020-12。
控制面在保存入站时按 `<protocol>-<transport>.schema.json` 校验；协议与传输名为 proto 枚举 `Protocol`、`Transport` 的小写形式。

- 组合的支持程度取 spec/21 21.2 协议矩阵与传输矩阵中较弱的一方；两个内核都不支持的组合没有 schema。
- 本表只是说明。强制校验以数据库基线 `kernel_protocols`、`kernel_transports` 与节点上报的能力为准（AGT-09）；实验组合需要节点 `allow_experimental=true`（AGT-10）。
- 每个 schema 的 `x-kernels` 注解记录同样的支持程度。
- `transport` 必填且等于本组合的传输，与 `Inbound.transport` 一致；数据库校验以 `settings->>'transport'` 为准。
- VLESS 与 AnyTLS 的 Reality 共用同一定义（`$defs/reality`）。
- 用户凭据（UUID、密码）不在 settings 中，由 `Credential` 下发。Reality 私钥、Shadowsocks 2022 主密钥属于节点密钥，控制面按 CONV-19 加密保存。
- 校验时应启用 `format` 断言（如 `hostname`、`uri`）；正则只使用 RE2 与 ECMA-262 共有的语法。
- `examples/` 中每个 schema 至少有一个合法示例，`make lint`（checkschema）校验。

| 组合 | 文件 | sing-box | Xray-core | 安全层 | 说明 |
|---|---|---|---|---|---|
| VLESS over TCP | [`vless-tcp.schema.json`](vless-tcp.schema.json) | 稳定 | 稳定 | none、tls、reality | 支持 Vision（`flow`）与 Reality |
| VLESS over WebSocket | [`vless-ws.schema.json`](vless-ws.schema.json) | 稳定 | 稳定 | none、tls |  |
| VLESS over gRPC | [`vless-grpc.schema.json`](vless-grpc.schema.json) | 稳定 | 稳定 | none、tls、reality | 支持 Reality |
| VLESS over HTTPUpgrade | [`vless-httpupgrade.schema.json`](vless-httpupgrade.schema.json) | 稳定 | 稳定 | none、tls |  |
| VLESS over XHTTP | [`vless-xhttp.schema.json`](vless-xhttp.schema.json) | 不支持 | 稳定 | none、tls、reality | 支持 Reality |
| VLESS over mKCP | [`vless-mkcp.schema.json`](vless-mkcp.schema.json) | 不支持 | 稳定 | none | 无 TLS |
| VMess over TCP | [`vmess-tcp.schema.json`](vmess-tcp.schema.json) | 稳定 | 稳定 | none、tls |  |
| VMess over WebSocket | [`vmess-ws.schema.json`](vmess-ws.schema.json) | 稳定 | 稳定 | none、tls |  |
| VMess over gRPC | [`vmess-grpc.schema.json`](vmess-grpc.schema.json) | 稳定 | 稳定 | none、tls |  |
| VMess over HTTPUpgrade | [`vmess-httpupgrade.schema.json`](vmess-httpupgrade.schema.json) | 稳定 | 稳定 | none、tls |  |
| VMess over XHTTP | [`vmess-xhttp.schema.json`](vmess-xhttp.schema.json) | 不支持 | 稳定 | none、tls |  |
| VMess over mKCP | [`vmess-mkcp.schema.json`](vmess-mkcp.schema.json) | 不支持 | 稳定 | none | 无 TLS |
| Trojan over TCP | [`trojan-tcp.schema.json`](trojan-tcp.schema.json) | 稳定 | 稳定 | tls | TLS 必填 |
| Trojan over WebSocket | [`trojan-ws.schema.json`](trojan-ws.schema.json) | 稳定 | 稳定 | none、tls | `none` 仅用于由前置代理终止 TLS |
| Trojan over gRPC | [`trojan-grpc.schema.json`](trojan-grpc.schema.json) | 稳定 | 稳定 | none、tls | `none` 仅用于由前置代理终止 TLS |
| Trojan over HTTPUpgrade | [`trojan-httpupgrade.schema.json`](trojan-httpupgrade.schema.json) | 稳定 | 稳定 | none、tls | `none` 仅用于由前置代理终止 TLS |
| Trojan over XHTTP | [`trojan-xhttp.schema.json`](trojan-xhttp.schema.json) | 不支持 | 稳定 | none、tls | `none` 仅用于由前置代理终止 TLS |
| Shadowsocks | [`shadowsocks-tcp.schema.json`](shadowsocks-tcp.schema.json) | 稳定 | 稳定 | none | 同端口承载 TCP 与 UDP |
| Hysteria2 | [`hysteria2-quic.schema.json`](hysteria2-quic.schema.json) | 稳定 | 实验 | tls | QUIC，TLS 必填 |
| TUIC v5 | [`tuic-quic.schema.json`](tuic-quic.schema.json) | 稳定 | 不支持 | tls | QUIC，TLS 必填 |
| AnyTLS | [`anytls-tcp.schema.json`](anytls-tcp.schema.json) | 稳定 | 不支持 | tls、reality | 支持 Reality；mihomo 不支持该组合，导出时跳过（spec/30 API-09） |

未提供的搭配及理由：

- Reality 只用于 VLESS 与 AnyTLS；VLESS 的 Reality 只搭配 TCP、gRPC、XHTTP。
- Shadowsocks 只搭配 TCP（sing-box 的 Shadowsocks 入站不支持 V2Ray 传输）。
- Trojan 不搭配 mKCP（mKCP 没有 TLS）。
- Hysteria2、TUIC 只使用自带的 QUIC；AnyTLS 只使用 TCP。
