package server

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"bootimus/internal/models"
)

func (s *Server) handleBootEvent(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	if mac, valid := bootMAC(query.Get("mac")); valid && mac != "unknown" && s.config.Storage != nil {
		if name, image := s.bootEventTarget(mac, query); name != "" {
			s.recordBoot(mac, name, image, r.RemoteAddr)
		}
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprint(w, "#!ipxe\n")
}

func (s *Server) bootEventTarget(mac string, query url.Values) (string, *models.Image) {
	switch {
	case query.Has("image"):
		id, err := strconv.ParseUint(query.Get("image"), 10, 32)
		if err != nil {
			return "", nil
		}
		image, err := s.bootImageForClient(mac, uint(id))
		if err != nil {
			return "", nil
		}
		return image.Name, image
	case query.Has("tool"):
		if s.toolsManager == nil {
			return "", nil
		}
		for _, tool := range s.toolsManager.GetEnabledTools("") {
			if tool.Name == query.Get("tool") {
				return tool.DisplayName, nil
			}
		}
	case query.Has("san"):
		id, err := strconv.ParseUint(query.Get("san"), 10, 32)
		if err != nil {
			return "", nil
		}
		target, err := s.config.Storage.GetSANTarget(uint(id))
		if err != nil {
			return "", nil
		}
		return "SAN target " + target.Name, nil
	case query.Has("diskimage"):
		if s.diskImages == nil {
			return "", nil
		}
		if job := s.diskImages.ActiveJob(mac); job != nil {
			return "Disk " + job.Kind, nil
		}
	}
	return "", nil
}
