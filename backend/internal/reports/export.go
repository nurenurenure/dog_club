package reports

import (
	"bytes"
	"encoding/csv"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/go-fonts/dejavu/dejavusans"
	"github.com/go-fonts/dejavu/dejavusansbold"
	"github.com/go-pdf/fpdf"
)

type metricRow struct {
	Scope, Metric, Label string
	TypeID               int32
	Value                int64
}

func metricRows(r Report) []metricRow {
	rows := []metricRow{
		{"activity", "members_joined", "Вступления в клуб", 0, r.Activity.MembersJoined},
		{"activity", "dogs_registered", "Регистрации собак", 0, r.Activity.DogsRegistered},
		{"activity", "events_completed", "Завершённые мероприятия, всего", 0, r.Activity.EventsCompleted},
	}
	for _, v := range r.Activity.EventsByType {
		rows = append(rows, metricRow{"activity", "events_completed_by_type", "Мероприятия: " + v.TypeName, v.TypeID, v.Count})
	}
	return append(rows,
		metricRow{"activity", "matings_performed", "Проведённые вязки", 0, r.Activity.MatingsPerformed},
		metricRow{"activity", "litters_born", "Родившиеся помёты", 0, r.Activity.LittersBorn},
		metricRow{"activity", "puppies_born", "Щенки в этих помётах, всего", 0, r.Activity.PuppiesBorn},
		metricRow{"current", "member_cards", "Карточки членов клуба", 0, r.Current.MemberCards},
		metricRow{"current", "active_members", "Активные члены клуба", 0, r.Current.ActiveMembers},
		metricRow{"current", "dog_cards", "Карточки собак", 0, r.Current.DogCards},
		metricRow{"current", "dogs_on_register", "Собаки на учёте", 0, r.Current.DogsOnRegister},
		metricRow{"data_quality", "members_without_membership_date", "Карточки без даты вступления", 0, r.DataQuality.MembersWithoutDate},
		metricRow{"data_quality", "dogs_without_registration_date", "Собаки без подтверждённой даты регистрации", 0, r.DataQuality.DogsWithoutDate},
		metricRow{"data_quality", "performed_matings_without_date", "Проведённые вязки без даты", 0, r.DataQuality.MatingsWithoutDate},
		metricRow{"data_quality", "events_without_period", "Мероприятия без полных дат или часового пояса", 0, r.DataQuality.EventsWithoutPeriod},
	)
}

// csvText prevents spreadsheet formulas in dictionary-controlled labels as well
// as escaping delimiters with encoding/csv. Numeric fields remain numeric.
func csvText(raw string) string {
	trimmed := strings.TrimLeftFunc(raw, unicode.IsSpace)
	if strings.HasPrefix(raw, "\t") || strings.HasPrefix(raw, "\r") || strings.HasPrefix(raw, "\n") || len(trimmed) > 0 && strings.ContainsRune("=+-@", rune(trimmed[0])) {
		return "'" + raw
	}
	return raw
}
func renderCSV(r Report) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("\xef\xbb\xbf")
	w := csv.NewWriter(&b)
	w.UseCRLF = true
	if err := w.Write([]string{"scope", "metric", "event_type_id", "label", "value", "from", "to", "generated_at", "methodology"}); err != nil {
		return nil, ErrUnavailable
	}
	for _, row := range metricRows(r) {
		id := ""
		if row.TypeID != 0 {
			id = strconv.FormatInt(int64(row.TypeID), 10)
		}
		if err := w.Write([]string{row.Scope, row.Metric, id, csvText(row.Label), strconv.FormatInt(row.Value, 10), r.Period.From, r.Period.To, r.GeneratedAt.UTC().Format(time.RFC3339Nano), csvText(r.Methodology)}); err != nil {
			return nil, ErrUnavailable
		}
	}
	w.Flush()
	if w.Error() != nil {
		return nil, ErrUnavailable
	}
	return b.Bytes(), nil
}
func renderPDF(r Report) ([]byte, error) {
	p := fpdf.New("P", "mm", "A4", "")
	p.SetMargins(17, 16, 17)
	p.SetAutoPageBreak(true, 18)
	p.AddUTF8FontFromBytes("DejaVu", "", dejavusans.TTF)
	p.AddUTF8FontFromBytes("DejaVu", "B", dejavusansbold.TTF)
	p.SetTitle("Отчёт о деятельности клуба", true)
	p.SetAuthor("Клуб собаководства", true)
	p.AliasNbPages("")
	p.SetFooterFunc(func() {
		p.SetY(-12)
		p.SetFont("DejaVu", "", 8)
		p.CellFormat(0, 5, "Отчёт v1 · Страница "+strconv.Itoa(p.PageNo())+"/{nb}", "", 0, "C", false, 0, "")
	})
	p.AddPage()
	p.SetFont("DejaVu", "B", 16)
	p.MultiCell(0, 8, "Отчёт о деятельности клуба", "", "L", false)
	p.SetFont("DejaVu", "", 10)
	p.MultiCell(0, 6, "Период: "+r.Period.From+" - "+r.Period.To+" (включительно)", "", "L", false)
	p.MultiCell(0, 6, "Сформирован: "+r.GeneratedAt.UTC().Format("2006-01-02 15:04:05 UTC"), "", "L", false)
	titles := map[string]string{"activity": "Деятельность за период", "current": "На момент формирования", "data_quality": "Полнота дат на момент формирования"}
	scope := ""
	for _, row := range metricRows(r) {
		if row.Scope != scope {
			if p.GetY() > 245 {
				p.AddPage()
			}
			p.Ln(4)
			p.SetFont("DejaVu", "B", 11)
			p.MultiCell(0, 7, titles[row.Scope], "", "L", false)
			scope = row.Scope
		}
		p.SetFont("DejaVu", "", 10)
		lines := p.SplitText(row.Label, 148)
		height := float64(len(lines))*5.5 + 1.5
		if p.GetY()+height > 278 {
			p.AddPage()
			p.SetFont("DejaVu", "B", 10)
			p.MultiCell(0, 6, titles[row.Scope]+" (продолжение)", "", "L", false)
			p.SetFont("DejaVu", "", 10)
		}
		x, y := p.GetXY()
		p.MultiCell(148, 5.5, row.Label, "", "L", false)
		p.SetXY(x+150, y)
		p.CellFormat(26, 5.5, strconv.FormatInt(row.Value, 10), "", 0, "R", false, 0, "")
		p.SetXY(x, y+height)
	}
	p.SetFont("DejaVu", "", 9)
	if p.GetY()+11+float64(len(p.SplitText(r.Methodology, 176)))*5 > 278 {
		p.AddPage()
	}
	p.Ln(5)
	p.SetFont("DejaVu", "B", 10)
	p.MultiCell(0, 6, "Правила подсчёта", "", "L", false)
	p.SetFont("DejaVu", "", 9)
	p.MultiCell(0, 5, r.Methodology, "", "L", false)
	var out bytes.Buffer
	if p.Output(&out) != nil {
		return nil, ErrUnavailable
	}
	return out.Bytes(), nil
}
