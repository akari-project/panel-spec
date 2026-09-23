// SPDX-License-Identifier: Apache-2.0

// Command checkschema 校验 schemas/inbound 下的入站 settings JSON Schema（spec/21 AGT-13）：
//
//   - 文件名为 <protocol>-<transport>.schema.json，两部分是 proto 枚举 Protocol、Transport 的小写形式；
//   - 是合法的 JSON Schema draft 2020-12（按元 schema 校验并能编译）；
//   - x-kernels 注解中至少一个内核为 stable 或 experimental；
//   - 属性名与枚举值不含 spec/30 API-01 的禁用词，也不含用户凭据字段（凭据由 Credential 下发）；
//   - examples/ 中每个 schema 至少一个示例（<组合>.json 或 <组合>.<变体>.json），且示例通过校验（含 format）。
//
// 用法：go run ./tools/checkschema [schemas/inbound]
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"

	nodev1 "github.com/akari-project/panel-spec/gen/go/node/v1"
	"github.com/akari-project/panel-spec/tools/internal/naming"
)

const draft2020 = "https://json-schema.org/draft/2020-12/schema"

var fileRE = regexp.MustCompile(`^([a-z0-9]+)-([a-z0-9]+)\.schema\.json$`)

// credentialFields 是不得出现在 settings 中的属性名：用户凭据由 Credential 下发（spec/21 AGT-13）。
var credentialFields = []string{
	"alter_id", "auth", "clients", "credential", "credentials", "email",
	"id", "password", "passwd", "token", "user", "users", "uuid",
}

func main() {
	dir := "schemas/inbound"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	problems, n, err := Check(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "checkschema:", err)
		os.Exit(2)
	}
	for _, p := range problems {
		fmt.Fprintln(os.Stderr, p)
	}
	if len(problems) > 0 {
		fmt.Fprintf(os.Stderr, "checkschema: %d 个问题\n", len(problems))
		os.Exit(1)
	}
	fmt.Printf("checkschema: %d 个 schema 通过\n", n)
}

// Check 校验 dir，返回问题列表与 schema 数量。
func Check(dir string) ([]string, int, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, 0, err
	}
	sort.Strings(files)
	var problems []string
	add := func(format string, a ...any) { problems = append(problems, fmt.Sprintf(format, a...)) }

	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()

	schemas := map[string]*jsonschema.Schema{}
	for _, f := range files {
		base := filepath.Base(f)
		m := fileRE.FindStringSubmatch(base)
		if m == nil {
			add("%s: 文件名必须为 <protocol>-<transport>.schema.json", f)
			continue
		}
		if !validEnum(nodev1.Protocol_value, "PROTOCOL_", m[1]) {
			add("%s: %q 不是 proto Protocol 枚举的小写形式", f, m[1])
		}
		if !validEnum(nodev1.Transport_value, "TRANSPORT_", m[2]) {
			add("%s: %q 不是 proto Transport 枚举的小写形式", f, m[2])
		}
		raw, err := os.ReadFile(f)
		if err != nil {
			return nil, 0, err
		}
		var doc map[string]any
		if err := strictUnmarshal(raw, &doc); err != nil {
			add("%s: 不是合法的 JSON 对象：%v", f, err)
			continue
		}
		if doc["$schema"] != draft2020 {
			add("%s: $schema 必须为 %s", f, draft2020)
		}
		problems = append(problems, checkKernels(f, doc)...)
		problems = append(problems, checkNames(f, doc)...)

		inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			add("%s: %v", f, err)
			continue
		}
		loc := "file://" + filepath.ToSlash(mustAbs(f))
		if err := compiler.AddResource(loc, inst); err != nil {
			add("%s: %v", f, err)
			continue
		}
		sch, err := compiler.Compile(loc)
		if err != nil {
			add("%s: 不是合法的 JSON Schema：%v", f, err)
			continue
		}
		schemas[strings.TrimSuffix(base, ".schema.json")] = sch
	}
	if len(files) == 0 {
		add("%s: 没有 schema", dir)
	}

	examples, err := filepath.Glob(filepath.Join(dir, "examples", "*.json"))
	if err != nil {
		return nil, 0, err
	}
	sort.Strings(examples)
	covered := map[string]bool{}
	for _, e := range examples {
		combo, _, _ := strings.Cut(strings.TrimSuffix(filepath.Base(e), ".json"), ".")
		sch, ok := schemas[combo]
		if !ok {
			add("%s: 没有对应的 schema %s.schema.json", e, combo)
			continue
		}
		raw, err := os.ReadFile(e)
		if err != nil {
			return nil, 0, err
		}
		inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			add("%s: 不是合法的 JSON：%v", e, err)
			continue
		}
		if err := sch.Validate(inst); err != nil {
			add("%s: 未通过 %s.schema.json：%v", e, combo, err)
			continue
		}
		covered[combo] = true
	}
	for combo := range schemas {
		if !covered[combo] {
			add("%s: 缺少通过校验的示例 examples/%s.json", filepath.Join(dir, combo+".schema.json"), combo)
		}
	}
	sort.Strings(problems)
	return problems, len(schemas), nil
}

