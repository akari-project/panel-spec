// SPDX-License-Identifier: Apache-2.0

// Command schemagen 生成入站 settings_json 的 JSON Schema（spec/21 AGT-13）与 schemas/inbound/README.md。
//
// 组合与支持程度来自 matrix.go（抄自 spec/21 21.2），字段定义来自本文件。
// schemas/inbound/*.schema.json 与 README.md 是生成物，修改请改本工具后运行 make gen。
//
// 用法：go run ./tools/schemagen -out schemas/inbound
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type obj = map[string]any

const (
	draft    = "https://json-schema.org/draft/2020-12/schema"
	idPrefix = "urn:akari-project:panel-spec:schemas:inbound:"
)

func main() {
	out := flag.String("out", "schemas/inbound", "输出目录")
	flag.Parse()
	if err := run(*out); err != nil {
		fmt.Fprintln(os.Stderr, "schemagen:", err)
		os.Exit(1)
	}
}

func run(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	combos := Combos()
	for _, c := range combos {
		b, err := encode(Schema(c))
		if err != nil {
			return fmt.Errorf("%s: %w", c.Name(), err)
		}
		if err := os.WriteFile(filepath.Join(dir, c.FileName()), b, 0o644); err != nil {
			return err
		}
	}
	return os.WriteFile(filepath.Join(dir, "README.md"), []byte(Readme(combos)), 0o644)
}

