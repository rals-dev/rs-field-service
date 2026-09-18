package controllers

import (
	"context"
	"encoding/json"
	errorConstants "field-service/constants/error"
	"field-service/domain/dto"
	serviceRegistry "field-service/services"
	fieldService "field-service/services/field"
	fieldScheduleService "field-service/services/field_schedule"
	timeService "field-service/services/time"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type fakeTimeService struct {
	response *dto.TimeResponse
	err      error
	calls    int
}

func (f *fakeTimeService) GetAll(context.Context) ([]dto.TimeResponse, error) {
	return nil, nil
}

func (f *fakeTimeService) GetByUUID(context.Context, string) (*dto.TimeResponse, error) {
	return nil, nil
}

func (f *fakeTimeService) Create(context.Context, *dto.TimeRequest) (*dto.TimeResponse, error) {
	f.calls++
	return f.response, f.err
}

type fakeServiceRegistry struct {
	scheduleTime timeService.ITimeService
}

func (f *fakeServiceRegistry) GetField() fieldService.IFieldService {
	return nil
}

func (f *fakeServiceRegistry) GetFieldSchedule() fieldScheduleService.IFieldScheduleService {
	return nil
}

func (f *fakeServiceRegistry) GetTime() timeService.ITimeService {
	return f.scheduleTime
}

var _ serviceRegistry.IServiceRegistry = (*fakeServiceRegistry)(nil)

// httpResponseBody mirrors common/response.Response with a typed Data so the
// tests can read the payload back.
type httpResponseBody struct {
	Status  string            `json:"status"`
	Message string            `json:"message"`
	Data    *dto.TimeResponse `json:"data"`
}

func postTimeCreate(t *testing.T, service *fakeTimeService, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(recorder)
	ginContext.Request = httptest.NewRequest(http.MethodPost, "/api/v1/time", strings.NewReader(body))
	ginContext.Request.Header.Set("Content-Type", "application/json")

	NewTimeController(&fakeServiceRegistry{scheduleTime: service}).Create(ginContext)
	return recorder
}

// Regression: Create ignored the error returned by the service and answered
// 201 Created with a nil payload, so a failed insert looked like a success.
func TestTimeCreateReportsAServiceFailure(t *testing.T) {
	service := &fakeTimeService{err: errorConstants.ErrSQLError}

	recorder := postTimeCreate(t, service, `{"startTime":"08:00:00","endTime":"09:00:00"}`)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d (body %s)", http.StatusBadRequest, recorder.Code, recorder.Body.String())
	}
	var body httpResponseBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("could not decode the response: %v", err)
	}
	if body.Message != errorConstants.ErrSQLError.Error() {
		t.Errorf("expected the mapped error message %q, got %q", errorConstants.ErrSQLError.Error(), body.Message)
	}
	if body.Data != nil {
		t.Errorf("expected no data on a failure, got %v", body.Data)
	}
}

func TestTimeCreateReturnsCreatedOnSuccess(t *testing.T) {
	service := &fakeTimeService{response: &dto.TimeResponse{StartTime: "08:00:00", EndTime: "09:00:00"}}

	recorder := postTimeCreate(t, service, `{"startTime":"08:00:00","endTime":"09:00:00"}`)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected %d, got %d (body %s)", http.StatusCreated, recorder.Code, recorder.Body.String())
	}
	var body httpResponseBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("could not decode the response: %v", err)
	}
	if body.Data == nil || body.Data.StartTime != "08:00:00" || body.Data.EndTime != "09:00:00" {
		t.Errorf("expected the created time to be echoed back, got %v", body.Data)
	}
}

func TestTimeCreateRejectsAnIncompleteRequestWithoutCallingTheService(t *testing.T) {
	service := &fakeTimeService{}

	recorder := postTimeCreate(t, service, `{"startTime":"08:00:00"}`)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d (body %s)", http.StatusBadRequest, recorder.Code, recorder.Body.String())
	}
	if service.calls != 0 {
		t.Errorf("expected the service not to be called, got %d calls", service.calls)
	}
}
