package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"bootimus/internal/models"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const bootSessionLifetime = 5 * time.Minute

type bootMenuAccess struct {
	global  bool
	session string
	imageID uint
}

type bootAttemptWindow struct {
	count int
	until time.Time
}

type bootAuthentication struct {
	once     sync.Once
	key      [32]byte
	keyError error
	mu       sync.Mutex
	attempts map[string]bootAttemptWindow
	workers  chan struct{}
	now      func() time.Time
}

func (authentication *bootAuthentication) initialise() {
	authentication.once.Do(func() {
		_, authentication.keyError = rand.Read(authentication.key[:])
		authentication.attempts = make(map[string]bootAttemptWindow)
		authentication.workers = make(chan struct{}, 4)
		if authentication.now == nil {
			authentication.now = time.Now
		}
	})
}

func (authentication *bootAuthentication) signature(mac, address, hash, expiry string) string {
	signer := hmac.New(sha256.New, authentication.key[:])
	fmt.Fprintf(signer, "%s\x00%s\x00%s\x00%s", mac, address, hash, expiry)
	return hex.EncodeToString(signer.Sum(nil))
}

func (authentication *bootAuthentication) session(mac, address, hash string) string {
	authentication.initialise()
	if authentication.keyError != nil {
		return ""
	}
	expiry := strconv.FormatInt(authentication.now().Add(bootSessionLifetime).Unix(), 10)
	return expiry + "." + authentication.signature(mac, address, hash, expiry)
}

func (authentication *bootAuthentication) accepts(token, mac, address, hash string) bool {
	authentication.initialise()
	if authentication.keyError != nil || hash == "" {
		return false
	}
	expiry, signature, found := strings.Cut(token, ".")
	if !found || len(signature) != 64 {
		return false
	}
	seconds, err := strconv.ParseInt(expiry, 10, 64)
	if err != nil || seconds <= authentication.now().Unix() || seconds > authentication.now().Add(bootSessionLifetime).Unix() {
		return false
	}
	return hmac.Equal([]byte(signature), []byte(authentication.signature(mac, address, hash, expiry)))
}

func (authentication *bootAuthentication) attempt(address string) bool {
	authentication.initialise()
	authentication.mu.Lock()
	defer authentication.mu.Unlock()
	now := authentication.now()
	for key, window := range authentication.attempts {
		if !window.until.After(now) {
			delete(authentication.attempts, key)
		}
	}
	window, exists := authentication.attempts[address]
	if !exists {
		if len(authentication.attempts) >= 4096 {
			return false
		}
		window.until = now.Add(time.Minute)
	}
	if window.count >= 5 {
		return false
	}
	window.count++
	authentication.attempts[address] = window
	return true
}

func (server *Server) checkBootPassword(writer http.ResponseWriter, request *http.Request, hash string) bool {
	if !server.bootAuth.attempt(hostOf(request.RemoteAddr)) {
		writer.Header().Set("Retry-After", "60")
		http.Error(writer, "Too many authentication attempts; wait a minute", http.StatusTooManyRequests)
		return false
	}
	select {
	case server.bootAuth.workers <- struct{}{}:
		defer func() { <-server.bootAuth.workers }()
	default:
		http.Error(writer, "Boot authentication is busy", http.StatusServiceUnavailable)
		return false
	}
	password := request.PostForm.Get("password")
	if password == "" || len(password) > 72 || strings.ContainsAny(password, "+; ") || bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		http.Error(writer, "Boot authentication failed", http.StatusForbidden)
		return false
	}
	return true
}

func bootMAC(value string) (string, bool) {
	value = normaliseMenuMAC(value)
	if value == "unknown" {
		return value, true
	}
	address, err := net.ParseMAC(value)
	if err != nil || len(address) != 6 {
		return "", false
	}
	return address.String(), true
}

