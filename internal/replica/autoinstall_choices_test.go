package replica

import (
	"bootimus/internal/models"
	"context"
	"slices"
	"testing"
)

func TestSyncAutoInstallChoices(t *testing.T) {
	e := newEnv(t, true)
	images := e.primary.data["/api/images"].([]models.Image)
	groups := e.primary.data["/api/client-groups"].([]models.ClientGroup)
	clients := e.primary.data["/api/clients"].([]models.Client)
	for _, choices := range []models.StringSlice{{"windows/desktop.xml", "windows/kiosk.xml"}, nil} {
		images[0].AutoInstallChoices = choices
		groups[0].AutoInstallChoices = choices
		clients[0].AutoInstallChoices = choices
		status := e.syncer.RunNow(context.Background())
		if status.LastError != "" {
			t.Fatal(status.LastError)
		}
		image, err := e.store.GetImage(images[0].Filename)
		if err != nil {
			t.Fatal(err)
		}
		group, err := e.store.GetClientGroupByName(groups[0].Name)
		if err != nil {
			t.Fatal(err)
		}
		client, err := e.store.GetClient("aa:bb:cc:dd:ee:01")
		if err != nil {
			t.Fatal(err)
		}
		for _, got := range [][]string{image.AutoInstallChoices, group.AutoInstallChoices, client.AutoInstallChoices} {
			if !slices.Equal(got, choices) {
				t.Fatalf("replica choices = %v, want %v", got, choices)
			}
		}
	}
}
