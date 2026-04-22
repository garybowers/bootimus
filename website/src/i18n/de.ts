const de = {
  meta: {
    title: 'Bootimus — Moderner PXE/HTTP-Boot-Server',
    description:
      'Eigenständiger PXE- und HTTP-Boot-Server. Ein Binary. Keine Konfiguration. 50+ Distros out-of-the-box.',
    downloadsTitle: 'Downloads — Bootimus',
    downloadsDescription:
      'Lade bootimus-Binaries, Docker-Images, die USB-Appliance und gespiegelte PXE-Tools herunter.',
    docsTitle: 'Dokumentation — Bootimus',
    docsDescription:
      'Bootimus-Dokumentation: Deployment, Image-Verwaltung, DHCP, Authentifizierung, Client-ACLs, Distro-Profile.',
  },

  nav: {
    features: 'Features',
    howItWorks: 'Funktionsweise',
    downloads: 'Downloads',
    docs: 'Doku',
    github: 'GitHub',
    toggleTheme: 'Theme wechseln',
    languageLabel: 'Sprache',
    comingSoon: 'demnächst',
  },

  footer: {
    project: 'projekt',
    licence: 'lizenz',
    licenceValue: 'Apache 2.0',
    lang: 'sprache',
    repo: 'repo',
    docker: 'docker',
    docs: 'doku',
    copy: '© {year} bootimus-Mitwirkende',
  },

  hero: {
    badgeStable: 'v1.x · apache 2.0',
    badgeStack: 'go · iPXE · sqlite/postgres',
    titleLine1: 'PXE-Boot,',
    titleLine2: 'ohne das Gefummel',
    sub: 'Eigenständiger PXE- und HTTP-Boot-Server. Ein Binary. Keine Konfiguration. Eingebautes proxyDHCP — du fasst deinen Router nie wieder an. 50+ Distros werden automatisch erkannt.',
    ctaPrimary: '$ get bootimus',
    ctaSecondary: 'quellcode ansehen',
    quickstartTitle: 'bootimus — schnellstart',
    statDistrosN: '50+',
    statDistrosL: 'distros erkannt',
    statBinaryN: '1',
    statBinaryL: 'binary, keine abhängigkeiten',
    statReconfigsN: '0',
    statReconfigsL: 'DHCP-anpassungen',
    statArchN: '2',
    statArchL: 'arch: amd64 · arm64',
  },

  features: {
    kicker: '// features',
    title: 'Alles, was ein modernes Netboot-Setup sein sollte.',
    sub: 'Kein Fork von 15 Jahre alten Perl-Skripten. Kein Wrapper um dnsmasq. Ein richtiger Server — in Go geschrieben, alles inklusive.',
    items: {
      '01': {
        title: 'Ein Binary',
        body: 'Go-Binary mit eingebettetem iPXE, Web-UI, SQLite und allen Assets. Keine Laufzeit-Abhängigkeiten. scp und los.',
      },
      '02': {
        title: 'Eingebautes proxyDHCP',
        body: 'Antwortet auf PXE über UDP/67, ohne dein bestehendes DHCP anzufassen. Null Router-Umbau. In jedes LAN einklinken.',
      },
      '03': {
        title: '50+ Distros',
        body: 'Automatische Kernel/initrd-Extraktion für Ubuntu, Debian, Arch, Fedora, NixOS, Alpine, FreeBSD, Windows (wimboot) und mehr.',
      },
      '04': {
        title: 'MAC-basierte ACL',
        body: 'Ordne spezifische Images pro MAC zu. Neue Clients werden beim ersten PXE-Request automatisch erkannt. Leases zu statischen Einträgen hochstufen, wenn du bereit bist.',
      },
      '05': {
        title: 'Tools per Klick',
        body: 'GParted, Clonezilla, Memtest86+, SystemRescue, ShredOS, netboot.xyz. Im UI aktivieren — und sie erscheinen im Menü.',
      },
      '06': {
        title: 'JWT + LDAP',
        body: 'Token-Auth mit bcrypt. Optionales LDAP/AD-Backend mit gruppenbasierter Admin-Vergabe. Lokale Accounts bleiben als Fallback.',
      },
      '07': {
        title: 'REST-API',
        body: 'Alles, was das UI kann, ist auch ein API-Call. Boot-Zuweisungen, Scans, WoL-Trigger scriptbar. Live-Log-Stream per SSE.',
      },
      '08': {
        title: 'Läuft überall',
        body: 'Multi-Arch-Docker (amd64/arm64), statisches Binary oder ein 2-GB-Appliance-Image auf Alpine-Basis, das du per USB flashen kannst.',
      },
    },
  },

  howItWorks: {
    kicker: '// funktionsweise',
    title: 'Der Lebenszyklus eines PXE-Boots.',
    sub: 'Client schickt DHCPDISCOVER. Bootimus beantwortet die PXE-Details per proxyDHCP, während dein normales DHCP weiter die IPs verteilt. iPXE lädt über TFTP, chained zu HTTP, holt das Menü. Nutzer wählt ein Image. Kernel und initrd streamen vom Server. Fertig.',
    termTitle: 'pxe boot trace — ubuntu-24.04',
  },

  transparency: {
    kicker: '// transparenz',
    title: '100% offen. Durchgängig nachprüfbar.',
    sub: 'Keine proprietären Blobs. Keine Telemetrie. Keine klammheimlich eingebundene Binary-Firmware. Der gesamte Stack liegt unter Apache 2.0 auf GitHub — klonen, prüfen, forken, selbst betreiben.',
    items: {
      binary: {
        t: 'Ein einziges Go-Binary',
        d: 'statisch gelinkt, ldd sagt "not a dynamic executable". Reproduzierbare Builds über make release.',
      },
      blobs: {
        t: 'Keine proprietären Blobs',
        d: 'eingebettetes iPXE ist upstream-FOSS (GPL-2.0). Keine Closed-Source-Firmware im Lieferumfang.',
      },
      telemetry: {
        t: 'Keine Telemetrie, nie',
        d: 'null call-home. Null Analytics. Null "anonyme Nutzungsstatistik". Air-gap-tauglich.',
      },
      licence: {
        t: 'Apache 2.0',
        d: 'permissive Lizenz. Kommerziell einsetzen, intern ausrollen, ohne Fallstricke forken.',
      },
      deps: {
        t: 'Vendored Deps, alles FOSS',
        d: 'jede transitive Go-Abhängigkeit ist Open Source. go mod why für jedes Package.',
      },
      byo: {
        t: 'Bring deinen eigenen Bootloader',
        d: 'dem eingebetteten iPXE nicht über den Weg trauen? Eigene signierte Binaries reinlegen. Siehe unten.',
      },
    },
    termTitle: 'bootimus version --verbose',
  },

  bootloaders: {
    kicker: '// bootloaders',
    title: 'Tausch iPXE gegen das, was du brauchst.',
    sub: 'Bootimus bringt eingebettetes iPXE für jede gängige Architektur mit. Brauchst du Microsoft-signierte Binaries für Secure Boot, ein custom-gebrandetes iPXE, GRUB, syslinux oder deinen eigenen von der Internal-CA signierten Loader? Ordner in data/bootloaders/ ablegen, im UI wählen, fertig. Fehlende Dateien fallen transparent auf das eingebettete Set zurück — nie ein kaputter Boot.',
    cards: {
      uefi64: {
        t: 'iPXE · UEFI x86_64',
        d: 'ipxe.efi · der Default. Aus Upstream-Master gebaut, im Binary eingebettet.',
        tag: 'eingebettet · fallback',
      },
      uefiArm: {
        t: 'iPXE · UEFI ARM64',
        d: 'ipxe-arm64.efi · für Raspberry Pi 4/5, Apple-Silicon-Hosts, ARM-Server.',
        tag: 'eingebettet · fallback',
      },
      bios: {
        t: 'iPXE · Legacy BIOS',
        d: 'undionly.kpxe · für alte Hardware ohne UEFI. Auch 2026 noch relevant.',
        tag: 'eingebettet · fallback',
      },
      shim: {
        t: 'Microsoft-signierter Shim',
        d: 'Signierte shimx64.efi + grubx64.efi reinwerfen — für Flotten mit erzwungenem Secure Boot. Kein Firmware-MOK-Enrolment nötig.',
        tag: 'custom · BYO',
      },
      themed: {
        t: 'Custom-gebrandetes iPXE',
        d: 'Bau dein eigenes iPXE mit Branding, individuellen Menüfarben, eingebetteten Skripten. Die .efi reinlegen.',
        tag: 'custom · BYO',
      },
      grub: {
        t: 'GRUB / syslinux / pxelinux',
        d: 'Nicht iPXE? Kein Problem. Alles, was TFTP und HTTP spricht, funktioniert. Bootimus liefert einfach Bytes aus.',
        tag: 'custom · BYO',
      },
    },
    termTitle: 'bootloader-sets — file fallthrough',
  },

  cta: {
    title: 'Keine Lust mehr, tftpd zu babysitten?',
    sub: 'Docker, Bare Metal oder flashbarer USB-Stick. Such dir aus, was dir taugt.',
    primary: '$ get bootimus',
    secondary: 'zur doku →',
  },

  downloads: {
    kicker: '// downloads',
    title: 'Schnapp dir einen Build.',
    lede:
      'Jedes Release gibt\'s als Docker-Image, als statisches Linux-Binary live aus dem neuesten GitHub-Release, und als 2-GiB-flashbares USB-Appliance-Image (Alpine + bootimus, bootet direkt rein). Tools werden separat gespiegelt, damit das Admin-UI sie ziehen kann, ohne auf Upstream-Ratelimits angewiesen zu sein.',
    badgeStable: 'aktuell · stabil',
    badgePrerelease: 'pre-release',
    badgeNone: 'noch keine releases',
    pillManifest: 'manifest.json',
    pillSource: 'github releases ↗',
    pillBuildSrc: 'aus quellcode bauen',
    released: 'veröffentlicht',
    via: 'via',
    sectionArtifacts: 'release-artefakte',
    sectionTools: 'gespiegelte tools',
    sectionApi: 'manifest verwenden',
    emptyTitle: 'Noch keine Binaries veröffentlicht',
    emptyBody:
      'Sobald du auf GitHub einen Tag setzt, tauchen die Binaries aus make release hier automatisch auf.',
    buildFromSource: 'aus quellcode bauen ↗',
    verifyTitle: 'verifizieren',
    apiTitle: 'api · manifest.json',
    toolsLede:
      'Das sind netboot-fertige Images, die das Admin-UI bei Bedarf herunterlädt und als PXE-Menüeinträge ausspielt. Gespiegelt auf dl.bootimus.com — damit du nicht auf die Upstream-Verfügbarkeit angewiesen bist, wenn du gerade einen Rescue-Einsatz fährst. Die Upstream-URLs bleiben die Wahrheit — jede Mirror-URL lässt sich auf der Tools-Seite überschreiben.',
    apiLede:
      'Das Admin-UI liest /api/manifest.json, um nach Updates zu schauen und verfügbare Tools anzuzeigen. Stabiles Schema — Felder werden ohne Major-Bump nicht umbenannt. Quelle der Wahrheit für Binaries ist die GitHub-Releases-API; dieser Endpoint normalisiert nur und mergt die statischen Stücke (Docker-Tag, Appliance-Image, gespiegelte Tools) mit ein.',
    mirror: 'mirror ↓',
    upstream: 'upstream ↗',
    get: 'holen ↓',
  },

  docs: {
    title: 'Dokumentation',
    subtitle: 'Alles, was du brauchst, um bootimus auszurollen, zu konfigurieren und zu betreiben.',
    sectionsTitle: 'Abschnitte',
    onThisPage: 'Auf dieser Seite',
    prev: 'Vorheriges',
    next: 'Nächstes',
    editOnGithub: 'Auf GitHub bearbeiten',
    notFound: 'Dokument nicht gefunden.',
    fallbackBanner: 'Diese Seite ist noch nicht übersetzt. Zeige die englische Version.',
    translateCta: 'Beim Übersetzen helfen →',
    pending: 'übersetzung ausstehend',
  },
};

export default de;
