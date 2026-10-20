package main

import (
	"strings"
	"testing"
)

func TestUUIDHostStableFormat(t *testing.T) {
	id, e := makeHostID()
	if e != nil {
		t.Fatal(e)
	}
	if !strings.HasPrefix(id, "urn:uuid:") || len(id) != 45 {
		t.Errorf("unexpected host ID %q", id)
	}
}
func TestScopeExactMatching(t *testing.T) {
	scopes := "offline_access resource.invoke chatgpt.tokens.use.direct"
	if !hasScope(scopes, "chatgpt.tokens.use.direct") {
		t.Fatal("required scope not detected")
	}
	if hasScope(scopes, "chatgpt.tokens.use") {
		t.Fatal("accepted a partial scope")
	}
}
func TestURLRandomness(t *testing.T) {
	a, e := randomURLBytes(32)
	if e != nil {
		t.Fatal(e)
	}
	b, e := randomURLBytes(32)
	if e != nil {
		t.Fatal(e)
	}
	if a == b {
		t.Fatal("random bytes repeated")
	}
}
