package mongofs

import "github.com/unxed/vtui"

// mongoText is the translation of key when the language files have one, and
// otherwise the English or Russian text that goes with it here (the SQLite and
// Docker plugins do the same).
func mongoText(key, english, russian string) string {
	if translated := vtui.Msg(key); translated != "{"+key+"}" {
		return translated
	}
	if vtui.Msg("Mongo.LanguageCode") == "ru" {
		return russian
	}
	return english
}
