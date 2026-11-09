package rules

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Santaniel0/normokontrol/internal/docxtest"
	"github.com/Santaniel0/normokontrol/internal/model"
	"github.com/Santaniel0/normokontrol/internal/ooxml"
	"github.com/Santaniel0/normokontrol/internal/profile"
)

var (
	P, T, R = docxtest.P, docxtest.T, docxtest.R
	h1      = func(s string) string { return P(`<w:pStyle w:val="1"/>`, T(s)) }
	h2      = func(s string) string { return P(`<w:pStyle w:val="2"/>`, T(s)) }
	body    = func(s string) string { return P("", T(s)) }
)

func doc(t *testing.T, d docxtest.Doc) *model.Document {
	t.Helper()
	if d.Styles == "" {
		d.Styles = docxtest.GOSTStyles
	}
	data, err := docxtest.Build(d)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := ooxml.Open(bytes.NewReader(data), int64(len(data)), ooxml.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	od, err := ooxml.Load(pkg)
	if err != nil {
		t.Fatal(err)
	}
	return model.Build(od, model.Options{})
}

func gost(t *testing.T) *profile.Profile {
	t.Helper()
	p, err := profile.Load(profile.DefaultName)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// only runs one rule and returns its messages.
func only(t *testing.T, id ID, d *model.Document, p *profile.Profile) []Finding {
	t.Helper()
	for _, r := range All() {
		if r.ID() == id {
			return r.Check(d, p)
		}
	}
	t.Fatalf("no rule %s", id)
	return nil
}

func messages(fs []Finding) []string {
	var out []string
	for _, f := range fs {
		out = append(out, f.Message)
	}
	return out
}

func TestRules(t *testing.T) {
	narrow := docxtest.SectA4()
	narrow.RightMM = 10
	letter := docxtest.Page{WidthMM: 215.9, HeightMM: 279.4, TopMM: 20, RightMM: 15, BottomMM: 20, LeftMM: 30}
	tests := []struct {
		name    string
		rule    ID
		body    string
		image   bool
		profile func(*profile.Profile)
		want    []string // substrings, one per expected finding, in order
		detail  string   // substring expected in the first finding's detail
	}{
		{
			name: "margins per section",
			rule: PageMargins,
			body: h1("ВВЕДЕНИЕ") + P(docxtest.Sect(narrow), T("x")) + body("y") + docxtest.Sect(letter),
			want: []string{"раздел документа 1 из 2: правое поле 10 мм", "раздел документа 2 из 2: формат страницы 215,9 × 279,4 мм"},
		},
		{
			name: "margins in the only section",
			rule: PageMargins,
			body: h1("ВВЕДЕНИЕ") + body("Текст.") + docxtest.Sect(narrow),
			want: []string{"правое поле 10 мм, требуется 15 мм"},
		},
		{
			name:   "recommended font gives a warning with its origin",
			rule:   BodyFont,
			body:   h1("ВВЕДЕНИЕ") + P("", R(`<w:rFonts w:ascii="Arial" w:hAnsi="Arial"/>`, "Текст абзаца")),
			want:   []string{"шрифт Arial, рекомендуется Times New Roman"},
			detail: "прямое форматирование",
		},
		{
			name:    "required font and exact size are errors",
			rule:    BodyFont,
			body:    h1("ВВЕДЕНИЕ") + P("", R(`<w:rFonts w:ascii="Arial" w:hAnsi="Arial"/><w:sz w:val="24"/>`, "Текст абзаца")),
			profile: func(p *profile.Profile) { p.Body.FontRequired = true; p.Body.SizePt = 14 },
			want:    []string{"шрифт Arial, требуется Times New Roman", "размер шрифта 12 пт, требуется 14 пт"},
		},
		{
			name: "pasted fragment in another font",
			rule: BodyFont,
			body: h1("ВВЕДЕНИЕ") + P("", T("Начало длинного абзаца текста, "), R(`<w:rFonts w:ascii="Calibri" w:hAnsi="Calibri"/>`, "вставка из сайта")),
			want: []string{"фрагмент абзаца (14 знаков) набран шрифтом Calibri"},
		},
		{
			name: "line spacing",
			rule: BodySpacing,
			body: h1("ВВЕДЕНИЕ") + P(`<w:spacing w:line="240" w:lineRule="auto"/>`, T("Текст")) +
				P(`<w:spacing w:line="320" w:lineRule="exact"/>`, T("Текст")) + body("Текст"),
			want: []string{"интервал одинарный, требуется 1,5", "интервал точно 16 пт, требуется 1,5"},
		},
		{
			name: "alignment is only checked for consistency by default",
			rule: BodyAlign,
			body: h1("ВВЕДЕНИЕ") + body("Один.") + body("Два.") + body("Три.") + P(`<w:jc w:val="left"/>`, T("Четыре.")),
			want: []string{"выравнивание по левому краю, а основной текст выровнен по ширине"},
		},
		{
			name:    "alignment required by profile",
			rule:    BodyAlign,
			body:    h1("ВВЕДЕНИЕ") + P(`<w:jc w:val="center"/>`, T("Текст.")),
			profile: func(p *profile.Profile) { _ = p.Body.Align.UnmarshalText([]byte("justify")) },
			want:    []string{"выравнивание по центру, требуется по ширине"},
		},
		{
			name: "indent wrong, faked with a tab and with spaces",
			rule: BodyIndent,
			body: h1("ВВЕДЕНИЕ") + P(`<w:ind w:firstLine="567"/>`, T("Текст")) +
				P(`<w:ind w:firstLine="0"/>`, `<w:r><w:tab/><w:t>Текст</w:t></w:r>`) +
				P(`<w:ind w:firstLine="0"/>`, T("    Текст")) + body("Нормальный"),
			want: []string{"абзацный отступ 1 см, требуется 1,25 см", "табуляцией", "пробелами"},
		},
		{
			name:  "empty paragraphs",
			rule:  BodyEmpty,
			image: true,
			body: h1("ВВЕДЕНИЕ") + body("Один.") + P("", "") + P("", "") + body("Два.") + P("", "") + body("Три.") +
				P("", "") + P("", docxtest.Picture(1)) + P("", T("Рисунок 1 – Схема")) + body("Четыре.") + P("", "") + P("", "") + h1("ЗАКЛЮЧЕНИЕ"),
			want: []string{"2 пустых абзаца подряд — интервалы", "пустая строка между абзацами", "2 пустых абзаца подряд перед заголовком"},
		},
		{
			name: "heading format",
			rule: HeadingFormat,
			body: h1("Введение") + body("x") + h2("1.1 обзор аналогов.") + body("x") +
				P("", R("<w:b/>", "2 Анализ требований")) + body("x") + P(`<w:pStyle w:val="2"/><w:jc w:val="left"/>`, T("ЗАКЛЮЧЕНИЕ")),
			want: []string{"прописными буквами", "точка в конце заголовка", "со строчной буквы", "без стиля", "в середине строки"},
		},
		{
			name: "structure presence and order",
			rule: StructureList,
			body: h1("СОДЕРЖАНИЕ") + h1("ЗАКЛЮЧЕНИЕ") + h1("ВВЕДЕНИЕ") + h1("СПИСОК ЛИТЕРАТУРЫ"),
			want: []string{"нет обязательного структурного элемента «РЕФЕРАТ»", "«ВВЕДЕНИЕ» должен идти раньше, чем «ЗАКЛЮЧЕНИЕ»",
				"озаглавлен «СПИСОК ЛИТЕРАТУРЫ», требуется «СПИСОК ИСПОЛЬЗОВАННЫХ ИСТОЧНИКОВ»"},
		},
		{
			name:  "captions",
			rule:  Captions,
			image: true,
			body: h1("ВВЕДЕНИЕ") + body("Схема показана на рисунках 1–3, данные в таблице 1.") +
				P("", docxtest.Picture(1)) + P("", T("Рисунок 1 – Схема")) +
				P("", docxtest.Picture(2)) + P("", T("Рис. 3. Схема")) +
				P("", docxtest.Picture(3)) + body("Текст после рисунка без подписи.") +
				docxtest.Table([]string{P("", T("a")), P("", T("b"))}) + body("x") +
				P("", T("Таблица 1 – Данные")) + docxtest.Table([]string{P("", T("a")), P("", T("b"))}) +
				P("", T("Таблица 2 – Ещё данные")) + docxtest.Table([]string{P("", T("c")), P("", T("d"))}),
			want: []string{"подпись рисунка не по образцу", "Рисунок 3, ожидался номер 2", "у рисунка нет подписи",
				"у таблицы нет надписи", "на таблицу 2 нет ссылки"},
		},
		{
			name:    "per-section numbering against a continuous profile",
			rule:    Captions,
			image:   true,
			profile: func(p *profile.Profile) { _ = p.Captions.Numbering.UnmarshalText([]byte("continuous")) },
			body: h1("ВВЕДЕНИЕ") + body("См. рис. 1.1 и 1.2.") + P("", docxtest.Picture(1)) + P("", T("Рисунок 1.1 – А")) +
				P("", docxtest.Picture(2)) + P("", T("Рисунок 1.2 – Б")),
			want: []string{"рисунки пронумерованы по разделам"},
		},
		{
			name: "citations",
			rule: Citations,
			body: h1("ВВЕДЕНИЕ") + body("Как показано в [1, с. 15] и [5], а также [2023] и [см. выше].") +
				h1("СПИСОК ИСПОЛЬЗОВАННЫХ ИСТОЧНИКОВ") + body("1. Иванов И. И. Книга. 2020.") + body("2. Петров П. П. Статья. 2021."),
			want: []string{"ссылка [5], а в списке источников 2 позиции", "на источник 2 нет ссылки"},
		},
		{
			name: "citation order",
			rule: CitationOrder,
			body: h1("ВВЕДЕНИЕ") + body("Сначала [2], потом [1; 3].") + body("Снова [2–3].") +
				h1("СПИСОК ИСПОЛЬЗОВАННЫХ ИСТОЧНИКОВ") + body("1. А") + body("2. Б") + body("3. В"),
			want: []string{"источник [2] упоминается раньше, чем [1]"},
		},
		{
			name: "homoglyphs",
			rule: Homoglyphs,
			body: h1("ВВЕДЕНИЕ") + body("Анализ прeдприятия и Windоws, но IT-отдел и Python в порядке."),
			want: []string{"«прeдприятия», «Windоws»"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := gost(t)
			if tt.profile != nil {
				tt.profile(p)
			}
			got := only(t, tt.rule, doc(t, docxtest.Doc{Body: tt.body, Image: tt.image}), p)
			msgs := messages(got)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d findings, want %d:\n%s", len(got), len(tt.want), strings.Join(msgs, "\n"))
			}
			for _, w := range tt.want {
				if !slices.ContainsFunc(msgs, func(m string) bool { return strings.Contains(m, w) }) {
					t.Errorf("no finding contains %q:\n%s", w, strings.Join(msgs, "\n"))
				}
			}
			if tt.detail != "" && !strings.Contains(got[0].Detail, tt.detail) {
				t.Errorf("detail = %q, want it to contain %q", got[0].Detail, tt.detail)
			}
		})
	}
}

func TestThemeFontOriginIsExplained(t *testing.T) {
	// Normal sets no font: Word falls back to docDefaults → theme → Calibri.
	styles := strings.Replace(docxtest.GOSTStyles,
		`<w:rFonts w:ascii="Times New Roman" w:hAnsi="Times New Roman" w:cs="Times New Roman"/>`, "", 1)
	d := doc(t, docxtest.Doc{Styles: styles, Body: h1("ВВЕДЕНИЕ") + body("Текст")})
	got := only(t, BodyFont, d, gost(t))
	if len(got) != 1 || !strings.Contains(got[0].Message, "Calibri") || !strings.Contains(got[0].Detail, "шрифт темы") {
		t.Fatalf("got %+v", got)
	}
}

func TestCleanDocumentHasNoFindings(t *testing.T) {
	b := h1("РЕФЕРАТ") + body("Работа содержит 1 рисунок и 1 таблицу.") +
		h1("СОДЕРЖАНИЕ") + body("Введение\t3") +
		h1("ВВЕДЕНИЕ") + body("Цель работы описана в [1]. Схема приведена на рисунке 1.") +
		P("", docxtest.Picture(1)) + P(`<w:ind w:firstLine="0"/><w:jc w:val="center"/>`, T("Рисунок 1 – Схема")) +
		body("Данные приведены в таблице 1.") +
		P(`<w:ind w:firstLine="0"/><w:jc w:val="left"/>`, T("Таблица 1 – Данные")) +
		docxtest.Table([]string{P("", T("a")), P("", T("b"))}) + body("Продолжение текста [2].") +
		h1("ЗАКЛЮЧЕНИЕ") + body("Выводы.") +
		h1("СПИСОК ИСПОЛЬЗОВАННЫХ ИСТОЧНИКОВ") + body("1. Иванов И. И. Книга. 2020.") + body("2. Петров П. П. Статья. 2021.")
	d := doc(t, docxtest.Doc{Body: b, Image: true})
	got, err := Run(context.Background(), d, gost(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no findings:\n%s", strings.Join(messages(got), "\n"))
	}
}

func TestDisableAndUnknownRule(t *testing.T) {
	p := gost(t)
	p.Rules.Disable = []string{string(Homoglyphs)}
	enabled, err := Enabled(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(enabled) != len(All())-1 {
		t.Errorf("enabled %d rules", len(enabled))
	}
	p.Rules.Disable = []string{"body.fonts"}
	var ue *UnknownRuleError
	if _, err := Enabled(p); !errors.As(err, &ue) || ue.ID != "body.fonts" {
		t.Errorf("err = %v", err)
	}
}

func TestRunHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Run(ctx, doc(t, docxtest.Doc{Body: body("x")}), gost(t)); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v", err)
	}
}

func TestExpandRefs(t *testing.T) {
	tests := map[string][]string{
		"2–4, 7":   {"2", "3", "4", "7"},
		"1.2–1.4":  {"1.2", "1.3", "1.4"},
		"3 и 5":    {"3", "5"},
		"а.1":      {"А.1"},
		"1.9–2.1":  {"1.9", "2.1"},
		"1-200":    {"1", "200"},
		"А.1, А.2": {"А.1", "А.2"},
	}
	for in, want := range tests {
		if got := expandRefs(in); !slices.Equal(got, want) {
			t.Errorf("expandRefs(%q) = %v, want %v", in, got, want)
		}
	}
	if got := parseCitation("1, 3–5; 9, с. 15"); !slices.Equal(got, []int{1, 3, 4, 5, 9}) {
		t.Errorf("parseCitation = %v", got)
	}
}
