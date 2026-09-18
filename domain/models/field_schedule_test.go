package models

import (
	"reflect"
	"strings"
	"testing"
)

const slotIndexName = "idx_field_schedules_field_time_date"

// Regression: without a unique index the "does this slot already exist?" check
// in the service is a TOCTOU window, so two concurrent requests could both
// create the same schedule.
func TestFieldScheduleHasUniqueSlotIndex(t *testing.T) {
	scheduleType := reflect.TypeOf(FieldSchedule{})

	expectedPriorities := map[string]string{
		"FieldID": "priority:1",
		"TimeID":  "priority:2",
		"Date":    "priority:3",
	}

	for fieldName, priority := range expectedPriorities {
		structField, ok := scheduleType.FieldByName(fieldName)
		if !ok {
			t.Fatalf("FieldSchedule has no field %q", fieldName)
		}
		tag := structField.Tag.Get("gorm")
		if !strings.Contains(tag, "uniqueIndex:"+slotIndexName) {
			t.Errorf("field %q is not part of the unique slot index, gorm tag is %q", fieldName, tag)
		}
		if !strings.Contains(tag, priority) {
			t.Errorf("field %q should declare %q so the composite index keeps its column order, gorm tag is %q",
				fieldName, priority, tag)
		}
	}
}

// Status must stay out of the slot index, otherwise booking a schedule would
// free the slot up for a second identical schedule.
func TestSlotIndexDoesNotCoverStatus(t *testing.T) {
	scheduleType := reflect.TypeOf(FieldSchedule{})

	structField, ok := scheduleType.FieldByName("Status")
	if !ok {
		t.Fatal("FieldSchedule has no field \"Status\"")
	}
	if strings.Contains(structField.Tag.Get("gorm"), slotIndexName) {
		t.Error("Status must not be part of the unique slot index")
	}
}
