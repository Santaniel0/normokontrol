package msg

import (
	"fmt"

	"github.com/Santaniel0/normokontrol/internal/model"
)

// Comment author shown in Word's review pane.
const (
	CommentAuthor   = "normokontrol"
	CommentInitials = "НК"
)

// Where renders a location for the report and for comments.
func Where(l model.Location) string {
	if l.Para < 0 {
		return "документ в целом"
	}
	if l.Role == model.RoleHeading {
		return "заголовок «" + l.Snippet + "»"
	}
	var at string
	switch {
	case l.Role == model.RoleEmpty && l.Ordinal == 0:
		at = "пустой абзац сразу после заголовка"
	case l.Role == model.RoleEmpty:
		at = fmt.Sprintf("пустой абзац после абз. %d", l.Ordinal)
	default:
		at = fmt.Sprintf("абз. %d", l.Ordinal)
	}
	switch {
	case l.Region == model.RegionFront:
		at = "до первого заголовка, " + at
	case l.Heading != "":
		at = "«" + l.Heading + "», " + at
	}
	if l.Snippet != "" && l.Role != model.RoleEmpty {
		at += ": «" + l.Snippet + "»"
	}
	return at
}

// Report wording.
const (
	ErrorsTitle   = "ОШИБКИ"
	WarningsTitle = "ПРЕДУПРЕЖДЕНИЯ"
	NoFindings    = "Замечаний нет."
	FirstLabel    = "первый случай"
)

// ProfileLine names the profile in the report header.
func ProfileLine(name string) string { return "Профиль: " + name }

// DocumentLine summarises what was checked.
func DocumentLine(file string, sections, paras, figures, tables, sources int) string {
	return fmt.Sprintf("Документ: %s — %d %s, %d %s, %d %s, %d %s, %d %s", file,
		sections, plural(sections, "раздел", "раздела", "разделов"),
		paras, plural(paras, "абзац", "абзаца", "абзацев"),
		figures, plural(figures, "рисунок", "рисунка", "рисунков"),
		tables, plural(tables, "таблица", "таблицы", "таблиц"),
		sources, plural(sources, "источник", "источника", "источников"))
}

// Times says how many paragraphs share a finding.
func Times(n int) string {
	return fmt.Sprintf("%d %s", n, plural(n, "место", "места", "мест"))
}

// Total is the closing line of a report.
func Total(errors, warnings int) string {
	return fmt.Sprintf("Итого: %d %s, %d %s.", errors, plural(errors, "ошибка", "ошибки", "ошибок"),
		warnings, plural(warnings, "предупреждение", "предупреждения", "предупреждений"))
}

// AnnotatedLine reports where the commented copy went.
func AnnotatedLine(path string, comments int) string {
	return fmt.Sprintf("Копия с примечаниями Word: %s (%d %s)", path, comments, plural(comments, "примечание", "примечания", "примечаний"))
}

// CommentText is the body of a Word comment.
func CommentText(severity, message, detail string) []string {
	head := "Ошибка: "
	if severity == "warning" {
		head = "Предупреждение: "
	}
	lines := []string{head + message}
	if detail != "" {
		lines = append(lines, detail)
	}
	return lines
}

// Errors shown by the command line. The mapping from error values lives in
// the main package; the wording lives here.
const (
	ErrLegacyDOC      = "файл в старом формате .doc или защищён паролем: откройте его в Word и сохраните как «Документ Word (.docx)» без пароля"
	ErrNotDOCX        = "это не документ .docx"
	ErrTooLarge       = "файл слишком большой для проверки"
	ErrMalformed      = "файл повреждён: Word-разметка внутри него не читается"
	ErrNotFound       = "файл не найден"
	ErrBaseCycle      = "профили ссылаются друг на друга по кругу (параметр base)"
	ErrAnnotate       = "не удалось вставить примечания в этот документ, отчёт выше остаётся верным"
	ErrUnknownProfile = "профиль не найден"
	ErrCanceled       = "проверка прервана"
)

// ErrProfileKey explains a bad key in a profile.
func ErrProfileKey(file, key string, outOfRange bool) string {
	if outOfRange {
		return fmt.Sprintf("в профиле %s значение %s вне допустимого диапазона", file, key)
	}
	return fmt.Sprintf("в профиле %s неизвестный параметр %s — проверьте написание", file, key)
}

