// emoteDir.go resolves the emote library directory the emote_find tool reads.
// The library is machine-local data (gitignored, >1GB), so it is never copied
// into the runtime tree: the path comes from configuration instead.
package bootstrap

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/RuiCat/Neko/internal/config"
)

// resolveEmoteDir picks the emote library directory. Precedence:
//
//  1. Options.EmoteDir (the -emote-dir flag; used as given),
//  2. $ASTRBOX_EMOTE_DIR,
//  3. config.json's emote_dir; a relative value resolves against the config
//     file's directory, so repo-local "data/emotes" keeps working when the
//     service starts from the repository root,
//  4. empty, which lets the emote package fall back to its legacy default.
func resolveEmoteDir(options Options, domainConfig config.Config) string {
	if value := strings.TrimSpace(options.EmoteDir); value != "" {
		return value
	}
	if value := strings.TrimSpace(os.Getenv("ASTRBOX_EMOTE_DIR")); value != "" {
		return value
	}
	value := strings.TrimSpace(domainConfig.EmoteDir)
	if value == "" {
		return ""
	}
	if filepath.IsAbs(value) {
		return value
	}
	base := "."
	if path := strings.TrimSpace(options.ConfigPath); path != "" {
		base = filepath.Dir(path)
	}
	return filepath.Join(base, value)
}
