package replica

import (
	"context"
	"testing"

	"bootimus/internal/models"
)

func TestReplicaLockedMenuSettings(test *testing.T) {
	environment := newEnv(test, false)
	for _, enabled := range []bool{true, false} {
		environment.primary.mu.Lock()
		environment.primary.data["/api/theme"] = models.MenuTheme{Title: "Local boot", LockedMenuLocalBoot: enabled, LockedMenuTimeout: 12}
		environment.primary.mu.Unlock()
		if status := environment.syncer.RunNow(context.Background()); status.LastError != "" {
			test.Fatal(status.LastError)
		}
		theme, err := environment.store.GetMenuTheme()
		if err != nil || theme.LockedMenuLocalBoot != enabled || theme.LockedMenuTimeout != 12 {
			test.Fatal("locked-menu settings were not synchronised")
		}
	}
}
