// Tests for event routing from .neko dispatches into character contexts.
package driver

import (
	"errors"
	"testing"
	"time"
)

// waitForPrompts waits until the fake DSH accepted want prompts.
func waitForPrompts(t *testing.T, fake *fakeDSH, want int) []promptRecord {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		fake.mu.Lock()
		prompts := append([]promptRecord(nil), fake.prompts...)
		fake.mu.Unlock()
		if len(prompts) >= want {
			return prompts
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("超时：等待 %d 条 prompt 未到", want)
	return nil
}

func TestOnEventRoutesToContext(t *testing.T) {
	fake := newFakeDSH(t)
	cfg := testConfig(t, fake)
	cfg.Routes = []Route{{
		Event:     "心情变化",
		ContextID: "ctx-cat",
		Template:  "事件 {{event}} 字段 {{field}} 新值 {{new}}",
	}}
	contextDriver, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	report := contextDriver.OnEvent(Event{
		Name: "心情变化", Entity: "猫猫", Field: "心情.当前", Old: "平静", New: "开心",
	})
	if report.Dispatched != 1 || len(report.Skipped) != 0 {
		t.Fatalf("report = %+v", report)
	}
	prompts := waitForPrompts(t, fake, 1)
	if prompts[0].Text != "事件 心情变化 字段 心情.当前 新值 开心" {
		t.Fatalf("text = %q", prompts[0].Text)
	}
	if prompts[0].SessionID != "dsh-session" {
		t.Fatalf("session = %q, want the created session", prompts[0].SessionID)
	}
}

func TestOnEventUsesDefaultTemplate(t *testing.T) {
	fake := newFakeDSH(t)
	cfg := testConfig(t, fake)
	cfg.Routes = []Route{{Event: "体力变化", ContextID: "ctx-cat"}}
	contextDriver, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	contextDriver.OnEvent(Event{Name: "体力变化", Entity: "猫猫", Field: "身体.体力", New: 42})
	prompts := waitForPrompts(t, fake, 1)
	want := "【neko 事件】event=体力变化 entity=猫猫 field=身体.体力 old= new=42"
	if prompts[0].Text != want {
		t.Fatalf("text = %q, want %q", prompts[0].Text, want)
	}
}

func TestOnEventCooldownSkips(t *testing.T) {
	fake := newFakeDSH(t)
	cfg := testConfig(t, fake)
	cfg.Routes = []Route{{Event: "心情变化", ContextID: "ctx-cat"}}
	contextDriver, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	first := contextDriver.OnEvent(Event{Name: "心情变化", Field: "心情.当前", New: "开心"})
	second := contextDriver.OnEvent(Event{Name: "心情变化", Field: "心情.当前", New: "更开心"})
	if first.Dispatched != 1 {
		t.Fatalf("first = %+v", first)
	}
	if second.Dispatched != 0 || len(second.Skipped) != 1 || second.Skipped[0] != "ctx-cat" {
		t.Fatalf("second = %+v", second)
	}
}

func TestOnEventUnknownEventIsIgnored(t *testing.T) {
	fake := newFakeDSH(t)
	contextDriver, _ := newTestDriver(t, fake)
	report := contextDriver.OnEvent(Event{Name: "未注册的事件"})
	if report.Dispatched != 0 || len(report.Skipped) != 0 {
		t.Fatalf("report = %+v", report)
	}
}

func TestOnEventReportsDeliveryFailure(t *testing.T) {
	fake := newFakeDSH(t)
	fake.failPromptCode = "session/agent-busy"
	cfg := testConfig(t, fake)
	cfg.Routes = []Route{{Event: "心情变化", ContextID: "ctx-cat"}}
	contextDriver, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	failures := make(chan error, 1)
	contextDriver.SetErrorHandler(func(deliverErr error) { failures <- deliverErr })
	contextDriver.OnEvent(Event{Name: "心情变化", Field: "心情.当前", New: "开心"})
	select {
	case deliverErr := <-failures:
		var apiError *APIError
		if !errors.As(deliverErr, &apiError) || !apiError.IsCode("session/agent-busy") {
			t.Fatalf("err = %v", deliverErr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("delivery failure was not reported")
	}
}