func validEnum(values map[string]int32, prefix, lower string) bool {
	v, ok := values[prefix+strings.ToUpper(lower)]
	return ok && v != 0
}

func strictUnmarshal(raw []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	return dec.Decode(v)
}

func mustAbs(p string) string {
	a, err := filepath.Abs(p)
	if err != nil {
		panic(err)
	}
	return a
}

func checkKernels(f string, doc map[string]any) []string {
	k, ok := doc["x-kernels"].(map[string]any)
	if !ok {
		return []string{fmt.Sprintf("%s: 缺少 x-kernels 注解", f)}
	}
	var problems []string
	usable := false
	for _, kernel := range []string{"singbox", "xray"} {
		s, _ := k[kernel].(string)
		switch s {
		case "stable", "experimental":
			usable = true
		case "unsupported":
		default:
			problems = append(problems, fmt.Sprintf("%s: x-kernels.%s 必须为 stable、experimental 或 unsupported", f, kernel))
		}
	}
	if !usable {
		problems = append(problems, fmt.Sprintf("%s: 两个内核都不支持的组合不应有 schema（spec/21 21.2）", f))
	}
	return problems
}

// checkNames 遍历 schema，检查 properties 的键与 enum、const 的字符串值。
func checkNames(f string, doc any) []string {
	var problems []string
	var walk func(v any, ptr string)
	walk = func(v any, ptr string) {
		switch t := v.(type) {
		case map[string]any:
			for _, key := range sortedKeys(t) {
				child := t[key]
				p := ptr + "/" + escape(key)
				switch key {
				case "properties", "patternProperties":
					if m, ok := child.(map[string]any); ok {
						for _, name := range sortedKeys(m) {
							np := p + "/" + escape(name)
							if w := naming.Forbidden(name); w != "" {
								problems = append(problems, fmt.Sprintf("%s#%s: 属性名含禁用词 %q（spec/30 API-01）", f, np, w))
							}
							if slices.Contains(credentialFields, strings.ToLower(name)) {
								problems = append(problems, fmt.Sprintf("%s#%s: 用户凭据不得出现在 settings 中，由 Credential 下发（spec/21 AGT-13）", f, np))
							}
							walk(m[name], np)
						}
						continue
					}
				case "enum":
					if arr, ok := child.([]any); ok {
						for i, e := range arr {
							if s, ok := e.(string); ok {
								if w := naming.Forbidden(s); w != "" {
									problems = append(problems, fmt.Sprintf("%s#%s/%d: 枚举值含禁用词 %q（spec/30 API-01）", f, p, i, w))
								}
							}
						}
					}
				case "const":
					if s, ok := child.(string); ok {
						if w := naming.Forbidden(s); w != "" {
							problems = append(problems, fmt.Sprintf("%s#%s: 常量含禁用词 %q（spec/30 API-01）", f, p, w))
						}
					}
				case "description", "title", "$comment", "examples", "default", "$id", "$schema":
					continue
				}
				walk(child, p)
			}
		case []any:
			for i, e := range t {
				walk(e, fmt.Sprintf("%s/%d", ptr, i))
			}
		}
	}
	walk(doc, "")
	return problems
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func escape(s string) string {
	return strings.NewReplacer("~", "~0", "/", "~1").Replace(s)
}
