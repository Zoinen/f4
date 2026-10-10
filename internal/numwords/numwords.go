// Package numwords writes numbers and amounts of money in words, in Russian
// and English: 1234.56 руб. becomes "одна тысяча двести тридцать четыре рубля
// 56 копеек" (f4#1463). It knows roubles, dollars and euros.
package numwords

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// Lang is the language the words are written in.
type Lang int

const (
	RU Lang = iota
	EN
)

// maxDigits is the most integer digits handled: the biggest scale known is a
// trillion, so 999 trillion and change.
const maxDigits = 15

var (
	ruUnitsMasc = []string{"", "один", "два", "три", "четыре", "пять", "шесть", "семь", "восемь", "девять", "десять",
		"одиннадцать", "двенадцать", "тринадцать", "четырнадцать", "пятнадцать", "шестнадцать", "семнадцать", "восемнадцать", "девятнадцать"}
	ruTens     = []string{"", "", "двадцать", "тридцать", "сорок", "пятьдесят", "шестьдесят", "семьдесят", "восемьдесят", "девяносто"}
	ruHundreds = []string{"", "сто", "двести", "триста", "четыреста", "пятьсот", "шестьсот", "семьсот", "восемьсот", "девятьсот"}
	// ruScales[i] is the word for 1000^(i+1) in its three plural forms; the
	// thousand is feminine, the rest masculine.
	ruScales = [][3]string{
		{"тысяча", "тысячи", "тысяч"},
		{"миллион", "миллиона", "миллионов"},
		{"миллиард", "миллиарда", "миллиардов"},
		{"триллион", "триллиона", "триллионов"},
	}

	enUnits = []string{"", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten",
		"eleven", "twelve", "thirteen", "fourteen", "fifteen", "sixteen", "seventeen", "eighteen", "nineteen"}
	enTens   = []string{"", "", "twenty", "thirty", "forty", "fifty", "sixty", "seventy", "eighty", "ninety"}
	enScales = []string{"thousand", "million", "billion", "trillion"}
)

// currency names a unit of money and its minor unit in both languages: the
// Russian forms are for 1, 2-4 and 5+, the English for 1 and the rest.
type currency struct {
	ruMajor, ruMinor [3]string
	enMajor, enMinor [2]string
}

var (
	rouble = currency{
		ruMajor: [3]string{"рубль", "рубля", "рублей"}, ruMinor: [3]string{"копейка", "копейки", "копеек"},
		enMajor: [2]string{"ruble", "rubles"}, enMinor: [2]string{"kopeck", "kopecks"},
	}
	dollar = currency{
		ruMajor: [3]string{"доллар", "доллара", "долларов"}, ruMinor: [3]string{"цент", "цента", "центов"},
		enMajor: [2]string{"dollar", "dollars"}, enMinor: [2]string{"cent", "cents"},
	}
	euro = currency{
		ruMajor: [3]string{"евро", "евро", "евро"}, ruMinor: [3]string{"цент", "цента", "центов"},
		enMajor: [2]string{"euro", "euros"}, enMinor: [2]string{"cent", "cents"},
	}
)

// ruForm picks the Russian plural form for n: 1, 21, ... / 2-4, 22-24, ... /
// the rest, with 11-14 among the rest.
func ruForm[T uint64 | int](n T, f [3]string) string {
	if r := n % 100; r >= 11 && r <= 14 {
		return f[2]
	}
	switch n % 10 {
	case 1:
		return f[0]
	case 2, 3, 4:
		return f[1]
	}
	return f[2]
}

// group writes 1..999 in words. feminine only matters in Russian (одна, две).
func group(n int, lang Lang, feminine bool) []string {
	var w []string
	if lang == EN {
		if h := n / 100; h > 0 {
			w = append(w, enUnits[h], "hundred")
		}
		n %= 100
		switch {
		case n == 0:
		case n < 20:
			w = append(w, enUnits[n])
		default:
			t := enTens[n/10]
			if n%10 != 0 {
				t += "-" + enUnits[n%10]
			}
			w = append(w, t)
		}
		return w
	}
	if h := n / 100; h > 0 {
		w = append(w, ruHundreds[h])
	}
	n %= 100
	switch {
	case n == 0:
	case n < 20:
		u := ruUnitsMasc[n]
		if feminine {
			switch n {
			case 1:
				u = "одна"
			case 2:
				u = "две"
			}
		}
		w = append(w, u)
	default:
		w = append(w, ruTens[n/10])
		if u := n % 10; u != 0 {
			s := ruUnitsMasc[u]
			if feminine {
				switch u {
				case 1:
					s = "одна"
				case 2:
					s = "две"
				}
			}
			w = append(w, s)
		}
	}
	return w
}

// Words writes n in words. feminine picks the Russian feminine forms of one
// and two (of the last group, and always for thousands).
func Words(n uint64, lang Lang, feminine bool) string {
	if n == 0 {
		if lang == EN {
			return "zero"
		}
		return "ноль"
	}
	var groups []int
	for v := n; v > 0; v /= 1000 {
		groups = append(groups, int(v%1000))
	}
	var w []string
	for i := len(groups) - 1; i >= 0; i-- {
		g := groups[i]
		if g == 0 {
			continue
		}
		if i == 0 {
			w = append(w, group(g, lang, feminine)...)
			continue
		}
		w = append(w, group(g, lang, lang == RU && i == 1)...)
		if lang == EN {
			w = append(w, enScales[i-1])
		} else {
			w = append(w, ruForm(g, ruScales[i-1]))
		}
	}
	return strings.Join(w, " ")
}