// ErrUnknownRule explains a bad rule id in [rules] disable.
func ErrUnknownRule(id string) string {
	return fmt.Sprintf("в профиле указано неизвестное правило %q, список правил: normokontrol rules", id)
}

// FileError prefixes an error with the file it is about.
func FileError(file, text string) string { return file + ": " + text }

// Usage is the help text.
const Usage = `normokontrol — проверка оформления курсовых, отчётов и ВКР (.docx)
по ГОСТ 7.32-2017 или по требованиям вашего вуза.

Использование:
  normokontrol ФАЙЛ.docx
      проверить, сохранить копию с примечаниями и дождаться Enter
      (так программа работает, если перетащить файл на значок)
  normokontrol check [--profile П] [--format text|json] [--annotate] ФАЙЛ.docx
  normokontrol batch [--profile П] [--out ПАПКА] [--jobs N] [--annotate] ПАПКА|ФАЙЛЫ...
  normokontrol profile [ИМЯ]     показать профиль с комментариями
  normokontrol rules             список правил

Профиль ищется так: --profile, затем normokontrol.toml рядом с программой,
затем встроенный gost-7.32-2017.

Код выхода: 0 — ошибок нет, 1 — найдены ошибки, 2 — проверку выполнить не удалось.`

// PressEnter is shown before the window closes in drag-and-drop mode.
const PressEnter = "Нажмите Enter, чтобы закрыть окно…"

// BatchHeader and BatchRow make the batch summary table.
func BatchHeader() string {
	return fmt.Sprintf("%-40s %7s %9s  %s", "ФАЙЛ", "ОШИБКИ", "ПРЕДУПР.", "ОТЧЁТ")
}

// BatchRow is one line of the summary.
func BatchRow(file string, errors, warnings int, report string) string {
	return fmt.Sprintf("%-40s %7d %9d  %s", file, errors, warnings, report)
}

// BatchFailed is a summary line for a file that could not be checked.
func BatchFailed(file, reason string) string {
	return fmt.Sprintf("%-40s %7s %9s  %s", file, "—", "—", reason)
}

// BatchTotal closes the summary.
func BatchTotal(files, clean, failed int) string {
	return fmt.Sprintf("%d %s: без ошибок %d, не проверено %d", files, plural(files, "файл", "файла", "файлов"), clean, failed)
}

// BatchNoFiles: nothing to check.
const BatchNoFiles = "не найдено ни одного файла .docx"

// RuleTitles describe rules for "normokontrol rules".
var RuleTitles = map[string]string{
	"page.margins":   "формат страницы и поля в каждом разделе документа (6.1.1)",
	"structure":      "обязательные структурные элементы и их порядок (раздел 4, 6.2.1)",
	"heading.format": "заголовки: без точки в конце, структурные — прописными и по центру (6.2.1, 6.2.3)",
	"body.font":      "шрифт и кегль основного текста, вставки другим шрифтом (6.1.1)",
	"body.spacing":   "межстрочный интервал (6.1.1)",
	"body.align":     "выравнивание основного текста (ГОСТ не задаёт, проверяется единообразие)",
	"body.indent":    "абзацный отступ, в том числе сделанный пробелами или табуляцией (6.1.1)",
	"body.empty":     "пустые абзацы вместо интервалов и разрывов страниц",
	"captions":       "подписи рисунков и таблиц, нумерация, ссылки в тексте (6.5, 6.6)",
	"refs.citations": "ссылки [N] и список источников (6.9.1)",
	"refs.order":     "порядок источников по первым ссылкам (6.16)",
	"text.homoglyph": "латинские буквы в русских словах и наоборот",
}

// Flag descriptions.
const (
	FlagProfile  = "профиль: имя встроенного или путь к .toml"
	FlagFormat   = "формат отчёта: text или json"
	FlagAnnotate = "сохранить копию документа с примечаниями Word"
	FlagOut      = "папка для отчётов и копий с примечаниями"
	FlagJobs     = "сколько файлов проверять одновременно"
)

// ErrBadFormat: unknown --format value.
func ErrBadFormat(v string) string {
	return fmt.Sprintf("неизвестный формат %q, нужен text или json", v)
}

// ErrNeedOneFile: check takes exactly one file.
const ErrNeedOneFile = "укажите один файл .docx"

// BuiltinProfiles lists the embedded profiles after an unknown name.
func BuiltinProfiles(names []string) string {
	return "встроенные профили: " + fmt.Sprint(names)
}
