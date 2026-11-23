package chroma

import (
	"reflect"
	"testing"

	"github.com/alecthomas/chroma/v2"
)

func TestPlaylistHighlighting(t *testing.T) {
	provider := &ChromaProvider{}
	for _, filename := range []string{"playlist.m3u8", "playlist.M3U8", "playlist.m3u"} {
		if !provider.Match(filename, "") {
			t.Fatalf("editor did not detect %q", filename)
		}
	}

	highlighter := provider.Create("playlist.m3u8", "").(*ChromaHighlighter)
	tests := []struct {
		line string
		want []chroma.TokenType
	}{
		{line: `#EXTM3U generation-date="2026.11.20 17:00"`,
			want: []chroma.TokenType{chroma.GenericHeading, chroma.Text, chroma.NameVariable, chroma.Operator, chroma.String}},
		{line: `#EXTINF:0 tvg-name="News, Москва",Новости, вечер`,
			want: []chroma.TokenType{chroma.Keyword, chroma.Operator, chroma.Number, chroma.Text,
				chroma.NameVariable, chroma.Operator, chroma.String, chroma.Operator, chroma.GenericHeading}},
		{line: "#EXTGRP:News", want: []chroma.TokenType{chroma.Keyword, chroma.Operator, chroma.String}},
		{line: `#EXT-X-STREAM-INF:BANDWIDTH=2500000,CODECS="avc1.4d401f,mp4a.40.2"`,
			want: []chroma.TokenType{chroma.Keyword, chroma.Operator, chroma.NameVariable, chroma.Operator,
				chroma.Number, chroma.Operator, chroma.NameVariable, chroma.Operator, chroma.String}},
		{line: "#EXT-X-ENDLIST", want: []chroma.TokenType{chroma.Keyword}},
		{line: "# a comment", want: []chroma.TokenType{chroma.Comment}},
		{line: "https://example.org/live.m3u8?a=1#stream", want: []chroma.TokenType{chroma.Text}},
		{line: "video/segment.ts", want: []chroma.TokenType{chroma.Text}},
	}
	for _, test := range tests {
		tokens, err := highlighter.tokens(test.line)
		if err != nil {
			t.Fatal(err)
		}
		got := make([]chroma.TokenType, 0, len(tokens))
		var source string
		for _, token := range tokens {
			got = append(got, token.Type)
			source += token.Value
		}
		if source != test.line || !reflect.DeepEqual(got, test.want) {
			t.Errorf("%q produced %v, want %v and unchanged source", test.line, tokens, test.want)
		}
	}
}
