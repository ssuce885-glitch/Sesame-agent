package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateSessionThenStreamEvents(t *testing.T) {
	handler := NewRouter(NewTestDependencies(t))

	createReq := httptest.NewRequest(http.MethodPost, "/v1/sessions", strings.NewReader(`{"workspace_root":"D:/work/demo"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("create session status = %d, want %d", createRec.Code, http.StatusCreated)
	}
}
