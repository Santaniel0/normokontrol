// Package sample builds the two synthetic theses in testdata: one formatted
// by ГОСТ 7.32-2017 and one with the mistakes students make most often.
// Names, the university and the sources are invented.
package sample

import (
	"strings"

	"github.com/Santaniel0/normokontrol/internal/docxtest"
)

var (
	P, T, R = docxtest.P, docxtest.T, docxtest.R
)

const (
	center   = `<w:ind w:firstLine="0"/><w:jc w:val="center"/>`
	h1       = `<w:pStyle w:val="1"/>`
	h2       = `<w:pStyle w:val="2"/>`
	section  = `<w:pStyle w:val="1"/><w:ind w:firstLine="709"/><w:jc w:val="left"/>` // numbered section: indented, not centred (6.2.3)
	tableCap = `<w:keepNext/><w:ind w:firstLine="0"/><w:jc w:val="left"/>`
)

func titlePage() string {
	var b strings.Builder
	for _, line := range []string{
		"Министерство науки и высшего образования Российской Федерации",
		"ФГБОУ ВО «Условный государственный университет»",
		"Кафедра информационных систем", "", "",
		"ВЫПУСКНАЯ КВАЛИФИКАЦИОННАЯ РАБОТА",
		"Разработка информационной системы учёта заявок", "", "",
		"Выполнил: студент группы ИС-41 Иванов И. И.",
		"Руководитель: доцент Петров П. П.", "", "",
		"Город 2026",
	} {
		if line == "" {
			b.WriteString(P(center))
			continue
		}
		b.WriteString(P(center, T(line)))
	}
	return b.String()
}

func table() string {
	return docxtest.Table(
		[]string{P(center, T("Модуль")), P(center, T("Назначение"))},
		[]string{P(`<w:ind w:firstLine="0"/>`, T("Заявки")), P(`<w:ind w:firstLine="0"/>`, T("Приём и учёт обращений"))},
		[]string{P(`<w:ind w:firstLine="0"/>`, T("Отчёты")), P(`<w:ind w:firstLine="0"/>`, T("Сводка по срокам"))},
	)
}

var sources = []string{
	"1. Иванов И. И. Проектирование информационных систем : учебное пособие. — Город : Учебное издательство, 2024. — 320 с.",
	"2. Сидоров С. С. Базы данных для прикладных задач. — Город : Учебное издательство, 2023. — 256 с.",
	"3. Кузнецова А. А. Учёт обращений граждан в организациях // Условный научный журнал. — 2025. — № 2. — С. 10–18.",
	"4. ГОСТ 7.32—2017. Отчет о научно-исследовательской работе. Структура и правила оформления. — Москва : Стандартинформ, 2017. — 32 с.",
}

// Good is a short thesis that follows the built-in profile.
func Good() ([]byte, error) {
	var b strings.Builder
	b.WriteString(titlePage())
	b.WriteString(P(h1, T("РЕФЕРАТ")))
	b.WriteString(P("", T("Пояснительная записка содержит 2 рисунка, 1 таблицу и 4 источника. Объект работы — процесс учёта заявок на предприятии.")))
	b.WriteString(P(h1, T("СОДЕРЖАНИЕ")))
	for _, line := range []string{"Введение\t3", "1 Анализ предметной области\t4", "2 Проектирование системы\t6", "Заключение\t8", "Список использованных источников\t9"} {
		b.WriteString(P(`<w:ind w:firstLine="0"/><w:jc w:val="left"/>`, T(line)))
	}
	b.WriteString(P(h1, T("ВВЕДЕНИЕ")))
	b.WriteString(P("", T("Учёт заявок на небольшом предприятии часто ведётся в электронных таблицах [1]. Цель работы — разработать информационную систему, которая сократит время обработки обращения.")))
	b.WriteString(P("", T("Для достижения цели нужно проанализировать предметную область, спроектировать базу данных [2] и проверить решение на данных отдела.")))
	b.WriteString(P(section, T("1 Анализ предметной области")))
	b.WriteString(P(h2, T("1.1 Описание процесса")))
	b.WriteString(P("", T("Процесс приёма заявки показан на рисунке 1. Типичные причины задержек описаны в работе [3].")))
	b.WriteString(P(center, docxtest.Picture(1)))
	b.WriteString(P(center, T("Рисунок 1 – Схема процесса приёма заявки")))
	b.WriteString(P("", T("Модули будущей системы перечислены в таблице 1.")))
	b.WriteString(P(tableCap, T("Таблица 1 – Модули системы")))
	b.WriteString(table())
	b.WriteString(P(section, T("2 Проектирование системы")))
	b.WriteString(P("", T("Структура базы данных приведена на рисунке 2. Оформление отчёта выполнено по ГОСТ 7.32-2017 [4].")))
	b.WriteString(P(center, docxtest.Picture(2)))
	b.WriteString(P(center, T("Рисунок 2 – Схема базы данных")))
	b.WriteString(P(h1, T("ЗАКЛЮЧЕНИЕ")))
	b.WriteString(P("", T("В работе спроектирована система учёта заявок. Время обработки обращения в тестовом отделе сократилось.")))
	b.WriteString(P(h1, T("СПИСОК ИСПОЛЬЗОВАННЫХ ИСТОЧНИКОВ")))
	for _, s := range sources {
		b.WriteString(P("", T(s)))
	}
	return docxtest.Build(docxtest.Doc{Styles: docxtest.GOSTStyles, Body: b.String(), Image: true})
}

