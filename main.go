package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"golang.org/x/term"
)

func main() { os.Exit(run()) }
func run() int {
	mode := flag.String("mode", "all", "login:扫码登录, book:更新书籍, readTime:更新阅读时长, all:全部同步")
	flag.Parse()
	if *mode != "all" && *mode != "book" && *mode != "readTime" && *mode != "login" {
		r := NewConsoleReporter(os.Stderr, false, 80)
		r.Report(Event{Kind: EventFinish, Err: fmt.Errorf("未知模式: %s", *mode)})
		r.Close()
		return 2
	}
	dynamic := term.IsTerminal(int(os.Stdout.Fd())) && os.Getenv("TERM") != "dumb"
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		width = 80
	}
	reporter := NewConsoleReporter(os.Stdout, dynamic, width)
	defer reporter.Close()
	started := time.Now()
	reporter.Report(Event{Kind: EventRun, Task: *mode})
	if *mode == "login" {
		err := runWeReadLoginWithReporter(reporter)
		reporter.Report(Event{Kind: EventFinish, Stage: "login", Err: err, Elapsed: time.Since(started)})
		if err != nil {
			return 1
		}
		return 0
	}
	config, err := loadAppConfig()
	if err != nil {
		reporter.Report(Event{Kind: EventFinish, Err: err})
		return 1
	}
	f := config.Feishu
	reporter.Redact(f.AppID, f.AppSecret, f.BaseAppID, f.BookListTableID, f.ReadTimeTableID)
	s := &syncRunner{config: f, reporter: reporter}
	failed := false
	if *mode == "all" || *mode == "readTime" {
		_, err := s.readTime()
		failed = failed || err != nil
	}
	if *mode == "all" || *mode == "book" {
		_, err := s.books()
		failed = failed || err != nil
	}
	reporter.Report(Event{Kind: EventSummary, Stats: SyncStats{Failed: failed}, Elapsed: time.Since(started)})
	if failed {
		return 1
	}
	return 0
}