func (c currency) major(n uint64, lang Lang) string {
	if lang == EN {
		return c.enMajor[b2i(n != 1)]
	}
	return ruForm(n, c.ruMajor)
}

func (c currency) minor(n int, lang Lang) string {
	if lang == EN {
		return c.enMinor[b2i(n != 1)]
	}
	return ruForm(n, c.ruMinor)
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ErrNoNumber is returned when the text holds no number.
var ErrNoNumber = errors.New("no number found")

// parsed is the number found in a text and what stood around it.
type parsed struct {
	negative bool
	whole    uint64
	minor    int  // 0..99
	hasMinor bool // the text had a non-zero fractional part
	rest     string
}

// parseAmount finds the number in text: an optional sign, digits that may be
// grouped by spaces, and an optional fraction after '.' or ','. A comma
// followed by exactly three digits is a thousands separator. Everything else
// in the text is returned as rest, for the currency and the language.
func parseAmount(text string) (parsed, error) {
	runes := []rune(text)
	start := -1
	for i, r := range runes {
		if isDigit(r) {
			start = i
			break
		}
	}
	if start < 0 {
		return parsed{}, ErrNoNumber
	}
	var p parsed
	before := runes[:start]
	if n := len(before); n > 0 && (before[n-1] == '-' || before[n-1] == '−') {
		p.negative = true
		before = before[:n-1]
	}
	var intDigits, frac strings.Builder
	i := start
	for i < len(runes) {
		r := runes[i]
		switch {
		case r >= '0' && r <= '9':
			intDigits.WriteRune(r)
			i++
			continue
		case (r == ' ' || r == ' ' || r == '\'') && i+3 < len(runes) && isDigits(runes[i+1:i+4]) &&
			(i+4 >= len(runes) || !isDigit(runes[i+4])):
			i++ // a group separator: 1 234 567
			continue
		case (r == ',' || r == '.') && i+1 < len(runes) && isDigit(runes[i+1]):
			j := i + 1
			for j < len(runes) && isDigit(runes[j]) {
				j++
			}
			digits := string(runes[i+1 : j])
			if r == ',' && len(digits) == 3 {
				intDigits.WriteString(digits) // 1,234 is a thousands separator
				i = j
				continue
			}
			frac.WriteString(digits)
			i = j
		}
		break
	}
	if intDigits.Len() > maxDigits {
		return parsed{}, fmt.Errorf("the number is too big (more than %d digits)", maxDigits)
	}
	whole, err := strconv.ParseUint(intDigits.String(), 10, 64)
	if err != nil {
		return parsed{}, err
	}
	p.whole = whole
	if f := frac.String(); f != "" {
		if len(f) > 2 {
			return parsed{}, errors.New("more than two digits after the decimal point")
		}
		if len(f) == 1 {
			f += "0"
		}
		p.minor, _ = strconv.Atoi(f)
		p.hasMinor = p.minor != 0
	}
	p.rest = strings.TrimSpace(string(before) + " " + string(runes[i:]))
	return p, nil
}

func isDigits(r []rune) bool {
	for _, c := range r {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// detect reads the currency and the language off what stood around the
// number: руб/rub/₽, $/usd/дол, €/eur/евр; a Cyrillic word means Russian, a
// Latin one English, a bare symbol leaves the language to def.
func detect(rest string, def Lang) (*currency, Lang, error) {
	low := strings.ToLower(rest)
	lang := def
	for _, r := range low {
		if unicode.Is(unicode.Cyrillic, r) {
			lang = RU
			break
		}
		if r < 128 && unicode.IsLetter(r) {
			lang = EN
		}
	}
	has := func(subs ...string) bool {
		for _, s := range subs {
			if strings.Contains(low, s) {
				return true
			}
		}
		return false
	}
	switch {
	case has("руб", "rub", "₽", "коп", "kop"):
		return &rouble, lang, nil
	case has("$", "usd", "дол", "dol"):
		return &dollar, lang, nil
	case has("€", "eur", "евр"):
		return &euro, lang, nil
	}
	for _, r := range low {
		if unicode.IsLetter(r) {
			return nil, lang, fmt.Errorf("unknown currency %q", strings.TrimSpace(rest))
		}
	}
	return nil, lang, nil
}

// Amount writes the amount found in text in words. text is a number, alone
// ("1234") or with a currency ("1234.56 руб.", "$12.50", "99 EUR"). def is the
// language for text that does not say one. A currency amount comes out as
// "<words> <currency>", with the minor unit in digits when there is one:
// "одна тысяча двести тридцать четыре рубля 56 копеек". A number with no
// currency has to be whole.
func Amount(text string, def Lang) (string, error) {
	p, err := parseAmount(text)
	if err != nil {
		return "", err
	}
	cur, lang, err := detect(p.rest, def)
	if err != nil {
		return "", err
	}
	words := Words(p.whole, lang, false)
	if p.negative {
		if lang == EN {
			words = "minus " + words
		} else {
			words = "минус " + words
		}
	}
	if cur == nil {
		if p.hasMinor {
			return "", errors.New("a fractional number needs a currency")
		}
		return words, nil
	}
	out := words + " " + cur.major(p.whole, lang)
	if p.hasMinor {
		out += fmt.Sprintf(" %02d %s", p.minor, cur.minor(p.minor, lang))
	}
	return out, nil
}

func isDigit(r rune) bool { return r >= '0' && r <= '9' }
