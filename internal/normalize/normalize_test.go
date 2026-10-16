package normalize

import (
	"reflect"
	"strings"
	"testing"
)

func texts(ts []Token) []string {
	var s []string
	for _, t := range ts {
		s = append(s, t.Text)
	}
	return s
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		name string
		lang Lang
		in   string
		want string
	}{
		{"punct+case", ID, "Halo, SEMUA! Apa kabar?", "halo semua apa kabar"},
		{"diacritics", EN, "Café naïve résumé", "cafe naive resume"},
		{"redup hyphen", ID, "Buku-buku itu", "buku itu"},
		{"redup digit", ID, "buku2 itu", "buku itu"},
		{"redup space", ID, "buku buku itu", "buku itu"},
		{"redup not across punct", ID, "ya, ya", "ya ya"},
		{"abbrev", ID, "Yg penting dgn tdk utk krn", "yang penting dengan tidak untuk karena"},
		{"year id", ID, "tahun 2026", "tahun dua ribu dua puluh enam"},
		{"year en", EN, "in 2026", "in twenty twenty six"},
		{"year en oh", EN, "in 1905", "in nineteen oh five"},
		{"percent id", ID, "naik 25%", "naik dua puluh lima persen"},
		{"percent en", EN, "up 25%", "up twenty five percent"},
		{"rupiah", ID, "Rp50.000", "lima puluh ribu rupiah"},
		{"rupiah spaced", ID, "Rp 1.500.000", "satu juta lima ratus ribu rupiah"},
		{"seratus", ID, "100 dan 1000", "seratus dan seribu"},
		{"decimal id", ID, "3,5", "tiga koma lima"},
		{"decimal en", EN, "3.5 and 1,200", "three point five and one thousand two hundred"},
		{"list not thousands", ID, "1,2,3", "satu dua tiga"},
		{"phone", ID, "0812", "nol delapan satu dua"},
		{"ordinal", EN, "the 1st and 22nd", "the first and twenty second"},
		{"contraction n't", EN, "Don't stop", "do not stop"},
		{"contraction I'm", EN, "I'm here", "i am here"},
		{"contraction we'll", EN, "we'll go, they've left, you'd see", "we will go they have left you would see"},
		{"contraction irregular", EN, "won't can't let's", "will not can not let us"},
		{"it's vs possessive", EN, "it's John's car", "it is john car"},
		{"curly apostrophe", EN, "don’t", "do not"},
		{"hyphen splits", EN, "well-known", "well known"},
		{"en no merge", EN, "that that", "that that"},
		{"empty", EN, "  ...  ", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.Join(texts(Normalize(tc.in, tc.lang)), " ")
			if got != tc.want {
				t.Errorf("Normalize(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestRanges(t *testing.T) {
	in := "Tahun 2026, buku-buku!"
	toks := Normalize(in, ID)
	for _, tk := range toks {
		if tk.Start < 0 || tk.End > len(in) || tk.Start >= tk.End {
			t.Fatalf("bad range %+v", tk)
		}
	}
	// 1 + 5 number tokens sharing "2026", then merged "buku-buku".
	want := []string{"Tahun", "2026", "2026", "2026", "2026", "2026", "buku-buku"}
	var got []string
	for _, tk := range toks {
		got = append(got, in[tk.Start:tk.End])
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ranges = %q, want %q", got, want)
	}
}
