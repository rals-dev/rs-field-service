package repositories

import (
	"context"
	"errors"
	errWrap "field-service/common/error"
	"field-service/constants"
	errorConstants "field-service/constants/error"
	errFieldSchedule "field-service/constants/error/field_schedule"
	"field-service/domain/dto"
	"field-service/domain/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"sort"
	"strings"
)

type IFieldScheduleRepository interface {
	FindAllWithPagination(context.Context, *dto.FieldScheduleRequestParam) ([]models.FieldSchedule, int64, error)
	FindAllByFieldIdAndDate(context.Context, int, string) ([]models.FieldSchedule, error)
	FindByUUID(context.Context, string) (*models.FieldSchedule, error)
	FindByDateAndTimeId(context.Context, string, int, int) (*models.FieldSchedule, error)
	Create(context.Context, []models.FieldSchedule) error
	Update(context.Context, string, *models.FieldSchedule) (*models.FieldSchedule, error)
	UpdateStatus(context.Context, constants.FieldScheduleStatus, []string) error
	Delete(context.Context, string) error
}

func NewFieldScheduleRepository(db *gorm.DB) IFieldScheduleRepository {
	return &FieldScheduleRepository{db: db}
}

type FieldScheduleRepository struct {
	db *gorm.DB
}

// sortableColumns is the allowlist of physical columns a client is allowed to
// order by. Ordering is the only dynamic fragment of the query, so anything
// outside this map is discarded instead of reaching the generated SQL.
var sortableColumns = map[string]string{
	"id":         "id",
	"field_id":   "field_id",
	"time_id":    "time_id",
	"date":       "date",
	"status":     "status",
	"created_at": "created_at",
	"updated_at": "updated_at",
}

const defaultSortColumn = "created_at"

// buildOrderBy resolves the requested column and direction against the
// allowlist and returns a GORM order clause instead of a raw SQL string.
// Unknown columns fall back to defaultSortColumn and any direction other than
// "asc" sorts descending.
func buildOrderBy(sortColumn *string, sortOrder *string) clause.OrderByColumn {
	column := defaultSortColumn
	if sortColumn != nil {
		if allowed, ok := sortableColumns[strings.ToLower(strings.TrimSpace(*sortColumn))]; ok {
			column = allowed
		}
	}

	desc := true
	if sortOrder != nil && strings.EqualFold(strings.TrimSpace(*sortOrder), "asc") {
		desc = false
	}

	return clause.OrderByColumn{Column: clause.Column{Name: column}, Desc: desc}
}

// isDuplicateKeyError reports whether err is a unique constraint violation.
// gorm.ErrDuplicatedKey is only produced when TranslateError is enabled, so the
// raw PostgreSQL SQLSTATE is matched as well.
func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "23505") || strings.Contains(message, "duplicate key value")
}

// updateColumns is the explicit column set written by Update. Naming every
// column keeps a zero TimeID or Date from being skipped the way GORM skips
// zero values when a struct is passed to Updates.
func updateColumns(req *models.FieldSchedule) map[string]interface{} {
	return map[string]interface{}{
		"date":    req.Date,
		"time_id": req.TimeID,
	}
}

