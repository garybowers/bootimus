package replica

import (
	"context"
	"net/http"
	"testing"
	"time"

	"bootimus/internal/models"
)

func TestReplicaImageMenuExceptions(test *testing.T) {
	environment := newEnv(test, false)
	for _, exempt := range []bool{true, false} {
		environment.primary.mu.Lock()
		images := environment.primary.data["/api/images"].([]models.Image)
		images[0].BootMenuExempt = exempt
		environment.primary.data["/api/images"] = images
		environment.primary.mu.Unlock()
		if status := environment.syncer.RunNow(context.Background()); status.LastError != "" {
			test.Fatal(status.LastError)
		}
		image, err := environment.store.GetImage(images[0].Filename)
		if err != nil || image.BootMenuExempt != exempt {
			test.Fatal("image exception was not synchronised")
		}
	}
}

func TestReplicaRevokesExceptionOnVerifierFailure(test *testing.T) {
	environment := newEnv(test, false)
	setProtectedPrimary(environment, testBootHash(test, "image"), testBootHash(test, "menu"), time.Now().UTC().Add(-time.Minute))
	environment.primary.mu.Lock()
	images := environment.primary.data["/api/images"].([]models.Image)
	images[0].BootMenuExempt = true
	environment.primary.data["/api/images"] = images
	environment.primary.mu.Unlock()
	if status := environment.syncer.RunNow(context.Background()); status.LastError != "" {
		test.Fatal(status.LastError)
	}
	environment.primary.mu.Lock()
	images[0].BootMenuExempt = false
	images[0].UpdatedAt = images[0].UpdatedAt.Add(time.Second)
	environment.primary.data["/api/images"] = images
	environment.primary.status["/api/sync/boot-verifiers"] = http.StatusServiceUnavailable
	environment.primary.mu.Unlock()
	if status := environment.syncer.RunNow(context.Background()); status.LastError == "" {
		test.Fatal("expected verifier failure")
	}
	image, err := environment.store.GetImage(images[0].Filename)
	if err != nil || image.BootMenuExempt {
		test.Fatal("observed revocation retained an exception during verifier failure")
	}
}
