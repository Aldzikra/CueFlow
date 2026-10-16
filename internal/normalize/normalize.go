// Package normalize turns free text (script or STT transcript) into a single
// canonical token form so the aligner can compare the two. It is pure: no I/O.
package normalize

import (
	"regexp"
	"strconv"
	"strings"
)

type Lang string

const (
	ID Lang = "id"
	EN Lang = "en"
)

// Token is one canonical word plus the byte range in the original text that
// produced it (for highlighting). One source word may yield several tokens
// (e.g. "2026" -> five tokens) that all share the same range.
type Token struct {
	Text       string
	Start, End int
}

// Chunks: Rp-amounts, numbers (with optional % or ordinal suffix), then words
// (letters/digits with inner apostrophes). Hyphens split words on purpose.
var chunkRe = regexp.MustCompile(`(?i)rp\.?\s?[0-9]+(?:[.,][0-9]+)*|[0-9]+(?:[.,][0-9]+)*(?:%|(?:st|nd|rd|th)\b)?|[\p{L}\p{N}]+(?:['’][\p{L}\p{N}]+)*`)

var numStartRe = regexp.MustCompile(`^(?:rp\.?\s?)?[0-9]`)

var fold = func() *strings.Replacer {
	var p []string
	for base, chars := range map[string]string{"a": "àáâãäå", "e": "èéêë", "i": "ìíîï", "o": "òóôõö", "u": "ùúûü", "n": "ñ", "c": "ç", "y": "ýÿ"} {
		for _, r := range chars {
			p = append(p, string(r), base)
		}
	}
	return strings.NewReplacer(p...)
}()

var idAbbrev = map[string]string{
	"yg": "yang", "dgn": "dengan", "dg": "dengan", "tdk": "tidak", "utk": "untuk", "krn": "karena",
	"dr": "dari", "sdh": "sudah", "udah": "sudah", "udh": "sudah", "blm": "belum", "jg": "juga",
	"tp": "tapi", "dlm": "dalam", "pd": "pada", "spt": "seperti", "bs": "bisa", "sy": "saya",
	"aja": "saja", "klo": "kalau", "kalo": "kalau", "gak": "tidak", "ga": "tidak", "nggak": "tidak",
	"enggak": "tidak", "engga": "tidak", "dll": "dan lain lain", "dsb": "dan sebagainya",
}

var enContr = map[string]string{
	"won't": "will not", "can't": "can not", "cannot": "can not", "shan't": "shall not",
	"ain't": "is not", "let's": "let us", "gonna": "going to", "wanna": "want to",
}

var enSuffix = [][2]string{{"n't", " not"}, {"'re", " are"}, {"'m", " am"}, {"'ll", " will"}, {"'ve", " have"}, {"'d", " would"}}

var enIsHost = map[string]bool{"it": true, "he": true, "she": true, "that": true, "what": true, "there": true, "here": true, "who": true, "where": true, "how": true}

// Normalize returns the canonical tokens of text. The same function must be
// used for script and transcript.
func Normalize(text string, lang Lang) []Token {
	var out []Token
	lastWord := false
	for _, m := range chunkRe.FindAllStringIndex(text, -1) {
		s := fold.Replace(strings.ToLower(strings.ReplaceAll(text[m[0]:m[1]], "’", "'")))
		isNum := numStartRe.MatchString(s)
		var words []string
		if isNum {
			words = numberWords(s, lang)
		} else {
			words = wordTokens(s, lang)
		}
		for _, w := range words {
			// "buku-buku", "buku2" and "buku buku" share one canonical form.
			if lang == ID && !isNum && lastWord && len(out) > 0 && out[len(out)-1].Text == w &&
				strings.Trim(text[out[len(out)-1].End:m[0]], " \t-") == "" {
				out[len(out)-1].End = m[1]
				continue
			}
			out = append(out, Token{w, m[0], m[1]})
		}
		if len(words) > 0 {
			lastWord = !isNum
		}
	}
	return out
}

func wordTokens(s string, lang Lang) []string {
	if lang == EN {
		return strings.Fields(enContraction(s))
	}
	s = strings.ReplaceAll(s, "'", "")
	if n := len(s); n >= 4 && s[n-1] == '2' && !strings.ContainsAny(s[:n-1], "0123456789") {
		s = s[:n-1] // buku2
	}
	if full, ok := idAbbrev[s]; ok {
		s = full
	}
	return strings.Fields(s)
}

func enContraction(s string) string {
	if full, ok := enContr[s]; ok {
		return full
	}
	for _, sf := range enSuffix {
		if strings.HasSuffix(s, sf[0]) && len(s) > len(sf[0]) {
			return strings.TrimSuffix(s, sf[0]) + sf[1]
		}
	}
	if base, ok := strings.CutSuffix(s, "'s"); ok && base != "" {
		if enIsHost[base] {
			return base + " is"
		}
		return base // possessive
	}
	return strings.ReplaceAll(s, "'", "")
}