// uniqueSorted removes duplicates and sorts the identifiers so that concurrent
// transactions always acquire their row locks in the same order and therefore
// cannot deadlock against each other.
func uniqueSorted(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func (f *FieldScheduleRepository) FindAllWithPagination(
	ctx context.Context,
	param *dto.FieldScheduleRequestParam,
) ([]models.FieldSchedule, int64, error) {
	var (
		fieldSchedules []models.FieldSchedule
		total          int64
	)

	limit := param.Limit
	offset := (param.Page - 1) * limit
	err := f.db.
		WithContext(ctx).
		Preload("Field").
		Preload("Time").
		Limit(limit).
		Offset(offset).
		Order(buildOrderBy(param.SortColumn, param.SortOrder)).
		Find(&fieldSchedules).
		Error
	if err != nil {
		return nil, 0, errWrap.WrapError(errorConstants.ErrSQLError)
	}
	err = f.db.
		WithContext(ctx).
		Model(&fieldSchedules).
		Count(&total).
		Error
	if err != nil {
		return nil, 0, errWrap.WrapError(errorConstants.ErrSQLError)
	}
	return fieldSchedules, total, nil
}

func (f *FieldScheduleRepository) FindAllByFieldIdAndDate(ctx context.Context, fieldId int, date string) ([]models.FieldSchedule, error) {
	var fieldSchedules []models.FieldSchedule
	err := f.db.
		WithContext(ctx).
		Preload("Field").
		Preload("Time").
		Where("field_id = ?", fieldId).
		Where("date = ?", date).
		Joins("LEFT JOIN times ON field_schedules.time_id = times.id").
		Order("times.start_time asc").
		Find(&fieldSchedules).
		Error
	if err != nil {
		return nil, errWrap.WrapError(errorConstants.ErrSQLError)
	}
	return fieldSchedules, nil
}

func (f *FieldScheduleRepository) FindByUUID(ctx context.Context, uuid string) (*models.FieldSchedule, error) {
	var fieldSchedule models.FieldSchedule
	err := f.db.
		WithContext(ctx).
		Preload("Field").
		Preload("Time").
		Where("uuid = ?", uuid).
		First(&fieldSchedule).
		Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errWrap.WrapError(errFieldSchedule.ErrFieldScheduleNotFound)
		}
		return nil, errWrap.WrapError(errorConstants.ErrSQLError)
	}
	return &fieldSchedule, nil
}
func (f *FieldScheduleRepository) FindByDateAndTimeId(ctx context.Context, date string, timeId int, fieldId int) (*models.FieldSchedule, error) {
	var fieldSchedule models.FieldSchedule
	err := f.db.
		WithContext(ctx).
		Where("date = ?", date).
		Where("time_id = ?", timeId).
		Where("field_id = ?", fieldId).
		First(&fieldSchedule).
		Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, errWrap.WrapError(errorConstants.ErrSQLError)
	}
	return &fieldSchedule, nil
}

func (f *FieldScheduleRepository) Create(ctx context.Context, req []models.FieldSchedule) error {
	// A slice insert is a single statement, so either every schedule is stored
	// or none is. The unique index on (field_id, time_id, date) is what makes
	// the caller's prior existence check race free.
	err := f.db.WithContext(ctx).Create(&req).Error
	if err != nil {
		if isDuplicateKeyError(err) {
			return errWrap.WrapError(errFieldSchedule.ErrFieldScheduleIsExist)
		}
		return errWrap.WrapError(errorConstants.ErrSQLError)
	}
	return nil
}

func (f *FieldScheduleRepository) Update(ctx context.Context, uuid string, req *models.FieldSchedule) (*models.FieldSchedule, error) {
	_, err := f.FindByUUID(ctx, uuid)
	if err != nil {
		return nil, err
	}

	err = f.db.
		WithContext(ctx).
		Model(&models.FieldSchedule{}).
		Where("uuid = ?", uuid).
		Updates(updateColumns(req)).
		Error
	if err != nil {
		if isDuplicateKeyError(err) {
			return nil, errWrap.WrapError(errFieldSchedule.ErrFieldScheduleIsExist)
		}
		return nil, errWrap.WrapError(errorConstants.ErrSQLError)
	}

	return f.FindByUUID(ctx, uuid)
}

func (f *FieldScheduleRepository) UpdateStatus(ctx context.Context, status constants.FieldScheduleStatus, uuids []string) error {
	if len(uuids) == 0 {
		return nil
	}

	return f.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, uuid := range uniqueSorted(uuids) {
			var fieldSchedule models.FieldSchedule
			err := tx.
				Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("uuid = ?", uuid).
				First(&fieldSchedule).
				Error
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return errWrap.WrapError(errFieldSchedule.ErrFieldScheduleNotFound)
				}
				return errWrap.WrapError(errorConstants.ErrSQLError)
			}

			// Conditional update: the row only moves while it still holds a
			// different status, so a racing booker updates 0 rows and the whole
			// transaction is rolled back instead of half booking the request.
			result := tx.
				Model(&models.FieldSchedule{}).
				Where("uuid = ?", uuid).
				Where("status <> ?", status).
				Update("status", status)
			if result.Error != nil {
				return errWrap.WrapError(errorConstants.ErrSQLError)
			}
			if result.RowsAffected == 0 {
				return errWrap.WrapError(errFieldSchedule.ErrFieldScheduleIsBooked)
			}
		}
		return nil
	})
}

func (f *FieldScheduleRepository) Delete(ctx context.Context, uuid string) error {
	err := f.db.WithContext(ctx).Where("uuid = ?", uuid).Delete(&models.FieldSchedule{}).Error
	if err != nil {
		return errWrap.WrapError(errorConstants.ErrSQLError)
	}
	return nil
}
