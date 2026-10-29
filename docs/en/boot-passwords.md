# Boot menu passwords

Menu protection does not protect image files themselves. Direct downloads, alternative boot scripts and other boot services remain outside its scope. Use a trusted provisioning network: DHCP, TFTP and the initial HTTP bootstrap are not authenticated by these password checks.

## Requirements

Enable admin HTTPS and use an iPXE build with HTTPS, POST parameters and masked password forms. The loader must trust the HTTPS certificate and validate the server address. The old `default` set lacks these features. The unchanged `secureboot-official` v2.0.0 set supports password checks with the character restriction below; a custom build is not required for that route. Test keyboard support on the intended hardware before deployment.

Use a stable CA for routine certificate renewal. Before changing the CA or a pinned self-signed certificate, provision both old and new trust on clients, then remove the old trust after migration. This requires rebuilding only when trust is embedded in a custom loader; firmware-provisioned TLS trust is updated separately. Do not disable TLS verification or weaken server cipher settings. Password requests go directly to the admin HTTPS listener; TLS termination at a reverse proxy is not supported for this endpoint.

The official signed loader retains its existing Secure Boot chain. HTTPS trust is separate: it can use iPXE's [public-CA trust](https://ipxe.org/crypto), or a private CA provisioned in the client's UEFI `TlsCaCertificate` variable where supported. The certificate must cover the configured server address. Provisioning TLS trust does not require changing the firmware's Secure Boot signing keys. Custom builds remain an alternative for deployments needing embedded trust; those binaries require deployment signing for Secure Boot.

The signed-loader route has been tested in QEMU with stock Microsoft signing trust and a disposable CA added only to `TlsCaCertificate`. Physical UEFI keyboard compatibility and other firmware trust-provisioning methods remain unverified. If a loader cannot authenticate, a protected boot fails; it never falls back to an unprotected image.

## Temporary character restriction

Global and image passwords must not contain `+`, `;` or ASCII spaces. iPXE v2.0.0 fails to escape literal `+` characters in form submissions, so the server reads them as spaces. Rejecting both prevents a mistyped `a+b` from matching `a b`. iPXE also leaves semicolons unescaped, which Go's form parser rejects. Other punctuation remains allowed; test it with the client's keyboard layout. The [upstream plus-encoding fix](https://github.com/ipxe/ipxe/commit/081d235ae2ec99e3ef2b8b7847ee1930896e77cf) does not fix semicolons. Reconsider the restrictions only after compatible loaders are available and tested.

The UI and API reject incompatible password changes without altering existing protection. Authentication rejects these characters too. Reset any password containing them that was set with an earlier development build; stored hashes cannot reveal whether a reset is needed. Leaving the field blank preserves the existing hash, including an incompatible one. Password removal remains available.

## Set or remove a password

In **Boot Menu**, set the optional global password. In an image's properties, set its optional image password. Both are disabled by default. Leave a password field blank to keep its current value; select **Remove** to disable protection. Setting a password requires opening the admin interface over HTTPS.

- A global password is required before entering the network-boot menu. Local boot remains available without it.
- An image password is requested when that image is selected. Without a global password, the menu remains browsable.
- With both configured, unlocking the menu does not unlock the image.
- Defaults, next-boot images and deployment-profile selections use the same image check. Cancelling or failing authentication does not consume a protected next-boot selection.
- Cancelling returns to the menu. Authentication or TLS failure never starts a protected image. A source address gets up to five password attempts per minute.

Passwords are stored as bcrypt hashes and are never returned by ordinary read APIs or included in boot scripts. A separate admin-authorised HTTPS sync endpoint supplies verifiers to replicas. The maximum length is 72 bytes. Test punctuation and your keyboard layout on real clients before deployment.

Image-authentication requests use a five-minute menu token, scoped to the client MAC and source address. Changing the global password or restarting the server invalidates it. MAC addresses are not device identities. Use **Lock network-boot menu** to leave the menu or authenticate again after expiry; expiry does not hide an already displayed menu.

The checks apply to ISO images and the global network-boot menu. There are no separate passwords for tools, SAN targets, disk-imaging jobs or Kubernetes node images; the global password covers entry to those boot paths.

## API

`PUT /api/theme` and `PUT /api/images?filename=...` accept `boot_password`. Omit it to preserve the password, supply a non-empty string to set it, or `""` to remove it. Responses contain `boot_password_required` and an opaque `boot_password_revision`, never the password or hash. The revision changes on every set or removal. Changing either status field directly has no effect.

Boot clients submit URL-encoded POST bodies to `/boot-auth` on the admin HTTPS port. Passwords and menu-session tokens must not appear in URLs. This endpoint grants no admin session or API access.

## Replicas

Upgrade every replica before enabling protection on the primary. Give each replica an admin-scope API token, a trusted HTTPS connection to the primary, and its own admin HTTPS certificate trusted by its boot clients. The replica copies verifiers through the restricted sync endpoint and checks each against the corresponding image or menu policy before saving it. A branch can unlock its last synced protection while the primary is unreachable. If it observes a newer protected policy without a matching verifier, that boot remains blocked until a successful sync. Password removal also waits for a successful sync. See [multi-site deployments](multi-site.md) for setup and failure behaviour.
