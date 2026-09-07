package sqlite

import "github.com/unxed/vtui"

func sqliteText(key, english, russian string) string {
	if translated := vtui.Msg(key); translated != "{"+key+"}" {
		return translated
	}
	if vtui.Msg("SQLite.LanguageCode") == "ru" {
		return russian
	}
	return english
}
