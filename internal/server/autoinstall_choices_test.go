package server

import (
	"bootimus/internal/autoinstall"
	"bootimus/internal/models"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
)

func TestAutoInstallChoicesInheritance(t *testing.T) {
	srv, store := newInstallEnv(t)
	group := &models.ClientGroup{Name: "lab", AutoInstallChoices: models.StringSlice{"windows/group.xml"}}
	if err := store.CreateClientGroup(group); err != nil {
		t.Fatal(err)
	}
	image := &models.Image{BootMethod: "kernel", Distro: "windows", AutoInstallChoices: models.StringSlice{"windows/image.xml"}}
	client := &models.Client{ClientGroupID: &group.ID, AutoInstallChoices: models.StringSlice{"windows/client.xml"}}
	for _, tc := range []struct {
		name   string
		client *models.Client
		want   []string
	}{
		{"client", client, client.AutoInstallChoices},
		{"group", &models.Client{ClientGroupID: &group.ID}, group.AutoInstallChoices},
		{"image", &models.Client{}, image.AutoInstallChoices},
		{"unknown client", nil, image.AutoInstallChoices},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := srv.autoInstallChoices(image, tc.client); !slices.Equal(got, tc.want) {
				t.Fatalf("choices = %v, want %v", got, tc.want)
			}
		})
	}
	image.Distro = "ubuntu"
	if got := srv.autoInstallChoices(image, client); len(got) != 0 {
		t.Fatalf("Windows choices leaked into Linux boot: %v", got)
	}
	image.Distro, image.BootMethod = "windows", "sanboot"
	if got := srv.autoInstallChoices(image, client); len(got) != 0 {
		t.Fatalf("choices offered without wimboot: %v", got)
	}
}

func TestAutoInstallChoicesServeOnlyOfferedFiles(t *testing.T) {
	srv, store := newInstallEnv(t)
	lib, err := autoinstall.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv.autoInstallLib = lib
	for _, name := range []string{"default.xml", "desktop.xml", "kiosk.xml", "private.xml"} {
		if err := lib.Write("windows", name, "<unattend>"+name+" {{HOSTNAME}} {{MAC}} {{CALLBACK_URL}}</unattend>"); err != nil {
			t.Fatal(err)
		}
	}
	image := &models.Image{Filename: "Win11.iso", Name: "Windows", Enabled: true, BootMethod: "kernel", Distro: "windows", AutoInstallFile: "windows/default.xml", AutoInstallChoices: models.StringSlice{"windows/desktop.xml", "windows/kiosk.xml"}}
	if err := store.CreateImage(image); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		query, want string
		status      int
	}{
		{"", "default.xml", http.StatusOK},
		{"&choice=default", "default.xml", http.StatusOK},
		{"&choice=" + autoInstallChoiceID("windows/desktop.xml"), "desktop.xml", http.StatusOK},
		{"&choice=" + autoInstallChoiceID("windows/kiosk.xml"), "kiosk.xml", http.StatusOK},
		{"&choice=" + autoInstallChoiceID("windows/private.xml"), "", http.StatusNotFound},
		{"&choice=windows%2Fkiosk.xml", "", http.StatusNotFound},
		{"&choice=..%2Fprivate.xml", "", http.StatusNotFound},
		{"&choice=", "", http.StatusBadRequest},
		{"&choice=default&choice=other", "", http.StatusBadRequest},
	} {
		t.Run(tc.query, func(t *testing.T) {
			rec := httptest.NewRecorder()
			srv.handleAutoInstallScript(rec, httptest.NewRequest(http.MethodGet, "/autoinstall/Win11.iso?mac="+installTestMAC+tc.query, nil))
			if rec.Code != tc.status {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
			if tc.status == http.StatusOK {
				for _, want := range []string{tc.want, "bench-1", installTestMAC, "/install/callback?token="} {
					if !strings.Contains(rec.Body.String(), want) {
						t.Errorf("missing %q in %s", want, rec.Body.String())
					}
				}
			}
		})
	}
	if err := lib.Delete("windows", "kiosk.xml"); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	srv.handleAutoInstallScript(rec, httptest.NewRequest(http.MethodGet, "/autoinstall/Win11.iso?mac="+installTestMAC+"&choice="+autoInstallChoiceID("windows/kiosk.xml"), nil))
	if rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "default.xml") {
		t.Fatalf("missing file fell back: %d %s", rec.Code, rec.Body.String())
	}
	image.AutoInstallChoices = nil
	if err := store.UpdateImage(image.Filename, image); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	srv.handleAutoInstallScript(rec, httptest.NewRequest(http.MethodGet, "/autoinstall/Win11.iso?choice=default", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("disabled choice accepted: %d", rec.Code)
	}
}

