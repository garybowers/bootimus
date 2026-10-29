package admin

import (
	"encoding/json"
	"net/http"
	"strings"
)

type APIParam struct {
	Name        string
	In          string
	Description string
	Required    bool
}

type APIEndpoint struct {
	Method      string
	Path        string
	Summary     string
	Params      []APIParam
	Body        string
	Multipart   bool
	FormEncoded bool
	Public      bool
	ReadOnly    bool
}

type APISection struct {
	Tag         string
	Description string
	Endpoints   []APIEndpoint
}

func qMAC(desc string) APIParam {
	return APIParam{Name: "mac", In: "query", Description: desc, Required: true}
}

func qFilename(desc string) APIParam {
	return APIParam{Name: "filename", In: "query", Description: desc, Required: true}
}

func qID(desc string) APIParam {
	return APIParam{Name: "id", In: "query", Description: desc, Required: true}
}

func qName(desc string) APIParam {
	return APIParam{Name: "name", In: "query", Description: desc, Required: true}
}

func pathParam(n, desc string) APIParam {
	return APIParam{Name: n, In: "path", Description: desc, Required: true}
}

var APISections = []APISection{
	{Tag: "Authentication", Endpoints: []APIEndpoint{
		{Method: "POST", Path: "/api/login", Summary: "Obtain a JWT token. Answers 401 with totp_required when the account needs a second factor; resend with code.", Body: "{username, password, auth_method, code}", Public: true},
		{Method: "GET", Path: "/api/auth-info", Summary: "Available authentication backends.", Public: true},
		{Method: "GET", Path: "/api/me", Summary: "The current user's name, admin flag and role.", Public: true},
		{Method: "GET", Path: "/api/oidc/login", Summary: "Start the OpenID Connect sign-in redirect.", Public: true},
		{Method: "GET", Path: "/api/oidc/callback", Summary: "OpenID Connect provider callback.", Public: true},
		{Method: "GET", Path: "/logout", Summary: "Redirect to the login page.", Public: true},
		{Method: "GET", Path: "/health", Summary: "Liveness probe.", Public: true},
		{Method: "GET", Path: "/api/openapi.json", Summary: "This OpenAPI document.", Public: true},
		{Method: "POST", Path: "/api/me/totp/setup", Summary: "Start two-factor enrolment for the signed-in local user; returns the otpauth URL, secret and a QR code.", ReadOnly: true},
		{Method: "POST", Path: "/api/me/totp/enable", Summary: "Confirm enrolment with a code from the authenticator app; returns recovery codes once.", Body: "{code}", ReadOnly: true},
		{Method: "POST", Path: "/api/me/totp/disable", Summary: "Turn off two-factor authentication for the signed-in user.", Body: "{password}", ReadOnly: true},
	}},
	{Tag: "Server", Description: "Server information and statistics.", Endpoints: []APIEndpoint{
		{Method: "GET", Path: "/api/server-info", Summary: "Version, uptime, paths and network information.", ReadOnly: true},
		{Method: "GET", Path: "/api/stats", Summary: "Counts of clients, images and boots.", ReadOnly: true},
		{Method: "GET", Path: "/api/active-sessions", Summary: "Currently active boot sessions.", ReadOnly: true},
		{Method: "GET", Path: "/metrics", Summary: "Prometheus metrics.", Public: true},
	}},
	{Tag: "Clients", Endpoints: []APIEndpoint{
		{Method: "GET", Path: "/api/clients", Summary: "List all clients, or one when mac is given.", Params: []APIParam{{Name: "mac", In: "query", Description: "MAC address of a single client"}}, ReadOnly: true},
		{Method: "POST", Path: "/api/clients", Summary: "Create a static client.", Body: "{mac_address, name, description, enabled, ...}"},
		{Method: "PUT", Path: "/api/clients", Summary: "Partial update; any client field is accepted.", Params: []APIParam{qMAC("MAC address of the client")}, Body: "{name, description, enabled, ...}"},
		{Method: "DELETE", Path: "/api/clients", Summary: "Delete a client.", Params: []APIParam{qMAC("MAC address of the client")}},
		{Method: "POST", Path: "/api/clients/wake", Summary: "Send a Wake-on-LAN packet.", Params: []APIParam{qMAC("MAC address of the client")}},
		{Method: "POST", Path: "/api/clients/next-boot", Summary: "Set a one-shot next-boot image, or san:<id> to boot from a SAN target once.", Params: []APIParam{qMAC("MAC address of the client")}, Body: "{filename}"},
		{Method: "POST", Path: "/api/clients/promote", Summary: "Promote a discovered client to static.", Params: []APIParam{qMAC("MAC address of the client")}},
		{Method: "GET", Path: "/api/clients/inventory", Summary: "Latest hardware inventory.", Params: []APIParam{qMAC("MAC address of the client")}, ReadOnly: true},
		{Method: "GET", Path: "/api/clients/inventory/history", Summary: "Historical inventory submissions.", Params: []APIParam{qMAC("MAC address of the client")}, ReadOnly: true},
		{Method: "GET", Path: "/api/clients/installs", Summary: "Unattended install history for a client, newest first.", Params: []APIParam{qMAC("MAC address of the client"), {Name: "limit", In: "query", Description: "Maximum entries, default 50"}}, ReadOnly: true},
		{Method: "GET", Path: "/api/clients/timeline", Summary: "Merged activity timeline for a client: boots, installs, disk imaging jobs, proxyDHCP requests and, for administrators, audit entries.", Params: []APIParam{qMAC("MAC address of the client"), {Name: "limit", In: "query", Description: "Page size, default 50, maximum 500"}, {Name: "before", In: "query", Description: "Opaque cursor from the previous page's next_before; RFC 3339 timestamps are also accepted as exclusive time bounds"}}, ReadOnly: true},
		{Method: "GET", Path: "/api/installs", Summary: "Recent unattended installs across all clients, newest first.", Params: []APIParam{{Name: "limit", In: "query", Description: "Maximum entries, default 100"}}, ReadOnly: true},
		{Method: "GET", Path: "/api/clients/effective", Summary: "Settings in force for a client after deployment profile and group precedence, with the source of each value.", Params: []APIParam{qMAC("MAC address of the client")}, ReadOnly: true},
		{Method: "GET", Path: "/api/clients/menu-preview", Summary: "Render the iPXE script a client would receive, without side effects.", Params: []APIParam{{Name: "mac", In: "query", Description: "MAC address of the client; omit or use unknown for the anonymous menu"}}, ReadOnly: true},
		{Method: "POST", Path: "/api/clients/power", Summary: "BMC (Redfish or Intel AMT) power control.", Params: []APIParam{qMAC("MAC address of the client"), {Name: "action", In: "query", Description: "On, ForceOff, ForceRestart, GracefulShutdown or GracefulRestart", Required: true}}},
		{Method: "GET", Path: "/api/clients/power/status", Summary: "BMC (Redfish or Intel AMT) power status.", Params: []APIParam{qMAC("MAC address of the client")}, ReadOnly: true},
		{Method: "GET", Path: "/api/clients/console", Summary: "WebSocket: interactive serial console over Intel AMT Serial-over-LAN or IPMI SOL. Pass the JWT as the token query parameter or as the second Sec-WebSocket-Protocol entry after bearer.", Params: []APIParam{qMAC("MAC address of the client"), {Name: "token", In: "query", Description: "JWT, for clients that cannot set the Sec-WebSocket-Protocol header"}}},
		{Method: "POST", Path: "/api/clients/import", Summary: "Import clients from a CSV file.", Body: "file", Multipart: true},
	}},
	{Tag: "SAN Targets", Description: "iSCSI, AoE and HTTP SAN boot targets that clients can boot from with iPXE sanboot.", Endpoints: []APIEndpoint{
		{Method: "GET", Path: "/api/san-targets", Summary: "List SAN targets with the number of clients and groups using each.", ReadOnly: true},
		{Method: "POST", Path: "/api/san-targets", Summary: "Create a SAN target.", Body: "{name, description, uri, initiator_iqn, chap_username, chap_password, reverse_chap_username, reverse_chap_password}"},
		{Method: "PUT", Path: "/api/san-targets", Summary: "Update a SAN target.", Params: []APIParam{qID("SAN target ID")}, Body: "{name, description, uri, initiator_iqn, chap_username, chap_password, reverse_chap_username, reverse_chap_password}"},
		{Method: "DELETE", Path: "/api/san-targets", Summary: "Delete a SAN target and clear it from clients and groups.", Params: []APIParam{qID("SAN target ID")}},
	}},
	{Tag: "Deployment Profiles", Description: "Named bundles of image, auto-install file, SAN target, bootloader set and first-boot action, resolved at boot time for the clients and groups they are assigned to.", Endpoints: []APIEndpoint{
		{Method: "GET", Path: "/api/deployment-profiles", Summary: "List deployment profiles with the number of clients and groups using each.", ReadOnly: true},
		{Method: "POST", Path: "/api/deployment-profiles", Summary: "Create a deployment profile.", Body: "{name, description, image_filename, auto_install_file, san_target_id, boot_from_san, bootloader_set, disk_image_id, first_boot_action, show_public_images}"},
		{Method: "PUT", Path: "/api/deployment-profiles", Summary: "Update a deployment profile.", Params: []APIParam{qID("Deployment profile ID")}, Body: "{name, description, image_filename, auto_install_file, san_target_id, boot_from_san, bootloader_set, disk_image_id, first_boot_action, show_public_images}"},
		{Method: "DELETE", Path: "/api/deployment-profiles", Summary: "Delete a deployment profile and clear it from clients and groups.", Params: []APIParam{qID("Deployment profile ID")}},
	}},
	{Tag: "Enrolment Rules", Description: "Rules that place newly discovered clients into a group and deployment profile from their MAC, inventory and firmware.", Endpoints: []APIEndpoint{
		{Method: "GET", Path: "/api/enrolment-rules", Summary: "List enrolment rules in priority order with match counts.", ReadOnly: true},
		{Method: "POST", Path: "/api/enrolment-rules", Summary: "Create an enrolment rule.", Body: "{name, priority, enabled, mac_prefix, manufacturer_contains, product_contains, serial_pattern, uuid_prefix, firmware, client_group_id, profile_id, make_static, name_pattern}"},
		{Method: "PUT", Path: "/api/enrolment-rules", Summary: "Update an enrolment rule.", Params: []APIParam{qID("Rule ID")}, Body: "{name, priority, enabled, mac_prefix, manufacturer_contains, product_contains, serial_pattern, uuid_prefix, firmware, client_group_id, profile_id, make_static, name_pattern}"},
		{Method: "DELETE", Path: "/api/enrolment-rules", Summary: "Delete an enrolment rule.", Params: []APIParam{qID("Rule ID")}},
		{Method: "POST", Path: "/api/enrolment-rules/test", Summary: "Dry run: which existing clients a rule would match, without applying it.", Body: "{mac_prefix, manufacturer_contains, product_contains, serial_pattern, uuid_prefix, firmware, name_pattern, ...}"},
		{Method: "POST", Path: "/api/enrolment-rules/apply", Summary: "Run every enabled rule over discovered clients that have no group, profile or rule yet."},
	}},
	{Tag: "Client Groups", Endpoints: []APIEndpoint{
		{Method: "GET", Path: "/api/client-groups", Summary: "List all client groups.", ReadOnly: true},
		{Method: "POST", Path: "/api/client-groups", Summary: "Create a client group.", Body: "{name, description, ...}"},
		{Method: "GET", Path: "/api/client-groups/get", Summary: "Get one client group.", Params: []APIParam{qID("Group ID")}, ReadOnly: true},
		{Method: "PUT", Path: "/api/client-groups/update", Summary: "Update a client group.", Params: []APIParam{qID("Group ID")}, Body: "{name, description, ...}"},
		{Method: "DELETE", Path: "/api/client-groups/delete", Summary: "Delete a client group.", Params: []APIParam{qID("Group ID")}},
		{Method: "POST", Path: "/api/client-groups/membership", Summary: "Set the members of a client group.", Body: "{group_id, client_macs[]}"},
		{Method: "POST", Path: "/api/client-groups/wake", Summary: "Wake all members of a group.", Params: []APIParam{qID("Group ID")}},
		{Method: "POST", Path: "/api/client-groups/next-boot", Summary: "Set a next-boot image on all members.", Params: []APIParam{qID("Group ID")}, Body: "{filename}"},
		{Method: "POST", Path: "/api/client-groups/power", Summary: "Power control for all members.", Params: []APIParam{qID("Group ID")}, Body: "{action}"},
	}},
	{Tag: "Images", Endpoints: []APIEndpoint{
		{Method: "GET", Path: "/api/images", Summary: "List all images, or one when filename is given.", Params: []APIParam{{Name: "filename", In: "query", Description: "ISO filename of a single image"}}, ReadOnly: true},
		{Method: "PUT", Path: "/api/images", Summary: "Partial update of an image. Omit boot_password to preserve it; an empty string removes it. Setting a password requires HTTPS. Passwords must be at most 72 bytes and cannot contain +, ; or ASCII spaces.", Params: []APIParam{qFilename("ISO filename")}, Body: "{name, description, enabled, public, group_id, order, boot_method, distro, boot_params, auto_install_file, kernel_override, initrd_override, boot_password?}"},
		{Method: "DELETE", Path: "/api/images", Summary: "Delete an image.", Params: []APIParam{qFilename("ISO filename"), {Name: "delete_file", In: "query", Description: "true to also remove the ISO from disk"}}},
		{Method: "POST", Path: "/api/images/upload", Summary: "Upload an ISO or unified kernel image.", Body: "file, public, description", Multipart: true},
		{Method: "POST", Path: "/api/images/download", Summary: "Download an ISO from a URL asynchronously, verifying it against a SHA-256 when one is given or published alongside.", Body: "{url, filename, description, sha256, checksum_url, catalogue_id, catalogue_release}"},
		{Method: "POST", Path: "/api/images/verify", Summary: "Hash an image on disk against a checksum in the background.", Params: []APIParam{qFilename("ISO filename")}, Body: "{sha256, checksum_url}"},
		{Method: "GET", Path: "/api/images/verify", Summary: "Progress and result of a running verification.", Params: []APIParam{qFilename("ISO filename")}},
		{Method: "POST", Path: "/api/images/extract", Summary: "Extract the kernel and initrd from an ISO.", Params: []APIParam{qFilename("ISO filename")}},
		{Method: "GET", Path: "/api/images/extract-progress", Summary: "Extraction progress.", Params: []APIParam{qFilename("ISO filename")}, ReadOnly: true},
		{Method: "POST", Path: "/api/images/redetect", Summary: "Re-run distribution detection and boot parameter resolution.", Params: []APIParam{qFilename("ISO filename")}},
		{Method: "GET", Path: "/api/images/boot-candidates", Summary: "Kernel and initrd files found in the extracted ISO.", Params: []APIParam{qFilename("ISO filename")}, ReadOnly: true},
		{Method: "GET", Path: "/api/images/files", Summary: "List the extracted files of an image.", Params: []APIParam{qFilename("ISO filename")}, ReadOnly: true},
		{Method: "POST", Path: "/api/images/files/delete", Summary: "Delete the extracted files of an image.", Params: []APIParam{qFilename("ISO filename")}},
		{Method: "POST", Path: "/api/images/patch-smb", Summary: "Patch boot.wim for a Windows SMB install.", Params: []APIParam{qFilename("ISO filename")}},
		{Method: "POST", Path: "/api/images/unpatch-smb", Summary: "Restore the original boot.wim, removing the SMB patch.", Params: []APIParam{qFilename("ISO filename")}},
		{Method: "POST", Path: "/api/images/boot-method", Summary: "Set the boot method.", Params: []APIParam{qFilename("ISO filename")}, Body: "{method} (sanboot, kernel, nbd or nfs)"},
		{Method: "PUT", Path: "/api/images/winpe", Summary: "Choose which WIM file and image index a Windows or WinPE image boots.", Params: []APIParam{qFilename("ISO filename")}, Body: "{wim_path, wim_index}"},
		{Method: "POST", Path: "/api/images/netboot/download", Summary: "Fetch the netboot kernel and initrd from the distribution mirror.", Params: []APIParam{qFilename("ISO filename")}},
		{Method: "GET", Path: "/api/images/autoinstall", Summary: "Get the auto-install script for an image.", Params: []APIParam{qFilename("ISO filename")}},
		{Method: "POST", Path: "/api/images/autoinstall", Summary: "Set the auto-install script for an image.", Params: []APIParam{qFilename("ISO filename")}, Body: "{script, type, enabled}"},
		{Method: "POST", Path: "/api/assign-images", Summary: "Assign images to a client.", Body: "{mac_address, image_filenames[]}"},
		{Method: "POST", Path: "/api/scan", Summary: "Scan the filesystem for new ISOs."},
		{Method: "GET", Path: "/api/isos", Summary: "List ISO files on disk.", Public: true},
		{Method: "GET", Path: "/api/downloads", Summary: "List active downloads.", ReadOnly: true},
		{Method: "GET", Path: "/api/downloads/progress", Summary: "Download progress.", Params: []APIParam{qFilename("ISO filename")}, ReadOnly: true},
	}},
	{Tag: "Disk Images", Description: "Clonezilla disk capture and deploy.", Endpoints: []APIEndpoint{
		{Method: "GET", Path: "/api/disk-images", Summary: "Disk imaging readiness and the list of captured disk images.", ReadOnly: true},
		{Method: "PUT", Path: "/api/disk-images", Summary: "Update a disk image's description.", Params: []APIParam{qID("Disk image ID")}, Body: "{description}"},
		{Method: "DELETE", Path: "/api/disk-images", Summary: "Delete a disk image and its files.", Params: []APIParam{qID("Disk image ID")}},
		{Method: "POST", Path: "/api/disk-images/capture", Summary: "Queue a disk capture from a client on its next PXE boot.", Body: "{mac_address, name, description, disk, wake}"},
		{Method: "POST", Path: "/api/disk-images/deploy", Summary: "Queue a disk image deploy to a client or every enabled member of a client group.", Body: "{mac_address | client_group_id, image_id, disk, wake}"},
		{Method: "GET", Path: "/api/disk-images/jobs", Summary: "Recent capture and deploy jobs.", Params: []APIParam{{Name: "mac", In: "query", Description: "Only jobs for this client"}, {Name: "limit", In: "query", Description: "Maximum number of jobs, default 100"}}, ReadOnly: true},
		{Method: "DELETE", Path: "/api/disk-images/jobs", Summary: "Cancel a pending or running job.", Params: []APIParam{qID("Job ID")}},
	}},
	{Tag: "Image Groups", Endpoints: []APIEndpoint{
		{Method: "GET", Path: "/api/groups", Summary: "List image groups.", ReadOnly: true},
		{Method: "POST", Path: "/api/groups", Summary: "Create an image group.", Body: "{name, parent_id, description, order}"},
		{Method: "PUT", Path: "/api/groups/update", Summary: "Update an image group.", Params: []APIParam{qID("Group ID")}, Body: "{name, parent_id, description, order}"},
		{Method: "DELETE", Path: "/api/groups/delete", Summary: "Delete an image group.", Params: []APIParam{qID("Group ID")}},
	}},
	{Tag: "Bootloaders", Endpoints: []APIEndpoint{
		{Method: "GET", Path: "/api/bootloaders", Summary: "List bootloader sets with their built_in flag.", ReadOnly: true},
		{Method: "POST", Path: "/api/bootloaders/create", Summary: "Create an empty user bootloader set.", Body: "{name}"},
		{Method: "POST", Path: "/api/bootloaders/upload", Summary: "Upload files into a bootloader set.", Body: "set, files[]", Multipart: true},
		{Method: "DELETE", Path: "/api/bootloaders/delete", Summary: "Delete a whole set, or one file when name is given.", Params: []APIParam{{Name: "set", In: "query", Description: "Bootloader set name", Required: true}, {Name: "name", In: "query", Description: "A single file within the set"}}},
		{Method: "POST", Path: "/api/bootloaders/select", Summary: "Set the active bootloader set.", Body: "{set}"},
		{Method: "GET", Path: "/api/usb", Summary: "List bundled USB boot images.", ReadOnly: true},
		{Method: "GET", Path: "/api/usb/download", Summary: "Download a USB boot image.", Params: []APIParam{qName("USB image name")}, ReadOnly: true},
	}},
	{Tag: "Tools", Endpoints: []APIEndpoint{
		{Method: "GET", Path: "/api/tools", Summary: "List tools.", ReadOnly: true},
		{Method: "POST", Path: "/api/tools/toggle", Summary: "Enable or disable a tool.", Params: []APIParam{qName("Tool name")}, Body: "{enabled}"},
		{Method: "POST", Path: "/api/tools/download", Summary: "Download a tool to disk.", Params: []APIParam{qName("Tool name")}},
		{Method: "DELETE", Path: "/api/tools/delete", Summary: "Delete the files of a tool.", Params: []APIParam{qName("Tool name")}},
		{Method: "GET", Path: "/api/tools/progress", Summary: "Tool download progress.", Params: []APIParam{qName("Tool name")}, ReadOnly: true},
		{Method: "POST", Path: "/api/tools/url", Summary: "Override the download URL of a tool.", Params: []APIParam{qName("Tool name")}, Body: "{url}"},
		{Method: "POST", Path: "/api/tools/custom", Summary: "Create a custom tool.", Body: "{name, display_name, ...}"},
		{Method: "DELETE", Path: "/api/tools/custom/delete", Summary: "Delete a custom tool.", Params: []APIParam{qName("Tool name")}},
		{Method: "POST", Path: "/api/tools/update", Summary: "Refresh the tools catalogue from the remote source."},
	}},
	{Tag: "Distro Profiles", Endpoints: []APIEndpoint{
		{Method: "GET", Path: "/api/profiles", Summary: "List distribution profiles.", ReadOnly: true},
		{Method: "POST", Path: "/api/profiles/save", Summary: "Create or update a custom profile.", Body: "{name, family, filename_patterns[], default_boot_params, ...}"},
		{Method: "DELETE", Path: "/api/profiles/delete", Summary: "Delete a custom profile.", Params: []APIParam{qID("Profile ID")}},
		{Method: "POST", Path: "/api/profiles/update", Summary: "Refresh the built-in profiles from the remote source."},
		{Method: "GET", Path: "/api/iso-catalog", Summary: "Curated catalogue of popular distributions and mirror URLs.", ReadOnly: true},
	}},
	{Tag: "Auto-Install Files", Endpoints: []APIEndpoint{
		{Method: "GET", Path: "/api/autoinstall-files", Summary: "List auto-install files."},
		{Method: "GET", Path: "/api/autoinstall-files/get", Summary: "Get the content of a file.", Params: []APIParam{qFilename("File name")}},
		{Method: "POST", Path: "/api/autoinstall-files/save", Summary: "Save a file. YAML, XML and JSON content is checked for well-formedness first; answers 400 unless force is true.", Body: "{distro, filename, content, force}"},
		{Method: "POST", Path: "/api/autoinstall-files/upload", Summary: "Upload a file. YAML, XML and JSON content is checked for well-formedness first; answers 400 unless force is true.", Body: "distro, filename, file, force", Multipart: true},
		{Method: "GET", Path: "/api/autoinstall-files/download", Summary: "Download a file.", Params: []APIParam{qFilename("File name")}},
		{Method: "DELETE", Path: "/api/autoinstall-files/delete", Summary: "Delete a file.", Params: []APIParam{qFilename("File name")}},
		{Method: "GET", Path: "/api/autoinstall-files/render", Summary: "Render a file with its placeholders substituted and list the ones left unresolved.", Params: []APIParam{{Name: "distro", In: "query", Description: "Distro profile ID", Required: true}, qFilename("File name"), {Name: "mac", In: "query", Description: "MAC address of the client to render for; without it client and group values are empty"}, {Name: "image", In: "query", Description: "ISO filename of the image to render for; without it image placeholders stay unresolved"}}},
	}},
	{Tag: "Custom Files", Endpoints: []APIEndpoint{
		{Method: "GET", Path: "/api/files", Summary: "List custom files.", ReadOnly: true},
		{Method: "POST", Path: "/api/files/upload", Summary: "Upload a custom file.", Body: "file, imageId, description, destinationPath, public", Multipart: true},
		{Method: "PUT", Path: "/api/files/update", Summary: "Update file metadata.", Params: []APIParam{qID("File ID")}, Body: "{description, destination_path, public}"},
		{Method: "DELETE", Path: "/api/files/delete", Summary: "Delete a custom file.", Params: []APIParam{qID("File ID")}},
	}},
	{Tag: "Driver Packs", Description: "Windows driver packs injected into boot.wim.", Endpoints: []APIEndpoint{
		{Method: "GET", Path: "/api/drivers", Summary: "List driver packs for an image.", Params: []APIParam{{Name: "imageId", In: "query", Description: "Image ID", Required: true}}, ReadOnly: true},
		{Method: "POST", Path: "/api/drivers/upload", Summary: "Upload a driver pack.", Body: "file, imageId", Multipart: true},
		{Method: "DELETE", Path: "/api/drivers/delete", Summary: "Delete a driver pack.", Params: []APIParam{qID("Driver pack ID")}},
		{Method: "POST", Path: "/api/drivers/rebuild", Summary: "Rebuild boot.wim with the driver packs.", Params: []APIParam{{Name: "imageId", In: "query", Description: "Image ID", Required: true}}},
	}},
	{Tag: "Kubernetes", Description: "Clusters, node images and node enrolment.", Endpoints: []APIEndpoint{
		{Method: "GET", Path: "/api/clusters", Summary: "List clusters.", ReadOnly: true},
		{Method: "POST", Path: "/api/clusters", Summary: "Create a cluster.", Body: "{name, description, node_image_id, kernel_params, default_install_disk}"},
		{Method: "PUT", Path: "/api/clusters/update", Summary: "Update a cluster.", Params: []APIParam{qID("Cluster ID")}, Body: "{name, description, node_image_id, kernel_params, default_install_disk}"},
		{Method: "DELETE", Path: "/api/clusters/delete", Summary: "Delete a cluster.", Params: []APIParam{qID("Cluster ID")}},
		{Method: "GET", Path: "/api/clusters/config", Summary: "Which machine configs a cluster holds.", Params: []APIParam{qID("Cluster ID")}},
		{Method: "POST", Path: "/api/clusters/config", Summary: "Upload a machine config for a role.", Params: []APIParam{qID("Cluster ID")}, Body: "{role, config}"},
		{Method: "GET", Path: "/api/clusters/config/template", Summary: "Machine config template for a provider and role.", Params: []APIParam{{Name: "provider", In: "query", Description: "Provider name", Required: true}, {Name: "role", In: "query", Description: "controlplane or worker", Required: true}}},
		{Method: "GET", Path: "/api/clusters/nodes", Summary: "List the nodes of a cluster.", Params: []APIParam{qID("Cluster ID")}, ReadOnly: true},
		{Method: "POST", Path: "/api/clusters/nodes/assign", Summary: "Assign a client to a cluster as a node.", Body: "{mac, cluster_id, role, install_disk, config_patch, reboot}"},
		{Method: "POST", Path: "/api/clusters/nodes/unassign", Summary: "Remove a node from its cluster.", Body: "{mac}"},
		{Method: "POST", Path: "/api/clusters/nodes/reinstall", Summary: "Reprovision a node on its next PXE boot.", Body: "{mac}"},
		{Method: "POST", Path: "/api/clusters/nodes/mark-installed", Summary: "Mark a node as installed.", Body: "{mac}"},
		{Method: "GET", Path: "/api/node-images", Summary: "List node images.", ReadOnly: true},
		{Method: "POST", Path: "/api/node-images", Summary: "Create a node image.", Body: "{name, provider, os_version, image_ref, ...}"},
		{Method: "POST", Path: "/api/node-images/download", Summary: "Download the assets of a node image.", Params: []APIParam{qID("Node image ID")}},
		{Method: "DELETE", Path: "/api/node-images/delete", Summary: "Delete a node image.", Params: []APIParam{qID("Node image ID")}},
		{Method: "GET", Path: "/api/kubernetes/providers", Summary: "Available Kubernetes providers.", ReadOnly: true},
	}},
	{Tag: "Webhooks", Endpoints: []APIEndpoint{
		{Method: "GET", Path: "/api/webhook", Summary: "Get the webhook configuration."},
		{Method: "PUT", Path: "/api/webhook", Summary: "Update the webhook configuration.", Body: "{url, enabled, on_boot_started, on_client_discovered, on_inventory_updated, on_enrollment, on_admin_action, on_disk_imaging}"},
		{Method: "POST", Path: "/api/webhook/test", Summary: "Send a test event."},
	}},
	{Tag: "Scheduled Tasks", Endpoints: []APIEndpoint{
		{Method: "GET", Path: "/api/scheduled-tasks", Summary: "List scheduled tasks.", ReadOnly: true},
		{Method: "POST", Path: "/api/scheduled-tasks", Summary: "Create a scheduled task.", Body: "{name, cron_expr, action_type, action_param, client_group_id, enabled}"},
		{Method: "PUT", Path: "/api/scheduled-tasks/update", Summary: "Update a scheduled task.", Params: []APIParam{qID("Task ID")}, Body: "{name, cron_expr, action_type, action_param, client_group_id, enabled}"},
		{Method: "DELETE", Path: "/api/scheduled-tasks/delete", Summary: "Delete a scheduled task.", Params: []APIParam{qID("Task ID")}},
		{Method: "POST", Path: "/api/scheduled-tasks/run", Summary: "Run a task now.", Params: []APIParam{qID("Task ID")}},
	}},
	{Tag: "Users", Endpoints: []APIEndpoint{
		{Method: "GET", Path: "/api/users", Summary: "List users."},
		{Method: "POST", Path: "/api/users", Summary: "Create a local user.", Body: "{username, password, is_admin}"},
		{Method: "PUT", Path: "/api/users", Summary: "Update a user.", Params: []APIParam{qID("User ID")}, Body: "{enabled, is_admin}"},
		{Method: "DELETE", Path: "/api/users", Summary: "Delete a user.", Params: []APIParam{qID("User ID")}},
		{Method: "POST", Path: "/api/users/reset-password", Summary: "Reset a local user's password.", Params: []APIParam{qID("User ID")}, Body: "{new_password}"},
		{Method: "POST", Path: "/api/users/totp/reset", Summary: "Remove two-factor authentication from a user who has lost their device.", Params: []APIParam{qID("User ID (or username=)")}},
		{Method: "GET", Path: "/api/tokens", Summary: "List API tokens."},
		{Method: "POST", Path: "/api/tokens", Summary: "Create a scoped API token; the secret is returned once.", Body: "{name, scope (read|admin), expires_in_days}"},
		{Method: "DELETE", Path: "/api/tokens", Summary: "Revoke an API token.", Params: []APIParam{qID("Token ID")}},
	}},
	{Tag: "Settings", Endpoints: []APIEndpoint{
		{Method: "GET", Path: "/api/theme", Summary: "Boot menu theme and defaults.", ReadOnly: true},
		{Method: "PUT", Path: "/api/theme", Summary: "Update the boot menu theme. Omit boot_password to preserve it; an empty string removes it. Setting a password requires HTTPS. Passwords must be at most 72 bytes and cannot contain +, ; or ASCII spaces.", Body: "{title, menu_timeout, default_menu_item, boot_password?}"},
		{Method: "GET", Path: "/api/backup/export", Summary: "Download a tar.gz backup of the database and data directory."},
		{Method: "POST", Path: "/api/backup/import", Summary: "Stage a backup archive for restore on the next restart.", Body: "backup=@bootimus-backup.tar.gz", Multipart: true},
		{Method: "GET", Path: "/api/backup/import", Summary: "Whether a restore is staged."},
		{Method: "DELETE", Path: "/api/backup/import", Summary: "Discard the staged restore."},
		{Method: "GET", Path: "/api/housekeeping", Summary: "Retention settings, last housekeeping report and data directory usage.", ReadOnly: true},
		{Method: "PUT", Path: "/api/housekeeping", Summary: "Update retention periods in days; 0 keeps records forever.", Body: "{boot_log_days, inventory_history_days, discovered_client_days, audit_log_days, catalogue_check_off}"},
		{Method: "POST", Path: "/api/housekeeping/run", Summary: "Run housekeeping now and return the report."},
		{Method: "POST", Path: "/api/catalogue/check", Summary: "Compare installed images and tools against the catalogue and flag newer releases."},
	}},
	{Tag: "Multi-site", Description: "Primary-side sync feed and replica control.", Endpoints: []APIEndpoint{
		{Method: "GET", Path: "/api/sync/manifest", Summary: "Counts and last-change times per kind, the boot base URL and the active bootloader set, for replicas.", ReadOnly: true},
		{Method: "GET", Path: "/api/sync/boot-verifiers", Summary: "Versioned menu and image boot password verifiers for replicas. Requires an admin-scope token and HTTPS."},
		{Method: "GET", Path: "/api/sync/tool-files", Summary: "Files on disk for a downloaded boot tool, for replicas.", Params: []APIParam{qName("Tool name")}, ReadOnly: true},
		{Method: "GET", Path: "/api/sync/asset", Summary: "Stream an ISO, tool file or custom file to a replica.", Params: []APIParam{{Name: "kind", In: "query", Description: "iso, tool or file", Required: true}, {Name: "filename", In: "query", Description: "ISO filename when kind is iso"}, {Name: "name", In: "query", Description: "Tool name when kind is tool"}, {Name: "path", In: "query", Description: "File path within the tool when kind is tool"}, {Name: "id", In: "query", Description: "Custom file ID when kind is file"}}, ReadOnly: true},
		{Method: "GET", Path: "/api/replica/status", Summary: "Whether this instance is a replica and the result of its last sync.", ReadOnly: true},
		{Method: "POST", Path: "/api/replica/sync", Summary: "Run a sync from the primary now and return the result."},
	}},
	{Tag: "Logs", Endpoints: []APIEndpoint{
		{Method: "GET", Path: "/api/logs", Summary: "Boot log entries.", Params: []APIParam{{Name: "limit", In: "query", Description: "Number of entries, 1 to 1000 (default 100)"}, {Name: "mac", In: "query", Description: "Only entries for this client"}}, ReadOnly: true},
		{Method: "GET", Path: "/api/logs/stream", Summary: "Server log as a server-sent event stream."},
		{Method: "GET", Path: "/api/logs/buffer", Summary: "Recent in-memory server log buffer."},
		{Method: "GET", Path: "/api/audit", Summary: "Admin action audit log, newest first.", Params: []APIParam{{Name: "username", In: "query", Description: "Only entries by this user"}, {Name: "path", In: "query", Description: "Only paths containing this text"}, {Name: "since", In: "query", Description: "RFC 3339 timestamp"}, {Name: "limit", In: "query", Description: "Page size, default 100, maximum 1000"}, {Name: "offset", In: "query", Description: "Entries to skip"}}},
		{Method: "GET", Path: "/api/proxydhcp/events", Summary: "Recent DHCP requests seen by the built-in proxyDHCP and the decision taken for each.", ReadOnly: true},
		{Method: "GET", Path: "/api/proxydhcp/stream", Summary: "Server-Sent Events stream of proxyDHCP request events.", ReadOnly: true},
		{Method: "POST", Path: "/api/proxydhcp/events/clear", Summary: "Clear the recorded proxyDHCP request history."},
	}},
	{Tag: "Boot", Description: "Endpoints served to PXE clients on the boot port; no authentication.", Endpoints: []APIEndpoint{
		{Method: "GET", Path: "/menu.ipxe", Summary: "Generated iPXE menu script.", Params: []APIParam{{Name: "mac", In: "query", Description: "MAC address of the booting client"}}, Public: true},
		{Method: "GET", Path: "/autoexec.ipxe", Summary: "iPXE autoexec for the chainloaded bootloader.", Public: true},
		{Method: "GET", Path: "/inventory", Summary: "iPXE-submitted hardware inventory.", Params: []APIParam{qMAC("MAC address of the booting client")}, Public: true},
		{Method: "GET", Path: "/isos/{filename}", Summary: "Direct ISO download.", Params: []APIParam{pathParam("filename", "ISO filename")}, Public: true},
		{Method: "GET", Path: "/boot/{cache_dir}/{path}", Summary: "Extracted boot files (kernel, initrd, squashfs).", Params: []APIParam{pathParam("cache_dir", "Image cache directory"), pathParam("path", "File path within the cache directory")}, Public: true},
		{Method: "GET", Path: "/autoinstall/{filename}", Summary: "Auto-install script (preseed, kickstart, cloud-init or autounattend).", Params: []APIParam{pathParam("filename", "Script filename")}, Public: true},
		{Method: "POST", Path: "/install/callback", Summary: "Report that an unattended install finished; also accepts GET. The token comes from the {{CALLBACK_URL}} placeholder.", Params: []APIParam{{Name: "token", In: "query", Description: "Install token issued with the auto-install file", Required: true}, {Name: "status", In: "query", Description: "completed (default) or failed"}, {Name: "message", In: "query", Description: "Optional free text, up to 500 characters"}}, Public: true},
		{Method: "GET", Path: "/files/{filename}", Summary: "Custom file download.", Params: []APIParam{pathParam("filename", "File name")}, Public: true},
		{Method: "GET", Path: "/bootenv/{filename}", Summary: "NBD boot environment kernel and initrd.", Params: []APIParam{pathParam("filename", "File name")}, Public: true},
		{Method: "GET", Path: "/tools/{path}", Summary: "Downloaded tool files.", Params: []APIParam{pathParam("path", "File path within the tools directory")}, Public: true},
		{Method: "GET", Path: "/node-images/{path}", Summary: "Kubernetes node image assets.", Params: []APIParam{pathParam("path", "Asset path")}, Public: true},
		{Method: "GET", Path: "/diskimage/job", Summary: "Shell script a Clonezilla client runs for its queued capture or deploy job.", Params: []APIParam{{Name: "token", In: "query", Description: "Job token", Required: true}}, Public: true},
		{Method: "GET", Path: "/diskimage/callback", Summary: "Progress report from a Clonezilla client.", Params: []APIParam{{Name: "token", In: "query", Description: "Job token", Required: true}, {Name: "status", In: "query", Description: "running, done or failed", Required: true}, {Name: "disk", In: "query", Description: "Disk device actually used"}, {Name: "message", In: "query", Description: "Short failure reason"}}, Public: true},
		{Method: "GET", Path: "/machineconfig", Summary: "Kubernetes machine config for an enrolled node.", Params: []APIParam{{Name: "m", In: "query", Description: "MAC address", Required: true}, {Name: "t", In: "query", Description: "Enrolment token", Required: true}, {Name: "u", In: "query", Description: "System UUID"}}, Public: true},
	}},
	{Tag: "Boot passwords", Description: "Menu and image authentication, separate from admin access.", Endpoints: []APIEndpoint{
		{Method: "POST", Path: "/boot-auth", Summary: "iPXE boot-password authentication on the admin HTTPS port. Returns an iPXE script; does not grant admin access. Credentials and scoped menu sessions are accepted only in the form body.", Body: "{action: menu|image, mac, image?, password?, session?}", FormEncoded: true, Public: true},
	}},
}

