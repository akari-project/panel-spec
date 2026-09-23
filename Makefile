# SPDX-License-Identifier: Apache-2.0
#
# panel-spec 的构建与检查（spec/42 42.2）。本地与 CI 执行同一目标：make ci。
# OpenAPI 文件缺失时，相关步骤跳过并提示。

SHELL := /usr/bin/env bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := ci

# 固定的工具版本。
REDOCLY_VERSION           := 2.54.2
OPENAPI_TYPESCRIPT_VERSION := 7.13.0
OASDIFF_VERSION           := v1.32.1
GOVULNCHECK_VERSION       := v1.8.0
GO_LICENSES_VERSION       := v2.0.1
# protoc-gen-go 与运行时同版本，取自 go.mod。
PROTOBUF_VERSION = $(shell go list -m -f '{{.Version}}' google.golang.org/protobuf)

REDOCLY := npx --yes @redocly/cli@$(REDOCLY_VERSION)
OPENAPI_TS := npx --yes openapi-typescript@$(OPENAPI_TYPESCRIPT_VERSION)
OASDIFF := go run github.com/oasdiff/oasdiff@$(OASDIFF_VERSION)

# 名称:路径。名称用于 gen/ts/<名称>.d.ts。
OPENAPI_SPECS := client:openapi/client/v1.yaml console:openapi/console/v1.yaml
OPENAPI_FILES := $(foreach s,$(OPENAPI_SPECS),$(word 2,$(subst :, ,$(s))))

BREAKING_AGAINST ?= .git\#branch=main
GENERATED := gen/ testdata/ schemas/

.PHONY: tools gen gen-proto gen-schemas gen-ts vectors lint lint-proto lint-openapi checkapi checkschema \
	check-spdx vulncheck licenses breaking breaking-proto breaking-openapi test ci check-generated

tools:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOBUF_VERSION)

## 生成 ---------------------------------------------------------------

gen: gen-proto gen-schemas gen-ts

gen-proto:
	buf generate

gen-schemas:
	go run ./tools/schemagen -out schemas/inbound

gen-ts:
	@mkdir -p gen/ts
	@for s in $(OPENAPI_SPECS); do \
	  name="$${s%%:*}"; file="$${s#*:}"; \
	  if [ ! -f "$$file" ]; then echo "gen-ts: 跳过 $$file（文件不存在）"; continue; fi; \
	  echo "gen-ts: $$file -> gen/ts/$$name.d.ts"; \
	  $(OPENAPI_TS) "$$file" -o "gen/ts/$$name.d.ts"; \
	done

vectors:
	@tmp="$$(mktemp)"; trap 'rm -f "$$tmp"' EXIT; \
	go run ./tools/vectors > "$$tmp"; \
	mv "$$tmp" testdata/node-v1-vectors.json; trap - EXIT

## 检查 ---------------------------------------------------------------

lint: lint-proto lint-openapi checkapi checkschema check-spdx

lint-proto:
	buf lint

lint-openapi:
	@for f in $(OPENAPI_FILES); do \
	  if [ ! -f "$$f" ]; then echo "lint-openapi: 跳过 $$f（文件不存在）"; continue; fi; \
	  $(REDOCLY) lint "$$f"; \
	done

checkapi:
	@files=""; for f in $(OPENAPI_FILES); do \
	  if [ -f "$$f" ]; then files="$$files $$f"; else echo "checkapi: 跳过 $$f（文件不存在）"; fi; \
	done; \
	if [ -z "$$files" ]; then echo "checkapi: 没有 OpenAPI 文件，跳过"; exit 0; fi; \
	go run ./tools/checkapi $$files

checkschema:
	go run ./tools/checkschema schemas/inbound

# 源文件前两行内必须有 SPDX 标识（CONV-25）。无法加头的文件登记在 REUSE.toml；完整检查由 CI 的 REUSE lint 执行。
check-spdx:
	@missing="$$(find . -path ./.git -prune -o -path ./gen -prune -o -path ./.claude -prune -o -path '*/testdata' -prune -o -path ./schemas -prune -o -path '*/node_modules' -prune -o \
	  -type f \( -name '*.go' -o -name '*.proto' -o -name '*.yaml' -o -name '*.yml' -o -name '*.sh' -o -name '*.toml' -o -name Makefile \) -print \
	  | while read -r f; do head -n 2 "$$f" | grep -q 'SPDX-License-Identif[i]er:' || echo "$$f"; done)"; \
	if [ -n "$$missing" ]; then echo "缺少 SPDX 头："; echo "$$missing"; exit 1; fi; \
	echo "check-spdx: 通过"

vulncheck:
	go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

# 依赖许可证扫描（spec/42 42.2，Apache-2.0 仓库的允许清单）。本模块自身不参与判定。
# 范围只含 Go 依赖：npm 工具（Redocly、openapi-typescript）只在构建期经 npx 使用，不进入产物，不在扫描范围内。
ALLOWED_LICENSES := MIT,BSD-2-Clause,BSD-3-Clause,Apache-2.0,ISC
licenses:
	go run github.com/google/go-licenses/v2@$(GO_LICENSES_VERSION) check ./... \
	  --allowed_licenses=$(ALLOWED_LICENSES) --ignore github.com/akari-project/panel-spec

breaking: breaking-proto breaking-openapi

# 基准为 HEAD 之前的最近一个 tag（在 tag 提交上构建时也不会与自己比较）；
# 还没有任何 tag 时退回 BREAKING_AGAINST（默认 main 分支）。
PREV_TAG = $(shell git describe --tags --abbrev=0 HEAD^ 2>/dev/null)

breaking-proto:
	@if [ -n "$(PREV_TAG)" ]; then \
	  echo "breaking-proto: 对比 $(PREV_TAG)"; buf breaking --against '.git#tag=$(PREV_TAG)'; \
	else \
	  echo "breaking-proto: 没有更早的 tag，对比 $(BREAKING_AGAINST)"; buf breaking --against '$(BREAKING_AGAINST)'; \
	fi

# 与 HEAD 之前的最近一个 tag 对比；没有 tag，或该 tag 中没有对应文件时跳过。
breaking-openapi:
	@tag="$(PREV_TAG)"; \
	if [ -z "$$tag" ]; then echo "breaking-openapi: 没有 tag，跳过 oasdiff"; exit 0; fi; \
	tmp="$$(mktemp -d)"; trap 'rm -rf "$$tmp"' EXIT; \
	for f in $(OPENAPI_FILES); do \
	  if [ ! -f "$$f" ]; then echo "breaking-openapi: 跳过 $$f（文件不存在）"; continue; fi; \
	  if ! git cat-file -e "$$tag:$$f" 2>/dev/null; then echo "breaking-openapi: 跳过 $$f（$$tag 中不存在）"; continue; fi; \
	  base="$$tmp/$$(echo "$$f" | tr / _)"; git show "$$tag:$$f" > "$$base"; \
	  echo "breaking-openapi: $$f 对比 $$tag"; \
	  $(OASDIFF) breaking "$$base" "$$f" --fail-on ERR; \
	done

test:
	go test -race ./...

## CI -----------------------------------------------------------------

ci: lint licenses breaking test gen vectors check-generated

# 生成物必须已提交且与源一致：既检查已跟踪文件的差异，也检查未跟踪的新文件。
check-generated:
	git diff --exit-code -- $(GENERATED)
	@untracked="$$(git ls-files --others --exclude-standard -- $(GENERATED))"; \
	if [ -n "$$untracked" ]; then echo "未提交的生成文件："; echo "$$untracked"; exit 1; fi