func TestAutoInstallChoicesDoNotChangeDefaults(t *testing.T) {
	srv, store := newInstallEnv(t)
	lib, err := autoinstall.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv.autoInstallLib = lib
	for _, name := range []string{"default.xml", "one.xml", "two.xml"} {
		if err := lib.Write("windows", name, "<unattend>"+name+"</unattend>"); err != nil {
			t.Fatal(err)
		}
	}
	image := &models.Image{Filename: "Win11.iso", BootMethod: "kernel", Distro: "windows", AutoInstallFile: "windows/default.xml", AutoInstallChoices: models.StringSlice{"windows/one.xml", "windows/two.xml"}}
	if err := store.CreateImage(image); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for _, path := range image.AutoInstallChoices {
		wg.Go(func() {
			rec := httptest.NewRecorder()
			srv.handleAutoInstallScript(rec, httptest.NewRequest(http.MethodGet, "/autoinstall/Win11.iso?choice="+autoInstallChoiceID(path), nil))
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), strings.TrimPrefix(path, "windows/")) {
				t.Errorf("selection %s: %d %s", path, rec.Code, rec.Body.String())
			}
		})
	}
	wg.Wait()
	got, err := store.GetImage(image.Filename)
	if err != nil {
		t.Fatal(err)
	}
	if got.AutoInstallFile != image.AutoInstallFile || !slices.Equal(got.AutoInstallChoices, image.AutoInstallChoices) {
		t.Fatalf("boot changed stored image: %+v", got)
	}
}

func TestAutoInstallChoicesMenuAndPasswordGate(t *testing.T) {
	mb := testMenuBuilder(map[uint]string{9: autoinstall.TypeAutounattend})
	mb.images = []models.Image{{ID: 9, Filename: "Win11.iso", Name: "Windows", BootMethod: "kernel", Distro: "windows", Enabled: true}}
	mb.autoInstallChoices = map[uint][]string{9: {"windows/kiosk.xml", "windows/lab ${name}\n&& reboot.xml"}}
	mb.nextBootImageID = 9
	out := mb.Build()
	for _, want := range []string{"goto iso9\n", "menu --name bootimus_answers Windows answer file: Windows || goto failed\n", "item --menu bootimus_answers default Use configured default || goto failed\n", "choose --menu bootimus_answers bootimus_answer || goto start\n", "iseq ${bootimus_answer} back && goto start ||\n", "&choice=${bootimus_answer} AutoUnattend.xml || goto failed\n", "imgfree\nkernel"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %s", want, out)
		}
	}
	if strings.Contains(out, "choose --menu bootimus_answers --timeout") || strings.Contains(out, "${name}") || strings.Contains(out, "\n&& reboot") {
		t.Fatalf("unsafe menu: %s", out)
	}
	mb.images[0].BootPasswordRequired = true
	out = mb.Build()
	if strings.Contains(out, "bootimus_answers") || strings.Contains(out, "AutoUnattend.xml") {
		t.Fatal("answer files available before the image password gate")
	}
	mb.bootAccess.imageID = 9
	out = mb.Build()
	if !strings.Contains(out, "bootimus_answers") || !strings.Contains(out, ":start\nexit\n") {
		t.Fatal("unlocked fragment lacks chooser or cancellation target")
	}
	mb.bootAccess.imageID = 0
	mb.images[0].BootPasswordRequired = false
	mb.autoInstallChoices = nil
	if strings.Contains(mb.Build(), "bootimus_answers") {
		t.Fatal("existing automatic boot gained a chooser")
	}
}
