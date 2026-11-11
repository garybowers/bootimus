package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bootimus/internal/storage/storagetest"
)

func TestLockedMenuSettings(test *testing.T) {
	store := storagetest.New(test)
	handler := &Handler{storage: store}
	update := func(body string) int {
		response := httptest.NewRecorder()
		handler.UpdateMenuTheme(response, httptest.NewRequest(http.MethodPut, "/api/theme", strings.NewReader(body)))
		return response.Code
	}
	if status := update(`{"locked_menu_local_boot":true,"locked_menu_timeout":12}`); status != http.StatusOK {
		test.Fatal(status)
	}
	if status := update(`{"title":"Keep local boot"}`); status != http.StatusOK {
		test.Fatal(status)
	}
	for _, body := range []string{`{"locked_menu_timeout":-1}`, `{"locked_menu_timeout":86401}`, `{"locked_menu_timeout":1.5}`, `{"locked_menu_timeout":"10"}`, `{"locked_menu_local_boot":"true"}`} {
		if status := update(body); status != http.StatusBadRequest {
			test.Fatalf("accepted %s: %d", body, status)
		}
	}
	theme, err := store.GetMenuTheme()
	if err != nil || !theme.LockedMenuLocalBoot || theme.LockedMenuTimeout != 12 {
		test.Fatal("invalid or omitted fields changed the settings")
	}
	if status := update(`{"locked_menu_local_boot":false,"locked_menu_timeout":0}`); status != http.StatusOK {
		test.Fatal(status)
	}
	theme, err = store.GetMenuTheme()
	if err != nil || theme.LockedMenuLocalBoot || theme.LockedMenuTimeout != 0 {
		test.Fatal("could not disable local boot countdown")
	}
}
