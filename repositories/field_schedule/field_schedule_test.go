package repositories

import (
	"errors"
	"field-service/domain/models"
	"fmt"
	"gorm.io/gorm"
	"reflect"
	"testing"
	"time"
)

func stringPtr(value string) *string {
	return &value
}

// Regression: the sort column used to be interpolated into the ORDER BY with
// fmt.Sprintf, so a crafted sortColumn/sortOrder went straight into the SQL.
func TestBuildOrderByRejectsColumnsOutsideAllowlist(t *testing.T) {
	testCases := []struct {
		name       string
		sortColumn *string
	}{
		{
			name:       "statement injection",
			sortColumn: stringPtr("created_at desc; DROP TABLE field_schedules;--"),
		},
		{
			name:       "subquery injection",
			sortColumn: stringPtr("(SELECT pg_sleep(10))"),
		},
		{
			name:       "quoted identifier break out",
			sortColumn: stringPtr(`date"; DELETE FROM field_schedules WHERE "1`),
		},
		{
			name:       "unknown column",
			sortColumn: stringPtr("deleted_at"),
		},
		{
			name:       "column of a joined table",
			sortColumn: stringPtr("times.start_time"),
		},
		{
			name:       "empty column",
			sortColumn: stringPtr(""),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			orderBy := buildOrderBy(testCase.sortColumn, stringPtr("asc"))
			if orderBy.Column.Name != defaultSortColumn {
				t.Errorf("expected fallback to %q, got %q", defaultSortColumn, orderBy.Column.Name)
			}
			if orderBy.Column.Raw {
				t.Error("expected the column to be quoted by GORM, got a raw expression")
			}
		})
	}
}

func TestBuildOrderByAcceptsAllowlistedColumns(t *testing.T) {
	testCases := []struct {
		name         string
		sortColumn   *string
		sortOrder    *string
		expectColumn string
		expectDesc   bool
	}{
		{
			name:         "defaults when nothing is requested",
			sortColumn:   nil,
			sortOrder:    nil,
			expectColumn: "created_at",
			expectDesc:   true,
		},
		{
			name:         "ascending on an allowlisted column",
			sortColumn:   stringPtr("date"),
			sortOrder:    stringPtr("asc"),
			expectColumn: "date",
			expectDesc:   false,
		},
		{
			name:         "case and padding are normalised",
			sortColumn:   stringPtr("  Field_ID  "),
			sortOrder:    stringPtr(" ASC "),
			expectColumn: "field_id",
			expectDesc:   false,
		},
		{
			name:         "unknown direction falls back to descending",
			sortColumn:   stringPtr("status"),
			sortOrder:    stringPtr("asc; DROP TABLE field_schedules"),
			expectColumn: "status",
			expectDesc:   true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			orderBy := buildOrderBy(testCase.sortColumn, testCase.sortOrder)
			if orderBy.Column.Name != testCase.expectColumn {
				t.Errorf("expected column %q, got %q", testCase.expectColumn, orderBy.Column.Name)
			}
			if orderBy.Desc != testCase.expectDesc {
				t.Errorf("expected desc=%v, got %v", testCase.expectDesc, orderBy.Desc)
			}
		})
	}
}

func TestSortableColumnsAreSelfMapping(t *testing.T) {
	for key, column := range sortableColumns {
		if key != column {
			t.Errorf("allowlist key %q maps to a different column %q", key, column)
		}
	}
	if _, ok := sortableColumns[defaultSortColumn]; !ok {
		t.Errorf("default sort column %q is missing from the allowlist", defaultSortColumn)
	}
}

// Regression: Update only copied Date onto the loaded record, so a schedule
// moved to a different time slot silently kept its old time_id.
func TestUpdateColumnsWritesDateAndTimeID(t *testing.T) {
	date := time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)
	columns := updateColumns(&models.FieldSchedule{Date: date, TimeID: 7})

	expected := map[string]interface{}{
		"date":    date,
		"time_id": uint(7),
	}
	if !reflect.DeepEqual(columns, expected) {
		t.Errorf("expected update columns %v, got %v", expected, columns)
	}
}

// A zero TimeID must still be written; GORM would drop it from a struct update.
func TestUpdateColumnsKeepsZeroValues(t *testing.T) {
	columns := updateColumns(&models.FieldSchedule{})

	if _, ok := columns["time_id"]; !ok {
		t.Error("expected time_id to be written even when it is the zero value")
	}
	if _, ok := columns["date"]; !ok {
		t.Error("expected date to be written even when it is the zero value")
	}
}

func TestIsDuplicateKeyError(t *testing.T) {
	testCases := []struct {
		name   string
		err    error
		expect bool
	}{
		{
			name:   "nil error",
			err:    nil,
			expect: false,
		},
		{
			name:   "gorm translated duplicate",
			err:    fmt.Errorf("create failed: %w", gorm.ErrDuplicatedKey),
			expect: true,
		},
		{
			name:   "postgres sqlstate",
			err:    errors.New(`ERROR: duplicate key value violates unique constraint "idx_field_schedules_field_time_date" (SQLSTATE 23505)`),
			expect: true,
		},
		{
			name:   "unrelated database error",
			err:    errors.New("connection refused"),
			expect: false,
		},
		{
			name:   "record not found",
			err:    gorm.ErrRecordNotFound,
			expect: false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := isDuplicateKeyError(testCase.err); got != testCase.expect {
				t.Errorf("expected %v, got %v", testCase.expect, got)
			}
		})
	}
}

// Row locks are taken in a deterministic order so two concurrent bookings of
// the same slots cannot deadlock waiting on each other.
func TestUniqueSortedIsDeterministicAndDeduplicated(t *testing.T) {
	first := uniqueSorted([]string{"c", "a", "b", "a"})
	second := uniqueSorted([]string{"a", "b", "c", "c"})

	expected := []string{"a", "b", "c"}
	if !reflect.DeepEqual(first, expected) {
		t.Errorf("expected %v, got %v", expected, first)
	}
	if !reflect.DeepEqual(first, second) {
		t.Errorf("expected a stable lock order, got %v and %v", first, second)
	}
}
