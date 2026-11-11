package site

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestParseSpec(t *testing.T) {
	cases := []struct {
		name string
		typ  Type
		raw  string
		ok   bool
	}{
		{"container image", TypeContainer, `{"image":"nginx:1.27","port":80}`, true},
		{"container missing source", TypeContainer, `{"port":80}`, false},
		{"unknown field rejected", TypeContainer, `{"image":"x","bogus":1}`, false},
		{"php", TypePHP, `{"php_version":"8.3"}`, true},
		{"php missing version", TypePHP, `{}`, false},
		{"wordpress", TypeWordPress, `{"php_version":"8.3","wp":{"auto_update":"minor"}}`, true},
		{"node needs start", TypeNode, `{"port":3000}`, false},
		{"proxy", TypeProxy, `{"upstream":"http://10.0.0.5:8080"}`, true},
		{"unknown type", Type("ftp"), `{}`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseSpec(c.typ, json.RawMessage(c.raw))
			if c.ok && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !c.ok && !errors.Is(err, ErrInvalidSpec) {
				t.Fatalf("want ErrInvalidSpec, got %v", err)
			}
		})
	}
}

func TestCanTransition(t *testing.T) {
	if !CanTransition(StateDraft, StateProvisioning) {
		t.Error("draft -> provisioning should be allowed")
	}
	if CanTransition(StateDeleted, StateRunning) {
		t.Error("deleted -> running must not be allowed")
	}
	if CanTransition(StateRunning, StateDraft) {
		t.Error("running -> draft must not be allowed")
	}
}
