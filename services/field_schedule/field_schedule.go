package services

import (
	"context"
	"field-service/common/utils"
	"field-service/constants"
	errorConstants "field-service/constants/error"
	errFieldSchedule "field-service/constants/error/field_schedule"
	"field-service/domain/dto"
	"field-service/domain/models"
	"field-service/repositories"
	"fmt"
	"github.com/google/uuid"
	"time"
)

type IFieldScheduleService interface {
	GetAllWithPagination(context.Context, *dto.FieldScheduleRequestParam) (*utils.PaginationResult, error)
	GetAllByFieldIdAndDate(context.Context, string, string) ([]dto.FieldScheduleForBookingResponse, error)
	GetByUUID(context.Context, string) (*dto.FieldScheduleResponse, error)
	GenerateScheduleForOneMonth(context.Context, *dto.GenerateFieldScheduleForOneMonthRequest) error
	Create(context.Context, *dto.FieldScheduleRequest) error
	Update(context.Context, string, *dto.UpdateFieldScheduleRequest) (*dto.FieldScheduleResponse, error)
	UpdateStatus(context.Context, *dto.UpdateStatusFieldScheduleRequest) error
	Delete(context.Context, string) error
}

func NewFieldScheduleService(repository repositories.IRepositoryRegistry) IFieldScheduleService {
	return &FieldScheduleService{repository: repository}
}

type FieldScheduleService struct {
	repository repositories.IRepositoryRegistry
}

func (f *FieldScheduleService) GetAllWithPagination(
	ctx context.Context,
	param *dto.FieldScheduleRequestParam,
) (*utils.PaginationResult, error) {
	fieldSchedules, total, err := f.repository.GetFieldSchedule().FindAllWithPagination(ctx, param)
	if err != nil {
		return nil, err
	}
	fieldScheduleResults := make([]dto.FieldScheduleResponse, 0, len(fieldSchedules))
	for _, schedule := range fieldSchedules {
		fieldScheduleResults = append(fieldScheduleResults, dto.FieldScheduleResponse{
			UUID:         schedule.UUID,
			FieldName:    schedule.Field.Name,
			PricePerHour: schedule.Field.PricePerHour,
			Date:         schedule.Date.Format("2006-01-02"),
			Status:       schedule.Status.GetStatusString(),
			Time:         fmt.Sprintf("%s - %s", schedule.Time.StartTime, schedule.Time.EndTime),
			CreateAt:     schedule.CreatedAt,
			UpdateAt:     schedule.UpdatedAt,
		})
	}
	pagination := utils.PaginationParam{
		Count: total,
		Page:  param.Page,
		Limit: param.Limit,
		Data:  fieldScheduleResults,
	}
	response := utils.GeneratePagination(pagination)
	return &response, nil
}

func (f *FieldScheduleService) GetAllByFieldIdAndDate(
	ctx context.Context, uuid string, date string,
) ([]dto.FieldScheduleForBookingResponse, error) {
	field, err := f.repository.GetField().FindByUUID(ctx, uuid)
	if err != nil {
		return nil, err
	}
	fieldSchedules, err := f.repository.GetFieldSchedule().FindAllByFieldIdAndDate(ctx, int(field.ID), date)
	if err != nil {
		return nil, err
	}
	fieldScheduleResults := make([]dto.FieldScheduleForBookingResponse, 0, len(fieldSchedules))
	for _, schedule := range fieldSchedules {
		pricePerHour := float64(schedule.Field.PricePerHour)
		fieldScheduleResults = append(fieldScheduleResults, dto.FieldScheduleForBookingResponse{
			UUID:         schedule.UUID,
			Date:         utils.ConvertMonthName(schedule.Date.Format(time.DateOnly)),
			Time:         schedule.Time.StartTime,
			Status:       schedule.Status.GetStatusString(),
			PricePerHour: utils.RupiahFormat(&pricePerHour),
		})
	}
	return fieldScheduleResults, nil
}

func (f *FieldScheduleService) GetByUUID(ctx context.Context, uuid string) (*dto.FieldScheduleResponse, error) {
	fieldSchedule, err := f.repository.GetFieldSchedule().FindByUUID(ctx, uuid)
	if err != nil {
		return nil, err
	}
	response := dto.FieldScheduleResponse{
		UUID:         fieldSchedule.UUID,
		FieldName:    fieldSchedule.Field.Name,
		PricePerHour: fieldSchedule.Field.PricePerHour,
		Date:         fieldSchedule.Date.Format(time.DateOnly),
		Status:       fieldSchedule.Status.GetStatusString(),
		Time:         fmt.Sprintf("%s - %s", fieldSchedule.Time.StartTime, fieldSchedule.Time.EndTime),
		CreateAt:     fieldSchedule.CreatedAt,
		UpdateAt:     fieldSchedule.UpdatedAt,
	}
	return &response, nil
}

