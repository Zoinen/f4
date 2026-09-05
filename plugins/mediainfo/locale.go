package mediainfo

import "github.com/unxed/vtui"

func (plugin *Plugin) text(key, english, russian string) string {
	language := plugin.settings().Language
	if language == "ru" {
		return russian
	}
	if language == "en" {
		return english
	}
	if translated := vtui.Msg(key); translated != "{"+key+"}" {
		return translated
	}
	return english
}

func (plugin *Plugin) reportLanguage() string {
	language := plugin.settings().Language
	if language == "en" || language == "ru" {
		return language
	}
	if vtui.Msg("MediaInfo.LanguageCode") == "ru" {
		return "ru"
	}
	return "en"
}
