# WeRead Reading Data Sync

将个人微信读书的书架、阅读过的书籍和每日阅读时长同步到飞书多维表格。程序会比较两端数据，新增、更新或删除对应记录，并把书籍封面上传为飞书附件。

## 准备工作

- Go 1.24 或更新版本。
- 可扫码登录的微信读书账号。
- 一个已有的飞书多维表格 Base，以及用于书籍列表和每日阅读时长的两张表。
- 可访问该 Base 的飞书自建应用；应用需要多维表格记录读写和云空间素材上传权限。

项目不会自动创建飞书表或字段。表格字段需与 [书籍映射](book.go) 和 [阅读时长映射](read_time.go) 中使用的字段名、类型一致。

## 配置与运行

1. 在仓库根目录复制配置模板：

   ```bash
   cp config.example.json config.local.json
   ```

2. 编辑 `config.local.json`，填写飞书应用和目标 Base：

   | 字段 | 用途 |
   | --- | --- |
   | `feishu.appId` / `feishu.appSecret` | 飞书自建应用凭证 |
   | `feishu.baseAppId` | 多维表格 Base 的 app token |
   | `feishu.bookListTableId` | 书籍列表的 table ID |
   | `feishu.readTimeTableId` | 每日阅读时长的 table ID |

3. 运行同步：

   ```bash
   go run . -mode all
   ```

首次运行或微信读书登录失效时，终端会显示二维码和临时图片路径。用微信扫码并确认后，程序会继续执行当前同步任务。平时程序自动刷新微信读书 `accessToken`；轮换后的刷新凭证保存在 `auth.local.json`。

| 模式 | 作用 |
| --- | --- |
| `all`（默认） | 同步每日阅读时长和书籍列表 |
| `book` | 只同步书籍列表 |
| `readTime` | 只同步每日阅读时长 |
| `login` | 主动重新扫码登录微信读书 |

> [!NOTE]
> `config.local.json` 和 `auth.local.json` 都在 `.gitignore` 中，属于本机私有文件。不要把它们复制到公开仓库或日志中。程序应从仓库根目录运行；需要自定义路径时，可分别设置 `WEREAD_SYNC_CONFIG` 和 `WEREAD_AUTH_FILE`。

## 验证

```bash
go test ./...
go build ./...
```

默认测试只运行本地测试。需要调用真实微信读书或飞书接口时，设置 `WEREAD_RUN_LIVE_TESTS=1`；会修改飞书记录的测试还需单独设置 `WEREAD_RUN_LIVE_MUTATION_TESTS=1`。

本项目使用微信读书客户端接口；若接口协议发生变化，登录或同步流程可能需要相应调整。
