// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const realDir = "../../schemas/inbound"

func TestRealSchemasPass(t *testing.T) {
	problems, n, err := Check(realDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) > 0 {
		t.Fatalf("problems:\n%s", strings.Join(problems, "\n"))
	}
	if n == 0 {
		t.Fatal("no schemas")
	}
}

const okSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "additionalProperties": false,
  "properties": {"path": {"type": "string"}},
  "x-kernels": {"singbox": "stable", "xray": "unsupported"}
}`

func TestCheck(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  []string // 每一项必须出现在某条问题中；nil 表示应当通过
	}{
		{
			name:  "ok",
			files: map[string]string{"vless-ws.schema.json": okSchema, "examples/vless-ws.json": `{"path": "/a"}`},
		},
		{
			name:  "variant example counts",
			files: map[string]string{"vless-ws.schema.json": okSchema, "examples/vless-ws.cdn.json": `{}`},
		},
		{
			name:  "bad file name",
			files: map[string]string{"VlessWs.json": okSchema},
			want:  []string{"文件名必须为"},
		},
		{
			name:  "unknown protocol",
			files: map[string]string{"wireguard-tcp.schema.json": okSchema, "examples/wireguard-tcp.json": `{}`},
			want:  []string{`"wireguard" 不是 proto Protocol`},
		},
		{
			name:  "unknown transport",
			files: map[string]string{"vless-h2.schema.json": okSchema, "examples/vless-h2.json": `{}`},
			want:  []string{`"h2" 不是 proto Transport`},
		},
		{
			name:  "unspecified is not a value",
			files: map[string]string{"unspecified-tcp.schema.json": okSchema, "examples/unspecified-tcp.json": `{}`},
			want:  []string{`"unspecified" 不是 proto Protocol`},
		},
		{
			name: "wrong draft",
			files: map[string]string{
				"vless-ws.schema.json":   strings.Replace(okSchema, "2020-12", "2019-09", 1),
				"examples/vless-ws.json": `{}`,
			},
			want: []string{"$schema 必须为"},
		},
		{
			name: "invalid schema",
			files: map[string]string{
				"vless-ws.schema.json":   strings.Replace(okSchema, `"type": "object"`, `"type": 5`, 1),
				"examples/vless-ws.json": `{}`,
			},
			want: []string{"不是合法的 JSON Schema"},
		},
		{
			name:  "not json",
			files: map[string]string{"vless-ws.schema.json": `{`},
			want:  []string{"不是合法的 JSON 对象"},
		},
		{
			name: "forbidden property",
			files: map[string]string{
				"vless-ws.schema.json":   strings.Replace(okSchema, `"path"`, `"server_name"`, 1),
				"examples/vless-ws.json": `{}`,
			},
			want: []string{`/properties/server_name: 属性名含禁用词 "server"`},
		},
		{
			name: "forbidden enum",
			files: map[string]string{
				"vless-ws.schema.json":   strings.Replace(okSchema, `{"type": "string"}`, `{"enum": ["a", "Traffic"]}`, 1),
				"examples/vless-ws.json": `{}`,
			},
			want: []string{`/properties/path/enum/1: 枚举值含禁用词 "traffic"`},
		},
		{
			name: "forbidden const",
			files: map[string]string{
				"vless-ws.schema.json":   strings.Replace(okSchema, `{"type": "string"}`, `{"const": "node"}`, 1),
				"examples/vless-ws.json": `{}`,
			},
			want: []string{`常量含禁用词 "node"`},
		},
		{
			name: "forbidden word in description is fine",
			files: map[string]string{
				"vless-ws.schema.json":   strings.Replace(okSchema, `{"type": "string"}`, `{"type": "string", "description": "server node traffic"}`, 1),
				"examples/vless-ws.json": `{}`,
			},
		},
		{
			name: "nested credential property",
			files: map[string]string{
				"vless-ws.schema.json":   strings.Replace(okSchema, `{"type": "string"}`, `{"type": "object", "properties": {"UUID": {"type": "string"}}}`, 1),
				"examples/vless-ws.json": `{}`,
			},
			want: []string{"/properties/path/properties/UUID: 用户凭据不得出现"},
		},
		{
			name: "no usable kernel",
			files: map[string]string{
				"vless-ws.schema.json":   strings.Replace(okSchema, `"stable"`, `"unsupported"`, 1),
				"examples/vless-ws.json": `{}`,
			},
			want: []string{"两个内核都不支持"},
		},
		{
			name: "bad kernel status",
			files: map[string]string{
				"vless-ws.schema.json":   strings.Replace(okSchema, `"stable"`, `"beta"`, 1),
				"examples/vless-ws.json": `{}`,
			},
			want: []string{"x-kernels.singbox 必须为"},
		},
		{
			name: "missing x-kernels",
			files: map[string]string{
				"vless-ws.schema.json":   strings.Replace(okSchema, `"x-kernels"`, `"x-other"`, 1),
				"examples/vless-ws.json": `{}`,
			},
			want: []string{"缺少 x-kernels"},
		},
		{
			name:  "missing example",
			files: map[string]string{"vless-ws.schema.json": okSchema},
			want:  []string{"缺少通过校验的示例 examples/vless-ws.json"},
		},
		{
			name:  "invalid example",
			files: map[string]string{"vless-ws.schema.json": okSchema, "examples/vless-ws.json": `{"path": 1}`},
			want:  []string{"examples/vless-ws.json: 未通过", "缺少通过校验的示例"},
		},
		{
			name:  "orphan example",
			files: map[string]string{"vless-ws.schema.json": okSchema, "examples/vless-ws.json": `{}`, "examples/vmess-ws.json": `{}`},
			want:  []string{"examples/vmess-ws.json: 没有对应的 schema"},
		},
		{
			name:  "empty dir",
			files: map[string]string{},
			want:  []string{"没有 schema"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, content := range tt.files {
				p := filepath.Join(dir, name)
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			problems, _, err := Check(dir)
			if err != nil {
				t.Fatal(err)
			}
			joined := strings.Join(problems, "\n")
			if tt.want == nil && len(problems) > 0 {
				t.Fatalf("want no problems, got:\n%s", joined)
			}
			if tt.want != nil && len(problems) == 0 {
				t.Fatal("want problems, got none")
			}
			for _, w := range tt.want {
				if !strings.Contains(joined, w) {
					t.Errorf("want problem containing %q, got:\n%s", w, joined)
				}
			}
		})
	}
}

// TestInvalidInstances：testdata/invalid/<组合>.<用例>.json 必须被对应的真实 schema 拒绝。
func TestInvalidInstances(t *testing.T) {
	files, err := filepath.Glob("testdata/invalid/*.json")
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	if len(files) == 0 {
		t.Fatal("no fixtures")
	}
	c := jsonschema.NewCompiler()
	c.AssertFormat()
	for _, f := range files {
		base := strings.TrimSuffix(filepath.Base(f), ".json")
		combo, _, _ := strings.Cut(base, ".")
		t.Run(base, func(t *testing.T) {
			sch, err := c.Compile(filepath.Join(realDir, combo+".schema.json"))
			if err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
			if err != nil {
				t.Fatal(err)
			}
			if err := sch.Validate(inst); err == nil {
				t.Fatalf("%s should be rejected by %s.schema.json", f, combo)
			}
		})
	}
}
