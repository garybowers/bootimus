package raspberrypi

import (
	"embed"
	"net"
	"regexp"
	"strings"
)

//go:embed all:assets
var assets embed.FS

var serialPrefix = regexp.MustCompile(`^[0-9a-fA-F]{8}/`)

var ouiPrefixes = []string{
	"b8:27:eb",
	"dc:a6:32",
	"e4:5f:01",
	"d8:3a:dd",
	"2c:cf:67",
}

func StripSerialPrefix(path string) string {
	return serialPrefix.ReplaceAllString(path, "")
}

func Resolve(path string) ([]byte, bool) {
	name := StripSerialPrefix(strings.TrimPrefix(path, "/"))
	if name == "" || strings.Contains(name, "..") {
		return nil, false
	}
	data, err := assets.ReadFile("assets/" + name)
	if err != nil {
		return nil, false
	}
	return data, true
}

func IsRaspberryPiMAC(mac net.HardwareAddr) bool {
	if len(mac) < 3 {
		return false
	}
	prefix := mac.String()[:8]
	for _, oui := range ouiPrefixes {
		if prefix == oui {
			return true
		}
	}
	return false
}