type OpenAPIDocument struct {
	OpenAPI    string                                 `json:"openapi"`
	Info       OpenAPIInfo                            `json:"info"`
	Servers    []OpenAPIServer                        `json:"servers,omitempty"`
	Tags       []OpenAPITag                           `json:"tags"`
	Paths      map[string]map[string]OpenAPIOperation `json:"paths"`
	Components OpenAPIComponents                      `json:"components"`
	Security   []map[string][]string                  `json:"security"`
}

type OpenAPIInfo struct {
	Title       string `json:"title"`
	Version     string `json:"version"`
	Description string `json:"description"`
}

type OpenAPIServer struct {
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
}

type OpenAPITag struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type OpenAPIParameter struct {
	Name        string                 `json:"name"`
	In          string                 `json:"in"`
	Description string                 `json:"description,omitempty"`
	Required    bool                   `json:"required,omitempty"`
	Schema      map[string]interface{} `json:"schema"`
}

type OpenAPIRequestBody struct {
	Description string                 `json:"description,omitempty"`
	Required    bool                   `json:"required,omitempty"`
	Content     map[string]interface{} `json:"content"`
}

type OpenAPIOperation struct {
	Tags        []string                          `json:"tags"`
	Summary     string                            `json:"summary"`
	OperationID string                            `json:"operationId"`
	Parameters  []OpenAPIParameter                `json:"parameters,omitempty"`
	RequestBody *OpenAPIRequestBody               `json:"requestBody,omitempty"`
	Responses   map[string]map[string]interface{} `json:"responses"`
	Security    []map[string][]string             `json:"security,omitempty"`
	Public      bool                              `json:"x-bootimus-public"`
	ReadOnly    bool                              `json:"x-bootimus-readonly"`
	Order       int                               `json:"x-bootimus-order"`
}

