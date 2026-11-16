package admin

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bootimus/internal/autoinstall"
	"bootimus/internal/smb"
)

func TestSMBPatchStagesOnlyStaticAnswers(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		enabled       bool
		wantShare     bool
	}{
		{"static answer refreshed", "<unattend><ComputerName>current</ComputerName></unattend>", true, true},
		{"client template", "<unattend><ComputerName>{{HOSTNAME}}</ComputerName><Role>{{VAR.role}}</Role></unattend>", true, false},
		{"callback template", "<unattend><Command>{{CALLBACK_URL}}</Command></unattend>", true, false},
		{"disabled answer", "<unattend/>", false, false},
		{"empty answer", "", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, image := newDriverHandler(t)
			wimPath := testDriverWim(t, h)
			var err error
			h.autoInstallLib, err = autoinstall.New(h.dataDir)
			if err != nil {
				t.Fatal(err)
			}
			image.AutoInstallFile = "windows/answer.xml"
			image.AutoInstallEnabled = tc.enabled
			if err := h.autoInstallLib.Write("windows", "answer.xml", tc.content); err != nil {
				t.Fatal(err)
			}
			if err := h.storage.UpdateImage(image.Filename, image); err != nil {
				t.Fatal(err)
			}
			shareAnswer := filepath.Join(h.isoDir, "windows/iso/AutoUnattend.xml")
			if err := os.WriteFile(shareAnswer, []byte("<unattend>stale {{HOSTNAME}}</unattend>"), 0644); err != nil {
				t.Fatal(err)
			}
			h.smbManager = smb.NewManager(h.dataDir, "127.0.0.1", "", 1445)
			for attempt := 0; attempt < 2; attempt++ {
				response := httptest.NewRecorder()
				h.PatchImageSMB(response, httptest.NewRequest(http.MethodPost, "/api/images/patch-smb?filename="+image.Filename, nil))
				if response.Code != http.StatusOK {
					t.Fatal(response.Code, response.Body.String())
				}
				content, err := os.ReadFile(shareAnswer)
				if tc.wantShare {
					if err != nil || string(content) != tc.content {
						t.Fatalf("static share answer = %q, error %v", content, err)
					}
				} else if !os.IsNotExist(err) {
					t.Fatalf("unsafe or stale answer remains on share: %q (error %v)", content, err)
				}
				startup := testWimCommand(t, "extract", wimPath, "2", "/Windows/System32/startnet.cmd", "--to-stdout")
				if !strings.Contains(startup, `if exist X:\Windows\System32\AutoUnattend.xml set UNATTEND=`) {
					t.Fatal("patched WIM lost the rendered answer lookup")
				}
				if got := strings.Contains(startup, `if not defined UNATTEND if exist Z:\AutoUnattend.xml (`); got != tc.wantShare {
					t.Fatalf("share fallback = %v, want %v", got, tc.wantShare)
				}
				saved, err := h.storage.GetImage(image.Filename)
				if err != nil || saved.AutoInstallFile != image.AutoInstallFile || saved.AutoInstallEnabled != tc.enabled {
					t.Fatalf("repatch changed answer configuration: %+v, %v", saved, err)
				}
			}
		})
	}
}

func TestSMBPatchFailsWhenStaleAnswerCannotBeRemoved(t *testing.T) {
	h, image := newDriverHandler(t)
	wimPath := testDriverWim(t, h)
	before, err := os.ReadFile(wimPath)
	if err != nil {
		t.Fatal(err)
	}
	// A non-empty directory at the target path cannot be removed as a file.
	if err := os.MkdirAll(filepath.Join(h.isoDir, "windows/iso/AutoUnattend.xml/blocked"), 0755); err != nil {
		t.Fatal(err)
	}
	h.smbManager = smb.NewManager(h.dataDir, "127.0.0.1", "", 1445)
	response := httptest.NewRecorder()
	h.PatchImageSMB(response, httptest.NewRequest(http.MethodPost, "/api/images/patch-smb?filename="+image.Filename, nil))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("patch succeeded despite stale answer: %d %s", response.Code, response.Body.String())
	}
	after, err := os.ReadFile(wimPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("failed answer cleanup changed the WIM", err)
	}
}
