# Console Performance Implementation Plan

> **For agentic workers:** Use superpowers:executing-plans in the current session. User approved continuous implementation.

**Goal:** 清晰动态输出与保留同步语义的性能优化，并报告整体耗时对照。
**Architecture:** 纯差异数据与执行结果，Reporter 事件接口，独立控制台渲染；有界读取及上传调度。
**Tech Stack:** Go，标准库与 golang.org/x/term 终端检测。
**Spec:** docs/superpowers/specs/2026-10-26-console-performance-design.md

## Global Constraints
- 只过滤评分展示，不改变评分同步；不限制变更条数；无 verbose。
- 上传间隔 205ms，最多 5 并发、总计最多 3 次尝试，永久错误不重试。
- 已有附件保留；所有数据源成功后才写对应表。
- 不自动提交、推送或运行真实数据写入。

## Review Focus
- 并发失败不能成为不完整成功或触发错误删除。
- 多个请求同时认证过期只恢复一次，二维码展示不与进度交错。
- 多行长文本和终端换行不能破坏局部刷新。
- 部分批次成功后失败的统计必须明确，不冒充整批成功。
- 仅评分变化仍写入但不重传封面。

### Task 1: Baseline, diff data and console reporter
**Files:** sync_benchmark_test.go, book_diff.go, reporter.go, reporter_test.go, book_diff_test.go
**Interfaces:** FieldChange, RecordChange, ProgressEvent, Reporter.Report(ProgressEvent), ConsoleReporter.Close().
- [x] 固定输入完整同步，旧版三次测量并保留结果。
- [x] 写失败测试：评分过滤、所有其他字段、超过20条、长文本、普通与动态模式以及错误去重。
- [x] 实现结构化模型与单一渲染入口，go test ./...。

### Task 2: Transport, cover upload and pure planning
**Files:** utils.go, lark_table.go, cover_upload.go, book.go, upload_test.go
**Interfaces:** HTTPStatusError 保存业务码与 reset；CoverUploadManager 以完成通道通知；bookSyncPlan 分开计算与附件处理。
- [x] 写失败测试：仅评分更新保留附件、缺附件补传、每次重试限速、总计三次、永久错误一次、限流共享冷却、上传结果去重。
- [x] 实现限速重试、配置/token复用、纯差异计划；go test ./...。

### Task 3: Concurrent orchestration and validation
**Files:** main.go, sync.go, weread.go, read_time.go, weread_auth.go, sync_test.go, README.md
**Interfaces:** updateReadTimeData/updateBookListData 兼容现有测试；syncRunner 注入 Reporter 并统一收集阶段统计。
- [x] 写失败测试：有界并发、完整结果、读取失败阻止写入、日期新增更新删除一致性。
- [x] 实现独立读取并行、日期索引、详情工作池、集中登录展示及最终统计。
- [x] 完整测试、race、vet、build、PTY 演示；同夹具三次对照测量。
- [x] 独立审查后修复重要发现，更新 README 和性能记录。

## Execution ledger
- Baseline unit suite passed before edits (go test ./..., 2.212s; not a sync timing).
- Workspace: managed worktree, feat/console-progress-performance.
- Ruling: preserve uncommitted changes for review; no automatic commit or cleanup deletion.

## 最终验证记录
- go test ./...、go test -race -count=1 ./...、go vet ./...、go build 全部通过。
- PTY 演示及普通文本测试通过，所有运行均为模拟数据，无真实同步写入。
- 独立审查的两个 Important 已以失败测试复现并修复：HTTP发送边界限速、已读分页业务错误阻止写入。
- Ruling: DNS永久错误重试、窄终端换行与测量遗漏渲染成本虽然被审查标为Minor，但直接违反已确认需求，按重要问题修复并覆盖测试。
- PTY发现进度事件可能乱序；新增回归测试后加入单调进度和200ms刷新合并。
- 并发登录成功恢复测试覆盖5请求同时过期且仅登录一次；不以此宣称真实网络登录已验证。
- 真实本地HTTP服务验证连接复用：两轮各10请求，连接数18降至10。
- 不修改真实配置或凭证，不自动提交/推送；代码保留在隔离分支供查看。
