package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	BootAttempts = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "bootimus_boot_attempts_total",
			Help: "Number of recorded boot attempts, labelled by image.",
		},
		[]string{"image"},
	)

	TFTPRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "bootimus_tftp_requests_total",
			Help: "TFTP file requests, labelled by filename.",
		},
		[]string{"file"},
	)

	TFTPBytes = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "bootimus_tftp_bytes_total",
			Help: "Total bytes served over TFTP.",
		},
	)

	HTTPBootRequests = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "bootimus_http_boot_requests_total",
			Help: "HTTP boot-file requests (kernel, initrd, squashfs, etc.).",
		},
	)

	ProxyDHCPOffers = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "bootimus_proxy_dhcp_offers_total",
			Help: "proxyDHCP offers sent, labelled by client architecture.",
		},
		[]string{"arch"},
	)

	ActiveSessions = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "bootimus_active_sessions",
			Help: "Current number of active TFTP/HTTP boot sessions.",
		},
	)

	ClientsTotal = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "bootimus_clients_total",
			Help: "Number of registered clients (static + discovered).",
		},
	)

	ImagesTotal = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "bootimus_images_total",
			Help: "Number of images known to bootimus.",
		},
	)
)
