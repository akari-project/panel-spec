// SPDX-License-Identifier: Apache-2.0

package main

// Status 是某个内核对协议或传输的支持程度（spec/21 21.2）。
type Status string

const (
	Stable       Status = "stable"
	Experimental Status = "experimental"
	Unsupported  Status = "unsupported"
)

// label 是 README 中的中文写法，与 spec/21 矩阵一致。
func (s Status) label() string {
	switch s {
	case Stable:
		return "稳定"
	case Experimental:
		return "实验"
	default:
		return "不支持"
	}
}

func (s Status) rank() int {
	switch s {
	case Stable:
		return 2
	case Experimental:
		return 1
	default:
		return 0
	}
}

// worse 返回两者中较弱的支持程度：组合的支持程度取协议与传输中较弱的一方。
func worse(a, b Status) Status {
	if a.rank() < b.rank() {
		return a
	}
	return b
}

// support 是一行矩阵：sing-box 与 Xray-core 的支持程度。
type support struct{ singbox, xray Status }

// protocolMatrix 抄自 spec/21 21.2 协议矩阵。键为 proto 枚举 Protocol 的小写形式。
var protocolMatrix = map[string]support{
	"vless":       {Stable, Stable},
	"vmess":       {Stable, Stable},
	"trojan":      {Stable, Stable},
	"shadowsocks": {Stable, Stable},
	"hysteria2":   {Stable, Experimental},
	"tuic":        {Stable, Unsupported},
	"anytls":      {Stable, Unsupported},
}

// transportMatrix 抄自 spec/21 21.2 传输矩阵。键为 proto 枚举 Transport 的小写形式。
// quic 不在传输矩阵中：它是 Hysteria2、TUIC 自带的传输，支持程度等于协议本身。
var transportMatrix = map[string]support{
	"tcp":         {Stable, Stable},
	"ws":          {Stable, Stable},
	"grpc":        {Stable, Stable},
	"httpupgrade": {Stable, Stable},
	"xhttp":       {Unsupported, Stable},
	"mkcp":        {Unsupported, Stable},
	"quic":        {Stable, Stable},
}

// security 是入站的安全层取值。
const (
	secNone    = "none"
	secTLS     = "tls"
	secReality = "reality"
)

// pairing 描述协议可以搭配的传输，以及每种搭配允许的安全层。
//   - 安全层只有一个取值时不出现 security 字段：取值为 tls 时 tls 必填，取值为 none 时不出现 tls。
//   - Reality 用于 VLESS（只搭配 TCP、gRPC、XHTTP，Xray 对其他传输拒绝 Reality）与 AnyTLS（仅 sing-box）。
//   - Shadowsocks 只搭配 TCP（sing-box 的 Shadowsocks 入站没有 V2Ray 传输）。
//   - Trojan 依赖 TLS，不搭配没有 TLS 的 mKCP；直连 TCP 时 TLS 必填。
//   - mKCP 基于 UDP，不带 TLS。
type pairing struct {
	protocol   string
	transport  string
	securities []string
	title      string
	note       string
}

var pairings = []pairing{
	{"vless", "tcp", []string{secNone, secTLS, secReality}, "VLESS over TCP", "支持 Vision（`flow`）与 Reality"},
	{"vless", "ws", []string{secNone, secTLS}, "VLESS over WebSocket", ""},
	{"vless", "grpc", []string{secNone, secTLS, secReality}, "VLESS over gRPC", "支持 Reality"},
	{"vless", "httpupgrade", []string{secNone, secTLS}, "VLESS over HTTPUpgrade", ""},
	{"vless", "xhttp", []string{secNone, secTLS, secReality}, "VLESS over XHTTP", "支持 Reality"},
	{"vless", "mkcp", []string{secNone}, "VLESS over mKCP", "无 TLS"},
	{"vmess", "tcp", []string{secNone, secTLS}, "VMess over TCP", ""},
	{"vmess", "ws", []string{secNone, secTLS}, "VMess over WebSocket", ""},
	{"vmess", "grpc", []string{secNone, secTLS}, "VMess over gRPC", ""},
	{"vmess", "httpupgrade", []string{secNone, secTLS}, "VMess over HTTPUpgrade", ""},
	{"vmess", "xhttp", []string{secNone, secTLS}, "VMess over XHTTP", ""},
	{"vmess", "mkcp", []string{secNone}, "VMess over mKCP", "无 TLS"},
	{"trojan", "tcp", []string{secTLS}, "Trojan over TCP", "TLS 必填"},
	{"trojan", "ws", []string{secNone, secTLS}, "Trojan over WebSocket", "`none` 仅用于由前置代理终止 TLS"},
	{"trojan", "grpc", []string{secNone, secTLS}, "Trojan over gRPC", "`none` 仅用于由前置代理终止 TLS"},
	{"trojan", "httpupgrade", []string{secNone, secTLS}, "Trojan over HTTPUpgrade", "`none` 仅用于由前置代理终止 TLS"},
	{"trojan", "xhttp", []string{secNone, secTLS}, "Trojan over XHTTP", "`none` 仅用于由前置代理终止 TLS"},
	{"shadowsocks", "tcp", []string{secNone}, "Shadowsocks", "同端口承载 TCP 与 UDP"},
	{"hysteria2", "quic", []string{secTLS}, "Hysteria2", "QUIC，TLS 必填"},
	{"tuic", "quic", []string{secTLS}, "TUIC v5", "QUIC，TLS 必填"},
	{"anytls", "tcp", []string{secTLS, secReality}, "AnyTLS", "支持 Reality；mihomo 不支持该组合，导出时跳过（spec/30 API-09）"},
}

// Combo 是一个生成 schema 的组合。
type Combo struct {
	pairing
	SingBox Status
	Xray    Status
}

// Name 是组合名，也是文件名前缀：<protocol>-<transport>。
func (c Combo) Name() string { return c.protocol + "-" + c.transport }

// FileName 是 schema 文件名。
func (c Combo) FileName() string { return c.Name() + ".schema.json" }

// Combos 返回至少一个内核为“稳定”或“实验”的组合，顺序与 pairings 相同。
func Combos() []Combo {
	var out []Combo
	for _, p := range pairings {
		ps, ok := protocolMatrix[p.protocol]
		if !ok {
			panic("unknown protocol " + p.protocol)
		}
		ts, ok := transportMatrix[p.transport]
		if !ok {
			panic("unknown transport " + p.transport)
		}
		c := Combo{pairing: p, SingBox: worse(ps.singbox, ts.singbox), Xray: worse(ps.xray, ts.xray)}
		if c.SingBox == Unsupported && c.Xray == Unsupported {
			continue
		}
		out = append(out, c)
	}
	return out
}