func (server *Server) bootAuthenticationURL() string {
	if !server.AdminTLSEnabled() || server.config.ServerAddr == "" || server.config.AdminPort == 0 {
		return ""
	}
	endpoint := url.URL{Scheme: "https", Host: net.JoinHostPort(strings.Trim(server.config.ServerAddr, "[]"), strconv.Itoa(server.config.AdminPort)), Path: "/boot-auth"}
	return endpoint.String()
}

const bootPasswordLocal = `:local
clear bootimus_password
clear bootimus_session
echo Booting from local disk...
set localboot 1
iseq ${platform} pcbios && sanboot --no-describe --drive 0x80 ||
exit
`

func bootProtectionUnavailable() string {
	return "#!ipxe\necho Protected boot is unavailable. Check HTTPS, certificates and bootloader support.\n:choice\nmenu Bootimus\nitem local Boot from Local Disk\nchoose selected || goto choice\ngoto ${selected}\n" + bootPasswordLocal
}

func (server *Server) bootPasswordMenu(mac string) string {
	mac, valid := bootMAC(mac)
	endpoint := server.bootAuthenticationURL()
	if !valid || endpoint == "" {
		return bootProtectionUnavailable()
	}
	return fmt.Sprintf(`#!ipxe
clear bootimus_password
clear bootimus_session
:start
menu Bootimus - Password required
item unlock Enter network-boot menu
item local Boot from Local Disk
choose selected || goto start
goto ${selected}
:unlock
form Boot menu password || goto rejected
item --secret bootimus_password Password: || goto rejected
present || goto cancelled
params --name bootimus || goto rejected
param --params bootimus action menu || goto rejected
param --params bootimus mac %s || goto rejected
param --params bootimus password ${bootimus_password} || goto rejected
clear bootimus_password
clear username
clear password
chain --autofree %s##params=bootimus || goto rejected
isset ${localboot} && exit ||
goto start
:rejected
echo Authentication failed or unavailable. Check the password, HTTPS trust and bootloader.
sleep 1
:cancelled
clear bootimus_password
params --name bootimus --delete ||
goto start
%s`, mac, endpoint, bootPasswordLocal)
}

func (builder *MenuBuilder) buildImagePasswordGate(imageID uint) string {
	mac, valid := bootMAC(builder.macAddress)
	if !valid || builder.bootAuthURL == "" {
		return "echo This image needs HTTPS password authentication and a compatible bootloader.\nsleep 2\ngoto start\n"
	}
	return fmt.Sprintf(`params --name bootimus || goto failed
param --params bootimus action image || goto failed
param --params bootimus mac %s || goto failed
param --params bootimus image %d || goto failed
param --params bootimus session ${bootimus_session} || goto failed
clear username
clear password
chain --autofree %s##params=bootimus || goto failed
isset ${localboot} && exit ||
goto start
`, mac, imageID, builder.bootAuthURL)
}

func (server *Server) imagePasswordForm(mac string, imageID uint) string {
	return fmt.Sprintf(`#!ipxe
clear bootimus_password
form Image password || goto rejected
item --secret bootimus_password Password: || goto rejected
present || goto cancelled
params --name bootimus || goto rejected
param --params bootimus action image || goto rejected
param --params bootimus mac %s || goto rejected
param --params bootimus image %d || goto rejected
param --params bootimus session ${bootimus_session} || goto rejected
param --params bootimus password ${bootimus_password} || goto rejected
clear bootimus_password
clear username
clear password
chain --autofree %s##params=bootimus || goto rejected
exit
:rejected
echo Authentication failed or unavailable. Return to the menu and try again.
sleep 1
:cancelled
clear bootimus_password
params --name bootimus --delete ||
exit
`, mac, imageID, server.bootAuthenticationURL())
}

