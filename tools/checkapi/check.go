// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/akari-project/panel-spec/tools/internal/naming"
)

// PermissionCatalog 是管理接口 x-permission 的合法取值。
// 来源：spec/10 AUTH-17 权限表（资源名使用管理接口路径名，如 hosts）。修改规格后同步此处。
var PermissionCatalog = []string{
	"accounts.read",
	"accounts.adjust",
	"credits.adjust",
	"orders.read",
	"orders.refund",
	"plans.*",
	"location-groups.*",
	"hosts.*",
	"kernels.write",
	"coupons.*",
	"content.*",
	"tickets.*",
	"payments.configure",
	"settings.read",
	"settings.write",
	"staff.*",
	"audit.read",
}

// PermissionSpecial 是目录之外允许的取值：none 表示只需管理员登录，superadmin 表示只允许超级管理员
// （如手动标记支付，spec/10 AUTH-22）。
var PermissionSpecial = []string{"none", "superadmin"}

var methods = []string{"get", "put", "post", "delete", "options", "head", "patch", "trace"}

// Problem 是一条带位置的检查结果。
type Problem struct {
	File string
	Line int
	Col  int
	Path string // JSON Pointer
	Msg  string
}

func (p Problem) String() string {
	return fmt.Sprintf("%s:%d:%d: %s: %s", p.File, p.Line, p.Col, p.Path, p.Msg)
}

// Options 控制启用哪些检查。
type Options struct {
	// Permissions 为 true 时检查每个操作的 x-permission（管理接口）。
	Permissions bool
}

type checker struct {
	file     string
	root     *yaml.Node
	problems []Problem
}

// Check 检查一份 OpenAPI 文档。
func Check(file string, data []byte, opt Options) ([]Problem, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s: 顶层不是映射", file)
	}
	c := &checker{file: file, root: doc.Content[0]}
	c.checkOperations(opt)
	c.walk(c.root, "", true)
	return c.problems, nil
}

func (c *checker) add(n *yaml.Node, ptr, format string, a ...any) {
	c.problems = append(c.problems, Problem{File: c.file, Line: n.Line, Col: n.Column, Path: ptr, Msg: fmt.Sprintf(format, a...)})
}