func (f *FieldScheduleService) GenerateScheduleForOneMonth(ctx context.Context, request *dto.GenerateFieldScheduleForOneMonthRequest) error {
	field, err := f.repository.GetField().FindByUUID(ctx, request.FieldID)
	if err != nil {
		return err
	}
	times, err := f.repository.GetTime().FindAll(ctx)
	if err != nil {
		return err
	}
	numberOfDays := 30
	fieldSchedules := make([]models.FieldSchedule, 0, numberOfDays)
	now := time.Now().Add(time.Duration(1) * 24 * time.Hour)
	for i := 0; i < numberOfDays; i++ {
		currentDate := now.AddDate(0, 0, i)
		for _, item := range times {
			schedule, err := f.repository.GetFieldSchedule().
				FindByDateAndTimeId(
					ctx, currentDate.Format(time.DateOnly), int(item.ID), int(field.ID),
				)
			if err != nil {
				return err
			}

			if schedule != nil {
				return errFieldSchedule.ErrFieldScheduleIsExist
			}
			fieldSchedules = append(fieldSchedules, models.FieldSchedule{
				UUID:    uuid.New(),
				FieldID: field.ID,
				TimeID:  item.ID,
				Date:    currentDate,
				Status:  constants.Available,
			})
		}
	}
	err = f.repository.GetFieldSchedule().Create(ctx, fieldSchedules)
	if err != nil {
		return err
	}
	return nil
}

func (f *FieldScheduleService) Create(ctx context.Context, request *dto.FieldScheduleRequest) error {
	field, err := f.repository.GetField().FindByUUID(ctx, request.FieldID)
	if err != nil {
		return err
	}
	// The date is parsed before anything is looked up: the existence check and
	// the row that is inserted have to agree on the day, and a string the
	// database would coerce differently than Go must not reach either of them.
	dateParsed, err := time.Parse(time.DateOnly, request.Date)
	if err != nil {
		return errorConstants.ErrRequestValidation
	}
	normalizedDate := dateParsed.Format(time.DateOnly)

	fieldSchedules := make([]models.FieldSchedule, 0, len(request.TimeIDs))
	for _, timeID := range request.TimeIDs {
		scheduleTime, err := f.repository.GetTime().FindByUUID(ctx, timeID)
		if err != nil {
			return err
		}
		schedule, err := f.repository.GetFieldSchedule().
			FindByDateAndTimeId(ctx, normalizedDate, int(scheduleTime.ID), int(field.ID))
		if err != nil {
			return err
		}
		if schedule != nil {
			return errFieldSchedule.ErrFieldScheduleIsExist
		}
		fieldSchedules = append(fieldSchedules, models.FieldSchedule{
			UUID:    uuid.New(),
			FieldID: field.ID,
			TimeID:  scheduleTime.ID,
			Date:    dateParsed,
			Status:  constants.Available,
		})
	}
	err = f.repository.GetFieldSchedule().Create(ctx, fieldSchedules)
	if err != nil {
		return err
	}
	return nil
}

func (f *FieldScheduleService) Update(
	ctx context.Context,
	uuid string,
	request *dto.UpdateFieldScheduleRequest,
) (*dto.FieldScheduleResponse, error) {
	fieldSchedule, err := f.repository.GetFieldSchedule().FindByUUID(ctx, uuid)
	if err != nil {
		return nil, err
	}
	scheduleTime, err := f.repository.GetTime().FindByUUID(ctx, request.TimeID)
	if err != nil {
		return nil, err
	}
	dateParsed, err := time.Parse(time.DateOnly, request.Date)
	if err != nil {
		return nil, errorConstants.ErrRequestValidation
	}
	normalizedDate := dateParsed.Format(time.DateOnly)

	// The slot moves whenever either the date or the time changes, so both have
	// to be re-checked for a conflict; checking only the date let a time-only
	// move collide with an existing schedule.
	dateChanged := normalizedDate != fieldSchedule.Date.Format(time.DateOnly)
	timeChanged := scheduleTime.ID != fieldSchedule.TimeID
	if dateChanged || timeChanged {
		existingSchedule, err := f.repository.GetFieldSchedule().FindByDateAndTimeId(
			ctx,
			normalizedDate,
			int(scheduleTime.ID),
			int(fieldSchedule.FieldID),
		)
		if err != nil {
			return nil, err
		}
		// The schedule being edited is not a conflict with itself.
		if existingSchedule != nil && existingSchedule.UUID != fieldSchedule.UUID {
			return nil, errFieldSchedule.ErrFieldScheduleIsExist
		}
	}
	fieldResult, err := f.repository.GetFieldSchedule().Update(ctx, uuid, &models.FieldSchedule{
		Date:   dateParsed,
		TimeID: scheduleTime.ID,
	})
	if err != nil {
		return nil, err
	}
	response := dto.FieldScheduleResponse{
		UUID:         fieldResult.UUID,
		FieldName:    fieldResult.Field.Name,
		Date:         fieldResult.Date.Format(time.DateOnly),
		PricePerHour: fieldResult.Field.PricePerHour,
		Status:       fieldResult.Status.GetStatusString(),
		Time:         fmt.Sprintf("%s - %s", scheduleTime.StartTime, scheduleTime.EndTime),
		CreateAt:     fieldResult.CreatedAt,
		UpdateAt:     fieldResult.UpdatedAt,
	}
	return &response, nil
}

func (f *FieldScheduleService) UpdateStatus(
	ctx context.Context,
	request *dto.UpdateStatusFieldScheduleRequest,
) error {
	// Handing the whole batch to the repository keeps the booking atomic: a
	// slot that is taken midway rolls the other slots back instead of leaving
	// the customer with a partial booking.
	return f.repository.GetFieldSchedule().UpdateStatus(ctx, constants.Booked, request.FieldScheduleIDs)
}

func (f *FieldScheduleService) Delete(ctx context.Context, uuid string) error {
	_, err := f.repository.GetFieldSchedule().FindByUUID(ctx, uuid)
	if err != nil {
		return err
	}
	err = f.repository.GetFieldSchedule().Delete(ctx, uuid)
	if err != nil {
		return err
	}
	return nil
}
