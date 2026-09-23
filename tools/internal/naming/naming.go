// SPDX-License-Identifier: Apache-2.0

// Package naming 保存对外命名约束，供 checkapi 与 checkschema 共用。
package naming

import "strings"

// ForbiddenWords 是对外标识符中禁止出现的词，不区分大小写，按子串匹配。
// 来源：spec/30 API-01。
var ForbiddenWords = []string{"subscribe", "server", "node", "traffic"}

// AllowedHeaders 是 API-01 唯一的例外：第三方客户端依赖的标准响应头。
var AllowedHeaders = []string{"Subscription-Userinfo"}

// Forbidden 返回 s 中出现的第一个禁用词；没有时返回空串。
func Forbidden(s string) string {
	l := strings.ToLower(s)
	for _, w := range ForbiddenWords {
		if strings.Contains(l, w) {
			return w
		}
	}
	return ""
}

// IsAllowedHeader 判断 name 是否为 API-01 允许的例外响应头（不区分大小写）。
func IsAllowedHeader(name string) bool {
	for _, h := range AllowedHeaders {
		if strings.EqualFold(h, name) {
			return true
		}
	}
	return false
}
