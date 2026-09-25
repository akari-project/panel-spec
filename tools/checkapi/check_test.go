// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// doc 把一段 paths 与 components 拼成完整文档。
func doc(paths, components string) string {
	s := "openapi: 3.1.0\ninfo:\n  title: T\n  version: '1'\n  description: server node traffic subscribe 都可以出现在描述中\n" +
		"servers:\n- url: https://api.example.invalid\npaths:\n" + indent(paths, "  ")
	if components != "" {
		s += "components:\n" + indent(components, "  ")
	}
	return s
}

func indent(s, pre string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = pre + l
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

const okOp = `/v1/me:
  get:
    x-permission: accounts.read
    responses:
      '200':
        description: OK
        content:
          application/json:
            schema: {type: object}
            example: {id: a}
`

// sensitiveComponents 是敏感操作引用的组件。
const sensitiveComponents = `responses:
  MfaRequired:
    description: x
parameters:
  MfaAssertion: {name: Mfa-Assertion, in: header, schema: {type: string}}
  AuditReason: {name: Audit-Reason, in: header, schema: {type: string}}
`

// sensitiveOp 构造一个 x-sensitive 操作；三个开关分别控制 401 MfaRequired、Mfa-Assertion 与 Audit-Reason 参数。
func sensitiveOp(method string, with401, withAssertion, withReason bool) string {
	s := "/v1/me:\n  " + method + ":\n    x-permission: accounts.read\n    x-sensitive: true\n    parameters:\n"
	if withAssertion {
		s += "    - $ref: '#/components/parameters/MfaAssertion'\n"
	}
	if withReason {
		s += "    - $ref: '#/components/parameters/AuditReason'\n"
	}
	s += "    - {name: q, in: query, schema: {type: string}}\n    responses:\n"
	if with401 {
		s += "      '401':\n        $ref: '#/components/responses/MfaRequired'\n"
	}
	return s + "      '204':\n        description: OK\n"
}

func TestCheck(t *testing.T) {
	tests := []struct {
		name    string
		doc     string
		console bool
		want    []string // 每项必须出现在问题中；nil 表示应当通过
		line    int      // 非 0 时第一条问题必须位于该行
	}{
		{name: "ok", doc: doc(okOp, ""), console: true},
		{
			name: "example in media examples",
			doc: doc(`/v1/a:
  get:
    responses:
      '200':
        description: OK
        content:
          application/json:
            examples:
              basic: {value: {id: a}}
`, ""),
		},
		{
			name: "example in referenced schema",
			doc: doc(`/v1/a:
  get:
    responses:
      '200':
        $ref: '#/components/responses/A'
`, `responses:
  A:
    description: OK
    content:
      application/json:
        schema: {$ref: '#/components/schemas/A'}
schemas:
  A:
    type: object
    example: {id: a}
`),
		},
		{
			name: "example in allOf member",
			doc: doc(`/v1/a:
  get:
    responses:
      '200':
        description: OK
        content:
          application/json:
            schema:
              allOf:
              - {$ref: '#/components/schemas/A'}
`, `schemas:
  A: {type: object, examples: [{id: a}]}
`),
		},
		{
			name: "text/plain example",
			doc: doc(`/v1/configurations/{token}:
  get:
    parameters:
    - {name: token, in: path, required: true, schema: {type: string}}
    responses:
      '200':
        description: OK
        headers:
          Subscription-Userinfo: {schema: {type: string}}
        content:
          text/plain:
            schema: {type: string}
            example: "vless://..."
`, ""),
		},
		{
			name: "204 only passes",
			doc: doc(`/v1/me/devices/{id}:
  delete:
    responses:
      '204': {description: No Content}
      default: {$ref: '#/components/responses/Problem'}
`, `responses:
  Problem:
    description: error
    content:
      application/problem+json:
        schema: {type: object}
`),
		},
		{
			name: "missing success example",
			doc: doc(`/v1/a:
  get:
    responses:
      '200':
        description: OK
        content:
          application/json:
            schema: {type: object}
      default:
        description: error
        content:
          application/problem+json:
            schema: {type: object}
            example: {code: internal}
`, ""),
			want: []string{"/paths/~1v1~1a/get: GET /v1/a 的成功响应缺少示例"},
			line: 11,
		},
		{
			name: "only error responses without example",
			doc: doc(`/v1/a:
  post:
    responses:
      default:
        description: error
        content:
          application/json:
            schema: {type: object}
`, ""),
			want: []string{"POST /v1/a 没有任何响应示例"},
		},
		{
			name: "no responses",
			doc:  doc("/v1/a:\n  get: {}\n", ""),
			want: []string{"GET /v1/a 没有 responses"},
		},
		{
			name: "unresolvable response refs are left to redocly",
			doc: doc(`/v1/a:
  get:
    responses:
      '200': {$ref: '#/components/responses/Missing'}
      '201': {$ref: 'other.yaml#/x'}
`, ""),
		},
		{
			name: "forbidden path",
			doc:  doc(strings.Replace(okOp, "/v1/me", "/v1/Nodes", 1), ""),
			want: []string{`/paths/~1v1~1Nodes: 路径含禁用词 "node"`},
			line: 9,
		},
		{
			name: "forbidden property",
			doc: doc(okOp, `schemas:
  Usage:
    type: object
    properties:
      bytes_used: {type: integer}
      traffic_up: {type: integer}
`),
			want: []string{`/components/schemas/Usage/properties/traffic_up: 属性名 "traffic_up" 含禁用词 "traffic"`},
		},
		{
			name: "nested property named properties",
			doc: doc(okOp, `schemas:
  A:
    type: object
    properties:
      properties:
        type: object
        properties:
          server_id: {type: string}
`),
			want: []string{`/components/schemas/A/properties/properties/properties/server_id`},
		},
		{
			name: "forbidden enum",
			doc: doc(okOp, `schemas:
  Code:
    type: string
    enum: [not_found, node_offline]
`),
			want: []string{`/components/schemas/Code/enum/1: 枚举值 "node_offline"`},
		},
		{
			name: "forbidden const",
			doc: doc(okOp, `schemas:
  Code: {const: subscribe_failed}
`),
			want: []string{`常量 "subscribe_failed" 含禁用词 "subscribe"`},
		},
		{
			name: "forbidden query parameter",
			doc: doc(`/v1/locations:
  get:
    parameters:
    - {name: server_id, in: query, schema: {type: string}}
    responses:
      '200':
        description: OK
        content:
          application/json: {example: {}}
`, ""),
			want: []string{`/paths/~1v1~1locations/get/parameters/0/name: 参数名 "server_id"`},
		},
		{
			name: "forbidden component parameter",
			doc: doc(okOp, `parameters:
  NodeId: {name: node_id, in: path, required: true, schema: {type: string}}
`),
			want: []string{`/components/parameters/NodeId/name: 参数名 "node_id"`},
		},
		{
			name: "forbidden response header",
			doc: doc(`/v1/a:
  get:
    responses:
      '200':
        description: OK
        headers:
          X-Traffic-Left: {schema: {type: string}}
        content:
          application/json: {example: {}}
`, ""),
			want: []string{`响应头 "X-Traffic-Left"`},
		},
		{
			name: "descriptions, examples and servers are exempt",
			doc: doc(`/v1/a:
  get:
    summary: list servers
    description: node traffic
    responses:
      '200':
        description: server
        content:
          application/json:
            schema:
              type: object
              description: subscribe
              properties:
                id: {type: string, description: node, default: server, example: traffic}
            example: {node: server}
`, `examples:
  Server: {value: {traffic: 1}}
`),
		},
		{
			name:    "missing x-permission",
			doc:     doc(strings.Replace(okOp, "    x-permission: accounts.read\n", "", 1), ""),
			console: true,
			want:    []string{"GET /v1/me 缺少 x-permission"},
		},
		{
			name:    "x-permission not checked for client",
			doc:     doc(strings.Replace(okOp, "    x-permission: accounts.read\n", "", 1), ""),
			console: false,
		},
		{
			name:    "x-permission unknown",
			doc:     doc(strings.Replace(okOp, "accounts.read", "nodes.write", 1), ""),
			console: true,
			want:    []string{`x-permission "nodes.write" 不在 AUTH-17 目录中`},
			line:    11,
		},
		{
			name:    "x-permission wildcard must match catalog exactly",
			doc:     doc(strings.Replace(okOp, "accounts.read", "accounts.*", 1), ""),
			console: true,
			want:    []string{`"accounts.*" 不在 AUTH-17 目录中`},
		},
		{
			name:    "x-permission none and superadmin",
			doc:     doc(strings.Replace(okOp, "accounts.read", "none", 1)+strings.Replace(strings.Replace(okOp, "/v1/me", "/v1/orders/{id}/mark-paid", 1), "accounts.read", "superadmin", 1), ""),
			console: true,
		},
		{
			name:    "x-permission catalog wildcard",
			doc:     doc(strings.Replace(okOp, "accounts.read", "location-groups.*", 1), ""),
			console: true,
		},
		{
			name:    "sensitive op complete",
			doc:     doc(sensitiveOp("get", true, true, false), sensitiveComponents),
			console: true,
		},
		{
			name:    "sensitive delete complete",
			doc:     doc(sensitiveOp("delete", true, true, true), sensitiveComponents),
			console: true,
		},
		{
			name:    "sensitive op with inline Mfa-Assertion header",
			doc:     doc(strings.Replace(sensitiveOp("get", true, false, false), "    parameters:\n", "    parameters:\n    - {name: Mfa-Assertion, in: header, schema: {type: string}}\n", 1), sensitiveComponents),
			console: true,
		},
		{
			name:    "sensitive op without 401",
			doc:     doc(sensitiveOp("get", false, true, false), sensitiveComponents),
			console: true,
			want:    []string{"GET /v1/me 是敏感操作，401 必须引用 #/components/responses/MfaRequired"},
		},
		{
			name:    "sensitive op with other 401",
			doc:     doc(strings.Replace(sensitiveOp("get", true, true, false), "MfaRequired'", "Problem'", 1), sensitiveComponents+"  Problem:\n    description: x\n"),
			console: true,
			want:    []string{"401 必须引用"},
		},
		{
			name:    "sensitive op without Mfa-Assertion",
			doc:     doc(sensitiveOp("get", true, false, false), sensitiveComponents),
			console: true,
			want:    []string{"必须声明请求头 Mfa-Assertion"},
		},
		{
			name:    "sensitive delete without Audit-Reason",
			doc:     doc(sensitiveOp("delete", true, true, false), sensitiveComponents),
			console: true,
			want:    []string{"DELETE /v1/me 是敏感的 DELETE，必须声明请求头 Audit-Reason"},
		},
		{
			name:    "non-sensitive op must not use MfaRequired",
			doc:     doc(strings.Replace(sensitiveOp("get", true, true, false), "    x-sensitive: true\n", "", 1), sensitiveComponents),
			console: true,
			want:    []string{"GET /v1/me 不是敏感操作，401 不得引用"},
		},
		{
			name:    "x-sensitive false with MfaRequired",
			doc:     doc(strings.Replace(sensitiveOp("get", true, true, false), "x-sensitive: true", "x-sensitive: false", 1), sensitiveComponents),
			console: true,
			want:    []string{"不是敏感操作"},
		},
		{
			name:    "x-sensitive not boolean",
			doc:     doc(strings.Replace(sensitiveOp("get", true, true, false), "x-sensitive: true", "x-sensitive: 'true'", 1), sensitiveComponents),
			console: true,
			want:    []string{"x-sensitive 必须是布尔值"},
		},
		{
			name: "x-sensitive ignored outside console",
			doc:  doc(sensitiveOp("get", false, false, false), ""),
		},
		{
			name:    "x-permission not scalar",
			doc:     doc(strings.Replace(okOp, "x-permission: accounts.read", "x-permission: [accounts.read]", 1), ""),
			console: true,
			want:    []string{"不在 AUTH-17 目录中"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			problems, err := Check("t.yaml", []byte(tt.doc), Options{Permissions: tt.console})
			if err != nil {
				t.Fatal(err)
			}
			var lines []string
			for _, p := range problems {
				lines = append(lines, p.String())
			}
			joined := strings.Join(lines, "\n")
			if tt.want == nil && len(problems) > 0 {
				t.Fatalf("want no problems, got:\n%s\n--- doc ---\n%s", joined, tt.doc)
			}
			if tt.want != nil && len(problems) == 0 {
				t.Fatalf("want problems, got none\n--- doc ---\n%s", tt.doc)
			}
			for _, w := range tt.want {
				if !strings.Contains(joined, w) {
					t.Errorf("want problem containing %q, got:\n%s", w, joined)
				}
			}
			if tt.line != 0 && problems[0].Line != tt.line {
				t.Errorf("want line %d, got %d (%s)", tt.line, problems[0].Line, problems[0])
			}
		})
	}
}

func TestCheckInvalidYAML(t *testing.T) {
	for _, in := range []string{"a: [", "- a\n- b\n"} {
		if _, err := Check("t.yaml", []byte(in), Options{}); err == nil {
			t.Errorf("%q: want error", in)
		}
	}
}

func TestRun(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, content string) string {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	good := write("client/v1.yaml", doc(strings.Replace(okOp, "    x-permission: accounts.read\n", "", 1), ""))
	badConsole := write("console/v1.yaml", doc(strings.Replace(okOp, "    x-permission: accounts.read\n", "", 1), ""))
	goodConsole := write("console2/console/v1.yaml", doc(okOp, ""))
	broken := write("broken.yaml", "a: [")
	missing := filepath.Join(dir, "missing/v1.yaml")

	tests := []struct {
		name  string
		files []string
		want  int
	}{
		{"no args", nil, 2},
		{"good client", []string{good}, 0},
		{"console needs permission", []string{badConsole}, 1},
		{"good console", []string{goodConsole}, 0},
		{"missing file is skipped", []string{missing, good}, 0},
		{"broken yaml", []string{broken}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := run(tt.files); got != tt.want {
				t.Errorf("run(%v) = %d, want %d", tt.files, got, tt.want)
			}
		})
	}
}

func TestPermissionCatalogMatchesSpec(t *testing.T) {
	// AUTH-17 表中共 17 项，全部为 资源.动作 或 资源.* 形式。
	if len(PermissionCatalog) != 17 {
		t.Fatalf("catalog has %d entries, spec/10 AUTH-17 has 17", len(PermissionCatalog))
	}
	seen := map[string]bool{}
	for _, p := range PermissionCatalog {
		res, act, ok := strings.Cut(p, ".")
		if !ok || res == "" || act == "" || seen[p] {
			t.Errorf("bad or duplicate entry %q", p)
		}
		seen[p] = true
	}
}
