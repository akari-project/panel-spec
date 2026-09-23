// SPDX-License-Identifier: Apache-2.0

// Command checkapi 对 OpenAPI 文档做 Redocly 覆盖不到的项目约定检查（spec/42 42.2）：
//
//   - 每个操作至少一个响应示例（content 层或 schema 层的 example / examples），供 Mock 服务使用；
//   - 禁用词（spec/30 API-01）：路径、属性名、枚举值与常量、参数名、响应头名中不得出现
//     subscribe、server、node、traffic（不区分大小写）；描述文字、servers 与响应头 Subscription-Userinfo 除外；
//   - 管理接口（位于 console/ 目录下的文件）的每个操作都有 x-permission，取值来自 spec/10 AUTH-17 目录，
//     或为 none、superadmin。
//
// 用法：go run ./tools/checkapi openapi/client/v1.yaml openapi/console/v1.yaml
// 不存在的文件跳过并提示；任一检查失败时以非零退出码结束。
package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(files []string) int {
	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "用法：checkapi <openapi.yaml>...")
		return 2
	}
	failed, checked := 0, 0
	for _, f := range files {
		data, err := os.ReadFile(f)
		if errors.Is(err, fs.ErrNotExist) {
			fmt.Printf("checkapi: 跳过 %s（文件不存在）\n", f)
			continue
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "checkapi:", err)
			return 2
		}
		problems, err := Check(f, data, Options{Permissions: isConsole(f)})
		if err != nil {
			fmt.Fprintln(os.Stderr, "checkapi:", err)
			return 2
		}
		checked++
		for _, p := range problems {
			fmt.Fprintln(os.Stderr, p)
		}
		failed += len(problems)
	}
	if failed > 0 {
		fmt.Fprintf(os.Stderr, "checkapi: %d 个问题\n", failed)
		return 1
	}
	fmt.Printf("checkapi: %d 份文档通过\n", checked)
	return 0
}

func isConsole(f string) bool {
	return filepath.Base(filepath.Dir(f)) == "console"
}