func encode(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Schema 构造一个组合的 schema。
func Schema(c Combo) obj {
	props := obj{
		"transport": obj{
			"const":       c.transport,
			"description": "必填，等于本组合的传输，与 Inbound.transport 一致。数据库校验以 settings->>'transport' 为准。",
		},
	}
	required := []string{"transport"}
	var allOf []any
	defs := obj{}

	hasTLS, hasReality := false, false
	for _, s := range c.securities {
		switch s {
		case secTLS:
			hasTLS = true
		case secReality:
			hasReality = true
		}
	}
	if hasTLS {
		props["tls"] = ref("tls")
		defs["tls"] = tlsDef(c.transport == "quic")
	}
	if hasReality {
		props["reality"] = ref("reality")
		defs["reality"] = realityDef()
	}
	switch {
	case len(c.securities) > 1:
		props["security"] = obj{
			"enum":        toAny(c.securities),
			"description": "安全层。tls 时必须提供 tls，reality 时必须提供 reality，其余情况二者都不得出现。",
		}
		required = append(required, "security")
		allOf = append(allOf, securityRules(c.securities)...)
	case c.securities[0] == secTLS:
		required = append(required, "tls")
	}

	// 传输参数。
	switch c.transport {
	case "ws", "grpc", "httpupgrade", "xhttp":
		props[c.transport] = ref(c.transport)
		defs[c.transport] = transportDef(c.transport)
		required = append(required, c.transport)
	case "mkcp":
		props["mkcp"] = ref("mkcp")
		defs["mkcp"] = transportDef("mkcp")
	}

	// 协议参数。
	switch c.protocol {
	case "vless":
		if c.transport == "tcp" {
			props["flow"] = obj{
				"enum":        []any{"none", "xtls-rprx-vision"},
				"description": "下发给该入站全部凭据的流控。xtls-rprx-vision 要求 security 为 tls 或 reality。默认 none。",
			}
			allOf = append(allOf, obj{
				"if": obj{
					"properties": obj{"flow": obj{"const": "xtls-rprx-vision"}},
					"required":   []any{"flow"},
				},
				"then": obj{"properties": obj{"security": obj{"enum": []any{secTLS, secReality}}}},
			})
		}
	case "shadowsocks":
		props["method"] = obj{
			"enum": []any{
				"2022-blake3-aes-128-gcm",
				"2022-blake3-aes-256-gcm",
				"aes-128-gcm",
				"aes-256-gcm",
				"chacha20-ietf-poly1305",
			},
			"description": "加密方式。2022-blake3-chacha20-poly1305 不在列表中：Xray-core 的该方式不支持多用户。" +
				"Agent 把 chacha20-ietf-poly1305 映射为 Xray-core 的 chacha20-poly1305。",
		}
		props["inbound_key"] = obj{
			"type":        "string",
			"pattern":     `^[A-Za-z0-9+/]+={0,2}$`,
			"minLength":   24,
			"maxLength":   44,
			"description": "2022 系列方式的入站主密钥（base64，长度与方式的密钥长度一致），由控制面生成并按 CONV-19 加密保存。用户密钥由 Credential 下发。",
		}
		required = append(required, "method")
		allOf = append(allOf, obj{
			"if":   obj{"properties": obj{"method": obj{"pattern": "^2022-"}}},
			"then": obj{"required": []any{"inbound_key"}},
			"else": obj{"not": obj{"required": []any{"inbound_key"}}},
		})
	case "hysteria2":
		props["up_mbps"] = mbps("服务端上行带宽上限（Mbps），0 表示不限并由客户端声明。")
		props["down_mbps"] = mbps("服务端下行带宽上限（Mbps），0 表示不限并由客户端声明。")
		props["is_client_bandwidth_ignored"] = obj{
			"type":        "boolean",
			"description": "忽略客户端声明的带宽，使用 BBR 拥塞控制。默认 false。",
		}
		props["obfs"] = obj{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []any{"type", "key"},
			"properties": obj{
				"type": obj{"const": "salamander"},
				"key": obj{
					"type": "string", "minLength": 8, "maxLength": 128,
					"description": "混淆密钥，入站内全部连接共用，不是用户凭据。",
				},
			},
			"description": "可选的 Salamander 混淆。启用后客户端必须配置相同的密钥。",
		}
		props["masquerade_url"] = obj{
			"type": "string", "format": "uri", "pattern": "^https?://", "maxLength": 2048,
			"description": "认证失败时反向代理到的网址，用于伪装。",
		}
	case "tuic":
		props["congestion_control"] = obj{
			"enum":        []any{"cubic", "new_reno", "bbr"},
			"description": "QUIC 拥塞控制算法，默认 cubic。",
		}
		props["auth_timeout_ms"] = obj{
			"type": "integer", "minimum": 100, "maximum": 60000,
			"description": "建立 QUIC 连接后等待客户端认证的时间，默认 3000。",
		}
		props["heartbeat_ms"] = obj{
			"type": "integer", "minimum": 1000, "maximum": 60000,
			"description": "心跳间隔，默认 10000。",
		}
		props["is_zero_rtt_enabled"] = obj{
			"type":        "boolean",
			"description": "启用 0-RTT。有重放风险，默认 false。",
		}
	case "anytls":
		props["padding_scheme"] = obj{
			"type": "array", "minItems": 1, "maxItems": 64,
			"items": obj{
				"type":    "string",
				"pattern": `^(stop=[0-9]+|[0-9]+=(c|[0-9]+(-[0-9]+)?)(,(c|[0-9]+(-[0-9]+)?))*)$`,
			},
			"description": "填充方案，每项一行，格式同 AnyTLS 规范。省略时使用内核默认方案。",
		}
	}

	s := obj{
		"$schema":              draft,
		"$id":                  idPrefix + c.Name(),
		"title":                c.title,
		"description":          fmt.Sprintf("Inbound.settings_json：protocol=%s，transport=%s（spec/21 AGT-13）。用户凭据不在此处，由 Credential 下发。", c.protocol, c.transport),
		"type":                 "object",
		"additionalProperties": false,
		"properties":           props,
		"x-kernels":            obj{"singbox": string(c.SingBox), "xray": string(c.Xray)},
	}
	s["required"] = toAny(required)
	if len(allOf) > 0 {
		s["allOf"] = allOf
	}
	if len(defs) > 0 {
		s["$defs"] = defs
	}
	return s
}

func ref(name string) obj { return obj{"$ref": "#/$defs/" + name} }

func toAny(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

func mbps(desc string) obj {
	return obj{"type": "integer", "minimum": 0, "maximum": 100000, "description": desc}
}

// securityRules：security 取某值时必须带对应对象，并且不得带其他安全层的对象。
func securityRules(securities []string) []any {
	objOf := map[string]string{secTLS: "tls", secReality: "reality"}
	var rules []any
	for _, s := range securities {
		then := obj{}
		if o, ok := objOf[s]; ok {
			then["required"] = []any{o}
		}
		var others []any
		for _, t := range securities {
			if o, ok := objOf[t]; ok && t != s {
				others = append(others, obj{"required": []any{o}})
			}
		}
		switch len(others) {
		case 0:
		case 1:
			then["not"] = others[0]
		default:
			then["not"] = obj{"anyOf": others}
		}
		rules = append(rules, obj{
			"if":   obj{"properties": obj{"security": obj{"const": s}}, "required": []any{"security"}},
			"then": then,
		})
	}
	return rules
}

func hostname(desc string) obj {
	h := obj{"type": "string", "format": "hostname", "minLength": 1, "maxLength": 253}
	if desc != "" {
		h["description"] = desc
	}
	return h
}

func tlsDef(quic bool) obj {
	alpn := obj{
		"type": "array", "minItems": 1, "uniqueItems": true,
		"items":       obj{"enum": []any{"h3", "h2", "http/1.1"}},
		"description": "ALPN，按优先级排列。",
	}
	if quic {
		alpn["items"] = obj{"enum": []any{"h3"}}
		alpn["description"] = "ALPN。基于 QUIC 的协议只使用 h3，默认 [\"h3\"]。"
	}
	return obj{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"sni", "cert_mode"},
		"properties": obj{
			"sni":         hostname("证书域名，同时是客户端使用的 SNI。"),
			"alpn":        alpn,
			"min_version": obj{"enum": []any{"1.2", "1.3"}, "description": "最低 TLS 版本，默认 1.2。"},
			"cert_mode": obj{
				"enum": []any{"acme_http", "acme_dns", "file"},
				"description": "证书来源：acme_http 为 HTTP-01，acme_dns 为 DNS-01（服务商凭据随 RoutesApply 加密下发，spec/20 NODE-25），" +
					"file 为节点本地文件。",
			},
			"certificate_path": obj{"type": "string", "pattern": "^/", "maxLength": 1024, "description": "cert_mode 为 file 时必填：证书链 PEM 的绝对路径。"},
			"key_path":         obj{"type": "string", "pattern": "^/", "maxLength": 1024, "description": "cert_mode 为 file 时必填：私钥 PEM 的绝对路径。"},
		},
		"allOf": []any{obj{
			"if":   obj{"properties": obj{"cert_mode": obj{"const": "file"}}, "required": []any{"cert_mode"}},
			"then": obj{"required": []any{"certificate_path", "key_path"}},
			"else": obj{"not": obj{"anyOf": []any{obj{"required": []any{"certificate_path"}}, obj{"required": []any{"key_path"}}}}},
		}},
	}
}

func realityDef() obj {
	key := `^[A-Za-z0-9_-]{43}$`
	return obj{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"private_key", "short_ids", "handshake_address", "handshake_port", "sni_names"},
		"properties": obj{
			"private_key": obj{
				"type": "string", "pattern": key,
				"description": "X25519 私钥，base64url 无填充。由控制面生成（spec/20 NODE-04），按 CONV-19 加密保存。",
			},
			"public_key": obj{
				"type": "string", "pattern": key,
				"description": "与 private_key 对应的公钥，供导出配置使用；Agent 忽略此字段。",
			},
			"short_ids": obj{
				"type": "array", "minItems": 1, "maxItems": 16, "uniqueItems": true,
				"items":       obj{"type": "string", "pattern": "^([0-9a-f]{2}){0,8}$"},
				"description": "短 ID，每项为 0 到 16 位偶数长度的小写十六进制。",
			},
			"handshake_address": hostname("握手目标站点的域名。"),
			"handshake_port":    obj{"type": "integer", "minimum": 1, "maximum": 65535, "description": "握手目标站点的端口，通常为 443。"},
			"sni_names": obj{
				"type": "array", "minItems": 1, "uniqueItems": true,
				"items":       hostname(""),
				"description": "允许的客户端 SNI，必须是握手目标站点证书覆盖的域名。",
			},
			"max_time_difference_ms": obj{
				"type": "integer", "minimum": 0, "maximum": 3600000,
				"description": "允许的客户端时钟偏差，0 表示不检查。默认 0。",
			},
		},
	}
}