// get 返回映射中 key 对应的值。
func get(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

func escape(s string) string { return strings.NewReplacer("~", "~0", "/", "~1").Replace(s) }

func unescape(s string) string { return strings.NewReplacer("~1", "/", "~0", "~").Replace(s) }

// resolve 跟随文档内的 $ref（#/...）。外部引用与无法解析的引用返回 nil。
func (c *checker) resolve(n *yaml.Node) *yaml.Node {
	for range 32 {
		if n == nil {
			return nil
		}
		if n.Kind == yaml.AliasNode {
			n = n.Alias
			continue
		}
		r := get(n, "$ref")
		if r == nil {
			return n
		}
		if !strings.HasPrefix(r.Value, "#/") {
			return nil
		}
		cur := c.root
		for _, part := range strings.Split(strings.TrimPrefix(r.Value, "#/"), "/") {
			part = unescape(part)
			switch cur.Kind {
			case yaml.MappingNode:
				cur = get(cur, part)
			case yaml.SequenceNode:
				i, err := strconv.Atoi(part)
				if err != nil || i < 0 || i >= len(cur.Content) {
					return nil
				}
				cur = cur.Content[i]
			default:
				return nil
			}
			if cur == nil {
				return nil
			}
		}
		n = cur
	}
	return nil
}

// checkOperations 做按操作的检查：响应示例、x-permission。
func (c *checker) checkOperations(opt Options) {
	paths := get(c.root, "paths")
	if paths == nil || paths.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(paths.Content); i += 2 {
		path := paths.Content[i].Value
		item := c.resolve(paths.Content[i+1])
		if item == nil || item.Kind != yaml.MappingNode {
			continue
		}
		for j := 0; j+1 < len(item.Content); j += 2 {
			method := item.Content[j].Value
			if !slices.Contains(methods, method) {
				continue
			}
			op := item.Content[j+1]
			ptr := "/paths/" + escape(path) + "/" + method
			label := strings.ToUpper(method) + " " + path
			c.checkExamples(op, ptr, label)
			if opt.Permissions {
				c.checkPermission(op, ptr, label)
			}
		}
	}
}

// checkExamples：每个操作至少有一个响应示例（content 层或 schema 层的 example / examples）。
//   - 有带 content 的成功响应（2xx、3xx）时，示例必须出现在某个成功响应中；
//   - 成功响应都没有 content（如只有 204）时，没有可示例的响应体，视为通过；
//   - 没有成功响应时，任一响应有示例即可。
func (c *checker) checkExamples(op *yaml.Node, ptr, label string) {
	responses := get(op, "responses")
	if responses == nil || responses.Kind != yaml.MappingNode || len(responses.Content) == 0 {
		c.add(op, ptr, "%s 没有 responses", label)
		return
	}
	var successWithContent, success, anyExample, successExample bool
	for i := 0; i+1 < len(responses.Content); i += 2 {
		code := responses.Content[i].Value
		resp := c.resolve(responses.Content[i+1])
		isSuccess := strings.HasPrefix(code, "2") || strings.HasPrefix(code, "3")
		content := get(resp, "content")
		hasContent := content != nil && content.Kind == yaml.MappingNode && len(content.Content) > 0
		ex := hasContent && c.contentHasExample(content)
		anyExample = anyExample || ex
		if isSuccess {
			success = true
			successWithContent = successWithContent || hasContent
			successExample = successExample || ex
		}
	}
	switch {
	case successWithContent && !successExample:
		c.add(op, ptr, "%s 的成功响应缺少示例（在 content 或 schema 层提供 example 或 examples）", label)
	case !success && !anyExample:
		c.add(op, ptr, "%s 没有任何响应示例", label)
	}
}

func (c *checker) contentHasExample(content *yaml.Node) bool {
	for i := 0; i+1 < len(content.Content); i += 2 {
		media := c.resolve(content.Content[i+1])
		if nonEmpty(get(media, "example")) || nonEmpty(get(media, "examples")) {
			return true
		}
		if c.schemaHasExample(get(media, "schema"), 0) {
			return true
		}
	}
	return false
}

func (c *checker) schemaHasExample(s *yaml.Node, depth int) bool {
	if depth > 16 {
		return false
	}
	s = c.resolve(s)
	if s == nil {
		return false
	}
	if nonEmpty(get(s, "example")) || nonEmpty(get(s, "examples")) {
		return true
	}
	for _, k := range []string{"allOf", "oneOf", "anyOf"} {
		if list := get(s, k); list != nil && list.Kind == yaml.SequenceNode {
			for _, sub := range list.Content {
				if c.schemaHasExample(sub, depth+1) {
					return true
				}
			}
		}
	}
	return false
}

func nonEmpty(n *yaml.Node) bool {
	if n == nil {
		return false
	}
	switch n.Kind {
	case yaml.MappingNode, yaml.SequenceNode:
		return len(n.Content) > 0
	case yaml.ScalarNode:
		return n.Tag != "!!null"
	}
	return true
}

func (c *checker) checkPermission(op *yaml.Node, ptr, label string) {
	p := get(op, "x-permission")
	if p == nil {
		c.add(op, ptr, "%s 缺少 x-permission（spec/10 AUTH-17）", label)
		return
	}
	if p.Kind != yaml.ScalarNode || (!slices.Contains(PermissionCatalog, p.Value) && !slices.Contains(PermissionSpecial, p.Value)) {
		c.add(p, ptr+"/x-permission", "%s 的 x-permission %q 不在 AUTH-17 目录中，也不是 none 或 superadmin", label, p.Value)
	}
}

// skipKeys 是不参与禁用词检查的关键字：描述性文字、示例、默认值与 servers。
var skipKeys = []string{
	"description", "summary", "title", "example", "examples", "default",
	"externalDocs", "servers", "operationId", "tags", "info", "$ref", "required",
}

// walk 做禁用词检查（spec/30 API-01）：路径、属性名、枚举值与常量、参数名、响应头名。
func (c *checker) walk(n *yaml.Node, ptr string, top bool) {
	switch n.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			key := k.Value
			p := ptr + "/" + escape(key)
			if slices.Contains(skipKeys, key) || strings.HasPrefix(key, "x-") {
				continue
			}
			switch {
			case top && key == "paths" && v.Kind == yaml.MappingNode:
				for j := 0; j+1 < len(v.Content); j += 2 {
					pk := v.Content[j]
					if w := naming.Forbidden(pk.Value); w != "" {
						c.add(pk, p+"/"+escape(pk.Value), "路径含禁用词 %q（spec/30 API-01）", w)
					}
					c.walk(v.Content[j+1], p+"/"+escape(pk.Value), false)
				}
				continue
			case key == "properties" && v.Kind == yaml.MappingNode:
				for j := 0; j+1 < len(v.Content); j += 2 {
					nk := v.Content[j]
					np := p + "/" + escape(nk.Value)
					if w := naming.Forbidden(nk.Value); w != "" {
						c.add(nk, np, "属性名 %q 含禁用词 %q（spec/30 API-01）", nk.Value, w)
					}
					c.walk(v.Content[j+1], np, false)
				}
				continue
			case key == "enum" && v.Kind == yaml.SequenceNode:
				for j, e := range v.Content {
					if e.Kind == yaml.ScalarNode {
						if w := naming.Forbidden(e.Value); w != "" {
							c.add(e, fmt.Sprintf("%s/%d", p, j), "枚举值 %q 含禁用词 %q（spec/30 API-01）", e.Value, w)
						}
					}
				}
				continue
			case key == "const" && v.Kind == yaml.ScalarNode:
				if w := naming.Forbidden(v.Value); w != "" {
					c.add(v, p, "常量 %q 含禁用词 %q（spec/30 API-01）", v.Value, w)
				}
				continue
			case key == "parameters":
				c.checkParameters(v, p)
				continue
			case key == "headers" && v.Kind == yaml.MappingNode:
				for j := 0; j+1 < len(v.Content); j += 2 {
					hk := v.Content[j]
					hp := p + "/" + escape(hk.Value)
					if w := naming.Forbidden(hk.Value); w != "" && !naming.IsAllowedHeader(hk.Value) {
						c.add(hk, hp, "响应头 %q 含禁用词 %q（spec/30 API-01）", hk.Value, w)
					}
					c.walk(v.Content[j+1], hp, false)
				}
				continue
			}
			c.walk(v, p, false)
		}
	case yaml.SequenceNode:
		for i, e := range n.Content {
			c.walk(e, fmt.Sprintf("%s/%d", ptr, i), false)
		}
	}
}

// checkParameters 处理操作中的参数列表，以及 components.parameters 映射。
func (c *checker) checkParameters(v *yaml.Node, ptr string) {
	var items []*yaml.Node
	var ptrs []string
	switch v.Kind {
	case yaml.SequenceNode:
		for i, e := range v.Content {
			items = append(items, e)
			ptrs = append(ptrs, fmt.Sprintf("%s/%d", ptr, i))
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(v.Content); i += 2 {
			items = append(items, v.Content[i+1])
			ptrs = append(ptrs, ptr+"/"+escape(v.Content[i].Value))
		}
	default:
		return
	}
	for i, it := range items {
		if name := get(it, "name"); name != nil && name.Kind == yaml.ScalarNode {
			in := get(it, "in")
			allowed := in != nil && in.Value == "header" && naming.IsAllowedHeader(name.Value)
			if w := naming.Forbidden(name.Value); w != "" && !allowed {
				c.add(name, ptrs[i]+"/name", "参数名 %q 含禁用词 %q（spec/30 API-01）", name.Value, w)
			}
		}
		c.walk(it, ptrs[i], false)
	}
}
