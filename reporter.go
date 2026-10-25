package main

import (
	"fmt"
	"golang.org/x/term"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

type EventKind int

const (
	EventStart EventKind = iota
	EventProgress
	EventChanges
	EventWarning
	EventRetry
	EventFinish
	EventSummary
	EventLogin
	EventQR
	EventPlan
	EventRun
	EventLoginSuccess
)

type SyncStats struct {
	Add, Update, Delete int
	Failed              bool
}
type Event struct {
	Kind                     EventKind
	Stage, Task              string
	Done, Total, Attempt     int
	Success, Failed, Retried int
	Changes                  []RecordChange
	Stats                    SyncStats
	Err                      error
	Elapsed, Wait            time.Duration
	Path, QR                 string
}
type Reporter interface{ Report(Event) }

func report(r Reporter, e Event) {
	if r != nil {
		r.Report(e)
	}
}

// ConsoleReporter is the only owner of terminal state and user-facing copy.
// Concurrent producers report structured events; they never manipulate the cursor.
type ConsoleReporter struct {
	mu           sync.Mutex
	out          io.Writer
	dynamic      bool
	width        int
	progress     string
	started      time.Time
	stop         chan struct{}
	stopped      chan struct{}
	closed       bool
	secrets      []string
	lastPlain    map[string]time.Time
	lastDone     map[string]int
	progressTask string
	warnings     int
}

func NewConsoleReporter(out io.Writer, dynamic bool, width int) *ConsoleReporter {
	if width <= 0 {
		width = 80
	}
	r := &ConsoleReporter{out: out, dynamic: dynamic, width: width, started: time.Now(), stop: make(chan struct{}), stopped: make(chan struct{}), lastPlain: make(map[string]time.Time), lastDone: make(map[string]int)}
	if dynamic {
		go func() {
			defer close(r.stopped)
			ticker := time.NewTicker(200 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-r.stop:
					return
				case <-ticker.C:
					r.mu.Lock()
					if !r.closed && r.progress != "" {
						r.draw()
					}
					r.mu.Unlock()
				}
			}
		}()
	} else {
		close(r.stopped)
	}
	return r
}
func (r *ConsoleReporter) Redact(values ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, v := range values {
		if v != "" {
			r.secrets = append(r.secrets, v)
		}
	}
}
func (r *ConsoleReporter) clean(s string) string {
	for _, v := range r.secrets {
		s = strings.ReplaceAll(s, v, "[已隐藏]")
	}
	// Strip controls, including CR/ESC, so remote text cannot alter terminal state.
	return strings.Map(func(c rune) rune {
		if unicode.IsControl(c) {
			return ' '
		}
		return c
	}, s)
}
func (r *ConsoleReporter) line(s string) {
	if r.dynamic && r.progress != "" {
		fmt.Fprint(r.out, "\r\x1b[2K")
	}
	fmt.Fprintln(r.out, s)
	if r.dynamic && r.progress != "" {
		r.draw()
	}
}
func (r *ConsoleReporter) draw() {
	if file, ok := r.out.(*os.File); ok {
		if width, _, err := term.GetSize(int(file.Fd())); err == nil && width > 0 {
			r.width = width
		}
	}
	s := r.progress + " · 总耗时 " + time.Since(r.started).Round(100*time.Millisecond).String()
	// Reserve one column to avoid automatic line wrap and accidental scroll/erase.
	var b strings.Builder
	n := 0
	for _, c := range s {
		w := 1
		if c >= 0x1100 {
			w = 2
		}
		if unicode.Is(unicode.Mn, c) {
			w = 0
		}
		if n+w > r.width-2 {
			break
		}
		b.WriteRune(c)
		n += w
	}
	fmt.Fprint(r.out, "\r\x1b[2K", b.String())
}
func (r *ConsoleReporter) Close() {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	if r.dynamic && r.progress != "" {
		fmt.Fprint(r.out, "\r\x1b[2K")
	}
	close(r.stop)
	r.mu.Unlock()
	<-r.stopped
}
func (r *ConsoleReporter) Report(e Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	stage := map[string]string{"book": "书籍列表", "readTime": "阅读时长", "login": "登录"}[e.Stage]
	task := map[string]string{"years": "读取各年阅读数据", "shelf": "读取书架", "readBooks": "读取已读书籍", "existing": "读取飞书记录", "details": "获取书籍详情", "covers": "处理封面", "add": "新增记录", "update": "更新记录", "delete": "删除记录"}[e.Task]
	switch e.Kind {
	case EventRun:
		r.line("微信读书 → 飞书同步 · " + time.Now().Format("2006-01-02 15:04:05.999") + " · 模式 " + r.clean(e.Task))
	case EventStart:
		for key := range r.lastDone {
			if strings.HasPrefix(key, e.Stage+"/") {
				delete(r.lastDone, key)
			}
		}
		r.progressTask = ""
		r.line("\n" + stage)
		if r.dynamic {
			r.progress = stage + " · 正在读取数据"
			r.draw()
		}
	case EventProgress:
		key := e.Stage + "/" + e.Task
		if e.Done < r.lastDone[key] {
			return
		}
		r.lastDone[key] = e.Done
		text := stage + " · " + task
		if e.Total > 0 {
			text += fmt.Sprintf(" %d/%d", e.Done, e.Total)
		} else if e.Done > 0 {
			text += fmt.Sprintf(" %d 条", e.Done)
		}
		if e.Task == "covers" {
			text += fmt.Sprintf(" · 成功 %d · 失败 %d · 重试 %d", e.Success, e.Failed, e.Retried)
		}
		if r.dynamic {
			changedTask := r.progressTask != key
			r.progress = text
			r.progressTask = key
			if changedTask || e.Total > 0 && e.Done == e.Total {
				r.draw()
			}
		} else {
			last := r.lastPlain[key]
			if last.IsZero() || time.Since(last) >= 3*time.Second || e.Total > 0 && e.Done == e.Total {
				r.line("  " + text)
				r.lastPlain[key] = time.Now()
			}
		}
	case EventChanges:
		r.changes(e.Changes)
	case EventPlan:
		if r.dynamic && r.progress != "" {
			fmt.Fprint(r.out, "\r\x1b[2K")
		}
		r.progress = ""
		r.line(fmt.Sprintf("  待同步：新增 %d · 更新 %d · 删除 %d", e.Stats.Add, e.Stats.Update, e.Stats.Delete))
	case EventRetry:
		r.progress = fmt.Sprintf("%s · %s · 重试 %d/2，等待 %s", stage, task, e.Attempt, e.Wait.Round(time.Millisecond))
		if r.dynamic {
			r.draw()
		} else {
			r.line("  " + r.progress)
		}
	case EventWarning:
		r.warnings++
		r.line("  警告：" + r.clean(fmt.Sprint(e.Err)))
	case EventFinish:
		if r.dynamic && r.progress != "" {
			fmt.Fprint(r.out, "\r\x1b[2K")
		}
		r.progress = ""
		if e.Err != nil {
			r.line("  失败：" + r.clean(e.Err.Error()))
		}
		label := "完成"
		if e.Err != nil {
			label = "失败（以下仅计已确认写入）"
		}
		r.line(fmt.Sprintf("  %s%s：新增 %d · 更新 %d · 删除 %d · 耗时 %s", stage, label, e.Stats.Add, e.Stats.Update, e.Stats.Delete, e.Elapsed.Round(time.Millisecond)))
	case EventSummary:
		label := "同步完成"
		if e.Stats.Failed {
			label = "同步部分失败"
		}
		r.line(fmt.Sprintf("\n%s · 警告 %d · 总耗时 %s", label, r.warnings, e.Elapsed.Round(time.Millisecond)))
	case EventLoginSuccess:
		r.line("微信读书登录完成，刷新凭证已保存。")
	case EventLogin:
		r.line("微信读书需要扫码登录，请用微信扫描二维码并确认。")
	case EventQR:
		old := r.progress
		if r.dynamic && old != "" {
			fmt.Fprint(r.out, "\r\x1b[2K")
		}
		r.progress = ""
		r.line("二维码图片：" + r.clean(e.Path))
		fmt.Fprintln(r.out, e.QR)
		r.progress = old
	}
}
func (r *ConsoleReporter) changes(changes []RecordChange) {
	// A copy prevents presentation ordering from mutating the write plan.
	ordered := append([]RecordChange(nil), changes...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Kind != ordered[j].Kind {
			return ordered[i].Kind < ordered[j].Kind
		}
		return ordered[i].Title < ordered[j].Title
	})
	ratings := 0
	for _, c := range ordered {
		visible := make([]FieldChange, 0, len(c.Fields))
		for _, f := range c.Fields {
			if f.Field != "Score" {
				visible = append(visible, f)
			}
		}
		if c.Kind == "update" && len(visible) == 0 {
			if len(c.Fields) > 0 {
				ratings++
			}
			continue
		}
		label := map[string]string{"add": "新增", "update": "更新", "delete": "删除"}[c.Kind]
		r.line("  待" + label + "《" + r.clean(c.Title) + "》")
		for _, f := range visible {
			names := map[string]string{"Title": "书名", "Author": "作者", "Price": "价格", "CanRead": "是否可读", "ReadTime": "阅读时长", "ShelfName": "书架分类", "Intro": "简介", "Words": "字数（万字）", "Progress": "阅读进度", "FinishTime": "读完时间", "Categories": "分类", "Date": "日期", "Cover": "封面"}
			name := names[f.Field]
			if name == "" {
				name = f.Field
			}
			if f.Field == "Intro" {
				r.line("    简介：")
				r.multiline("原", fmt.Sprint(f.Before))
				r.multiline("新", fmt.Sprint(f.After))
				continue
			}
			text := "    " + name + "：" + r.clean(formatValue(f.Field, f.Before)) + " → " + r.clean(formatValue(f.Field, f.After))
			if f.Field == "ReadTime" {
				if a, ok := f.Before.(int64); ok {
					if b, ok := f.After.(int64); ok {
						text += "（" + signedDuration(b-a) + "）"
					}
				}
			}
			r.line(text)
		}
	}
	if ratings > 0 {
		r.line(fmt.Sprintf("  另有 %d 本仅更新评分", ratings))
	}
}
func formatValue(field string, value any) string {
	switch field {
	case "ReadTime":
		if n, ok := value.(int64); ok {
			return humanDuration(n)
		}
	case "Progress":
		if n, ok := value.(float64); ok {
			return fmt.Sprintf("%g%%", n*100)
		}
	case "FinishTime":
		if n, ok := value.(int64); ok {
			if n == 0 {
				return "未记录"
			}
			return time.UnixMilli(n).Format("2006-01-02 15:04:05.999")
		}
	case "CanRead":
		if b, ok := value.(bool); ok {
			if b {
				return "是"
			}
			return "否"
		}
	}
	if fmt.Sprint(value) == "" {
		return "空"
	}
	return fmt.Sprint(value)
}
func humanDuration(n int64) string {
	sign := ""
	if n < 0 {
		sign = "-"
		n = -n
	}
	return fmt.Sprintf("%s%d时%d分%d秒", sign, n/3600, n%3600/60, n%60)
}
func signedDuration(n int64) string {
	if n >= 0 {
		return "+" + humanDuration(n)
	}
	return humanDuration(n)
}

func (r *ConsoleReporter) multiline(label, value string) {
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	for i, line := range lines {
		prefix := "        "
		if i == 0 {
			prefix = "      " + label + "："
		}
		r.line(prefix + r.clean(line))
	}
}
