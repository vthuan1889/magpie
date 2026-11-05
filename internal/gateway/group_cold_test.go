package gateway

import (
	"net/http"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/provider"
)

// A group's Cold keeps its conversations on the member that answered them
// for as long as the user says their vendors keep a prompt cached: a relay
// in front of DeepSeek keeps one for hours, and moving on cacheCold's five
// minutes paid the whole conversation again.
func TestGroupColdKeepsConversation(t *testing.T) {
	a := candidate{p: provider.Provider{ID: "relay-a", Chat: "https://a.example.com/v1"}, model: "deepseek-v4-flash", rest: "relay-a"}
	b := candidate{p: provider.Provider{ID: "relay-b", Chat: "https://b.example.com/v1"}, model: "deepseek-v4-flash", rest: "relay-b"}
	first := []byte(`{"messages":[{"role":"user","content":"fix the bug"}]}`)
	again := []byte(`{"messages":[{"role":"user","content":"fix the bug"},{"role":"assistant","content":"done"},{"role":"user","content":"now the tests"}]}`)
	for i, c := range []struct {
		name string
		cold time.Duration
		ago  time.Duration
		why  string
	}{
		{"no cold set, 6 minutes on", 0, 6 * time.Minute, "cold"},
		{"no cold set, 4 minutes on", 0, 4 * time.Minute, "cache"},
		{"cold=48h, 6 minutes on", 48 * time.Hour, 6 * time.Minute, "cache"},
		{"cold=48h, 30 hours on", 48 * time.Hour, 30 * time.Hour, "cache"},
		{"cold=48h, past 48 hours", 48 * time.Hour, 48*time.Hour + time.Minute, "first"},
	} {
		h := http.Header{}
		h.Set("x-claude-code-session-id", "group-cold-"+string(rune('a'+i)))
		cs := []candidate{b, a}
		pl := planned{order: []Weighed{{}, {}}}
		_, _, _, key := affine("t", "", false, false, false, c.cold, h, provider.Chat, first, first, cs, pl)
		answered(key, a, 1, Usage{Input: 300, CacheRead: 13_696})
		t.Cleanup(func() { sticks.Lock(); delete(sticks.m, key); sticks.Unlock() })
		sticks.Lock()
		st := sticks.m[key]
		st.at = st.at.Add(-c.ago)
		sticks.m[key] = st
		sticks.Unlock()
		got, _, af, _ := affine("t", "", false, false, false, c.cold, h, provider.Chat, again, again, cs, pl)
		if af.Why != c.why || af.Kept != (c.why == "cache") {
			t.Fatalf("%s: why %q kept %v, want %q", c.name, af.Why, af.Kept, c.why)
		}
		want := b
		if c.why == "cache" {
			want = a
		}
		if got[0].who() != want.who() {
			t.Fatalf("%s: went to %s, want %s", c.name, got[0].who(), want.who())
		}
	}
}

func TestColdOfAGroup(t *testing.T) {
	p := provider.Provider{ID: "relay", Chat: "https://relay.example.com/v1"}
	if got := coldOf(p, 0); got != cacheCold {
		t.Fatalf("a group without a cold: %v, want %v", got, cacheCold)
	}
	if got := coldOf(p, 48*time.Hour); got != 48*time.Hour {
		t.Fatalf("cold=48h: %v", got)
	}
	if got := (provider.Group{Cold: 48 * 60 * 60}).ColdFor(); got != 48*time.Hour {
		t.Fatalf("Group.ColdFor: %v", got)
	}
	if got := (provider.Group{Cold: provider.ColdLongest + 1}).ColdFor(); got != provider.ColdLongest*time.Second {
		t.Fatalf("Group.ColdFor past the most: %v", got)
	}
}
