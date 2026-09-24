# panel-spec

协议与接口定义，Apache-2.0。只有定义与生成代码，没有业务逻辑。

## 文件
- `proto/node/v1/`：节点协议（spec/20）。`envelope.proto` 为帧、握手、信封；`messages.proto` 为业务消息；`common.proto` 为枚举；`enrollment.proto` 为接入接口。
- `openapi/client/v1.yaml`：客户端接口（spec/30）。
- `openapi/console/v1.yaml`：管理接口（spec/31，M0-03 编写）。每个操作带 `x-permission`（spec/10 AUTH-17）。
- `schemas/inbound/`：入站 `settings_json` 的 JSON Schema，按 `<protocol>-<transport>.schema.json` 命名（spec/21 AGT-13）；`examples/` 为每个 schema 的合法示例（手写，`<组合>.json` 或 `<组合>.<变体>.json`）。schema 与 `README.md` 由 `tools/schemagen` 生成，禁止手改。
- `gen/`：生成代码，禁止手改。`gen/go/` 来自 proto，`gen/ts/client.d.ts`、`gen/ts/console.d.ts` 来自 OpenAPI。
- `testdata/node-v1-vectors.json`：握手、会话加密与代理凭据各协议形式等测试向量，由 `tools/vectors` 生成。
- `testdata/mkcp-finalmask.json`：mKCP `finalmask` 与 Xray-core `streamSettings.finalmask.udp` 的对照用例（手写，`tools/checkschema` 测试）。
- `tools/schemagen`：入站 schema 的生成源。组合矩阵在 `matrix.go`（抄自 spec/21 21.2），字段在 `main.go`。
- `tools/checkapi`：OpenAPI 检查：每个操作有响应示例、禁用词（spec/30 API-01）、管理接口 `x-permission`（目录硬编码，来源 spec/10 AUTH-17，规格变更时同步）。
- `tools/checkschema`：校验 schema 合法、文件名与 proto 枚举一致、无禁用词与凭据字段、示例通过校验；`testdata/invalid/` 为必须被拒绝的实例。
- `LICENSE`：Apache-2.0 全文，与 `LICENSES/Apache-2.0.txt` 相同，供 go-licenses 等只认根目录许可证文件的工具识别。
- `REUSE.toml`、`LICENSES/`：REUSE 登记。新源文件在前两行内写 SPDX 许可证标识（Apache-2.0，CONV-25）。

## 命令
- `make tools`：安装与 go.mod 中 `google.golang.org/protobuf` 同版本的 `protoc-gen-go`。
- `make gen`：`buf generate`、`tools/schemagen`、`openapi-typescript` 生成 `gen/ts/*.d.ts`。
- `make vectors`：重新生成测试向量。
- `make lint`：buf lint、两份 OpenAPI 的 Redocly lint、`checkapi`、`checkschema`、SPDX 头检查。
- `make breaking`：`buf breaking` 与 oasdiff 都对比 HEAD 之前的最近一个 tag（在 tag 提交上构建时也不会与自己比较）；还没有更早的 tag 时，`buf breaking` 对比 main 分支（`BREAKING_AGAINST`），oasdiff 跳过。
- `make test`：`go test -race ./...`。
- `make vulncheck`：govulncheck。
- `make licenses`：go-licenses 按 spec/42 42.2 允许清单（MIT、BSD-2-Clause、BSD-3-Clause、Apache-2.0、ISC）检查 Go 依赖；npm 工具只在构建期经 npx 使用，不在扫描范围内。
- `make ci`：lint、licenses、breaking、test、gen、vectors，最后检查 `gen/`、`testdata/`、`schemas/` 无差异且无未提交的生成文件。
- OpenAPI 文件缺失时，相关步骤跳过并提示。工具版本固定在 `Makefile` 顶部。

## 规则
- ENG-01、ENG-02：字段只增不改；删除编号写入 `reserved`；新行为配能力位。
- spec/02 与 spec/30 API-01 的命名约定；每个 OpenAPI 操作至少一个响应示例。
- 修改后使用 protocol-reviewer 审查。