func httpPath() obj {
	return obj{"type": "string", "pattern": "^/", "minLength": 1, "maxLength": 256, "description": "请求路径，以 / 开头。"}
}

func transportDef(t string) obj {
	d := obj{"type": "object", "additionalProperties": false}
	switch t {
	case "ws":
		d["required"] = []any{"path"}
		d["properties"] = obj{
			"path":                   httpPath(),
			"host":                   hostname("期望的 Host 请求头；省略时不检查。"),
			"max_early_data":         obj{"type": "integer", "minimum": 0, "maximum": 8192, "description": "0-RTT 早期数据的最大字节数，0 表示关闭。默认 0。"},
			"early_data_header_name": obj{"type": "string", "pattern": "^[A-Za-z0-9-]+$", "maxLength": 64, "description": "携带早期数据的请求头名。省略时早期数据放在路径中（Xray-core 兼容方式）。"},
		}
	case "grpc":
		d["required"] = []any{"service_name"}
		d["properties"] = obj{
			"service_name": obj{"type": "string", "pattern": `^[A-Za-z0-9_.-]+(/[A-Za-z0-9_.-]+)*$`, "maxLength": 128, "description": "gRPC 服务名。"},
		}
	case "httpupgrade":
		d["required"] = []any{"path"}
		d["properties"] = obj{
			"path": httpPath(),
			"host": hostname("期望的 Host 请求头；省略时不检查。"),
		}
	case "xhttp":
		d["required"] = []any{"path"}
		d["properties"] = obj{
			"path": httpPath(),
			"host": hostname("期望的 Host 请求头；省略时不检查。"),
			"mode": obj{"enum": []any{"auto", "packet-up", "stream-up", "stream-one"}, "description": "XHTTP 模式，默认 auto。"},
		}
	case "mkcp":
		d["properties"] = obj{
			"mtu":                    obj{"type": "integer", "minimum": 576, "maximum": 1460, "description": "默认 1350。"},
			"tti_ms":                 obj{"type": "integer", "minimum": 10, "maximum": 100, "description": "传输时间间隔，默认 50。"},
			"uplink_capacity_mbps":   obj{"type": "integer", "minimum": 0, "maximum": 10000, "description": "上行容量，默认 5。"},
			"downlink_capacity_mbps": obj{"type": "integer", "minimum": 0, "maximum": 10000, "description": "下行容量，默认 20。"},
			"is_congestion_enabled":  obj{"type": "boolean", "description": "启用拥塞控制，默认 false。"},
			"read_buffer_mb":         obj{"type": "integer", "minimum": 1, "maximum": 64, "description": "单连接读缓冲，默认 2。"},
			"write_buffer_mb":        obj{"type": "integer", "minimum": 1, "maximum": 64, "description": "单连接写缓冲，默认 2。"},
			"header_type":            obj{"enum": []any{"none", "srtp", "utp", "wechat-video", "dtls", "wireguard"}, "description": "包头伪装，默认 none。"},
			"seed":                   obj{"type": "string", "minLength": 1, "maxLength": 64, "description": "混淆种子，入站内全部连接共用，不是用户凭据。"},
		}
	}
	return d
}

