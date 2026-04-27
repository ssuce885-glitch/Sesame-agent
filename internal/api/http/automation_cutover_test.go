package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIncidentRoutesAreNotRegistered(t *testing.T) {
	router := NewRouter(NewTestDependencies(t))

	req := httptest.NewRequest(http.MethodGet, "/v1/incidents", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET /v1/incidents status = %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/incidents/incident_1/ack", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("POST /v1/incidents/{id}/ack status = %d", rec.Code)
	}
}
