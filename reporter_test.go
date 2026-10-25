package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestConsoleDynamicProgressDoesNotEraseHistory(t *testing.T) {
	var out bytes.Buffer
	r := NewConsoleReporter(&out, true, 40)
	r.Report(Event{Kind: EventProgress, Stage: "book", Task: "details", Done: 1, Total: 5})
	r.Report(Event{Kind: EventChanges, Stage: "book", Changes: []RecordChange{{Kind: "add", Title: "测试书\n伪造日志\x1b[2J"}}})
	r.Report(Event{Kind: EventProgress, Stage: "book", Task: "details", Done: 5, Total: 5})
	r.Close()
	if !strings.Contains(out.String(), "\r\x1b[2K") {
		t.Fatal("expected local progress refresh")
	}
	if strings.Contains(out.String(), "\x1b[2J") {
		t.Fatal("untrusted book text contains control sequence")
	}
	if !strings.Contains(out.String(), "新增《测试书") {
		t.Fatal("history is missing")
	}
}

func TestConsoleLongTextAndDuplicateErrors(t *testing.T) {
	var out bytes.Buffer
	r := NewConsoleReporter(&out, false, 80)
	r.Report(Event{Kind: EventChanges, Changes: []RecordChange{{Kind: "update", Title: "书", Fields: []FieldChange{{Field: "Intro", Before: "旧简介\n第二行", After: "新简介\n第二行"}}}}})
	r.Close()
	if !strings.Contains(out.String(), "旧简介") || !strings.Contains(out.String(), "新简介") {
		t.Fatal(out.String())
	}
	if strings.Contains(out.String(), "map[") {
		t.Fatal("raw record printed")
	}
}

func TestConsoleLongTextKeepsIndentationAndRedactsIdentifiers(t *testing.T) {
	var out bytes.Buffer
	r := NewConsoleReporter(&out, false, 80)
	r.Redact("secret-base")
	r.Report(Event{Kind: EventChanges, Changes: []RecordChange{{Kind: "update", Title: "书", Fields: []FieldChange{{Field: "Intro", Before: "第一行\nsecret-base", After: "新第一行\n新第二行"}}}}})
	r.Close()
	if !strings.Contains(out.String(), "\n        新第二行") {
		t.Fatal("multiline value lost indentation")
	}
	if strings.Contains(out.String(), "secret-base") {
		t.Fatal("private identifier leaked")
	}
}

func TestConsoleProgressNeverMovesBackwardAndCoalescesRefreshes(t *testing.T) {
	var out bytes.Buffer
	r := NewConsoleReporter(&out, true, 120)
	r.Report(Event{Kind: EventProgress, Stage: "book", Task: "details", Done: 2, Total: 100})
	r.Report(Event{Kind: EventProgress, Stage: "book", Task: "details", Done: 1, Total: 100})
	for i := 3; i <= 100; i++ {
		r.Report(Event{Kind: EventProgress, Stage: "book", Task: "details", Done: i, Total: 100})
	}
	r.Close()
	if strings.Contains(out.String(), "详情 1/100") {
		t.Fatal("progress moved backwards")
	}
	if draws := strings.Count(out.String(), "\r\x1b[2K"); draws > 5 {
		t.Fatalf("%d redraws for one short burst", draws)
	}
}
