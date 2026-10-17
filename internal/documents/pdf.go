package documents

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-fonts/dejavu/dejavusans"
	"github.com/go-fonts/dejavu/dejavusansbold"
	"github.com/go-pdf/fpdf"
)

func render(s sourceSnapshot, kind string) ([]byte, error) {
	p := fpdf.New("P", "mm", "A4", "")
	p.SetMargins(18, 18, 18)
	p.SetAutoPageBreak(true, 20)
	p.AddUTF8FontFromBytes("DejaVu", "", dejavusans.TTF)
	p.AddUTF8FontFromBytes("DejaVu", "B", dejavusansbold.TTF)
	p.SetTitle("Документы клуба собаководства", true)
	p.SetAuthor("Клуб собаководства", true)
	p.AliasNbPages("")
	p.SetFooterFunc(func() {
		p.SetY(-14)
		p.SetFont("DejaVu", "", 8)
		p.SetTextColor(90, 90, 90)
		p.CellFormat(0, 5, fmt.Sprintf("Форма v1 • Страница %d/{nb}", p.PageNo()), "", 0, "C", false, 0, "")
	})
	p.AddPage()
	p.SetFont("DejaVu", "", 10)
	field := func(label, value string) {
		if p.GetY() > 260 {
			p.AddPage()
		}
		if value == "" {
			value = "Не указано"
		}
		p.SetTextColor(25, 25, 25)
		p.SetFont("DejaVu", "B", 10)
		p.MultiCell(0, 5.5, label, "", "L", false)
		p.SetFont("DejaVu", "", 10)
		p.MultiCell(0, 5.5, value, "", "L", false)
		p.Ln(2)
	}
	title := map[string]string{"member_card": "Карточка члена клуба", "dog_card": "Карточка собаки", "veterinary_conclusion": "Ветеринарное заключение"}[kind]
	p.SetFont("DejaVu", "B", 17)
	p.MultiCell(0, 8, title, "", "L", false)
	p.Ln(4)
	field("Номер документа", s.Document.Number)
	field("Дата выдачи", value(s.Document.IssueDate))
	field("Кем выдан", s.Document.Issuer)
	if s.Document.ExpiresOn != nil {
		field("Действует по", *s.Document.ExpiresOn)
	}
	if s.Member != nil {
		m := s.Member
		field("Номер карточки / ФИО", fmt.Sprintf("%d / %s", m.ID, m.FullName))
		field("Дата вступления", value(m.MembershipDate))
		field("Статус членства", m.MembershipStatus.Name)
		for _, c := range m.Contacts {
			field(c.Type.Name, c.Value)
		}
		for _, a := range m.Addresses {
			field(a.Type.Name, strings.Join([]string{a.Locality, a.Street, a.BuildingNumber, a.ApartmentNumber}, ", "))
		}
		field("Собаки на дату формирования", strconv.Itoa(len(s.Dogs)))
		for _, d := range s.Dogs {
			field(d.Nickname, d.Breed.Name+"; "+join(d.Identifier.Type.Name, d.Identifier.Number))
		}
	}
	if s.Dog != nil {
		d := s.Dog
		field("Собака", fmt.Sprintf("%s (карточка %d)", d.Nickname, d.ID))
		field("Порода / пол / окрас", d.Breed.Name+" / "+d.Sex.Name+" / "+d.CoatColor.Name)
		field("Дата рождения", d.DateOfBirth)
		field("Идентификатор", join(d.Identifier.Type.Name, d.Identifier.Number))
		if s.Recipient != nil {
			field("Получатель — владелец", s.Recipient.FullName)
		}
		if kind == "dog_card" {
			field("Рост, см / вес, кг", decimal(d.HeightCM)+" / "+decimal(d.WeightKG))
			field("Родословная", join(d.Pedigree.Number, d.Pedigree.Issuer))
			for _, v := range s.Vaccinations {
				state := ""
				if v.IsVoid {
					state = " (запись отменена)"
				}
				field("Вакцинация"+state, v.Date+" — "+v.Vaccine.Name+"; действует по "+value(v.ValidUntil)+"; следующая "+value(v.NextDate))
			}
			if len(s.Vaccinations) == 0 {
				field("Вакцинации", "Записи отсутствуют")
			}
		}
	}
	if s.Medical != nil {
		e := s.Medical.Examination
		field("Осмотр", fmt.Sprintf("№ %d от %s, версия %d", e.ID, e.Date, e.Version))
		field("Описание состояния", e.Description)
		field("Результат осмотра", e.Result)
		field("Рекомендации", e.Recommendations)
		for _, d := range e.Diagnoses {
			h := "не уточнена"
			if d.IsHereditary != nil {
				h = "нет"
				if *d.IsHereditary {
					h = "да"
				}
			}
			field("Диагноз", d.Disease.Name+"; наследственность: "+h)
		}
		if c := s.Medical.Clearance; c != nil {
			purpose := map[string]string{"events": "мероприятия", "breeding": "племенная работа"}[c.Purpose]
			decision := map[string]string{"allowed": "разрешено", "denied": "отказано", "not_assessed": "не оценивалось"}[c.Decision]
			field("Заключение о допуске", fmt.Sprintf("№ %d; %s: %s", c.ID, purpose, decision))
			field("Дата окончания заключения", value(c.ValidUntil))
			field("Комментарий ветеринара", c.Comment)
			for _, v := range s.Medical.Vaccinations {
				field("Основание: вакцинация", v.Date+" — "+v.Vaccine.Name+"; действует по "+value(v.ValidUntil))
			}
			field("Проверка актуальности", "Это сохранённое заключение по указанному осмотру. Текущий допуск проверяется системой отдельно с учётом сроков и оснований.")
		} else {
			field("Допуск", "Заключение о допуске не выбрано; из результата осмотра допуск автоматически не следует.")
		}
		field("Подпись ветеринарного специалиста", "________________________________")
	}
	var out bytes.Buffer
	if err := p.Output(&out); err != nil {
		return nil, ErrUnavailable
	}
	return out.Bytes(), nil
}
func value(s *string) string {
	if s == nil {
		return "Не указано"
	}
	return *s
}
func decimal(v *float64) string {
	if v == nil {
		return "Не указано"
	}
	return strconv.FormatFloat(*v, 'f', -1, 64)
}

func join(parts ...string) string {
	nonempty := []string{}
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			nonempty = append(nonempty, part)
		}
	}
	return strings.Join(nonempty, "; ")
}