type OpenAPIComponents struct {
	Schemas         map[string]interface{} `json:"schemas"`
	SecuritySchemes map[string]interface{} `json:"securitySchemes"`
}

func BuildOpenAPI(version, serverURL string) *OpenAPIDocument {
	doc := &OpenAPIDocument{
		OpenAPI: "3.0.3",
		Info: OpenAPIInfo{
			Title:       "Bootimus API",
			Version:     version,
			Description: "REST API of the Bootimus PXE and HTTP boot server. Responses are JSON of the form {success, message, error, data}. Endpoints marked x-bootimus-readonly are available to read-only users; all others need an admin token.",
		},
		Paths: map[string]map[string]OpenAPIOperation{},
		Components: OpenAPIComponents{
			Schemas: map[string]interface{}{
				"Response": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"success": map[string]interface{}{"type": "boolean"},
						"message": map[string]interface{}{"type": "string"},
						"error":   map[string]interface{}{"type": "string"},
						"data":    map[string]interface{}{},
					},
					"required": []string{"success"},
				},
			},
			SecuritySchemes: map[string]interface{}{
				"bearerAuth": map[string]interface{}{
					"type":         "http",
					"scheme":       "bearer",
					"bearerFormat": "JWT or API token",
					"description":  "A JWT from POST /api/login, or a long-lived API token (btk_...) created under Users.",
				},
			},
		},
		Security: []map[string][]string{{"bearerAuth": {}}},
	}
	if serverURL != "" {
		doc.Servers = []OpenAPIServer{{URL: serverURL}}
	}
	order := 0
	for _, section := range APISections {
		doc.Tags = append(doc.Tags, OpenAPITag{Name: section.Tag, Description: section.Description})
		for _, ep := range section.Endpoints {
			order++
			ops := doc.Paths[ep.Path]
			if ops == nil {
				ops = map[string]OpenAPIOperation{}
				doc.Paths[ep.Path] = ops
			}
			ops[strings.ToLower(ep.Method)] = buildOperation(section.Tag, ep, order)
		}
	}
	return doc
}

