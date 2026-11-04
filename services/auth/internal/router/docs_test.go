package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDocsEndpoints(t *testing.T) {
	mux := http.NewServeMux()
	registerDocs(mux)

	for path, want := range map[string]string{
		"/openapi.yaml": "openapi: 3.0.3",
		"/docs":         "SwaggerUIBundle",
	} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), want) {
			t.Errorf("%s: status=%d, missing %q", path, rec.Code, want)
		}
	}
}