// numberWords expands a numeric chunk (already lowercased) into spoken words.
func numberWords(s string, lang Lang) []string {
	var tail []string
	ordinal := false
	switch {
	case strings.HasPrefix(s, "rp"):
		s = strings.TrimLeft(strings.TrimPrefix(s, "rp"), ". ")
		tail = []string{"rupiah"}
	case strings.HasSuffix(s, "%"):
		s = strings.TrimSuffix(s, "%")
		tail = []string{map[Lang]string{ID: "persen", EN: "percent"}[lang]}
	default:
		for _, sf := range []string{"st", "nd", "rd", "th"} {
			if t, ok := strings.CutSuffix(s, sf); ok && lang == EN {
				s, ordinal = t, true
				break
			}
		}
	}

	thou := byte('.')
	if lang == EN {
		thou = ','
	}
	groups := strings.FieldsFunc(s, func(r rune) bool { return r == '.' || r == ',' })
	var words []string
	switch {
	case len(groups) == 1:
		words = intWords(groups[0], lang, true)
	case validThousands(s, groups, thou):
		words = intWords(strings.Join(groups, ""), lang, false)
	case len(groups) == 2: // decimal, whichever separator was used
		words = append(intWords(groups[0], lang, false), map[Lang]string{ID: "koma", EN: "point"}[lang])
		for _, d := range groups[1] {
			words = append(words, intWords(string(d), lang, false)...)
		}
	default: // e.g. "1,2,3": read each group on its own
		for _, g := range groups {
			words = append(words, intWords(g, lang, false)...)
		}
	}
	if ordinal && len(words) > 0 {
		words[len(words)-1] = ordinalWord(words[len(words)-1])
	}
	return append(words, tail...)
}

// validThousands: "50.000" / "1,234,567" — one separator kind equal to thou,
// every group after the first exactly 3 digits.
func validThousands(s string, groups []string, thou byte) bool {
	if len(groups) < 2 || len(groups[0]) > 3 || strings.Count(s, string(thou)) != len(groups)-1 {
		return false
	}
	for _, g := range groups[1:] {
		if len(g) != 3 {
			return false
		}
	}
	return true
}

// intWords reads a digit string. year enables the EN "nineteen ninety nine" form.
func intWords(d string, lang Lang, year bool) []string {
	if len(d) > 1 && d[0] == '0' || len(d) > 15 { // phone-like: digit by digit
		var w []string
		for _, c := range d {
			w = append(w, intWords(string(c), lang, false)...)
		}
		return w
	}
	n, _ := strconv.ParseInt(d, 10, 64)
	if lang == ID {
		return strings.Fields(idInt(n))
	}
	// ponytail: only 4-digit plain numbers 1100-1999 / 2010-2099 are read as years; "two thousand twenty six" speakers will miss, window absorbs it.
	if year && len(d) == 4 && n%100 != 0 && (n >= 1100 && n < 2000 || n >= 2010 && n < 2100) {
		lo := n % 100
		s := enInt(n / 100)
		if lo < 10 {
			return strings.Fields(s + " oh " + enInt(lo))
		}
		return strings.Fields(s + " " + enInt(lo))
	}
	return strings.Fields(enInt(n))
}

var idUnits = []string{"nol", "satu", "dua", "tiga", "empat", "lima", "enam", "tujuh", "delapan", "sembilan", "sepuluh", "sebelas"}

func idInt(n int64) string {
	j := func(a, b string) string { return strings.TrimSpace(a + " " + b) }
	rest := func(n, unit int64) string {
		if n%unit == 0 {
			return ""
		}
		return idInt(n % unit)
	}
	switch {
	case n < 12:
		return idUnits[n]
	case n < 20:
		return idUnits[n-10] + " belas"
	case n < 100:
		return j(idUnits[n/10]+" puluh", rest(n, 10))
	case n < 200:
		return j("seratus", rest(n, 100))
	case n < 1000:
		return j(idUnits[n/100]+" ratus", rest(n, 100))
	case n < 2000:
		return j("seribu", rest(n, 1000))
	case n < 1e6:
		return j(idInt(n/1000)+" ribu", rest(n, 1000))
	case n < 1e9:
		return j(idInt(n/1e6)+" juta", rest(n, 1e6))
	case n < 1e12:
		return j(idInt(n/1e9)+" miliar", rest(n, 1e9))
	}
	return j(idInt(n/1e12)+" triliun", rest(n, 1e12))
}

var enUnits = []string{"zero", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten", "eleven", "twelve", "thirteen", "fourteen", "fifteen", "sixteen", "seventeen", "eighteen", "nineteen"}
var enTens = []string{"", "", "twenty", "thirty", "forty", "fifty", "sixty", "seventy", "eighty", "ninety"}

func enInt(n int64) string {
	j := func(a, b string) string { return strings.TrimSpace(a + " " + b) }
	rest := func(n, unit int64) string {
		if n%unit == 0 {
			return ""
		}
		return enInt(n % unit)
	}
	switch {
	case n < 20:
		return enUnits[n]
	case n < 100:
		return j(enTens[n/10], rest(n, 10))
	case n < 1000:
		return j(enUnits[n/100]+" hundred", rest(n, 100))
	case n < 1e6:
		return j(enInt(n/1000)+" thousand", rest(n, 1000))
	case n < 1e9:
		return j(enInt(n/1e6)+" million", rest(n, 1e6))
	case n < 1e12:
		return j(enInt(n/1e9)+" billion", rest(n, 1e9))
	}
	return j(enInt(n/1e12)+" trillion", rest(n, 1e12))
}

var ordSpecial = map[string]string{"one": "first", "two": "second", "three": "third", "five": "fifth", "eight": "eighth", "nine": "ninth", "twelve": "twelfth"}

func ordinalWord(w string) string {
	if o, ok := ordSpecial[w]; ok {
		return o
	}
	if base, ok := strings.CutSuffix(w, "y"); ok {
		return base + "ieth"
	}
	return w + "th"
}
