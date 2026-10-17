package documents

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"

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
	title := map[string]string{"member_card": "Карточка члена клуба", "dog_card": "Карточка собаки", "veterinary_conclusion": "Ветеринарное заключение", "registration_certificate": "Свидетельство о регистрации собаки", "breeding_conclusion": "Заключение о вязке", "litter_act": "Акт о регистрации помёта", "event_participants": "Список участников мероприятия", "event_protocol": "Итоговый протокол мероприятия", "event_diploma": "Диплом участника"}[kind]
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
		if kind == "registration_certificate" {
			field("Дата регистрации в клубе", value(d.RegistrationDate))
			field("Подтверждение", "Собака с указанным идентификатором поставлена на учёт клуба.")
			field("Подпись уполномоченного сотрудника", "________________________________")
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
	if b := s.Breeding; b != nil {
		if b.Personal {
			field("Вид документа", "Персональная копия")
			if s.Recipient != nil {
				field("Получатель", s.Recipient.FullName)
			}
		} else {
			field("Вид документа", "Клубный экземпляр")
		}
		field("Мать / самка", fmt.Sprintf("%s; %s; %s: %s", b.Female.Nickname, b.Female.Breed.Name, b.Female.Identifier.Type.Name, b.Female.Identifier.Number))
		field("Отец / самец", fmt.Sprintf("%s; %s; %s: %s", b.Male.Nickname, b.Male.Breed.Name, b.Male.Identifier.Type.Name, b.Male.Identifier.Number))
		if decision := b.Decision; decision != nil {
			field("Основание", fmt.Sprintf("Решение эксперта № %d по заявке № %d от %s", decision.ID, decision.RequestID, decision.DecidedAt.UTC().Format("2006-01-02")))
			field("Эксперт-кинолог", s.ExpertName)
			field("Результат решения", map[string]string{"approved": "Разрешено", "rejected": "Отказано"}[decision.Decision])
			if b.Request != nil {
				field("Плановая дата вязки", b.Request.PlannedDate)
				if b.Request.ExpectedBirthDate != nil {
					field("Ожидаемая дата рождения помёта", *b.Request.ExpectedBirthDate)
				}
			}
			if !b.Personal {
				field("Обоснование эксперта", decision.Reason)
			}
			field("Условия", "Документ воспроизводит указанное решение эксперта. Медицинский допуск и остальные условия повторно проверяются при регистрации фактической вязки.")
			field("Подпись эксперта-кинолога", "________________________________")
		}
		if litter := b.Litter; litter != nil {
			field("Номер помёта / вязки", fmt.Sprintf("%d / %d", litter.ID, litter.BreedingID))
			field("Дата рождения помёта", litter.DateOfBirth)
			field("Общее количество щенков", strconv.Itoa(int(litter.TotalPuppies)))
			field("Зарегистрировано с идентификаторами", strconv.Itoa(len(b.Puppies)))
			if litter.IntervalViolation {
				field("Нарушение интервала между помётами", fmt.Sprintf("Зафиксировано: %d дней вместо не менее 365. Факт рождения сохранён.", *litter.IntervalDays))
			}
			for _, puppy := range b.Puppies {
				field("Зарегистрированный щенок", puppy.Nickname+"; "+puppy.Identifier.Type.Name+": "+puppy.Identifier.Number)
			}
			field("Подпись уполномоченного сотрудника", "________________________________")
		}
	}
	if s.Event != nil {
		e := s.Event
		field("Мероприятие", e.Name+"; "+e.Type)
		if e.Timezone != nil {
			loc, err := time.LoadLocation(*e.Timezone)
			if err != nil {
				return nil, ErrUnavailable
			}
			period := []string{}
			for _, t := range []*time.Time{e.StartsAt, e.EndsAt} {
				if t != nil {
					period = append(period, t.In(loc).Format("02.01.2006 15:04 -07:00"))
				}
			}
			field("Период и часовой пояс", strings.Join(period, " — ")+"; "+*e.Timezone)
		}
		if s.Recipient != nil {
			field("Получатель — владелец", s.Recipient.FullName)
		}
		if kind == "event_participants" {
			for _, people := range []bool{false, true} {
				heading := "Собаки и представители"
				if people {
					heading = "Участники без собак"
				}
				count := 0
				for _, entry := range e.Entries {
					if (entry.Dog == nil) == people {
						count++
					}
				}
				field(heading, fmt.Sprintf("Регистраций: %d", count))
				for _, entry := range e.Entries {
					if (entry.Dog == nil) != people {
						continue
					}
					label := fmt.Sprintf("Регистрация № %d", entry.ID)
					if entry.Dog != nil {
						field(label, entry.Dog.Nickname+"; "+join(entry.Dog.Identifier.Type.Name, entry.Dog.Identifier.Number)+"; "+entry.Class)
						field("Представитель", entry.Participant)
					} else {
						field(label, entry.Participant)
					}
				}
			}
		} else {
			if len(e.Entries) == 0 {
				field("Результаты", "Участвовавших собак нет")
			}
			for _, entry := range e.Entries {
				if entry.Dog != nil {
					field("Собака", entry.Dog.Nickname+"; "+entry.Dog.Breed.Name+"; "+join(entry.Dog.Identifier.Type.Name, entry.Dog.Identifier.Number))
				}
				field("Класс", entry.Class)
				if entry.Result == nil {
					field("Результат", "Не внесён")
					continue
				}
				r := entry.Result
				field("Основание", fmt.Sprintf("Результат № %d; регистрация № %d", r.ID, entry.ID))
				field("Оценка", r.Grade)
				if r.Place != nil {
					field("Место", strconv.Itoa(int(*r.Place)))
				}
				field("Эксперт", r.ExpertName)
			}
		}
		field("Подпись ответственного организатора", "________________________________")
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
