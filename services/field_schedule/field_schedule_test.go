package services

import (
	"context"
	"errors"
	"field-service/constants"
	errorConstants "field-service/constants/error"
	errFieldSchedule "field-service/constants/error/field_schedule"
	"field-service/domain/dto"
	"field-service/domain/models"
	repositoryRegistry "field-service/repositories"
	fieldRepo "field-service/repositories/field"
	fieldScheduleRepo "field-service/repositories/field_schedule"
	timeRepo "field-service/repositories/time"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
)

var errNotImplemented = errors.New("not implemented")

type fakeFieldRepository struct {
	field *models.Field
	err   error
}

func (f *fakeFieldRepository) FindAllWithPagination(context.Context, *dto.FieldRequestParam) ([]models.Field, int64, error) {
	return nil, 0, errNotImplemented
}

func (f *fakeFieldRepository) FindAllWithoutPagination(context.Context) ([]models.Field, error) {
	return nil, errNotImplemented
}

func (f *fakeFieldRepository) FindByUUID(context.Context, string) (*models.Field, error) {
	return f.field, f.err
}

func (f *fakeFieldRepository) Create(context.Context, *models.Field) (*models.Field, error) {
	return nil, errNotImplemented
}

func (f *fakeFieldRepository) Update(context.Context, string, *models.Field) (*models.Field, error) {
	return nil, errNotImplemented
}

func (f *fakeFieldRepository) Delete(context.Context, string) error {
	return errNotImplemented
}

type fakeTimeRepository struct {
	byUUID map[string]*models.Time
}

func (f *fakeTimeRepository) FindAll(context.Context) ([]models.Time, error) {
	return nil, errNotImplemented
}

func (f *fakeTimeRepository) FindByUUID(_ context.Context, id string) (*models.Time, error) {
	scheduleTime, ok := f.byUUID[id]
	if !ok {
		return nil, errNotImplemented
	}
	return scheduleTime, nil
}

func (f *fakeTimeRepository) FindByID(context.Context, int) (*models.Time, error) {
	return nil, errNotImplemented
}

func (f *fakeTimeRepository) Create(context.Context, *models.Time) (*models.Time, error) {
	return nil, errNotImplemented
}

// slotLookup records one FindByDateAndTimeId call so the tests can assert which
// slot the conflict check actually asked about.
type slotLookup struct {
	date    string
	timeID  int
	fieldID int
}

type statusUpdate struct {
	status constants.FieldScheduleStatus
	uuids  []string
}

type fakeFieldScheduleRepository struct {
	schedule     *models.FieldSchedule
	existing     *models.FieldSchedule
	updateResult *models.FieldSchedule
	updateErr    error
	statusErr    error

	lookups       []slotLookup
	created       [][]models.FieldSchedule
	statusUpdates []statusUpdate
}

func (f *fakeFieldScheduleRepository) FindAllWithPagination(
	context.Context, *dto.FieldScheduleRequestParam,
) ([]models.FieldSchedule, int64, error) {
	return nil, 0, errNotImplemented
}

func (f *fakeFieldScheduleRepository) FindAllByFieldIdAndDate(context.Context, int, string) ([]models.FieldSchedule, error) {
	return nil, errNotImplemented
}

func (f *fakeFieldScheduleRepository) FindByUUID(context.Context, string) (*models.FieldSchedule, error) {
	return f.schedule, nil
}

func (f *fakeFieldScheduleRepository) FindByDateAndTimeId(
	_ context.Context, date string, timeID int, fieldID int,
) (*models.FieldSchedule, error) {
	f.lookups = append(f.lookups, slotLookup{date: date, timeID: timeID, fieldID: fieldID})
	return f.existing, nil
}

func (f *fakeFieldScheduleRepository) Create(_ context.Context, schedules []models.FieldSchedule) error {
	f.created = append(f.created, schedules)
	return nil
}

func (f *fakeFieldScheduleRepository) Update(context.Context, string, *models.FieldSchedule) (*models.FieldSchedule, error) {
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	return f.updateResult, nil
}

func (f *fakeFieldScheduleRepository) UpdateStatus(
	_ context.Context, status constants.FieldScheduleStatus, uuids []string,
) error {
	f.statusUpdates = append(f.statusUpdates, statusUpdate{status: status, uuids: uuids})
	return f.statusErr
}

func (f *fakeFieldScheduleRepository) Delete(context.Context, string) error {
	return errNotImplemented
}

type fakeRepositoryRegistry struct {
	field         fieldRepo.IFieldRepository
	fieldSchedule fieldScheduleRepo.IFieldScheduleRepository
	scheduleTime  timeRepo.ITimeRepository
}

func (r *fakeRepositoryRegistry) GetField() fieldRepo.IFieldRepository {
	return r.field
}

func (r *fakeRepositoryRegistry) GetFieldSchedule() fieldScheduleRepo.IFieldScheduleRepository {
	return r.fieldSchedule
}

func (r *fakeRepositoryRegistry) GetTime() timeRepo.ITimeRepository {
	return r.scheduleTime
}

