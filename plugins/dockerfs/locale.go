package dockerfs

import "github.com/unxed/vtui"

// dockerText is the translation of key when the language files have one, and
// otherwise the English or Russian text that goes with it here. It is the same
// arrangement the SQLite plugin uses.
func dockerText(key, english, russian string) string {
	if translated := vtui.Msg(key); translated != "{"+key+"}" {
		return translated
	}
	if vtui.Msg("Docker.LanguageCode") == "ru" {
		return russian
	}
	return english
}