// Readme 生成 schemas/inbound/README.md。
func Readme(combos []Combo) string {
	var b strings.Builder
	b.WriteString(`<!-- SPDX-License-Identifier: Apache-2.0 -->
<!-- 生成文件：由 tools/schemagen 生成，请勿手改，修改后运行 make gen。 -->

# 入站 settings 的 JSON Schema

` + "`Inbound.settings_json`" + ` 的结构按“协议 + 传输”分别定义（spec/21 AGT-13），JSON Schema draft 2020-12。
控制面在保存入站时按 ` + "`<protocol>-<transport>.schema.json`" + ` 校验；协议与传输名为 proto 枚举 ` + "`Protocol`" + `、` + "`Transport`" + ` 的小写形式。

- 组合的支持程度取 spec/21 21.2 协议矩阵与传输矩阵中较弱的一方；两个内核都不支持的组合没有 schema。
- 本表只是说明。强制校验以数据库基线 ` + "`kernel_protocols`" + `、` + "`kernel_transports`" + ` 与节点上报的能力为准（AGT-09）；实验组合需要节点 ` + "`allow_experimental=true`" + `（AGT-10）。
- 每个 schema 的 ` + "`x-kernels`" + ` 注解记录同样的支持程度。
- ` + "`transport`" + ` 必填且等于本组合的传输，与 ` + "`Inbound.transport`" + ` 一致；数据库校验以 ` + "`settings->>'transport'`" + ` 为准。
- VLESS 与 AnyTLS 的 Reality 共用同一定义（` + "`$defs/reality`" + `）。
- 用户凭据（UUID、密码）不在 settings 中，由 ` + "`Credential`" + ` 下发。Reality 私钥、Shadowsocks 2022 主密钥属于节点密钥，控制面按 CONV-19 加密保存。
- 校验时应启用 ` + "`format`" + ` 断言（如 ` + "`hostname`" + `、` + "`uri`" + `）；正则只使用 RE2 与 ECMA-262 共有的语法。
- ` + "`examples/`" + ` 中每个 schema 至少有一个合法示例，` + "`make lint`" + `（checkschema）校验。

| 组合 | 文件 | sing-box | Xray-core | 安全层 | 说明 |
|---|---|---|---|---|---|
`)
	for _, c := range combos {
		sec := strings.Join(c.securities, "、")
		fmt.Fprintf(&b, "| %s | [`%s`](%s) | %s | %s | %s | %s |\n",
			c.title, c.FileName(), c.FileName(), c.SingBox.label(), c.Xray.label(), sec, c.note)
	}
	b.WriteString(`
未提供的搭配及理由：

- Reality 只用于 VLESS 与 AnyTLS；VLESS 的 Reality 只搭配 TCP、gRPC、XHTTP。
- Shadowsocks 只搭配 TCP（sing-box 的 Shadowsocks 入站不支持 V2Ray 传输）。
- Trojan 不搭配 mKCP（mKCP 没有 TLS）。
- Hysteria2、TUIC 只使用自带的 QUIC；AnyTLS 只使用 TCP。
`)
	return b.String()
}