func buildOperation(tag string, ep APIEndpoint, order int) OpenAPIOperation {
	op := OpenAPIOperation{
		Tags:        []string{tag},
		Summary:     ep.Summary,
		OperationID: operationID(ep.Method, ep.Path),
		Responses: map[string]map[string]interface{}{
			"200": {
				"description": "Successful response",
				"content": map[string]interface{}{
					"application/json": map[string]interface{}{
						"schema": map[string]interface{}{"$ref": "#/components/schemas/Response"},
					},
				},
			},
		},
		Public:   ep.Public,
		ReadOnly: ep.ReadOnly || ep.Public,
		Order:    order,
	}
	for _, p := range ep.Params {
		op.Parameters = append(op.Parameters, OpenAPIParameter{
			Name:        p.Name,
			In:          p.In,
			Description: p.Description,
			Required:    p.Required || p.In == "path",
			Schema:      map[string]interface{}{"type": "string"},
		})
	}
	if ep.Body != "" {
		contentType := "application/json"
		schema := map[string]interface{}{"type": "object"}
		if ep.FormEncoded {
			contentType = "application/x-www-form-urlencoded"
			op.Responses["200"]["content"] = map[string]interface{}{"text/plain": map[string]interface{}{"schema": map[string]interface{}{"type": "string"}}}
		}
		if ep.Multipart {
			contentType = "multipart/form-data"
			props := map[string]interface{}{}
			for _, field := range strings.Split(ep.Body, ",") {
				field = strings.TrimSpace(field)
				if strings.HasSuffix(field, "[]") {
					props[strings.TrimSuffix(field, "[]")] = map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string", "format": "binary"}}
				} else if field == "file" {
					props[field] = map[string]interface{}{"type": "string", "format": "binary"}
				} else {
					props[field] = map[string]interface{}{"type": "string"}
				}
			}
			schema["properties"] = props
		}
		op.RequestBody = &OpenAPIRequestBody{
			Description: ep.Body,
			Required:    true,
			Content:     map[string]interface{}{contentType: map[string]interface{}{"schema": schema}},
		}
	}
	if ep.Public {
		op.Security = []map[string][]string{}
	}
	return op
}

func operationID(method, path string) string {
	var b strings.Builder
	b.WriteString(strings.ToLower(method))
	upper := true
	for _, r := range path {
		switch {
		case r == '/' || r == '-' || r == '.' || r == '_' || r == '{' || r == '}':
			upper = true
		case upper:
			b.WriteString(strings.ToUpper(string(r)))
			upper = false
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (h *Handler) OpenAPI(version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
			return
		}
		scheme := "http"
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		doc := BuildOpenAPI(version, scheme+"://"+r.Host)
		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.Encode(doc)
	}
}
