package studio

import (
	"strings"
	"testing"
)

func TestChatGPTPlanStreamSuccess(t *testing.T) {
	s := "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"{\\\"ids\\\":\"}\n\nevent: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"[1,3]}\"}\n\nevent: response.completed\ndata: {\"type\":\"response.completed\"}\n\n"
	got, e := parseChatGPTStream(strings.NewReader(s))
	if e != nil {
		t.Fatal(e)
	}
	if got != `{"ids":[1,3]}` {
		t.Fatalf("unexpected text %q", got)
	}
}
func TestChatGPTPlanStreamRejectsIncomplete(t *testing.T) {
	for _, s := range []string{
		"event: response.output_text.delta\ndata: {\"delta\":\"partial\"}\n\n",
		"event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"subscription_sharing_usage_limit_exceeded\"}}}\n\n",
		"event: response.completed\ndata: {\"type\":\"response.completed\"}\n\n",
	} {
		if got, e := parseChatGPTStream(strings.NewReader(s)); e == nil {
			t.Errorf("expected error, got %q", got)
		}
	}
}