// badStyles differ from GOSTStyles in one place: Normal has single line
// spacing, so every body paragraph inherits the mistake from the style.
var badStyles = strings.Replace(docxtest.GOSTStyles,
	`<w:spacing w:after="0" w:line="360" w:lineRule="auto"/>`, `<w:spacing w:after="0" w:line="240" w:lineRule="auto"/>`, 1)

// Bad is the same thesis with typical mistakes.
func Bad() ([]byte, error) {
	narrow := docxtest.SectA4()
	narrow.RightMM = 10
	var b strings.Builder
	b.WriteString(titlePage())
	b.WriteString(P(h1, T("СОДЕРЖАНИЕ")))
	for _, line := range []string{"Введение\t3", "1 Анализ предметной области\t4", "2 Проектирование системы\t6", "Заключение\t8", "Список использованных источников\t9"} {
		b.WriteString(P(`<w:ind w:firstLine="0"/><w:jc w:val="left"/>`, T(line)))
	}
	b.WriteString(P(h1, T("Введение")))
	b.WriteString(P("", T("Учёт заявок на небольшом прeдприятии часто ведётся в электронных таблицах [2]. Цель работы — разработать информационную систему, которая сократит время обработки обращения.")))
	b.WriteString(P(`<w:ind w:firstLine="567"/>`, T("Для достижения цели нужно проанализировать предметную область [1], спроектировать базу данных и проверить решение на данных отдела.")))
	b.WriteString(P("", ""))
	b.WriteString(P("", T("Система будет работать в среде Windоws на компьютерах отдела.")))
	b.WriteString(P(section, T("1 Анализ предметной области")))
	b.WriteString(P(h2, T("1.1 Описание процесса.")))
	b.WriteString(P("", T("Процесс приёма заявки показан на рисунке 1.")))
	b.WriteString(P("", R(`<w:rFonts w:asciiTheme="minorHAnsi" w:hAnsiTheme="minorHAnsi"/><w:sz w:val="22"/>`,
		"Заявки поступают по телефону, по электронной почте и через сайт; при пиковой нагрузке оператор не успевает их регистрировать.")))
	b.WriteString(P(center, docxtest.Picture(1)))
	b.WriteString(P(center, T("Рисунок 1 – Схема процесса приёма заявки")))
	b.WriteString(P(`<w:ind w:firstLine="0"/>`, T("    Модули будущей системы перечислены ниже.")))
	b.WriteString(table())
	b.WriteString(P("", T("Сравнение решений приведено на рисунке 2 [5].")))
	b.WriteString(P(""))
	b.WriteString(P(""))
	// Students often end a chapter with a section break on an empty line;
	// the new section gets a 10 mm right margin by mistake.
	b.WriteString(P(docxtest.Sect(docxtest.SectA4())))
	b.WriteString(P("", R("<w:b/>", "2 Проектирование системы")))
	b.WriteString(P(`<w:jc w:val="left"/>`, T("Структура базы данных приведена на рис. 2, интерфейс оператора — на рисунке 3.")))
	b.WriteString(P(center, docxtest.Picture(2)))
	b.WriteString(P(center, T("Рис. 2. Схема базы данных")))
	b.WriteString(P(center, docxtest.Picture(3)))
	b.WriteString(P("", T("Интерфейс построен на типовых формах.")))
	b.WriteString(P(center, docxtest.Picture(4)))
	b.WriteString(P(center, T("Рисунок 4 – Отчёт по срокам")))
	b.WriteString(P(h1, T("ЗАКЛЮЧЕНИЕ")))
	b.WriteString(P("", T("В работе спроектирована система учёта заявок. Время обработки обращения в тестовом отделе сократилось [4].")))
	b.WriteString(P(h1, T("СПИСОК ИСПОЛЬЗОВАННЫХ ИСТОЧНИКОВ")))
	for _, s := range sources {
		b.WriteString(P("", T(s)))
	}
	b.WriteString(docxtest.Sect(narrow))
	return docxtest.Build(docxtest.Doc{Styles: badStyles, Body: b.String(), Image: true})
}
