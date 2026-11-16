package admin

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"bootimus/internal/models"
	"bootimus/internal/smb"
	"bootimus/internal/storage/storagetest"
)

func TestGetImageMarksLegacySMBPatchesForRepatch(t *testing.T) {
	store := storagetest.New(t)
	handler := &Handler{storage: store, isoDir: t.TempDir(), serverAddr: "192.0.2.1"}
	image := &models.Image{Name: "Windows", Filename: "windows.iso", SMBInstallEnabled: true}
	legacyHash := sha256.New()
	fmt.Fprintf(legacyHash, "addr=%s\nfile=%s\nai_enabled=%v\nai_file=%s\nai_content=", handler.serverAddr, image.Filename, image.AutoInstallEnabled, image.AutoInstallFile)
	legacyFingerprint := hex.EncodeToString(legacyHash.Sum(nil))
	sharedTemplateHash := sha256.New()
	fmt.Fprintf(sharedTemplateHash, "share=%s\n", smb.ShareName(image.Filename))
	fmt.Fprintf(sharedTemplateHash, "addr=%s\nfile=%s\nai_enabled=%v\nai_file=%s\nai_content=", handler.serverAddr, image.Filename, image.AutoInstallEnabled, image.AutoInstallFile)
	sharedTemplateFingerprint := hex.EncodeToString(sharedTemplateHash.Sum(nil))
	currentFingerprint := handler.computeSMBPatchFingerprint(image)
	if currentFingerprint == legacyFingerprint {
		t.Fatal("changed share names did not invalidate the patch fingerprint")
	}
	if err := store.CreateImage(image); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name        string
		fingerprint string
		repatch     bool
	}{
		{"missing fingerprint", "", true},
		{"legacy fingerprint", legacyFingerprint, true},
		{"shared template fingerprint", sharedTemplateFingerprint, true},
		{"current fingerprint", currentFingerprint, false},
	}
	for _, testcase := range cases {
		t.Run(testcase.name, func(t *testing.T) {
			image.SMBPatchFingerprint = testcase.fingerprint
			if err := store.UpdateImage(image.Filename, image); err != nil {
				t.Fatal(err)
			}
			recorder := httptest.NewRecorder()
			handler.GetImage(recorder, httptest.NewRequest(http.MethodGet, "/api/images?filename=windows.iso", nil))
			if recorder.Code != http.StatusOK {
				t.Fatalf("get image: %d %s", recorder.Code, recorder.Body.String())
			}
			var response struct {
				Data models.Image `json:"data"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Data.SMBNeedsRepatch != testcase.repatch {
				t.Errorf("needs repatch=%v, want %v", response.Data.SMBNeedsRepatch, testcase.repatch)
			}
		})
	}
}

func TestSMBFingerprintTracksStaticAnswersOnly(t *testing.T) {
	handler := &Handler{serverAddr: "192.0.2.1"}
	image := &models.Image{Filename: "windows.iso", AutoInstallEnabled: true, AutoInstallScript: "<unattend>{{HOSTNAME}} first</unattend>"}
	first := handler.computeSMBPatchFingerprint(image)
	image.AutoInstallScript = "<unattend>{{HOSTNAME}} edited {{VAR.role}}</unattend>"
	if handler.computeSMBPatchFingerprint(image) != first {
		t.Fatal("editing a dynamically rendered answer unnecessarily requires an SMB re-patch")
	}
	image.AutoInstallScript = "<unattend>static</unattend>"
	static := handler.computeSMBPatchFingerprint(image)
	if static == first {
		t.Fatal("changing to a static SMB fallback did not require a re-patch")
	}
	image.AutoInstallScript = "<unattend>changed static</unattend>"
	if handler.computeSMBPatchFingerprint(image) == static {
		t.Fatal("editing the static SMB fallback did not require a re-patch")
	}
}