var _ repositoryRegistry.IRepositoryRegistry = (*fakeRepositoryRegistry)(nil)

const (
	firstTimeUUID  = "11111111-1111-1111-1111-111111111111"
	secondTimeUUID = "22222222-2222-2222-2222-222222222222"
)

func newScheduleServiceForTest(scheduleRepository *fakeFieldScheduleRepository) IFieldScheduleService {
	return NewFieldScheduleService(&fakeRepositoryRegistry{
		field:         &fakeFieldRepository{field: &models.Field{ID: 5}},
		fieldSchedule: scheduleRepository,
		scheduleTime: &fakeTimeRepository{byUUID: map[string]*models.Time{
			firstTimeUUID:  {ID: 1, StartTime: "08:00:00", EndTime: "09:00:00"},
			secondTimeUUID: {ID: 2, StartTime: "09:00:00", EndTime: "10:00:00"},
		}},
	})
}

func scheduleUnderEdit() *models.FieldSchedule {
	return &models.FieldSchedule{
		ID:      1,
		UUID:    uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"),
		FieldID: 5,
		TimeID:  1,
		Date:    time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC),
		Status:  constants.Available,
	}
}

// Regression: the conflict check only looked at the date, so moving a schedule
// to a different time slot on the same day skipped the check entirely and
// collided with the schedule already sitting in that slot.
func TestUpdateDetectsConflictWhenOnlyTheTimeChanges(t *testing.T) {
	scheduleRepository := &fakeFieldScheduleRepository{
		schedule: scheduleUnderEdit(),
		existing: &models.FieldSchedule{
			UUID:    uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"),
			FieldID: 5,
			TimeID:  2,
		},
	}
	service := newScheduleServiceForTest(scheduleRepository)

	_, err := service.Update(context.Background(), "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		&dto.UpdateFieldScheduleRequest{Date: "2026-09-18", TimeID: secondTimeUUID})

	if !errors.Is(err, errFieldSchedule.ErrFieldScheduleIsExist) {
		t.Fatalf("expected %v, got %v", errFieldSchedule.ErrFieldScheduleIsExist, err)
	}
	expectedLookups := []slotLookup{{date: "2026-09-18", timeID: 2, fieldID: 5}}
	if !reflect.DeepEqual(scheduleRepository.lookups, expectedLookups) {
		t.Errorf("expected lookups %v, got %v", expectedLookups, scheduleRepository.lookups)
	}
}

// Moving a schedule onto a different day must still be checked.
func TestUpdateDetectsConflictWhenOnlyTheDateChanges(t *testing.T) {
	scheduleRepository := &fakeFieldScheduleRepository{
		schedule: scheduleUnderEdit(),
		existing: &models.FieldSchedule{
			UUID:    uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"),
			FieldID: 5,
			TimeID:  1,
		},
	}
	service := newScheduleServiceForTest(scheduleRepository)

	_, err := service.Update(context.Background(), "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		&dto.UpdateFieldScheduleRequest{Date: "2026-09-19", TimeID: firstTimeUUID})

	if !errors.Is(err, errFieldSchedule.ErrFieldScheduleIsExist) {
		t.Fatalf("expected %v, got %v", errFieldSchedule.ErrFieldScheduleIsExist, err)
	}
	expectedLookups := []slotLookup{{date: "2026-09-19", timeID: 1, fieldID: 5}}
	if !reflect.DeepEqual(scheduleRepository.lookups, expectedLookups) {
		t.Errorf("expected lookups %v, got %v", expectedLookups, scheduleRepository.lookups)
	}
}

// The schedule being edited occupies the slot it is already in, so finding
// itself there must not be reported as a conflict.
func TestUpdateDoesNotConflictWithItself(t *testing.T) {
	current := scheduleUnderEdit()
	scheduleRepository := &fakeFieldScheduleRepository{
		schedule:     current,
		existing:     current,
		updateResult: current,
	}
	service := newScheduleServiceForTest(scheduleRepository)

	result, err := service.Update(context.Background(), current.UUID.String(),
		&dto.UpdateFieldScheduleRequest{Date: "2026-09-19", TimeID: firstTimeUUID})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result == nil {
		t.Fatal("expected a response")
	}
}

// Nothing moved, so there is no slot to check.
func TestUpdateSkipsTheConflictLookupWhenNothingMoves(t *testing.T) {
	current := scheduleUnderEdit()
	scheduleRepository := &fakeFieldScheduleRepository{
		schedule:     current,
		updateResult: current,
	}
	service := newScheduleServiceForTest(scheduleRepository)

	_, err := service.Update(context.Background(), current.UUID.String(),
		&dto.UpdateFieldScheduleRequest{Date: "2026-09-18", TimeID: firstTimeUUID})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(scheduleRepository.lookups) != 0 {
		t.Errorf("expected no conflict lookup, got %v", scheduleRepository.lookups)
	}
}

// A date string Go cannot parse used to reach the conflict query verbatim while
// the stored row silently fell back to the zero time, so the row that was
// checked and the row that was written were not the same day.
func TestUpdateRejectsADateGoCannotParse(t *testing.T) {
	current := scheduleUnderEdit()
	scheduleRepository := &fakeFieldScheduleRepository{
		schedule:     current,
		updateResult: current,
	}
	service := newScheduleServiceForTest(scheduleRepository)

	_, err := service.Update(context.Background(), current.UUID.String(),
		&dto.UpdateFieldScheduleRequest{Date: "2026-09-18T00:00:00Z", TimeID: firstTimeUUID})

	if !errors.Is(err, errorConstants.ErrRequestValidation) {
		t.Fatalf("expected %v, got %v", errorConstants.ErrRequestValidation, err)
	}
	if len(scheduleRepository.lookups) != 0 {
		t.Errorf("expected no conflict lookup for an unparseable date, got %v", scheduleRepository.lookups)
	}
}

func TestCreateChecksAndStoresTheSameDay(t *testing.T) {
	scheduleRepository := &fakeFieldScheduleRepository{}
	service := newScheduleServiceForTest(scheduleRepository)

	err := service.Create(context.Background(), &dto.FieldScheduleRequest{
		FieldID: "field-uuid",
		Date:    "2026-09-18",
		TimeIDs: []string{firstTimeUUID, secondTimeUUID},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	expectedLookups := []slotLookup{
		{date: "2026-09-18", timeID: 1, fieldID: 5},
		{date: "2026-09-18", timeID: 2, fieldID: 5},
	}
	if !reflect.DeepEqual(scheduleRepository.lookups, expectedLookups) {
		t.Fatalf("expected lookups %v, got %v", expectedLookups, scheduleRepository.lookups)
	}

	// One batch, so the insert is a single statement and cannot store a subset.
	if len(scheduleRepository.created) != 1 {
		t.Fatalf("expected a single create call, got %d", len(scheduleRepository.created))
	}
	batch := scheduleRepository.created[0]
	if len(batch) != 2 {
		t.Fatalf("expected 2 schedules in the batch, got %d", len(batch))
	}
	expectedDate := time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)
	for _, schedule := range batch {
		if !schedule.Date.Equal(expectedDate) {
			t.Errorf("expected the stored date to match the checked day %v, got %v", expectedDate, schedule.Date)
		}
		if schedule.Status != constants.Available {
			t.Errorf("expected a new schedule to be available, got %v", schedule.Status)
		}
	}
}

func TestCreateRejectsADateGoCannotParse(t *testing.T) {
	scheduleRepository := &fakeFieldScheduleRepository{}
	service := newScheduleServiceForTest(scheduleRepository)

	err := service.Create(context.Background(), &dto.FieldScheduleRequest{
		FieldID: "field-uuid",
		Date:    "18-09-2026",
		TimeIDs: []string{firstTimeUUID},
	})

	if !errors.Is(err, errorConstants.ErrRequestValidation) {
		t.Fatalf("expected %v, got %v", errorConstants.ErrRequestValidation, err)
	}
	if len(scheduleRepository.created) != 0 {
		t.Errorf("expected nothing to be created, got %v", scheduleRepository.created)
	}
}

// Regression: booking used to walk the requested slots one at a time, so a slot
// taken midway left the earlier slots booked. The whole batch now goes to the
// repository in one call, which wraps it in a single transaction.
func TestUpdateStatusBooksTheWholeBatchInOneCall(t *testing.T) {
	scheduleRepository := &fakeFieldScheduleRepository{}
	service := newScheduleServiceForTest(scheduleRepository)

	uuids := []string{"c", "a", "b"}
	err := service.UpdateStatus(context.Background(), &dto.UpdateStatusFieldScheduleRequest{
		FieldScheduleIDs: uuids,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(scheduleRepository.statusUpdates) != 1 {
		t.Fatalf("expected a single UpdateStatus call, got %d", len(scheduleRepository.statusUpdates))
	}
	update := scheduleRepository.statusUpdates[0]
	if update.status != constants.Booked {
		t.Errorf("expected status %v, got %v", constants.Booked, update.status)
	}
	if !reflect.DeepEqual(update.uuids, uuids) {
		t.Errorf("expected the whole batch %v, got %v", uuids, update.uuids)
	}
}

// A slot that is already booked must surface as a booking failure rather than
// being reported as a success.
func TestUpdateStatusPropagatesAnAlreadyBookedSlot(t *testing.T) {
	scheduleRepository := &fakeFieldScheduleRepository{statusErr: errFieldSchedule.ErrFieldScheduleIsBooked}
	service := newScheduleServiceForTest(scheduleRepository)

	err := service.UpdateStatus(context.Background(), &dto.UpdateStatusFieldScheduleRequest{
		FieldScheduleIDs: []string{"a", "b"},
	})

	if !errors.Is(err, errFieldSchedule.ErrFieldScheduleIsBooked) {
		t.Fatalf("expected %v, got %v", errFieldSchedule.ErrFieldScheduleIsBooked, err)
	}
}
