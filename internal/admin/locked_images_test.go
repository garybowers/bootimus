package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bootimus/internal/models"
	"bootimus/internal/storage/storagetest"
)

func TestImageMenuExceptionUpdates(test *testing.T) {
	store := storagetest.New(test)
	image := &models.Image{Name: "Kiosk", Filename: "kiosk.iso", Enabled: true, BootPasswordRequired: true, BootPasswordHash: "retained-verifier"}
	if err := store.CreateImage(image); err != nil {
		test.Fatal(err)
	}
	handler := &Handler{storage: store}
	for _, scenario := range []struct {
		body   string
		status int
		exempt bool
	}{
		{`{"boot_menu_exempt":true}`, 200, true},
		{`{"name":"Renamed"}`, 200, true},
		{`{"boot_menu_exempt":"false"}`, 400, true},
		{`{"boot_menu_exempt":null}`, 400, true},
		{`{"boot_menu_exempt":false}`, 200, false},
	} {
		response := httptest.NewRecorder()
		handler.UpdateImage(response, httptest.NewRequest(http.MethodPut, "/api/images?filename=kiosk.iso", strings.NewReader(scenario.body)))
		if response.Code != scenario.status {
			test.Fatalf("%s: %d", scenario.body, response.Code)
		}
		stored, err := store.GetImage(image.Filename)
		if err != nil || stored.BootMenuExempt != scenario.exempt || !stored.BootPasswordRequired || stored.BootPasswordHash != image.BootPasswordHash {
			test.Fatal("exception edit changed password or lost the flag")
		}
	}
}
