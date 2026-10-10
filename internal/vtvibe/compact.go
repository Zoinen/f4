package vtvibe

import (
	"fmt"
	"regexp"
	"strings"
)

// Shortening the model's own answers (unxed/f4#1842, docs/VTVIBE.md
// § 19a.3, stage H4, and § 19a.9 item 10). When the main dialog no longer
// fits the model's context, f4 sends it again with the model's earlier
// answers shortened — code blocks reduced to a mark, long text cut — and
// the user's messages in full: what the user wrote is never shortened. The
// dialog itself is kept whole; only that request is shortened.

// keepFullAnswers is how many of the latest answers stay whole.
const keepFullAnswers = 2

// maxShortAnswer bounds an older answer once shortened, in runes.
const maxShortAnswer = 1200

var fencedCode = regexp.MustCompile("(?s)```[^\\n]*\\n(.*?)```")

// shortenAnswer is an older answer of the model, made short.
func shortenAnswer(text string) string {
	text = fencedCode.ReplaceAllStringFunc(text, func(block string) string {
		lines := strings.Count(fencedCode.FindStringSubmatch(block)[1], "\n")
		return fmt.Sprintf("[code block of %d lines left out]", lines)
	})
	if r := []rune(text); len(r) > maxShortAnswer {
		text = string(r[:maxShortAnswer]) + " … [shortened]"
	}
	return text
}

// historyMessages turns the dialog into messages; compact shortens the
// model's answers except the latest keepFullAnswers, never the user's.
func historyMessages(history []Turn, compact bool) []Message {
	answers := 0
	for _, t := range history {
		if t.Role == "assistant" {
			answers++
		}
	}
	msgs := make([]Message, 0, len(history))
	seen := 0
	for _, t := range history {
		if t.Text == "RCtrl+A to hide" {
			continue
		}
		text := t.Text
		if t.Role == "assistant" {
			seen++
			if compact && seen <= answers-keepFullAnswers {
				text = shortenAnswer(text)
			}
		}
		msgs = append(msgs, Message{Role: t.Role, Content: text})
	}
	return msgs
}

// compactNote tells the user, in the dialog, that a request went shortened.
const compactNote = "f4: the dialog no longer fit the model's context, so the model's earlier answers were sent shortened (your messages were sent in full; the dialog itself is kept whole)."

// errStillTooLong explains a dialog that does not fit even shortened.
func errStillTooLong(err error) error {
	return fmt.Errorf("%w — the dialog does not fit the model's context even with the model's earlier answers shortened; your own messages are never shortened, so start a new dialog with ai:new (this one stays in ai:dialogs)", err)
}
