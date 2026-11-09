package model_test

import (
	"bytes"
	"testing"

	"github.com/Santaniel0/normokontrol/internal/docxtest"
	"github.com/Santaniel0/normokontrol/internal/model"
	"github.com/Santaniel0/normokontrol/internal/ooxml"
)

func build(t *testing.T, body string) *model.Document {
	t.Helper()
	data, err := docxtest.Build(docxtest.Doc{Body: body, Image: true})
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := ooxml.Open(bytes.NewReader(data), int64(len(data)), ooxml.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	doc, err := ooxml.Load(pkg)
	if err != nil {
		t.Fatal(err)
	}
	return model.Build(doc, model.Options{})
}

func TestClassification(t *testing.T) {
	P, T, R := docxtest.P, docxtest.T, docxtest.R
	body := P(`<w:jc w:val="center"/>`, T("МИНИСТЕРСТВО НАУКИ И ВЫСШЕГО ОБРАЗОВАНИЯ")) + // 0
		P("", T("Иванов И. И.")) + // 1
		P("", T("СОДЕРЖАНИЕ")) + // 2 guessed heading
		P("", T("Введение\t3")) + // 3 toc line
		P(`<w:pStyle w:val="1"/>`, T("ВВЕДЕНИЕ")) + // 4
		P("", T("Обычный абзац текста.")) + // 5
		P("", R("<w:b/>", "1.2 Обзор аналогов")) + // 6 guessed level 2
		P("", T("1 января прошёл без происшествий.")) + // 7 not bold: body
		P("", docxtest.Picture(1)) + // 8
		P("", T("Рисунок 1 – Схема")) + // 9
		P("", T("Таблица 3 показывает рост")) + // 10 sentence
		P("", T("Таблица 1 – Данные")) + // 11
		docxtest.Table([]string{P("", T("a")), P("", T("b"))}) + // 12, 13
		P("", T("– первый пункт")) + // 14
		P("", "") + // 15
		P("", T("СПИСОК ИСПОЛЬЗОВАННЫХ ИСТОЧНИКОВ")) + // 16
		P("", T("1. Иванов И. И. Книга. – М., 2020.")) + // 17
		P("", T("ПРИЛОЖЕНИЕ А")) + // 18
		P("", T("Текст приложения")) // 19
	d := build(t, body)
	want := []struct {
		role   model.Role
		level  int
		region model.Region
	}{
		{model.RoleFrontMatter, 0, model.RegionFront},
		{model.RoleFrontMatter, 0, model.RegionFront},
		{model.RoleHeading, 1, model.RegionMain},
		{model.RoleTOC, 0, model.RegionMain},
		{model.RoleHeading, 1, model.RegionMain},
		{model.RoleBody, 0, model.RegionMain},
		{model.RoleHeading, 2, model.RegionMain},
		{model.RoleBody, 0, model.RegionMain},
		{model.RoleFigure, 0, model.RegionMain},
		{model.RoleCaption, 0, model.RegionMain},
		{model.RoleBody, 0, model.RegionMain},
		{model.RoleCaption, 0, model.RegionMain},
		{model.RoleTableCell, 0, model.RegionMain},
		{model.RoleTableCell, 0, model.RegionMain},
		{model.RoleListItem, 0, model.RegionMain},
		{model.RoleEmpty, 0, model.RegionMain},
		{model.RoleHeading, 1, model.RegionMain},
		{model.RoleBibEntry, 0, model.RegionMain},
		{model.RoleHeading, 1, model.RegionAppendix},
		{model.RoleBody, 0, model.RegionAppendix},
	}
	if len(d.Paras) != len(want) {
		t.Fatalf("got %d paragraphs, want %d", len(d.Paras), len(want))
	}
	for i, w := range want {
		p := d.Paras[i]
		if p.Role != w.role || p.Level != w.level || p.Region != w.region {
			t.Errorf("paragraph %d %q: role %v level %d region %d, want %v %d %d",
				i, p.Text, p.Role, p.Level, p.Region, w.role, w.level, w.region)
		}
	}
	if !d.Paras[4].Styled || d.Paras[2].Styled {
		t.Error("styled flag is wrong")
	}
	if c := d.Paras[11].Caption; c == nil || c.Kind != model.CaptionTable || c.Number != "1" {
		t.Errorf("table caption = %+v", c)
	}
	if got := len(d.BibEntries()); got != 1 {
		t.Errorf("bib entries = %d", got)
	}
	loc := d.Locate(7)
	if loc.Heading != "1.2 Обзор аналогов" || loc.Ordinal != 1 {
		t.Errorf("location = %+v", loc)
	}
}

func TestDominantFontUsesHAnsiForCyrillic(t *testing.T) {
	rpr := `<w:rFonts w:ascii="Times New Roman" w:hAnsi="Arial"/><w:sz w:val="28"/>`
	d := build(t, docxtest.P("", docxtest.R(rpr, "Кириллица"), docxtest.R(rpr, "abc")))
	f := d.Paras[0].Font
	if f.Name.Val != "Arial" || f.Size.Val != 28 || f.Chars != 9 {
		t.Errorf("font = %+v", f)
	}
	d = build(t, docxtest.P("", docxtest.R(rpr, "Latin only text")))
	if got := d.Paras[0].Font.Name.Val; got != "Times New Roman" {
		t.Errorf("ASCII text font = %q", got)
	}
}

func TestParseCaption(t *testing.T) {
	tests := []struct {
		in        string
		ok        bool
		kind      model.CaptionKind
		number    string
		sep       string
		canonical bool
	}{
		{"Рисунок 1 – Схема прибора", true, model.CaptionFigure, "1", "–", true},
		{"Рисунок 2 — Оформление таблицы", true, model.CaptionFigure, "2", "—", true},
		{"Рисунок 2.1 – Схема", true, model.CaptionFigure, "2.1", "–", true},
		{"Рисунок А.3 – Схема", true, model.CaptionFigure, "3", "–", true},
		{"Рис. 5. Схема БД", true, model.CaptionFigure, "5", ".", false},
		{"Рисунок 4 - Схема", true, model.CaptionFigure, "4", "-", false},
		{"Рисунок 6 – схема", true, model.CaptionFigure, "6", "–", false},
		{"Рисунок 7 – Схема.", true, model.CaptionFigure, "7", "–", false},
		{"Таблица 1 – Исходные данные", true, model.CaptionTable, "1", "–", true},
		{"Табл.2 Данные", true, model.CaptionTable, "2", "", false},
		{"Таблица 3 показывает рост", true, model.CaptionTable, "3", "", false},
		{"Продолжение таблицы 2", true, model.CaptionTable, "2", "", true},
		{"Рисунок 12а – Схема", false, 0, "", "", false},
		{"Рисунки показывают", false, 0, "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			c, ok := model.ParseCaption(tt.in)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v (%+v)", ok, tt.ok, c)
			}
			if !ok {
				return
			}
			if c.Kind != tt.kind || c.Number != tt.number || c.Sep != tt.sep || c.Canonical() != tt.canonical {
				t.Errorf("got %+v canonical=%v", c, c.Canonical())
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	if got := model.Normalize("  Список  использованных источников. "); got != "СПИСОК ИСПОЛЬЗОВАННЫХ ИСТОЧНИКОВ" {
		t.Errorf("got %q", got)
	}
	if got := model.Snippet("один два три четыре пять шесть", 14); got != "один два три…" {
		t.Errorf("snippet = %q", got)
	}
}
