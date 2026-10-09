package k8sfs

import "github.com/unxed/vtui"

// k8sText is the translation of key when the language files have one, and
// otherwise the English or Russian text that goes with it here (the SQLite and
// Docker plugins do the same).
func k8sText(key, english, russian string) string {
	if translated := vtui.Msg(key); translated != "{"+key+"}" {
		return translated
	}
	if vtui.Msg("K8s.LanguageCode") == "ru" {
		return russian
	}
	return english
}
