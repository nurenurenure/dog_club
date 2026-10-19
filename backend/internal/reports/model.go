// Package reports orchestrates aggregate reads and ephemeral report exports.
package reports

import (
	"errors"
	"time"

	"dogclub.local/backend/internal/events"
	"dogclub.local/backend/internal/platform/reporting"
)

var ErrUnavailable = errors.New("reports unavailable")

type ValidationError struct{ Field, Message string }

func (e *ValidationError) Error() string  { return "invalid report parameters" }
func invalid(field, message string) error { return &ValidationError{field, message} }

type Activity struct {
	MembersJoined    int64                      `json:"members_joined"`
	DogsRegistered   int64                      `json:"dogs_registered"`
	EventsCompleted  int64                      `json:"events_completed"`
	EventsByType     []events.ActivityEventType `json:"events_by_type"`
	MatingsPerformed int64                      `json:"matings_performed"`
	LittersBorn      int64                      `json:"litters_born"`
	PuppiesBorn      int64                      `json:"puppies_born"`
}
type Current struct {
	MemberCards    int64 `json:"member_cards"`
	ActiveMembers  int64 `json:"active_members"`
	DogCards       int64 `json:"dog_cards"`
	DogsOnRegister int64 `json:"dogs_on_register"`
}
type DataQuality struct {
	MembersWithoutDate  int64 `json:"members_without_membership_date"`
	DogsWithoutDate     int64 `json:"dogs_without_registration_date"`
	MatingsWithoutDate  int64 `json:"performed_matings_without_date"`
	EventsWithoutPeriod int64 `json:"events_without_period"`
}
type Report struct {
	Period      reporting.Period `json:"period"`
	GeneratedAt time.Time        `json:"generated_at"`
	Methodology string           `json:"methodology"`
	Activity    Activity         `json:"activity"`
	Current     Current          `json:"current"`
	DataQuality DataQuality      `json:"data_quality"`
}

const methodology = "Период включает обе даты. Вступления и регистрации учитываются по предметным датам независимо от текущего статуса. Завершённые мероприятия — по местной дате окончания; вязки — только проведённые по фактической дате. Щенки — общее количество в родившихся за период помётах. Текущие итоги и полнота дат показаны на момент формирования, а не на конец периода. Неизвестные даты не восстанавливаются из аудита. Каждый запрос пересчитывает актуальные исправленные данные; серверного архива нет."
