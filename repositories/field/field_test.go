package repositories

import (
	"testing"
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
			sortColumn: stringPtr("create_at desc; DROP TABLE fields;--"),
		},
		{
			name:       "subquery injection",
			sortColumn: stringPtr("(SELECT pg_sleep(10))"),
		},
		{
			name:       "quoted identifier break out",
			sortColumn: stringPtr(`name"; DELETE FROM fields WHERE "1`),
		},
		{
			name:       "unknown column",
			sortColumn: stringPtr("password"),
		},
		{
			name:       "column of another table",
			sortColumn: stringPtr("fields.name"),
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
			expectColumn: "create_at",
			expectDesc:   true,
		},
		{
			name:         "ascending on an allowlisted column",
			sortColumn:   stringPtr("name"),
			sortOrder:    stringPtr("asc"),
			expectColumn: "name",
			expectDesc:   false,
		},
		{
			name:         "case and padding are normalised",
			sortColumn:   stringPtr("  PRICE_PER_HOUR  "),
			sortOrder:    stringPtr(" AsC "),
			expectColumn: "price_per_hour",
			expectDesc:   false,
		},
		{
			name:         "unknown direction falls back to descending",
			sortColumn:   stringPtr("code"),
			sortOrder:    stringPtr("asc; DROP TABLE fields"),
			expectColumn: "code",
			expectDesc:   true,
		},
		{
			name:         "missing direction falls back to descending",
			sortColumn:   stringPtr("update_at"),
			sortOrder:    nil,
			expectColumn: "update_at",
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

// Every allowlisted value must map onto a real column of the fields table; a
// typo here would reintroduce a crafted identifier through the front door.
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
