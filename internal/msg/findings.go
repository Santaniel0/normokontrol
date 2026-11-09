// Package msg holds every user-facing Russian string, so wording can be
// reviewed in one place. Other packages call these functions instead of
// writing Russian literals.
package msg

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/Santaniel0/normokontrol/internal/format"
)

// Num formats a number the Russian way: decimal comma, no trailing zeros.
func Num(f float64) string {
	s := strconv.FormatFloat(f, 'f', 2, 64)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	return strings.Replace(s, ".", ",", 1)
}

// Side names a page margin.
type Side int

const (
	SideLeft Side = iota + 1
	SideRight
	SideTop
	SideBottom
)

var sideNames = map[Side]string{SideLeft: "левое", SideRight: "правое", SideTop: "верхнее", SideBottom: "нижнее"}

// Margin: a margin differs from the profile.
func Margin(s Side, got, want float64) string {
	return fmt.Sprintf("%s поле %s мм, требуется %s мм", sideNames[s], Num(got), Num(want))
}

// PageSize: the page is not the required format.
func PageSize(gotW, gotH, wantW, wantH float64) string {
	return fmt.Sprintf("формат страницы %s × %s мм, требуется %s × %s мм", Num(gotW), Num(gotH), Num(wantW), Num(wantH))
}

// SectionPrefix names a section when the document has more than one.
func SectionPrefix(i, n int) string {
	if n < 2 {
		return ""
	}
	return fmt.Sprintf("раздел документа %d из %d: ", i+1, n)
}

// FontFamily: body text font differs from the profile.
func FontFamily(got, want string, required bool) string {
	if required {
		return fmt.Sprintf("шрифт %s, требуется %s", got, want)
	}
	return fmt.Sprintf("шрифт %s, рекомендуется %s", got, want)
}

// FontMixed: a paragraph's font differs from the rest of the text.
func FontMixed(got, dominant string) string {
	return fmt.Sprintf("шрифт %s, а в основном тексте %s", got, dominant)
}

// FontFragment: part of a paragraph uses another font, typically pasted text.
func FontFragment(font string, chars int) string {
	return fmt.Sprintf("фрагмент абзаца (%d знаков) набран шрифтом %s — похоже на вставку из другого источника", chars, font)
}

// FontSize: exact size mismatch.
func FontSize(got, want float64) string {
	return fmt.Sprintf("размер шрифта %s пт, требуется %s пт", Num(got), Num(want))
}

// FontSizeMin: size below the minimum.
func FontSizeMin(got, min float64) string {
	return fmt.Sprintf("размер шрифта %s пт, требуется не менее %s пт", Num(got), Num(min))
}

// FontSizeMixed: size differs from the rest of the text.
func FontSizeMixed(got, dominant float64) string {
	return fmt.Sprintf("размер шрифта %s пт, а в основном тексте %s пт", Num(got), Num(dominant))
}

// LineSpacing describes a line spacing value.
func LineSpacing(l format.LineSpacing) string {
	switch l.Rule {
	case format.LineExact:
		return "точно " + Num(float64(l.Value)/20) + " пт"
	case format.LineAtLeast:
		return "минимум " + Num(float64(l.Value)/20) + " пт"
	}
	m, _ := l.Multiple()
	switch m {
	case 1:
		return "одинарный"
	case 1.5:
		return "полуторный (1,5)"
	case 2:
		return "двойной"
	}
	return "множитель " + Num(m)
}

// Spacing: line spacing mismatch.
func Spacing(got format.LineSpacing, want float64) string {
	return fmt.Sprintf("межстрочный интервал %s, требуется %s", LineSpacing(got), Num(want))
}

// SpacingMixed: line spacing differs from the rest of the text.
func SpacingMixed(got, dominant format.LineSpacing) string {
	return fmt.Sprintf("межстрочный интервал %s, а в основном тексте %s", LineSpacing(got), LineSpacing(dominant))
}

var alignNames = map[format.Alignment]string{
	format.AlignLeft: "по левому краю", format.AlignCenter: "по центру",
	format.AlignRight: "по правому краю", format.AlignJustify: "по ширине",
}

// Align: alignment mismatch.
func Align(got, want format.Alignment) string {
	return fmt.Sprintf("выравнивание %s, требуется %s", alignNames[got], alignNames[want])
}