func (server *Server) bootImageForClient(mac string, imageID uint) (*models.Image, error) {
	images, err := server.config.Storage.GetImagesForClient(mac)
	if err != nil {
		return nil, err
	}
	client, err := server.config.Storage.GetClient(mac)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if client != nil {
		effective, group := server.effectiveFor(client)
		images = server.applyProfileImages(images, client, group, effective)
	}
	for _, image := range images {
		if image.ID == imageID && image.Enabled {
			return &image, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (server *Server) handleBootAuthentication(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Referrer-Policy", "no-referrer")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	if request.TLS == nil || server.bootAuthenticationURL() == "" || server.config.Storage == nil {
		http.Error(writer, "Boot authentication requires HTTPS", http.StatusServiceUnavailable)
		return
	}
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		http.Error(writer, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	contentType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || contentType != "application/x-www-form-urlencoded" || request.URL.RawQuery != "" {
		http.Error(writer, "Use form fields in the POST body", http.StatusBadRequest)
		return
	}
	request.Body = http.MaxBytesReader(writer, request.Body, 4096)
	if err := request.ParseForm(); err != nil {
		http.Error(writer, "Invalid authentication request", http.StatusBadRequest)
		return
	}
	mac, valid := bootMAC(request.PostForm.Get("mac"))
	if !valid {
		http.Error(writer, "Invalid client address", http.StatusBadRequest)
		return
	}
	theme, err := server.config.Storage.GetMenuTheme()
	if err != nil {
		http.Error(writer, "Boot protection is unavailable", http.StatusServiceUnavailable)
		return
	}
	globalRequired := theme.BootPasswordRequired || theme.BootPasswordHash != ""
	if globalRequired && theme.BootPasswordHash == "" {
		http.Error(writer, "Boot password verification is unavailable on this server", http.StatusServiceUnavailable)
		return
	}
	address := hostOf(request.RemoteAddr)
	session := request.PostForm.Get("session")
	globalAccepted := !globalRequired || server.bootAuth.accepts(session, mac, address, theme.BootPasswordHash)
	writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	switch request.PostForm.Get("action") {
	case "menu":
		if !globalAccepted && !server.checkBootPassword(writer, request, theme.BootPasswordHash) {
			return
		}
		if globalRequired {
			session = server.bootAuth.session(mac, address, theme.BootPasswordHash)
			if session == "" {
				http.Error(writer, "Boot protection is unavailable", http.StatusServiceUnavailable)
				return
			}
		} else {
			session = ""
		}
		rendered := server.renderBootMenuWithAccess(mac, request.RemoteAddr, true, bootMenuAccess{global: true, session: session})
		fmt.Fprint(writer, rendered.Script)
	case "image":
		if !globalAccepted {
			http.Error(writer, "Unlock the boot menu first", http.StatusForbidden)
			return
		}
		identifier, err := strconv.ParseUint(request.PostForm.Get("image"), 10, 32)
		if err != nil || identifier == 0 {
			http.Error(writer, "Invalid image", http.StatusBadRequest)
			return
		}
		image, err := server.bootImageForClient(mac, uint(identifier))
		if err != nil {
			http.Error(writer, "Image unavailable", http.StatusForbidden)
			return
		}
		if image.BootPasswordRequired || image.BootPasswordHash != "" {
			if image.BootPasswordHash == "" {
				http.Error(writer, "Image password verification is unavailable on this server", http.StatusServiceUnavailable)
				return
			}
			if _, supplied := request.PostForm["password"]; !supplied {
				fmt.Fprint(writer, server.imagePasswordForm(mac, image.ID))
				return
			}
			if !server.checkBootPassword(writer, request, image.BootPasswordHash) {
				return
			}
		}
		if client, err := server.config.Storage.GetClient(mac); err == nil && client.NextBootImage == image.Filename {
			if err := server.config.Storage.ClearNextBootImage(mac); err != nil {
				http.Error(writer, "Unable to confirm next boot", http.StatusServiceUnavailable)
				return
			}
		}
		script := server.generateIPXEMenuWithAccess([]models.Image{*image}, mac, image.ID, 0, bootMenuAccess{global: true, session: session, imageID: image.ID})
		fmt.Fprint(writer, script)
	default:
		http.Error(writer, "Invalid authentication action", http.StatusBadRequest)
	}
}
