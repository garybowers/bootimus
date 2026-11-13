package replica

import (
	"bootimus/internal/models"
	"testing"
	"time"
)

func TestNativeTFTPClientSyncPreservesLocalRetryWindow(t *testing.T) {
	stamp := time.Now()
	local := models.Client{NativeTFTPLastBoot: &stamp}
	remote := models.Client{NativeTFTPBootFile: "ec84083c/kernel7.img"}
	applyClient(&local, remote, nil, nil)
	if local.NativeTFTPBootFile != remote.NativeTFTPBootFile || local.NativeTFTPLastBoot != &stamp {
		t.Fatalf("mapping or retry marker lost: %+v", local)
	}
	if !clientEqual(local, remote) {
		t.Fatal("local retry marker causes repeated sync updates")
	}
	remote.NativeTFTPBootFile = ""
	applyClient(&local, remote, nil, nil)
	if local.NativeTFTPBootFile != "" {
		t.Fatal("cleared mapping not synced")
	}
}