// AlignMixed: alignment differs from the rest of the text.
func AlignMixed(got, dominant format.Alignment) string {
	return fmt.Sprintf("выравнивание %s, а основной текст выровнен %s", alignNames[got], alignNames[dominant])
}

// Indent: first-line indent mismatch.
func Indent(gotCM, wantCM float64) string {
	if gotCM < 0 {
		return fmt.Sprintf("выступ первой строки %s см вместо абзацного отступа %s см", Num(-gotCM), Num(wantCM))
	}
	return fmt.Sprintf("абзацный отступ %s см, требуется %s см", Num(gotCM), Num(wantCM))
}

// IndentMixed: first-line indent differs from the rest of the text.
func IndentMixed(gotCM, dominantCM float64) string {
	return fmt.Sprintf("абзацный отступ %s см, а в основном тексте %s см", Num(gotCM), Num(dominantCM))
}

// IndentFake: the indent is typed with spaces or a tab.
func IndentFake(tab bool) string {
	if tab {
		return "абзацный отступ сделан табуляцией, а не настройкой абзаца"
	}
	return "абзацный отступ сделан пробелами, а не настройкой абзаца"
}

// EmptyParagraphs: empty paragraphs used as spacing.
func EmptyParagraphs(n int, beforeHeading bool) string {
	run := fmt.Sprintf("%d %s подряд", n, plural(n, "пустой абзац", "пустых абзаца", "пустых абзацев"))
	switch {
	case n >= 2 && beforeHeading:
		return run + " перед заголовком — для перехода на новую страницу нужен разрыв страницы (Ctrl+Enter)"
	case n >= 2:
		return run + " — интервалы задают настройкой абзаца, а не пустыми строками"
	}
	return "пустая строка между абзацами — интервалы задают настройкой абзаца"
}

// HeadingPeriod: heading ends with a period (6.2.1, 6.2.3).
const HeadingPeriod = "точка в конце заголовка (ГОСТ 7.32-2017, 6.2.1 и 6.2.3)"

// StructuralCase: structural element heading not in capitals (6.2.1).
const StructuralCase = "заголовок структурного элемента пишется прописными буквами (6.2.1)"

// StructuralCenter: structural element heading not centred (6.2.1).
const StructuralCenter = "заголовок структурного элемента располагают в середине строки (6.2.1)"

// HeadingLowercase: section heading starts with a small letter (6.2.3).
const HeadingLowercase = "заголовок раздела начинается со строчной буквы (6.2.3)"

// HeadingNoStyle: heading made with direct formatting only.
const HeadingNoStyle = "заголовок оформлен без стиля «Заголовок» — он не попадёт в автоматическое содержание"

// StructureMissing: required structural element is absent.
func StructureMissing(name string) string {
	return fmt.Sprintf("нет обязательного структурного элемента «%s»", name)
}

// StructureOrder: structural element out of order.
func StructureOrder(name, after string) string {
	return fmt.Sprintf("«%s» должен идти раньше, чем «%s»", name, after)
}

// BibHeading: reference list heading differs from the profile.
func BibHeading(got, want string) string {
	return fmt.Sprintf("список источников озаглавлен «%s», требуется «%s»", got, want)
}

func kindTitle(table bool) string {
	if table {
		return "Таблица"
	}
	return "Рисунок"
}

// FigureNoCaption: a picture without a caption below it.
const FigureNoCaption = "у рисунка нет подписи «Рисунок N – Название» (6.5.7)"

// TableNoCaption: a table without a "Таблица N" line above it.
const TableNoCaption = "у таблицы нет надписи «Таблица N – Название» над ней (6.6.3)"

// CaptionFormat: caption does not follow the pattern.
func CaptionFormat(table bool) string {
	if table {
		return "надпись таблицы не по образцу «Таблица 1 – Название»: слово целиком, тире, название с прописной без точки в конце (6.6.3)"
	}
	return "подпись рисунка не по образцу «Рисунок 1 – Название»: слово целиком, тире, название с прописной без точки в конце (6.5.7)"
}

// NumberingGap: numbers are not consecutive.
func NumberingGap(table bool, got, want string) string {
	return fmt.Sprintf("%s %s, ожидался номер %s", kindTitle(table), got, want)
}

