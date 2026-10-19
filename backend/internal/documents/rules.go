package documents

import (
	"bytes"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"dogclub.local/backend/internal/platform/filestore"
)

func category(name string) string {
	switch name {
	case "Заключение о вязке", "Акт о регистрации помёта":
		return "breeding"
	case "Ветеринарный паспорт", "Ветеринарная справка", "Документ о результатах обследования":
		return "medical"
	default:
		return "general"
	}
}
func text(s string, max int) bool {
	return utf8.ValidString(s) && utf8.RuneCountInString(s) <= max && !strings.ContainsFunc(s, unicode.IsControl)
}
func date(s string) bool { t, e := time.Parse(time.DateOnly, s); return e == nil && t.Year() > 0 }
func validate(in *Input, today string) error {
	if in.BreedingDecisionID != nil && (*in.BreedingDecisionID < 1 || in.LitterID != nil) || in.LitterID != nil && *in.LitterID < 1 {
		return invalid("basis", "Укажите одно положительное племенное основание.")
	}
	in.Number = strings.TrimSpace(in.Number)
	in.Issuer = strings.TrimSpace(in.Issuer)
	if in.TypeID < 1 || !text(in.Number, 50) || !text(in.Issuer, 2000) {
		return invalid("fields", "Проверьте тип, номер (до 50) и выдавшую организацию (до 2000 символов).")
	}
	if in.EventID == nil && (in.DogID == nil) == (in.MemberID == nil) {
		return invalid("subject", "Укажите либо собаку, либо человека.")
	}
	for _, id := range []*int32{in.DogID, in.MemberID, in.RecipientMemberID, in.EventID, in.EventResultID} {
		if id != nil && *id < 1 {
			return invalid("subject", "Укажите положительный ID.")
		}
	}
	for _, d := range []*string{in.IssueDate, in.ExpiresOn} {
		if d != nil && !date(*d) {
			return invalid("date", "Укажите дату YYYY-MM-DD.")
		}
	}
	// Lab 3 fn_document_dates_check: a document cannot be issued in the future.
	if in.IssueDate != nil && *in.IssueDate > today {
		return invalid("issue_date", "Дата выдачи не может быть в будущем.")
	}
	if in.ExpiresOn != nil && in.IssueDate != nil && *in.ExpiresOn < *in.IssueDate {
		return invalid("expires_on", "Окончание не может быть раньше выдачи.")
	}
	if in.MemberID != nil {
		if in.RecipientMemberID != nil && *in.RecipientMemberID != *in.MemberID {
			return invalid("recipient_member_id", "Для личного документа получатель должен совпадать с человеком.")
		}
		in.RecipientMemberID = in.MemberID
	}
	return nil
}
func correction(c *Correction) error {
	c.Reason = strings.TrimSpace(c.Reason)
	if c.Version < 1 || c.Reason == "" || !text(c.Reason, 500) {
		return invalid("correction", "Укажите текущую версию и причину до 500 символов.")
	}
	return nil
}
func validFilter(f Filter) error {
	if f.After < 0 || f.Limit < 1 || f.Limit > 100 || f.EventID < 0 || f.TypeID < 0 || f.MemberID < 0 || f.DogID < 0 || !text(f.Number, 50) {
		return invalid("query", "Проверьте параметры фильтра и страницы.")
	}
	for _, d := range []*string{f.From, f.To} {
		if d != nil && !date(*d) {
			return invalid("date", "Укажите дату YYYY-MM-DD.")
		}
	}
	if f.From != nil && f.To != nil && *f.To < *f.From {
		return invalid("date", "Проверьте порядок дат.")
	}
	return nil
}
func validateFile(name, media string, data []byte) error {
	if len(data) == 0 || len(data) > filestore.MaxSize || !text(name, 150) || strings.ContainsAny(name, `/\`) || name == "" || name == "." || name == ".." {
		return invalid("file", "Укажите безопасное имя и файл размером до 10 МиБ.")
	}
	ext := strings.ToLower(filepath.Ext(name))
	switch media {
	case "application/pdf":
		if ext != ".pdf" || !bytes.HasPrefix(data, []byte("%PDF-")) || !bytes.HasSuffix(bytes.TrimSpace(data), []byte("%%EOF")) {
			return invalid("file", "Ожидается PDF-файл.")
		}
	case "image/png", "image/jpeg":
		config, format, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil || config.Width < 1 || config.Height < 1 || int64(config.Width)*int64(config.Height) > 40000000 {
			return invalid("file", "Изображение повреждено или превышает 40 мегапикселей.")
		}
		if media == "image/png" && (format != "png" || ext != ".png") || media == "image/jpeg" && (format != "jpeg" || (ext != ".jpg" && ext != ".jpeg")) {
			return invalid("file", "Расширение и содержимое изображения должны совпадать.")
		}
	default:
		return invalid("file", "Разрешены PDF, JPEG и PNG.")
	}
	return nil
}
