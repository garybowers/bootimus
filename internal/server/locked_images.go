package server

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"bootimus/internal/models"
)

func (server *Server) lockedMenuImages(mac string) (string, string, error) {
	images, err := server.bootImagesForClient(mac)
	if err != nil {
		return "", "", err
	}
	sort.SliceStable(images, func(first, second int) bool {
		if images[first].Order != images[second].Order {
			return images[first].Order < images[second].Order
		}
		return images[first].Name < images[second].Name
	})
	var items, sections strings.Builder
	for _, image := range images {
		if !image.Enabled || !image.BootMenuExempt {
			continue
		}
		name := strings.Map(func(character rune) rune {
			if unicode.IsControl(character) || strings.ContainsRune("$&|", character) {
				return ' '
			}
			return character
		}, image.Name)
		if image.BootPasswordRequired || image.BootPasswordHash != "" {
			name += " (image password)"
		}
		fmt.Fprintf(&items, "item public%d %s\n", image.ID, name)
		endpoint := url.URL{Scheme: "http", Host: net.JoinHostPort(strings.Trim(server.config.ServerAddr, "[]"), strconv.Itoa(server.config.HTTPPort)), Path: "/boot-menu-image"}
		endpoint.RawQuery = url.Values{"mac": {mac}, "image": {strconv.FormatUint(uint64(image.ID), 10)}}.Encode()
		fmt.Fprintf(&sections, ":public%d\nchain --autofree %s || goto image_failed\nisset ${localboot} && exit ||\ngoto start\n", image.ID, endpoint.String())
	}
	if items.Len() != 0 {
		sections.WriteString(":image_failed\necho Image unavailable. Return to the locked menu.\nsleep 1\ngoto start\n")
	}
	return items.String(), sections.String(), nil
}

func (server *Server) handleBootMenuImage(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		http.Error(writer, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	mac, valid := bootMAC(request.URL.Query().Get("mac"))
	identifier, err := strconv.ParseUint(request.URL.Query().Get("image"), 10, 32)
	if !valid || err != nil || identifier == 0 {
		http.Error(writer, "Invalid image selection", http.StatusBadRequest)
		return
	}
	if server.config.Storage == nil {
		http.Error(writer, "Image unavailable", http.StatusServiceUnavailable)
		return
	}
	image, err := server.bootImageForClient(mac, uint(identifier))
	if err != nil || !image.BootMenuExempt {
		http.Error(writer, "Image unavailable", http.StatusForbidden)
		return
	}
	if image.BootPasswordRequired || image.BootPasswordHash != "" {
		if image.BootPasswordHash == "" || server.bootAuthenticationURL() == "" {
			http.Error(writer, "Image password verification is unavailable", http.StatusServiceUnavailable)
			return
		}
		fmt.Fprint(writer, server.imagePasswordForm(mac, image.ID))
		return
	}
	server.serveBootMenuImage(writer, mac, image, bootMenuAccess{imageID: image.ID})
}

func (server *Server) serveBootMenuImage(writer http.ResponseWriter, mac string, image *models.Image, access bootMenuAccess) {
	if client, err := server.config.Storage.GetClient(mac); err == nil && client.NextBootImage == image.Filename {
		if err := server.config.Storage.ClearNextBootImage(mac); err != nil {
			http.Error(writer, "Unable to confirm next boot", http.StatusServiceUnavailable)
			return
		}
	}
	fmt.Fprint(writer, server.generateIPXEMenuWithAccess([]models.Image{*image}, mac, image.ID, 0, access))
}
