package chroma

import "github.com/alecthomas/chroma/v2"

var playlistLexer = chroma.MustNewLexer(&chroma.Config{Name: "M3U playlist"}, playlistRules)

func playlistRules() chroma.Rules {
	const number = `[+-]?[0-9]+(?:\.[0-9]+)?`
	return chroma.Rules{
		"root": {
			{Pattern: `[\x{FEFF} \t\r\n]+`, Type: chroma.Text},
			{
				Pattern: `#EXTM3U(?=[ \t\r\n]|$)`,
				Type:    chroma.GenericHeading,
				Mutator: chroma.Push("metadata"),
			},
			{
				Pattern: `(#EXTINF)(:)(` + number + `)`,
				Type:    chroma.ByGroups(chroma.Keyword, chroma.Operator, chroma.Number),
				Mutator: chroma.Push("duration"),
			},
			{
				Pattern: `(#EXT[A-Z0-9-]+)(:)`,
				Type:    chroma.ByGroups(chroma.Keyword, chroma.Operator),
				Mutator: chroma.Push("metadata"),
			},
			{Pattern: `#EXT[A-Z0-9-]+(?=[ \t\r\n]|$)`, Type: chroma.Keyword},
			{Pattern: `#[^\r\n]*`, Type: chroma.Comment},
			{Pattern: `[^#\r\n][^\r\n]*`, Type: chroma.Text},
		},
		"duration": {
			// Only an unquoted comma starts the title. The title can contain commas.
			{
				Pattern: `(,)([^\r\n]*)`,
				Type:    chroma.ByGroups(chroma.Operator, chroma.GenericHeading),
			},
			chroma.Include("metadata"),
		},
		"metadata": {
			{
				Pattern: `([A-Za-z][\w-]*)([ \t]*)(=)`,
				Type:    chroma.ByGroups(chroma.NameVariable, chroma.Text, chroma.Operator),
			},
			{Pattern: `"[^"\r\n]*"?`, Type: chroma.String},
			{Pattern: number + `(?=[ \t,\r\n]|$)`, Type: chroma.Number},
			{Pattern: `,`, Type: chroma.Operator},
			{Pattern: `[ \t]+`, Type: chroma.Text},
			{Pattern: `[^" ,\t\r\n]+`, Type: chroma.String},
			{Pattern: `\r?\n|\r`, Type: chroma.Text, Mutator: chroma.Pop(1)},
		},
	}
}
