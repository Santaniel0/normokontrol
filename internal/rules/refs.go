package rules

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/Santaniel0/normokontrol/internal/model"
	"github.com/Santaniel0/normokontrol/internal/msg"
	"github.com/Santaniel0/normokontrol/internal/profile"
)

var (
	bracketRe = regexp.MustCompile(`\[([^\[\]]{1,80})\]`)
	numTokRe  = regexp.MustCompile(`\d+|[–—-]`)
)

// maxSource keeps years in brackets ("[2023]") from being read as citations.
const maxSource = 500

// parseCitation extracts source numbers from the inside of [...]: "1",
// "1, 3", "2–5", "1, с. 15", "1; 4, с. 7". Parts that start with a letter
// ("см. выше") are not citations.
func parseCitation(s string) []int {
	var out []int
	for _, part := range strings.Split(s, ";") {
		part = strings.TrimSpace(part)
		if part == "" || !unicode.IsDigit([]rune(part)[0]) {
			continue
		}
		if i := strings.IndexFunc(part, unicode.IsLetter); i >= 0 {
			part = part[:i] // page reference: "с. 15", "p. 3"
		}
		toks := numTokRe.FindAllString(part, -1)
		for i := 0; i < len(toks); i++ {
			n, err := strconv.Atoi(toks[i])
			if err != nil || n < 1 || n > maxSource {
				continue
			}
			out = append(out, n)
			if i+2 < len(toks) && strings.ContainsAny(toks[i+1], "–—-") {
				if end, err := strconv.Atoi(toks[i+2]); err == nil && end > n && end-n <= 50 {
					for k := n + 1; k <= end; k++ {
						out = append(out, k)
					}
					i += 2
				}
			}
		}
	}
	return out
}

type citation struct {
	para int
	nums []int
}

func citations(d *model.Document) []citation {
	var out []citation
	for _, p := range d.Paras {
		switch p.Role {
		case model.RoleBody, model.RoleListItem, model.RoleTableCell, model.RoleCaption:
		default:
			continue
		}
		var nums []int
		for _, m := range bracketRe.FindAllStringSubmatch(p.Text, -1) {
			nums = append(nums, parseCitation(m[1])...)
		}
		if len(nums) > 0 {
			out = append(out, citation{para: p.Index, nums: nums})
		}
	}
	return out
}

func checkCitations(d *model.Document, _ *profile.Profile) []Finding {
	var out []Finding
	entries := d.BibEntries()
	cites := citations(d)
	if len(entries) == 0 {
		if len(cites) > 0 {
			out = append(out, finding(Citations, SeverityError, -1, msg.CitationNoList, ""))
		}
		return out
	}
	cited := map[int]bool{}
	for _, c := range cites {
		reported := map[int]bool{}
		for _, n := range c.nums {
			cited[n] = true
			if n > len(entries) && !reported[n] {
				reported[n] = true
				out = append(out, finding(Citations, SeverityError, c.para, msg.CitationMissing(n, len(entries)), ""))
			}
		}
	}
	for i, e := range entries {
		if !cited[i+1] {
			out = append(out, finding(Citations, SeverityWarning, e.Index, msg.SourceUncited(i+1), ""))
		}
	}
	return out
}

// checkCitationOrder follows 6.16: sources are listed in the order of their
// first citation, so the first mention of [n] must come after [1]..[n-1].
// Many universities require an alphabetical list instead; they disable
// this rule in their profile.
func checkCitationOrder(d *model.Document, _ *profile.Profile) []Finding {
	var out []Finding
	total := len(d.BibEntries())
	cites := citations(d)
	// Sources that are never cited are reported by refs.citations; they
	// must not make every later citation look out of order.
	var pending []int
	ever := map[int]bool{}
	for _, c := range cites {
		for _, n := range c.nums {
			if n <= total && !ever[n] {
				ever[n] = true
				pending = append(pending, n)
			}
		}
	}
	slices.Sort(pending)
	seen := map[int]bool{}
	for _, c := range cites {
		for _, n := range c.nums {
			if seen[n] || n > total {
				continue
			}
			seen[n] = true
			if lowest := pending[0]; n > lowest {
				out = append(out, finding(CitationOrder, SeverityWarning, c.para, msg.CitationOrder(n, lowest), ""))
			}
			pending = slices.DeleteFunc(pending, func(v int) bool { return v == n })
		}
	}
	return out
}

// Latin letters that look like Cyrillic ones and vice versa.
const (
	latinLookalikes    = "aceopxyABCEHKMOPTX"
	cyrillicLookalikes = "асеорхуАВСЕНКМОРТХ"
)

// checkHomoglyphs finds words that mix scripts where the minority letters
// are lookalikes: "прeдприятие" with a Latin e, or "Windоws" with a
// Cyrillic о. Such words break search and spell checking, and swapping
// letters is a known trick against plagiarism checkers, so reviewers look
// for it.
func checkHomoglyphs(d *model.Document, _ *profile.Profile) []Finding {
	var out []Finding
	for _, p := range d.Paras {
		switch p.Role {
		case model.RoleCode, model.RoleFormula, model.RoleTOC, model.RoleEmpty, model.RoleFigure:
			continue
		}
		var bad []string
		for _, w := range strings.FieldsFunc(p.Text, func(r rune) bool { return !unicode.IsLetter(r) }) {
			if mixedLookalike(w) && len(bad) < 3 {
				bad = append(bad, w)
			}
		}
		if len(bad) > 0 {
			out = append(out, finding(Homoglyphs, SeverityError, p.Index, msg.Homoglyph(bad), ""))
		}
	}
	return out
}

func mixedLookalike(w string) bool {
	var cyr, lat []rune
	for _, r := range w {
		switch {
		case unicode.Is(unicode.Cyrillic, r):
			cyr = append(cyr, r)
		case unicode.Is(unicode.Latin, r):
			lat = append(lat, r)
		}
	}
	if len(cyr) == 0 || len(lat) == 0 {
		return false
	}
	minority, set := lat, latinLookalikes
	if len(lat) > len(cyr) {
		minority, set = cyr, cyrillicLookalikes
	}
	for _, r := range minority {
		if !strings.ContainsRune(set, r) {
			return false
		}
	}
	return true
}