// NumberingMixed: continuous and per-section numbering are mixed.
func NumberingMixed(table bool) string {
	if table {
		return "в документе смешана сквозная нумерация таблиц и нумерация в пределах раздела"
	}
	return "в документе смешана сквозная нумерация рисунков и нумерация в пределах раздела"
}

// NumberingScheme: numbering scheme contradicts the profile.
func NumberingScheme(table, gotContinuous bool) string {
	what := "рисунки"
	if table {
		what = "таблицы"
	}
	got, want := "по разделам (2.1, 2.2)", "сквозную (1, 2, 3)"
	if gotContinuous {
		got, want = "сквозной нумерацией (1, 2, 3)", "по разделам (2.1, 2.2)"
	}
	return fmt.Sprintf("%s пронумерованы %s, а профиль требует нумерацию %s", what, got, want)
}

// NotReferenced: a figure or table is never mentioned in the text.
func NotReferenced(table bool, number string) string {
	if table {
		return fmt.Sprintf("на таблицу %s нет ссылки в тексте (6.6.2)", number)
	}
	return fmt.Sprintf("на рисунок %s нет ссылки в тексте (6.5.1)", number)
}

// CitationMissing: [N] points past the end of the reference list.
func CitationMissing(n, total int) string {
	return fmt.Sprintf("ссылка [%d], а в списке источников %d %s", n, total, plural(total, "позиция", "позиции", "позиций"))
}

// CitationNoList: citations exist but the reference list is not found.
const CitationNoList = "в тексте есть ссылки вида [N], но список использованных источников не найден"

// SourceUncited: an entry of the reference list is never cited.
func SourceUncited(n int) string {
	return fmt.Sprintf("на источник %d нет ссылки в тексте", n)
}

// CitationOrder: a source is cited before a smaller number (6.16).
func CitationOrder(n, before int) string {
	return fmt.Sprintf("источник [%d] упоминается раньше, чем [%d]; по 6.16 список упорядочивают по первым ссылкам", n, before)
}

// Homoglyph: Latin letters hidden in Russian words or the reverse.
func Homoglyph(words []string) string {
	return fmt.Sprintf("смешаны латинские и русские буквы: %s", strings.Join(quoteAll(words), ", "))
}

func quoteAll(ws []string) []string {
	out := make([]string, len(ws))
	for i, w := range ws {
		out[i] = "«" + w + "»"
	}
	return out
}

func plural(n int, one, few, many string) string {
	n %= 100
	if n >= 11 && n <= 14 {
		return many
	}
	switch n % 10 {
	case 1:
		return one
	case 2, 3, 4:
		return few
	}
	return many
}

// Plural is exported for the report.
func Plural(n int, one, few, many string) string { return plural(n, one, few, many) }

var builtinStyleNames = map[string]string{
	"normal": "Обычный", "caption": "Название объекта", "title": "Заголовок",
	"list paragraph": "Абзац списка", "body text": "Основной текст",
	"default paragraph font": "Основной шрифт абзаца", "no spacing": "Без интервала",
}

var numberedStyle = regexp.MustCompile(`^(heading|toc) ([1-9])$`)

// StyleName shows a style the way Russian Word names it in its UI.
func StyleName(name string) string {
	low := strings.ToLower(name)
	if ru, ok := builtinStyleNames[low]; ok {
		return ru
	}
	if m := numberedStyle.FindStringSubmatch(low); m != nil {
		if m[1] == "heading" {
			return "Заголовок " + m[2]
		}
		return "Оглавление " + m[2]
	}
	return name
}

// Origin explains where a value comes from.
func Origin(o format.Origin) string {
	var s string
	switch o.Source {
	case format.SourceBuiltin:
		return "в файле не задано, действует значение Word по умолчанию"
	case format.SourceDocDefaults:
		s = "параметры документа по умолчанию"
	case format.SourceParagraphStyle:
		s = "стиль «" + StyleName(o.StyleName) + "»"
	case format.SourceCharacterStyle:
		s = "стиль знака «" + StyleName(o.StyleName) + "»"
	case format.SourceDirect:
		return "прямое форматирование"
	}
	if o.Theme != 0 {
		s += " → шрифт темы (" + o.Theme.String() + ")"
	}
	if o.Source == format.SourceParagraphStyle || o.Source == format.SourceCharacterStyle {
		s += " — исправьте стиль, а не каждый абзац"
	}
	return "источник: " + s
}
