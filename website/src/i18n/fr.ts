const fr = {
  meta: {
    title: 'Bootimus — Serveur de boot PXE/HTTP moderne',
    description:
      'Serveur de boot PXE et HTTP autonome. Un binaire. Zéro config. 50+ distros out-of-the-box.',
    downloadsTitle: 'Téléchargements — Bootimus',
    downloadsDescription:
      'Télécharger les binaires Bootimus, les images Docker et les liens vers les outils PXE.',
    docsTitle: 'Documentation — Bootimus',
    docsDescription:
      'Documentation Bootimus : déploiement, gestion des images, DHCP, authentification, ACL client, profils de distro.',
  },

  nav: {
    features: 'Fonctionnalités',
    howItWorks: 'Fonctionnement',
    downloads: 'Téléchargements',
    docs: 'Docs',
    github: 'GitHub',
    toggleTheme: 'Changer de thème',
    languageLabel: 'Langue',
    comingSoon: 'bientôt',
    madeInBritain: 'made in britain',
  },

  footer: {
    project: 'projet',
    licence: 'licence',
    licenceValue: 'Apache 2.0',
    lang: 'langage',
    repo: 'repo',
    docker: 'docker',
    docs: 'docs',
    copy: '© {year} contributeurs bootimus',
  },

  hero: {
    badgeStable: 'v1.x · apache 2.0',
    badgeStack: 'go · iPXE · sqlite/postgres',
    titleLine1: 'Boot PXE,',
    titleLine2: 'sans prise de tête',
    sub: 'Serveur de boot PXE et HTTP autonome. Un seul binaire. Zéro config. proxyDHCP intégré, tu ne touches jamais à ton routeur. 50+ distros détectées automatiquement.',
    ctaPrimary: '$ get bootimus',
    ctaSecondary: 'voir le code source',
    quickstartTitle: 'bootimus — démarrage rapide',
    statDistrosN: '50+',
    statDistrosL: 'distros détectées',
    statBinaryN: '1',
    statBinaryL: 'binaire, zéro dépendance',
    statReconfigsN: '0',
    statReconfigsL: 'DHCP à reconfigurer',
    statArchN: '2',
    statArchL: 'arch : amd64 · arm64',
  },

  features: {
    kicker: '// fonctionnalités',
    title: 'Tout ce qu\'un setup netboot moderne doit être.',
    sub: 'Pas un fork de scripts Perl vieux de 15 ans. Pas un wrapper autour de dnsmasq. Un vrai serveur, écrit en Go, batteries incluses.',
    items: {
      '01': {
        title: 'Un seul binaire',
        body: 'Binaire Go avec iPXE embarqué, UI web, SQLite et tous les assets. Aucune dépendance à l\'exécution. scp et go.',
      },
      '02': {
        title: 'proxyDHCP intégré',
        body: 'Répond au PXE sur UDP/67 sans toucher à ton DHCP existant. Zéro reconfiguration du routeur. Plug & play sur n\'importe quel LAN.',
      },
      '03': {
        title: '50+ distros',
        body: 'Extraction automatique du kernel/initrd pour Ubuntu, Debian, Arch, Fedora, NixOS, Alpine, FreeBSD, Windows (wimboot) et plus. Netboot OpenBSD et NetBSD et médias de secours WinPE aussi.',
      },
      '04': {
        title: 'ACL par MAC',
        body: 'Assigne des images spécifiques par MAC. Les nouveaux clients sont auto-détectés au premier PXE. Passe les leases en statique quand c\'est prêt.',
      },
      '05': {
        title: 'Outils en un clic',
        body: 'GParted, Clonezilla, Memtest86+, SystemRescue, ShredOS, netboot.xyz. Active-les depuis l\'UI, ils apparaissent dans le menu.',
      },
      '06': {
        title: 'JWT + LDAP + OIDC + 2FA',
        body: 'Auth par token avec bcrypt, double facteur TOTP et verrouillage après échecs répétés. Connexion optionnelle par LDAP/AD ou OpenID Connect avec admin par groupes. Tokens d\'API à portée limitée pour les scripts.',
      },
      '07': {
        title: 'API REST + OpenAPI',
        body: 'Tout ce que fait l\'UI passe par un appel API, décrit dans un document OpenAPI généré. Assignations de boot scriptables, scans, WoL. Flux de logs live en SSE. Chaque changement admin finit dans le journal d\'audit.',
      },
      '08': {
        title: 'Tourne partout',
        body: 'Docker multi-architecture (amd64/arm64) ou binaire Linux statique.',
      },
      '09': {
        title: 'Installations sans surveillance',
        body: 'Déposez autounattend.xml, kickstart, preseed ou cloud-init. Attachez à une image par défaut, surchargez par client. Bootimus l\'injecte au boot — zéro clic, zéro assistant.',
      },
      '10': {
        title: 'Imagerie disque',
        body: 'Capture le disque d\'un client avec le Clonezilla intégré et déploie-le sur une machine ou un groupe entier au prochain boot PXE. Les jobs rendent compte à la fin.',
      },
      '11': {
        title: 'Pilotage lights-out',
        body: 'Allume, éteint et redémarre via Redfish ou Intel AMT, ouvre une console Serial-over-LAN AMT ou IPMI dans le navigateur, et suis chaque requête PXE sur la page de debug.',
      },
      '12': {
        title: 'Multi-site',
        body: 'Un primaire où tu gères tout et des répliques en lecture seule sur chaque site qui le reflètent et bootent les clients locaux depuis des copies locales.',
      },
    },
  },

  howItWorks: {
    kicker: '// fonctionnement',
    title: 'Le cycle de vie d\'un boot PXE.',
    sub: 'Le client envoie un DHCPDISCOVER. Bootimus répond avec les détails PXE via proxyDHCP pendant que ton DHCP habituel distribue toujours l\'IP. iPXE charge via TFTP, chain vers HTTP, récupère le menu. L\'utilisateur choisit une image. Kernel et initrd streament depuis le serveur. Fin.',
    termTitle: 'pxe boot trace — ubuntu-24.04',
  },

  transparency: {
    kicker: '// transparence',
    title: '100% ouvert. Auditable de bout en bout.',
    sub: 'Pas de blobs propriétaires. Pas de télémétrie. Pas de firmware binaire planqué dans un vendor/. Tout le stack est sur GitHub sous Apache 2.0 — clone, audite, fork, auto-héberge.',
    items: {
      binary: {
        t: 'Un seul binaire Go',
        d: 'lié statiquement, ldd renvoie "not a dynamic executable". Builds reproductibles via make release.',
      },
      blobs: {
        t: 'Zéro blob propriétaire',
        d: 'l\'iPXE embarqué est du FOSS upstream (GPL-2.0). Aucun firmware closed-source embarqué.',
      },
      telemetry: {
        t: 'Aucune télémétrie, jamais',
        d: 'zéro call-home. Zéro analytics. Zéro "stats d\'usage anonymes". Compatible air-gap.',
      },
      licence: {
        t: 'Apache 2.0',
        d: 'licence permissive. Utilise-le en commercial, distribue-le en interne, forke-le sans contraintes.',
      },
      deps: {
        t: 'Dépendances vendored, toutes FOSS',
        d: 'chaque dépendance Go transitive est open source. go mod why sur n\'importe quel package.',
      },
      byo: {
        t: 'Apporte ton propre bootloader',
        d: 'pas confiance dans l\'iPXE embarqué ? Dépose tes binaires signés. Voir plus bas.',
      },
    },
    termTitle: 'bootimus version --verbose',
  },

  bootloaders: {
    kicker: '// bootloaders',
    title: 'Remplace iPXE par ce qu\'il te faut.',
    sub: 'Bootimus embarque iPXE pour chaque architecture courante — y compris une chaîne Secure Boot signée Microsoft. Besoin d\'un iPXE customisé, de GRUB, syslinux, ou de ton loader signé par ta CA interne ? Dépose un dossier dans data/bootloaders/, sélectionne-le dans l\'UI, c\'est fait. Les fichiers manquants basculent transparent sur le set embarqué — jamais un boot cassé.',
    cards: {
      uefi64: {
        t: 'iPXE · UEFI x86_64',
        d: 'ipxe.efi · la valeur par défaut. Buildé depuis upstream master, embarqué dans le binaire.',
        tag: 'embarqué · fallback',
      },
      uefiArm: {
        t: 'iPXE · UEFI ARM64',
        d: 'ipxe-arm64.efi · pour les Raspberry Pi 4/5, les hôtes Apple Silicon, les serveurs ARM.',
        tag: 'embarqué · fallback',
      },
      bios: {
        t: 'iPXE · BIOS legacy',
        d: 'undionly.kpxe · pour le vieux matos qui ne veut rien savoir de l\'UEFI. Toujours d\'actualité en 2026.',
        tag: 'embarqué · fallback',
      },
      shim: {
        t: 'Secure Boot · shim signé',
        d: 'ipxe-shimx64.efi · shim signé Microsoft + iPXE signé pour les flottes en Secure Boot obligatoire. Pas besoin d\'enrôlement MOK dans le firmware.',
        tag: 'embarqué · secureboot-official',
      },
      themed: {
        t: 'iPXE customisé',
        d: 'Compile ton propre iPXE avec branding, couleurs de menu perso, scripts embarqués. Dépose le .efi.',
        tag: 'custom · BYO',
      },
      grub: {
        t: 'GRUB / syslinux / pxelinux',
        d: 'Pas d\'iPXE ? Pas de souci. Tout ce qui parle TFTP et HTTP fonctionne. Bootimus sert juste des octets.',
        tag: 'custom · BYO',
      },
    },
    termTitle: 'sets de bootloader — file fallthrough',
  },

  cta: {
    title: 'Marre de babysitter tftpd ?',
    sub: 'Exécutez Bootimus dans Docker ou directement sous Linux.',
    primary: '$ get bootimus',
    secondary: 'lire la doc →',
  },

  downloads: {
    kicker: '// téléchargements',
    title: 'Chope un build.',
    lede:
      'Bootimus est disponible sous forme d’image Docker et de binaires Linux statiques sur GitHub Releases. Les outils de secours se téléchargent sur les sites de leurs projets.',
    badgeStable: 'latest · stable',
    badgePrerelease: 'pre-release',
    badgeNone: 'aucune release pour l\'instant',
    pillManifest: 'manifest.json',
    pillSource: 'github releases ↗',
    pillBuildSrc: 'builder depuis les sources',
    released: 'publié le',
    via: 'via',
    sectionArtifacts: 'artefacts de release',
    sectionTools: 'outils de secours',
    sectionApi: 'utiliser le manifest',
    emptyTitle: 'Aucun binaire publié pour l\'instant',
    emptyBody:
      'Pose un tag sur GitHub et les binaires de make release apparaîtront ici automatiquement.',
    buildFromSource: 'builder depuis les sources ↗',
    verifyTitle: 'vérifier',
    apiTitle: 'api · manifest.json',
    toolsLede:
      'Ces outils peuvent être ajoutés à Bootimus. Les sites des projets proposent les téléchargements, les versions actuelles et les sommes de contrôle.',
    apiLede:
      'Le manifeste répertorie les fichiers des versions GitHub, l’image Docker et les liens des projets. Un champ mirror vide indique qu’aucun miroir du projet n’est disponible.',
    mirror: 'mirror ↓',
    upstream: 'upstream ↗',
    get: 'get ↓',
  },

  docs: {
    title: 'Documentation',
    subtitle: 'Tout ce qu\'il te faut pour déployer, configurer et opérer bootimus.',
    sectionsTitle: 'Sections',
    onThisPage: 'Sur cette page',
    prev: 'Précédent',
    next: 'Suivant',
    editOnGithub: 'Éditer sur GitHub',
    notFound: 'Document introuvable.',
    fallbackBanner: 'Cette page n\'est pas encore traduite. Affichage de la version anglaise.',
    translateCta: 'Aider à traduire →',
    pending: 'traduction en attente',
  },
};

export default fr;
