// Tests for cross-context sends: guards, validation and prompt delivery.
package driver

import (
	"context"
	"errors"
	"testing"
)

// twoContextConfig extends the base config with a second context.
func twoContextConfig(t *testing.T, fake *fakeDSH) Config {
	t.Helper()
	cfg := testConfig(t, fake)
	cfg.Contexts = append(cfg.Contexts, Spec{
		ContextID: "ctx-other", CharacterID: "另一只", SessionID: "neko-other",
		PresetID: "standard", Cwd: cfg.Contexts[0].Cwd,
	})
	return cfg
}

func TestSendDeliversToTarget(t *testing.T) {
	fake := newFakeDSH(t)
	contextDriver, err := New(twoContextConfig(t, fake))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	result, err := contextDriver.Send(context.Background(), SendRequest{
		SourceContextID: "ctx-cat", TargetContextID: "ctx-other", Text: "来客厅",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if result.ContextID != "ctx-other" || result.Mode != MODE_QUEUE {
		t.Fatalf("result = %+v", result)
	}
	prompts := waitForPrompts(t, fake, 1)
	if prompts[0].Text != "来客厅" {
		t.Fatalf("text = %q", prompts[0].Text)
	}
}

func TestSendRejectsInvalidRequests(t *testing.T) {
	fake := newFakeDSH(t)
	contextDriver, err := New(twoContextConfig(t, fake))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	cases := []SendRequest{
		{TargetContextID: "", Text: "x"},
		{TargetContextID: "ctx-other", Text: "   "},
		{SourceContextID: "missing", TargetContextID: "ctx-other", Text: "x"},
		{SourceContextID: "ctx-cat", TargetContextID: "missing", Text: "x"},
		{SourceContextID: "ctx-cat", TargetContextID: "ctx-cat", Text: "x"},
	}
	for index, request := range cases {
		_, err := contextDriver.Send(context.Background(), request)
		var requestError *RequestError
		if !errors.As(err, &requestError) {
			t.Fatalf("case %d: err = %v, want RequestError", index, err)
		}
	}
}

func TestSendDedupesRepeatedText(t *testing.T) {
	fake := newFakeDSH(t)
	contextDriver, err := New(twoContextConfig(t, fake))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	request := SendRequest{SourceContextID: "ctx-cat", TargetContextID: "ctx-other", Text: "同样的话"}
	if _, err := contextDriver.Send(context.Background(), request); err != nil {
		t.Fatalf("first Send: %v", err)
	}
	_, err = contextDriver.Send(context.Background(), request)
	var requestError *RequestError
	if !errors.As(err, &requestError) {
		t.Fatalf("second Send err = %v, want RequestError", err)
	}
}

func TestSendGuardBurst(t *testing.T) {
	guard := newSendGuard()
	for index := 0; index < SEND_BURST; index++ {
		if !guard.allow("ctx-cat", "ctx-other", "消息 "+string(rune('a'+index))) {
			t.Fatalf("send %d was rejected before the burst cap", index)
		}
	}
	if guard.allow("ctx-cat", "ctx-other", "超限消息") {
		t.Fatal("send past the burst cap was allowed")
	}
}
