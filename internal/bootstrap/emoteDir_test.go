package bootstrap

import (
	"path/filepath"
	"testing"

	"github.com/RuiCat/Neko/internal/config"
)

// TestResolveEmoteDir checks the flag > env > config precedence and the
// config-relative path base.
func TestResolveEmoteDir(t *testing.T) {
	t.Setenv("ASTRBOX_EMOTE_DIR", "")
	domain := config.Config{EmoteDir: "data/emotes"}
	cases := []struct {
		name    string
		options Options
		cfg     config.Config
		want    string
	}{
		{"flag wins", Options{EmoteDir: "/lib/flag", ConfigPath: "/repo/config.json"}, domain, "/lib/flag"},
		{"config relative joins config dir", Options{ConfigPath: "/repo/config.json"}, domain, "/repo/data/emotes"},
		{"config absolute kept", Options{ConfigPath: "/repo/config.json"}, config.Config{EmoteDir: "/abs/emotes"}, "/abs/emotes"},
		{"empty everything falls back", Options{}, config.Config{}, ""},
	}
	for _, testCase := range cases {
		if got := resolveEmoteDir(testCase.options, testCase.cfg); got != testCase.want {
			t.Fatalf("%s: resolveEmoteDir = %q, want %q", testCase.name, got, testCase.want)
		}
	}

	t.Setenv("ASTRBOX_EMOTE_DIR", "/env/emotes")
	if got := resolveEmoteDir(Options{ConfigPath: "/repo/config.json"}, domain); got != "/env/emotes" {
		t.Fatalf("env should beat config: got %q", got)
	}
	if got := resolveEmoteDir(Options{EmoteDir: "/lib/flag"}, domain); got != "/lib/flag" {
		t.Fatalf("flag should beat env: got %q", got)
	}
}

// TestResolveEmoteDirMissingConfig keeps the relative base at the working
// directory when no config path is configured.
func TestResolveEmoteDirMissingConfig(t *testing.T) {
	t.Setenv("ASTRBOX_EMOTE_DIR", "")
	got := resolveEmoteDir(Options{}, config.Config{EmoteDir: "data/emotes"})
	if got != filepath.Join(".", "data/emotes") {
		t.Fatalf("resolveEmoteDir = %q, want working-directory relative path", got)
	}
}
