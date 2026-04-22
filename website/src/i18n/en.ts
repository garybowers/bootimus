export default {
  meta: {
    title: 'Bootimus — Modern PXE/HTTP boot server',
    description:
      'Self-contained PXE and HTTP boot server. Single binary. Zero config. 50+ distros out of the box.',
    downloadsTitle: 'Downloads — Bootimus',
    downloadsDescription:
      'Download bootimus binaries, Docker images, the USB appliance, and mirrored PXE tools.',
    docsTitle: 'Docs — Bootimus',
    docsDescription:
      'Bootimus documentation: deployment, image management, DHCP, authentication, client ACLs, distro profiles.',
  },

  nav: {
    features: 'Features',
    howItWorks: 'How it works',
    downloads: 'Downloads',
    docs: 'Docs',
    github: 'GitHub',
    toggleTheme: 'Toggle theme',
    languageLabel: 'Language',
    comingSoon: 'coming soon',
  },

  footer: {
    project: 'project',
    licence: 'licence',
    licenceValue: 'Apache 2.0',
    lang: 'lang',
    repo: 'repo',
    docker: 'docker',
    docs: 'docs',
    copy: '© {year} bootimus contributors',
  },

  hero: {
    badgeStable: 'v1.x · apache 2.0',
    badgeStack: 'go · iPXE · sqlite/postgres',
    titleLine1: 'PXE boot,',
    titleLine2: 'without the pain',
    sub: 'Self-contained PXE and HTTP boot server. One binary. Zero config. Built-in proxyDHCP so you never touch your router. 50+ distros detected automatically.',
    ctaPrimary: '$ get bootimus',
    ctaSecondary: 'view source',
    quickstartTitle: 'bootimus — quickstart',
    statDistrosN: '50+',
    statDistrosL: 'distros detected',
    statBinaryN: '1',
    statBinaryL: 'binary, zero deps',
    statReconfigsN: '0',
    statReconfigsL: 'DHCP reconfigs',
    statArchN: '2',
    statArchL: 'arch: amd64 · arm64',
  },

  features: {
    kicker: '// features',
    title: 'Everything a modern netboot setup should be.',
    sub: 'Not a fork of 15-year-old Perl scripts. Not a wrapper around dnsmasq. A proper server, written in Go, with batteries included.',
    items: {
      '01': { title: 'Single binary', body: 'Go binary with embedded iPXE, web UI, SQLite, and all assets. No runtime deps. Scp it and run.' },
      '02': { title: 'Built-in proxyDHCP', body: 'Answers PXE on UDP/67 without touching your existing DHCP. Zero router reconfig. Drop in on any LAN.' },
      '03': { title: '50+ distros', body: 'Automatic kernel/initrd extraction for Ubuntu, Debian, Arch, Fedora, NixOS, Alpine, FreeBSD, Windows (wimboot), and more.' },
      '04': { title: 'MAC-based ACL', body: 'Assign specific images per MAC. Auto-discover new clients on first PXE. Promote leases to static when ready.' },
      '05': { title: 'One-click tools', body: 'GParted, Clonezilla, Memtest86+, SystemRescue, ShredOS, netboot.xyz. Enable from the UI, they show up in the menu.' },
      '06': { title: 'JWT + LDAP', body: 'Token auth with bcrypt. Optional LDAP/AD backend with group-based admin. Local accounts stay as fallback.' },
      '07': { title: 'REST API', body: 'Everything the UI does is an API call. Script boot assignments, scans, WOL triggers. Live log stream over SSE.' },
      '08': { title: 'Runs anywhere', body: 'Multi-arch Docker (amd64/arm64), static binary, or a 2GB Alpine-based appliance image you can flash to USB.' },
    },
  },

  howItWorks: {
    kicker: '// how it works',
    title: 'The lifecycle of a PXE boot.',
    sub: 'Client sends DHCPDISCOVER. Bootimus answers PXE details via proxyDHCP while your normal DHCP still hands out the IP. iPXE loads over TFTP, chains to HTTP, fetches the menu. User picks an image. Kernel and initrd stream from the server. Done.',
    termTitle: 'pxe boot trace — ubuntu-24.04',
  },

  transparency: {
    kicker: '// transparency',
    title: '100% open. Auditable end-to-end.',
    sub: 'No proprietary blobs. No telemetry. No sneaky binary firmware vendored in. The whole stack is on GitHub under Apache 2.0 — clone it, audit it, fork it, fly your own.',
    items: {
      binary: { t: 'Single Go binary', d: 'statically linked, ldd returns "not a dynamic executable". Reproducible builds from make release.' },
      blobs: { t: 'No proprietary blobs', d: 'embedded iPXE is upstream FOSS (GPL-2.0). No closed-source firmware shipped.' },
      telemetry: { t: 'No telemetry, ever', d: 'zero call-home. Zero analytics. Zero "anonymous usage stats". Air-gapped LAN safe.' },
      licence: { t: 'Apache 2.0', d: 'permissive licence. Use in commercial environments, ship internally, fork without strings.' },
      deps: { t: 'Vendored deps, all FOSS', d: 'every transitive Go dependency is open source. go mod why any package.' },
      byo: { t: 'Bring your own bootloader', d: 'don\'t trust the embedded iPXE? Drop your own signed binaries in. See below.' },
    },
    termTitle: 'bootimus version --verbose',
  },

  bootloaders: {
    kicker: '// bootloaders',
    title: 'Swap iPXE for whatever you need.',
    sub: 'Bootimus ships with embedded iPXE for every common arch. Need Microsoft-signed binaries for Secure Boot, a custom-themed iPXE, GRUB, syslinux, or your own internal-CA-signed loader? Drop a folder in data/bootloaders/, pick it from the UI, done. Missing files transparently fall back to the embedded set — never a broken boot.',
    cards: {
      uefi64: { t: 'iPXE · UEFI x86_64', d: 'ipxe.efi · the default. Built from upstream master, embedded in the binary.', tag: 'embedded · fallback' },
      uefiArm: { t: 'iPXE · UEFI ARM64', d: 'ipxe-arm64.efi · for Raspberry Pi 4/5, Apple Silicon hosts, ARM servers.', tag: 'embedded · fallback' },
      bios: { t: 'iPXE · Legacy BIOS', d: 'undionly.kpxe · for old kit that won\'t UEFI. Still relevant in 2026.', tag: 'embedded · fallback' },
      shim: { t: 'Microsoft-signed shim', d: 'Drop a signed shimx64.efi + grubx64.efi in for Secure-Boot-enforced fleets. No firmware MOK enrolment needed.', tag: 'custom · BYO' },
      themed: { t: 'Custom-themed iPXE', d: 'Compile your own iPXE with branding, custom menu colours, embedded scripts. Drop the .efi in.', tag: 'custom · BYO' },
      grub: { t: 'GRUB / syslinux / pxelinux', d: 'Not iPXE? No problem. Anything that speaks TFTP and HTTP works. Bootimus just serves bytes.', tag: 'custom · BYO' },
    },
    termTitle: 'bootloader sets — file fallthrough',
  },

  cta: {
    title: 'Ready to stop babysitting tftpd?',
    sub: 'Docker, bare metal, or flashable USB. Pick your poison.',
    primary: '$ get bootimus',
    secondary: 'read the docs →',
  },

  downloads: {
    kicker: '// downloads',
    title: 'Grab a build.',
    lede:
      'Every release ships as a Docker image, static Linux binaries pulled live from the latest GitHub release, and a 2 GiB flashable USB appliance image (Alpine + bootimus, boots straight in). Tools are mirrored separately so the admin UI can pull them without leaning on upstream rate limits.',
    badgeStable: 'latest · stable',
    badgePrerelease: 'pre-release',
    badgeNone: 'no releases yet',
    pillManifest: 'manifest.json',
    pillSource: 'github releases ↗',
    pillBuildSrc: 'build from source',
    released: 'released',
    via: 'via',
    sectionArtifacts: 'release artifacts',
    sectionTools: 'mirrored tools',
    sectionApi: 'consume the manifest',
    emptyTitle: 'No binaries published yet',
    emptyBody:
      'Cut a tag on GitHub and the binaries from make release will appear here automatically.',
    buildFromSource: 'build from source ↗',
    verifyTitle: 'verify',
    apiTitle: 'api · manifest.json',
    toolsLede:
      'These are netboot-ready images the admin UI can download on demand and expose as PXE menu entries. Mirrored on dl.bootimus.com so you don\'t depend on upstream availability when kicking off a rescue job. Upstream URLs remain the source of truth — override any mirror URL from the Tools page.',
    apiLede:
      'The admin UI reads /api/manifest.json to check for updates and show available tools. Stable schema — fields won\'t be renamed without a major bump. Source-of-truth for binaries is the GitHub Releases API; this endpoint just normalises and merges in the static bits (Docker tag, appliance image, mirrored tools).',
    mirror: 'mirror ↓',
    upstream: 'upstream ↗',
    get: 'get ↓',
  },

  docs: {
    title: 'Documentation',
    subtitle: 'Everything you need to deploy, configure, and operate bootimus.',
    sectionsTitle: 'Sections',
    onThisPage: 'On this page',
    prev: 'Previous',
    next: 'Next',
    editOnGithub: 'Edit on GitHub',
    notFound: 'Doc not found.',
    fallbackBanner: 'This page hasn\'t been translated yet. Showing the English version.',
    translateCta: 'Help translate →',
    pending: 'translation pending',
  },
} as const;
