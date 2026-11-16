package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bootimus/internal/models"
	"bootimus/internal/storage/storagetest"
)

func TestImageBootScriptUpdates(test *testing.T) {
	store := storagetest.New(test)
	image := &models.Image{Name: "Windows PE", Filename: "winpe.iso", Enabled: true}
	if err := store.CreateImage(image); err != nil {
		test.Fatal(err)
	}
	handler := &Handler{storage: store}
	oversized := `{"boot_script":"` + strings.Repeat("a", models.MaxBootScriptBytes+1) + `"}`
	for _, scenario := range []struct {
		body   string
		status int
		script string
	}{
		{`{"boot_script":"\r\nkernel {{BASE_URL}}/wimboot\r\nboot || goto failed\r\n"}`, 200, "kernel {{BASE_URL}}/wimboot\nboot || goto failed"},
		{`{"name":"Renamed"}`, 200, "kernel {{BASE_URL}}/wimboot\nboot || goto failed"},
		{`{"boot_script":null}`, 400, "kernel {{BASE_URL}}/wimboot\nboot || goto failed"},
		{`{"boot_script":["boot"]}`, 400, "kernel {{BASE_URL}}/wimboot\nboot || goto failed"},
		{`{"boot_script":"echo \u0000"}`, 400, "kernel {{BASE_URL}}/wimboot\nboot || goto failed"},
		{oversized, 400, "kernel {{BASE_URL}}/wimboot\nboot || goto failed"},
		{`{"boot_script":"  \n"}`, 200, ""},
	} {
		response := httptest.NewRecorder()
		handler.UpdateImage(response, httptest.NewRequest(http.MethodPut, "/api/images?filename=winpe.iso", strings.NewReader(scenario.body)))
		label := scenario.body
		if len(label) > 60 {
			label = label[:60]
		}
		if response.Code != scenario.status {
			test.Fatalf("%s: status %d, want %d", label, response.Code, scenario.status)
		}
		stored, err := store.GetImage(image.Filename)
		if err != nil {
			test.Fatal(err)
		}
		if stored.BootScript != scenario.script {
			test.Fatalf("%s: stored script %q, want %q", label, stored.BootScript, scenario.script)
		}
	}
}
