package models

import (
	"field-service/constants"
	"github.com/google/uuid"
	"time"
)

// FieldSchedule holds one bookable slot. The unique index over
// (field_id, time_id, date) is what makes concurrent creations safe: the
// database, not an application side existence check, rejects the duplicate.
type FieldSchedule struct {
	ID        uint                          `gorm:"primaryKey;autoIncrement"`
	UUID      uuid.UUID                     `gorm:"type:uuid;not null;uniqueIndex:idx_field_schedules_uuid"`
	FieldID   uint                          `gorm:"type:int;not null;uniqueIndex:idx_field_schedules_field_time_date,priority:1"`
	TimeID    uint                          `gorm:"type:int;not null;uniqueIndex:idx_field_schedules_field_time_date,priority:2"`
	Date      time.Time                     `gorm:"type:date;not null;uniqueIndex:idx_field_schedules_field_time_date,priority:3"`
	Status    constants.FieldScheduleStatus `gorm:"type:int;not null"`
	CreatedAt *time.Time
	UpdatedAt *time.Time
	DeletedAt *time.Time

	Field Field `gorm:"foreignKey:field_id;references:id;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Time  Time  `gorm:"foreignKey:time_id;references:id;"`
}
