package admin

import (
	"github.com/gougoujiang/buildmax/internal/testsupport"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// Fixtures this package's tests share. They are copies of the root package's
// rather than an import: a test helper shared across a package boundary makes
// the boundary softer than the code's, which is what splitting these packages
// was for.
const testSecret = "matrix-secret"

const matrixTeam = "tm_matrix"

func adminRequestAs(t *testing.T, mux *http.ServeMux, c adminCase, userID string) *httptest.ResponseRecorder {
	t.Helper()
	path := regexp.MustCompile(`\{[^}]+\}`).ReplaceAllString(c.path, "nonexistent")
	req := httptest.NewRequest(c.method, path, strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	if userID != "" {
		req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT(userID, testSecret))
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}
