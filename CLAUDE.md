# panel-spec

协议与接口定义，Apache-2.0。只有定义与生成代码，没有业务逻辑。

## 文件
- `proto/node/v1/`：节点协议（spec/20）。`envelope.proto` 为帧、握手、信封；`messages.proto` 为业务消息；`common.proto` 为枚举。
- `openapi/client/v1.yaml`：客户端接口（spec/30）。
- `openapi/console/v1.yaml`：管理接口（spec/31，M0-03 编写）。
- `gen/`：生成代码，禁止手改。

## 命令
- `make gen`、`make lint`（buf lint + Redocly）、`make breaking`（`buf breaking --against '.git#branch=main'`）

## 规则
- ENG-01、ENG-02：字段只增不改；删除编号写入 `reserved`；新行为配能力位。
- spec/02 与 spec/30 API-01 的命名约定；每个 OpenAPI 操作至少一个响应示例。
- 修改后使用 protocol-reviewer 审查。
