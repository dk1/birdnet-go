package notifications

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tphakala/birdnet-go/internal/api/v2/apicore"
	"github.com/tphakala/birdnet-go/internal/api/v2/apitest"
	"github.com/tphakala/birdnet-go/internal/conf"
	"github.com/tphakala/birdnet-go/internal/notification"
)

func TestCreateTestLiferNotification_ServiceNotInitialized(t *testing.T) {
	t.Parallel()

	require.False(t, notification.IsInitialized(),
		"no api/v2 test may initialize the global notification singleton; this test relies on it staying unset")

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/notifications/test/lifer", http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	controller := New(&apicore.Core{}, nil, nil)
	controller.Settings.Store(&conf.Settings{})

	handler := controller.requireNotificationService(controller.CreateTestLiferNotification)
	err := handler(c)
	require.NoError(t, err)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Contains(t, rec.Body.String(), "Notification service not available")
}

func TestCreateTestLiferNotification_Success(t *testing.T) {
	// No t.Parallel(): publishes to the process-global settings singleton, see
	// TestCreateTestNewSpeciesNotification_Success for the same rationale.
	config := &notification.ServiceConfig{
		Debug:              true,
		MaxNotifications:   100,
		CleanupInterval:    30 * time.Minute,
		RateLimitWindow:    1 * time.Minute,
		RateLimitMaxEvents: 10,
	}

	service := notification.NewService(config)
	t.Cleanup(service.Stop)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/notifications/test/lifer", http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	controller := New(&apicore.Core{}, service, nil)
	settings := &conf.Settings{}
	settings.Security.Host = "localhost"
	settings.WebServer.Port = "8080"
	settings.Main.TimeAs24h = true
	settings.Notification.Templates.Lifer.Title = "Lifer: {{.CommonName}}"
	settings.Notification.Templates.Lifer.Message = "{{.CommonName}} ({{.ScientificName}}) is not on your life list, with {{.ConfidencePercent}}% confidence at {{.DetectionTime}}. View: {{.DetectionURL}}"
	controller.Settings.Store(settings)
	apitest.PublishTestSettings(t, settings)

	err := controller.CreateTestLiferNotification(c)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, rec.Code)

	var response notification.Notification
	err = parseJSONResponse(rec.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, notification.TypeDetection, response.Type)
	assert.Equal(t, notification.PriorityHigh, response.Priority)
	assert.Equal(t, "detection", response.Component)
	assert.Equal(t, notification.StatusUnread, response.Status)
	assert.Equal(t, "Lifer: Test Bird Species", response.Title)
	assert.Contains(t, response.Message, "Test Bird Species")
	assert.Contains(t, response.Message, "Testus birdicus")
	assert.Contains(t, response.Message, "99% confidence")
	assert.NotEmpty(t, response.ID)
	assert.False(t, response.Timestamp.IsZero())

	require.NotNil(t, response.Metadata)
	assert.Equal(t, "Test Bird Species", response.Metadata["species"])
	assert.Equal(t, "Testus birdicus", response.Metadata["scientific_name"])
	assert.InDelta(t, 0.99, response.Metadata["confidence"], 0.001)
	assert.Equal(t, "Test Location (Sample Data)", response.Metadata["location"])
	assert.Equal(t, true, response.Metadata["is_lifer"])

	require.NotNil(t, response.ExpiresAt)
	expectedExpiry := response.Timestamp.Add(24 * time.Hour)
	assert.WithinDuration(t, expectedExpiry, *response.ExpiresAt, time.Second)
}
