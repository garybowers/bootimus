const TRANSLATIONS = {
    'en-GB': {
        'app.title': 'Bootimus Admin Panel',
        'app.subtitle': 'PXE/HTTP Boot Server Management Interface',
        'app.product_subtitle': 'PXE/HTTP Boot Server',

        'nav.group.overview': 'Overview',
        'nav.group.management': 'Management',
        'nav.group.boot': 'Boot',
        'nav.group.access': 'Access',
        'nav.group.system': 'System',

        'nav.server': 'Server Info',
        'nav.images': 'Images',
        'nav.public_files': 'Public Files',
        'nav.autoinstall': 'Auto-Install',
        'nav.bootloaders': 'Bootloaders',
        'nav.tools': 'Tools',
        'nav.profiles': 'Distro Profiles',
        'nav.clients': 'Clients',
        'nav.users': 'Users',
        'nav.settings': 'Settings',
        'nav.logs': 'Logs',

        'common.cancel': 'Cancel',
        'common.save': 'Save',
        'common.delete': 'Delete',
        'common.close': 'Close',
        'common.upload': 'Upload',
        'common.refresh': 'Refresh',
        'common.refresh_tooltip': 'Reload',
        'common.enabled': 'Enabled',
        'common.loading': 'Loading...',

        'login.auth_method': 'Authentication',
        'login.username': 'Username',
        'login.password': 'Password',
        'login.signin': 'Sign In',

        'about.website': 'Website',
        'about.github': 'GitHub',
        'about.report_issues': 'Report Issues',
        'about.docker_hub': 'Docker Hub',
        'about.licence': 'Licence',

        'user.logout': 'Logout',
        'user.role.user': 'User',
        'user.role.admin': 'Admin',

        'ui.toggle_dark_mode': 'Toggle dark mode',

        'server.info.title': 'Server Information',
        'server.info.loading': 'Loading server info...',
        'sessions.title': 'Active Sessions',

        'server.section.running_status': 'Running Status',
        'server.section.system_resources': 'System Resources',
        'server.section.configuration': 'Configuration',
        'server.section.environment': 'Environment',

        'server.field.version': 'Version',
        'server.field.uptime': 'Uptime',
        'server.field.runtime_mode': 'Runtime Mode',
        'server.field.os': 'OS',
        'server.field.arch': 'Arch',

        'server.metric.cpu': 'CPU Usage',
        'server.metric.memory': 'Memory',
        'server.metric.disk': 'Disk',
        'server.metric.cores_available': '{{n}} cores available',
        'server.metric.memory_detail': '{{used}} used of {{total}}',
        'server.metric.disk_detail': '{{free}} free of {{total}}',

        'server.env.empty': 'No environment variables set',

        'server.config.boot_directory': 'Boot Directory',
        'server.config.data_directory': 'Data Directory',
        'server.config.database_mode': 'Database Mode',
        'server.config.http_port': 'HTTP Port',
        'server.config.iso_directory': 'ISO Directory',
        'server.config.ldap_enabled': 'LDAP Enabled',
        'server.config.proxy_dhcp': 'Proxy DHCP',
        'server.config.windows_smb': 'Windows SMB',
        'server.config.windows_smb_patcher': 'Windows SMB Patcher',

        'stats.total_clients': 'Total Clients',
        'stats.active_clients': 'Active Clients',
        'stats.total_images': 'Total Images',
        'stats.enabled_images': 'Enabled Images',
        'stats.total_boots': 'Total Boots',

        'page.clients.title': 'Client Management',
        'page.clients.intro': 'Manage PXE boot clients. Discovered clients appear automatically when they connect.',
        'page.client_groups.title': 'Client Groups',
        'page.client_groups.intro': 'Group clients by location, role, or fleet. Bulk wake all members, set a next-boot image on all, and overlay assigned images.',
        'page.images.title': 'Image Management',
        'page.images.intro': 'Upload, download, and manage OS images available for PXE booting.',
        'page.public_files.title': 'Public Files',
        'page.public_files.intro': 'Files available to all clients. For image-specific files, manage them from the Images tab.',
        'page.users.title': 'User Management',
        'page.users.intro': 'Manage admin panel user accounts and permissions.',
        'page.tools.title': 'Boot Tools',
        'page.tools.intro': 'Download a tool to make it available, then enable it to show in the boot menu.',
        'page.profiles.title': 'Distro Profiles',
        'page.profiles.intro': 'Distro profiles define how ISOs are detected and booted. Built-in profiles are updated from the central repository. Custom profiles take priority.',
        'page.autoinstall.title': 'Auto-Install Files',
        'page.autoinstall.intro': 'Install operating systems without manual prompts. Windows uses autounattend.xml; Ubuntu uses cloud-init; Red Hat and its family use kickstart; Debian uses preseed. Attach a file to an image for a default config, or override it per client for machine-specific setups.',
        'page.bootloaders.title': 'Bootloader Management',
        'page.bootloaders.intro': 'Manage iPXE bootloader sets served to PXE clients. Create custom sets with your own bootloader files.',
        'page.logs.title': 'Server Logs',
        'page.logs.intro': 'Live server log output for TFTP, HTTP, and PXE boot activity.',

        'props.modal.title': 'Image Properties',
        'props.tab.properties': 'Properties',
        'props.tab.autoinstall': 'Auto-Install',
        'props.tab.files': 'Files',

        'props.field.display_name': 'Display Name',
        'props.field.description': 'Description',
        'props.field.group': 'Group Assignment',
        'props.field.group_unassigned': 'Unassigned',
        'props.field.order': 'Order',
        'props.field.boot_method': 'Boot Method',
        'props.field.distro': 'Distro Profile',
        'props.field.distro_auto': 'Auto-detect',
        'props.field.boot_params': 'Boot Parameters',
        'props.field.boot_params_placeholder': 'Optional kernel parameters',
        'props.field.boot_params_hint': 'Leave empty for distro defaults.',
        'props.field.placeholders_label': 'Placeholders:',
        'props.field.public': 'Public (available to all clients)',
        'props.field.default_autoinstall': 'Default Auto-Install File',
        'props.field.autoinstall_none': '(None — manual install)',
        'props.field.autoinstall_hint': "Only files matching this image's distro are shown. Manage files in the Auto-Install section.",

        'props.boot_method.sanboot': 'SAN Boot',
        'props.boot_method.kernel': 'Kernel/Initrd',
        'props.boot_method.nbd': 'NBD Mount',

        'props.action.extract': 'Extract',
        'props.action.re_extract': 'Re-Extract',
        'props.action.extracting': 'Extracting...',
        'props.action.extract_now': 'Extract now',
        'props.action.patch_smb': 'Patch SMB',
        'props.action.re_patch_smb': 'Re-patch SMB',
        'props.action.patching': 'Patching...',
        'props.action.save_and_repatch': 'Save & Re-patch',
        'props.action.redetect': 'Re-detect',
        'props.action.download_iso': 'Download ISO',
        'props.action.download_netboot': 'Download netboot files',
        'props.action.download_netboot_tooltip': 'Download the kernel/initrd netboot bundle from the distro mirror',
        'props.action.open_settings': 'Open Settings',
        'props.action.save_properties': 'Save Properties',

        'props.notify.patch_success': 'boot.wim patched for SMB auto-install',
        'props.notify.patch_failed': 'Patch failed',

        'props.warn.not_extracted': "Not extracted yet. This OS likely won't boot reliably without extraction — kernel and initrd need to be served directly.",
        'props.warn.netboot_required': "Netboot files required. This Debian/Ubuntu DVD ISO ships an installer that needs a separate kernel/initrd bundle from the distro mirror — it can't boot directly from the ISO contents.",
        'props.warn.smb_disabled': 'Server SMB share is disabled. Windows installs need it. Enable Windows SMB in Settings.',
        'props.warn.wimlib_missing': 'wimlib-imagex is not installed on the server. boot.wim cannot be patched without it. Install wimtools (or wimlib-imagex) on the host running bootimus, then restart.',
        'props.warn.repatch': 'Re-patch needed. Auto-install changed since boot.wim was last embedded — save and re-patch SMB so the change takes effect.',

        'lang.label': 'Language',
    },

    de: {
        'app.title': 'Bootimus Verwaltung',
        'app.subtitle': 'Verwaltungsoberfläche für PXE/HTTP-Bootserver',
        'app.product_subtitle': 'PXE/HTTP-Bootserver',

        'nav.group.overview': 'Überblick',
        'nav.group.management': 'Verwaltung',
        'nav.group.boot': 'Boot',
        'nav.group.access': 'Zugriff',
        'nav.group.system': 'System',

        'nav.server': 'Serverinfo',
        'nav.images': 'Abbilder',
        'nav.public_files': 'Öffentliche Dateien',
        'nav.autoinstall': 'Auto-Installation',
        'nav.bootloaders': 'Bootloader',
        'nav.tools': 'Werkzeuge',
        'nav.profiles': 'Distro-Profile',
        'nav.clients': 'Clients',
        'nav.users': 'Benutzer',
        'nav.settings': 'Einstellungen',
        'nav.logs': 'Protokolle',

        'common.cancel': 'Abbrechen',
        'common.save': 'Speichern',
        'common.delete': 'Löschen',
        'common.close': 'Schließen',
        'common.upload': 'Hochladen',
        'common.refresh': 'Aktualisieren',
        'common.refresh_tooltip': 'Neu laden',
        'common.enabled': 'Aktiviert',
        'common.loading': 'Wird geladen…',

        'login.auth_method': 'Authentifizierung',
        'login.username': 'Benutzername',
        'login.password': 'Passwort',
        'login.signin': 'Anmelden',

        'about.website': 'Website',
        'about.github': 'GitHub',
        'about.report_issues': 'Fehler melden',
        'about.docker_hub': 'Docker Hub',
        'about.licence': 'Lizenz',

        'user.logout': 'Abmelden',
        'user.role.user': 'Benutzer',
        'user.role.admin': 'Administrator',

        'ui.toggle_dark_mode': 'Dunkelmodus umschalten',

        'server.info.title': 'Serverinformationen',
        'server.info.loading': 'Serverinfo wird geladen…',
        'sessions.title': 'Aktive Sitzungen',

        'server.section.running_status': 'Betriebsstatus',
        'server.section.system_resources': 'Systemressourcen',
        'server.section.configuration': 'Konfiguration',
        'server.section.environment': 'Umgebung',

        'server.field.version': 'Version',
        'server.field.uptime': 'Laufzeit',
        'server.field.runtime_mode': 'Laufzeitmodus',
        'server.field.os': 'Betriebssystem',
        'server.field.arch': 'Architektur',

        'server.metric.cpu': 'CPU-Auslastung',
        'server.metric.memory': 'Arbeitsspeicher',
        'server.metric.disk': 'Datenträger',
        'server.metric.cores_available': '{{n}} Kerne verfügbar',
        'server.metric.memory_detail': '{{used}} von {{total}} belegt',
        'server.metric.disk_detail': '{{free}} frei von {{total}}',

        'server.env.empty': 'Keine Umgebungsvariablen gesetzt',

        'server.config.boot_directory': 'Boot-Verzeichnis',
        'server.config.data_directory': 'Datenverzeichnis',
        'server.config.database_mode': 'Datenbankmodus',
        'server.config.http_port': 'HTTP-Port',
        'server.config.iso_directory': 'ISO-Verzeichnis',
        'server.config.ldap_enabled': 'LDAP aktiviert',
        'server.config.proxy_dhcp': 'Proxy-DHCP',
        'server.config.windows_smb': 'Windows-SMB',
        'server.config.windows_smb_patcher': 'Windows-SMB-Patcher',

        'stats.total_clients': 'Clients gesamt',
        'stats.active_clients': 'Aktive Clients',
        'stats.total_images': 'Abbilder gesamt',
        'stats.enabled_images': 'Aktivierte Abbilder',
        'stats.total_boots': 'Boots gesamt',

        'page.clients.title': 'Client-Verwaltung',
        'page.clients.intro': 'PXE-Boot-Clients verwalten. Gefundene Clients erscheinen automatisch, sobald sie sich verbinden.',
        'page.client_groups.title': 'Client-Gruppen',
        'page.client_groups.intro': 'Clients nach Standort, Rolle oder Flotte gruppieren. Alle Mitglieder gleichzeitig per Wake-on-LAN starten, ein Boot-Abbild für alle festlegen und zugeordnete Abbilder überlagern.',
        'page.images.title': 'Abbild-Verwaltung',
        'page.images.intro': 'Betriebssystem-Abbilder für den PXE-Start hochladen, herunterladen und verwalten.',
        'page.public_files.title': 'Öffentliche Dateien',
        'page.public_files.intro': 'Dateien, die allen Clients zur Verfügung stehen. Abbild-spezifische Dateien werden im Bereich „Abbilder“ verwaltet.',
        'page.users.title': 'Benutzerverwaltung',
        'page.users.intro': 'Benutzerkonten und Berechtigungen für die Verwaltungsoberfläche verwalten.',
        'page.tools.title': 'Boot-Werkzeuge',
        'page.tools.intro': 'Ein Werkzeug herunterladen, um es verfügbar zu machen, und anschließend aktivieren, damit es im Boot-Menü erscheint.',
        'page.profiles.title': 'Distro-Profile',
        'page.profiles.intro': 'Distro-Profile legen fest, wie ISOs erkannt und gestartet werden. Eingebaute Profile werden zentral aktualisiert. Eigene Profile haben Vorrang.',
        'page.autoinstall.title': 'Auto-Installations-Dateien',
        'page.autoinstall.intro': 'Betriebssysteme ohne manuelle Eingaben installieren. Windows nutzt autounattend.xml, Ubuntu cloud-init, Red Hat und Verwandte kickstart, Debian preseed. Als Standardkonfiguration an ein Abbild anhängen oder pro Client für einzelne Rechner überschreiben.',
        'page.bootloaders.title': 'Bootloader-Verwaltung',
        'page.bootloaders.intro': 'iPXE-Bootloader-Sätze verwalten, die an PXE-Clients ausgeliefert werden. Eigene Sätze mit eigenen Bootloader-Dateien anlegen.',
        'page.logs.title': 'Server-Protokolle',
        'page.logs.intro': 'Live-Protokollausgabe des Servers für TFTP-, HTTP- und PXE-Boot-Aktivität.',

        'props.modal.title': 'Abbild-Eigenschaften',
        'props.tab.properties': 'Eigenschaften',
        'props.tab.autoinstall': 'Auto-Installation',
        'props.tab.files': 'Dateien',

        'props.field.display_name': 'Anzeigename',
        'props.field.description': 'Beschreibung',
        'props.field.group': 'Gruppenzuordnung',
        'props.field.group_unassigned': 'Nicht zugeordnet',
        'props.field.order': 'Reihenfolge',
        'props.field.boot_method': 'Boot-Methode',
        'props.field.distro': 'Distro-Profil',
        'props.field.distro_auto': 'Automatisch erkennen',
        'props.field.boot_params': 'Boot-Parameter',
        'props.field.boot_params_placeholder': 'Optionale Kernel-Parameter',
        'props.field.boot_params_hint': 'Leer lassen für Distro-Standardwerte.',
        'props.field.placeholders_label': 'Platzhalter:',
        'props.field.public': 'Öffentlich (für alle Clients verfügbar)',
        'props.field.default_autoinstall': 'Standard-Auto-Installations-Datei',
        'props.field.autoinstall_none': '(Keine — manuelle Installation)',
        'props.field.autoinstall_hint': 'Nur Dateien, die zur Distro dieses Abbilds passen, werden angezeigt. Dateien werden im Bereich „Auto-Installation“ verwaltet.',

        'props.boot_method.sanboot': 'SAN-Boot',
        'props.boot_method.kernel': 'Kernel/Initrd',
        'props.boot_method.nbd': 'NBD-Mount',

        'props.action.extract': 'Extrahieren',
        'props.action.re_extract': 'Neu extrahieren',
        'props.action.extracting': 'Wird extrahiert…',
        'props.action.extract_now': 'Jetzt extrahieren',
        'props.action.patch_smb': 'SMB patchen',
        'props.action.re_patch_smb': 'SMB neu patchen',
        'props.action.patching': 'Wird gepatcht…',
        'props.action.save_and_repatch': 'Speichern & neu patchen',
        'props.action.redetect': 'Neu erkennen',
        'props.action.download_iso': 'ISO herunterladen',
        'props.action.download_netboot': 'Netboot-Dateien herunterladen',
        'props.action.download_netboot_tooltip': 'Kernel-/Initrd-Netboot-Bundle vom Distro-Spiegel herunterladen',
        'props.action.open_settings': 'Einstellungen öffnen',
        'props.action.save_properties': 'Eigenschaften speichern',

        'props.notify.patch_success': 'boot.wim für SMB-Auto-Installation gepatcht',
        'props.notify.patch_failed': 'Patch fehlgeschlagen',

        'props.warn.not_extracted': 'Noch nicht extrahiert. Dieses Betriebssystem startet ohne Extraktion vermutlich nicht zuverlässig — Kernel und Initrd müssen direkt ausgeliefert werden.',
        'props.warn.netboot_required': 'Netboot-Dateien erforderlich. Diese Debian-/Ubuntu-DVD-ISO bringt einen Installer mit, der ein eigenes Kernel-/Initrd-Bundle vom Distro-Spiegel benötigt — sie kann nicht direkt aus dem ISO-Inhalt starten.',
        'props.warn.smb_disabled': 'Die Server-SMB-Freigabe ist deaktiviert. Windows-Installationen benötigen sie. Aktivieren Sie Windows-SMB in den Einstellungen.',
        'props.warn.wimlib_missing': 'wimlib-imagex ist nicht auf dem Server installiert. boot.wim kann ohne dieses Werkzeug nicht gepatcht werden. Installieren Sie wimtools (oder wimlib-imagex) auf dem Bootimus-Host und starten Sie ihn neu.',
        'props.warn.repatch': 'Neuer Patch erforderlich. Die Auto-Installation hat sich seit dem letzten Einbetten in boot.wim geändert — speichern und SMB neu patchen, damit die Änderung wirksam wird.',

        'lang.label': 'Sprache',
    },

    fr: {
        'app.title': "Console d'administration Bootimus",
        'app.subtitle': 'Interface de gestion du serveur de démarrage PXE/HTTP',
        'app.product_subtitle': 'Serveur de démarrage PXE/HTTP',

        'nav.group.overview': 'Aperçu',
        'nav.group.management': 'Gestion',
        'nav.group.boot': 'Démarrage',
        'nav.group.access': 'Accès',
        'nav.group.system': 'Système',

        'nav.server': 'Infos serveur',
        'nav.images': 'Images',
        'nav.public_files': 'Fichiers publics',
        'nav.autoinstall': 'Installation auto',
        'nav.bootloaders': "Chargeurs d'amorçage",
        'nav.tools': 'Outils',
        'nav.profiles': 'Profils de distribution',
        'nav.clients': 'Clients',
        'nav.users': 'Utilisateurs',
        'nav.settings': 'Paramètres',
        'nav.logs': 'Journaux',

        'common.cancel': 'Annuler',
        'common.save': 'Enregistrer',
        'common.delete': 'Supprimer',
        'common.close': 'Fermer',
        'common.upload': 'Téléverser',
        'common.refresh': 'Actualiser',
        'common.refresh_tooltip': 'Recharger',
        'common.enabled': 'Activé',
        'common.loading': 'Chargement…',

        'login.auth_method': 'Authentification',
        'login.username': "Nom d'utilisateur",
        'login.password': 'Mot de passe',
        'login.signin': 'Connexion',

        'about.website': 'Site web',
        'about.github': 'GitHub',
        'about.report_issues': 'Signaler un problème',
        'about.docker_hub': 'Docker Hub',
        'about.licence': 'Licence',

        'user.logout': 'Déconnexion',
        'user.role.user': 'Utilisateur',
        'user.role.admin': 'Administrateur',

        'ui.toggle_dark_mode': 'Basculer le mode sombre',

        'server.info.title': 'Informations du serveur',
        'server.info.loading': 'Chargement des informations du serveur…',
        'sessions.title': 'Sessions actives',

        'server.section.running_status': 'État de fonctionnement',
        'server.section.system_resources': 'Ressources système',
        'server.section.configuration': 'Configuration',
        'server.section.environment': 'Environnement',

        'server.field.version': 'Version',
        'server.field.uptime': 'Durée de fonctionnement',
        'server.field.runtime_mode': "Mode d'exécution",
        'server.field.os': 'OS',
        'server.field.arch': 'Architecture',

        'server.metric.cpu': 'Utilisation du processeur',
        'server.metric.memory': 'Mémoire',
        'server.metric.disk': 'Disque',
        'server.metric.cores_available': '{{n}} cœurs disponibles',
        'server.metric.memory_detail': '{{used}} utilisés sur {{total}}',
        'server.metric.disk_detail': '{{free}} libres sur {{total}}',

        'server.env.empty': "Aucune variable d'environnement définie",

        'server.config.boot_directory': 'Répertoire de démarrage',
        'server.config.data_directory': 'Répertoire de données',
        'server.config.database_mode': 'Mode base de données',
        'server.config.http_port': 'Port HTTP',
        'server.config.iso_directory': 'Répertoire ISO',
        'server.config.ldap_enabled': 'LDAP activé',
        'server.config.proxy_dhcp': 'Proxy DHCP',
        'server.config.windows_smb': 'SMB Windows',
        'server.config.windows_smb_patcher': 'Patcheur SMB Windows',

        'stats.total_clients': 'Total des clients',
        'stats.active_clients': 'Clients actifs',
        'stats.total_images': 'Total des images',
        'stats.enabled_images': 'Images activées',
        'stats.total_boots': 'Total des démarrages',

        'page.clients.title': 'Gestion des clients',
        'page.clients.intro': 'Gérez les clients de démarrage PXE. Les clients détectés apparaissent automatiquement dès leur connexion.',
        'page.client_groups.title': 'Groupes de clients',
        'page.client_groups.intro': 'Regroupez les clients par site, rôle ou flotte. Réveillez tous les membres en masse, définissez une image de démarrage pour tous et superposez les images attribuées.',
        'page.images.title': "Gestion des images",
        'page.images.intro': "Téléversez, téléchargez et gérez les images de système d'exploitation disponibles pour le démarrage PXE.",
        'page.public_files.title': 'Fichiers publics',
        'page.public_files.intro': "Fichiers accessibles à tous les clients. Pour les fichiers propres à une image, gérez-les depuis l'onglet Images.",
        'page.users.title': 'Gestion des utilisateurs',
        'page.users.intro': "Gérez les comptes utilisateurs et les permissions de la console d'administration.",
        'page.tools.title': 'Outils de démarrage',
        'page.tools.intro': "Téléchargez un outil pour le rendre disponible, puis activez-le pour qu'il apparaisse dans le menu de démarrage.",
        'page.profiles.title': 'Profils de distribution',
        'page.profiles.intro': "Les profils de distribution définissent comment les ISO sont détectées et démarrées. Les profils intégrés sont mis à jour depuis le dépôt central. Les profils personnalisés sont prioritaires.",
        'page.autoinstall.title': "Fichiers d'installation automatique",
        'page.autoinstall.intro': "Installez les systèmes d'exploitation sans intervention manuelle. Windows utilise autounattend.xml ; Ubuntu utilise cloud-init ; Red Hat et sa famille utilisent kickstart ; Debian utilise preseed. Attachez un fichier à une image pour une configuration par défaut, ou surchargez-le par client pour des installations spécifiques à une machine.",
        'page.bootloaders.title': "Gestion des chargeurs d'amorçage",
        'page.bootloaders.intro': "Gérez les jeux de chargeurs iPXE servis aux clients PXE. Créez des jeux personnalisés avec vos propres fichiers de chargeur d'amorçage.",
        'page.logs.title': 'Journaux du serveur',
        'page.logs.intro': "Sortie en direct des journaux du serveur pour l'activité TFTP, HTTP et de démarrage PXE.",

        'props.modal.title': "Propriétés de l'image",
        'props.tab.properties': 'Propriétés',
        'props.tab.autoinstall': 'Installation auto',
        'props.tab.files': 'Fichiers',

        'props.field.display_name': "Nom d'affichage",
        'props.field.description': 'Description',
        'props.field.group': 'Affectation au groupe',
        'props.field.group_unassigned': 'Non affecté',
        'props.field.order': 'Ordre',
        'props.field.boot_method': 'Méthode de démarrage',
        'props.field.distro': 'Profil de distribution',
        'props.field.distro_auto': 'Détection automatique',
        'props.field.boot_params': 'Paramètres de démarrage',
        'props.field.boot_params_placeholder': 'Paramètres noyau optionnels',
        'props.field.boot_params_hint': 'Laissez vide pour utiliser les valeurs par défaut de la distribution.',
        'props.field.placeholders_label': 'Espaces réservés :',
        'props.field.public': 'Public (disponible pour tous les clients)',
        'props.field.default_autoinstall': "Fichier d'installation auto par défaut",
        'props.field.autoinstall_none': '(Aucun — installation manuelle)',
        'props.field.autoinstall_hint': "Seuls les fichiers correspondant à la distribution de cette image sont affichés. Gérez les fichiers dans la section Installation auto.",

        'props.boot_method.sanboot': 'Démarrage SAN',
        'props.boot_method.kernel': 'Noyau/Initrd',
        'props.boot_method.nbd': 'Montage NBD',

        'props.action.extract': 'Extraire',
        'props.action.re_extract': 'Ré-extraire',
        'props.action.extracting': 'Extraction…',
        'props.action.extract_now': 'Extraire maintenant',
        'props.action.patch_smb': 'Patcher SMB',
        'props.action.re_patch_smb': 'Re-patcher SMB',
        'props.action.patching': 'Application du patch…',
        'props.action.save_and_repatch': 'Enregistrer & re-patcher',
        'props.action.redetect': 'Re-détecter',
        'props.action.download_iso': "Télécharger l'ISO",
        'props.action.download_netboot': 'Télécharger les fichiers netboot',
        'props.action.download_netboot_tooltip': 'Télécharger le bundle noyau/initrd netboot depuis le miroir de la distribution',
        'props.action.open_settings': 'Ouvrir les paramètres',
        'props.action.save_properties': 'Enregistrer les propriétés',

        'props.notify.patch_success': "boot.wim patché pour l'installation auto SMB",
        'props.notify.patch_failed': 'Échec du patch',

        'props.warn.not_extracted': "Pas encore extraite. Sans extraction, ce système ne démarrera probablement pas correctement — le noyau et l'initrd doivent être servis directement.",
        'props.warn.netboot_required': "Fichiers netboot requis. Cette ISO DVD Debian/Ubuntu embarque un installeur qui nécessite un bundle noyau/initrd séparé depuis le miroir de la distribution — elle ne peut pas démarrer directement à partir du contenu de l'ISO.",
        'props.warn.smb_disabled': "Le partage SMB du serveur est désactivé. Les installations Windows en ont besoin. Activez Windows SMB dans les paramètres.",
        'props.warn.wimlib_missing': "wimlib-imagex n'est pas installé sur le serveur. boot.wim ne peut pas être patché sans cet outil. Installez wimtools (ou wimlib-imagex) sur l'hôte exécutant bootimus, puis redémarrez.",
        'props.warn.repatch': "Re-patch nécessaire. L'installation auto a changé depuis le dernier patch de boot.wim — enregistrez et re-patchez SMB pour appliquer la modification.",

        'lang.label': 'Langue',
    },

    es: { 'lang.label': 'Idioma' },
    it: { 'lang.label': 'Lingua' },
    nl: { 'lang.label': 'Taal' },

    ru: {
        'app.title': 'Панель администратора Bootimus',
        'app.subtitle': 'Интерфейс управления сервером PXE/HTTP-загрузки',
        'app.product_subtitle': 'Сервер PXE/HTTP-загрузки',

        'nav.group.overview': 'Обзор',
        'nav.group.management': 'Управление',
        'nav.group.boot': 'Загрузка',
        'nav.group.access': 'Доступ',
        'nav.group.system': 'Система',

        'nav.server': 'Информация о сервере',
        'nav.images': 'Образы',
        'nav.public_files': 'Общие файлы',
        'nav.autoinstall': 'Автоустановка',
        'nav.bootloaders': 'Загрузчики',
        'nav.tools': 'Инструменты',
        'nav.profiles': 'Профили дистрибутивов',
        'nav.clients': 'Клиенты',
        'nav.users': 'Пользователи',
        'nav.settings': 'Настройки',
        'nav.logs': 'Журналы',

        'common.cancel': 'Отмена',
        'common.save': 'Сохранить',
        'common.delete': 'Удалить',
        'common.close': 'Закрыть',
        'common.upload': 'Загрузить',
        'common.refresh': 'Обновить',
        'common.refresh_tooltip': 'Перезагрузить',
        'common.enabled': 'Включено',
        'common.loading': 'Загрузка…',

        'login.auth_method': 'Аутентификация',
        'login.username': 'Имя пользователя',
        'login.password': 'Пароль',
        'login.signin': 'Войти',

        'about.website': 'Сайт',
        'about.github': 'GitHub',
        'about.report_issues': 'Сообщить об ошибке',
        'about.docker_hub': 'Docker Hub',
        'about.licence': 'Лицензия',

        'user.logout': 'Выйти',
        'user.role.user': 'Пользователь',
        'user.role.admin': 'Администратор',

        'ui.toggle_dark_mode': 'Переключить тёмную тему',

        'server.info.title': 'Информация о сервере',
        'server.info.loading': 'Загрузка информации о сервере…',
        'sessions.title': 'Активные сеансы',

        'server.section.running_status': 'Состояние работы',
        'server.section.system_resources': 'Системные ресурсы',
        'server.section.configuration': 'Конфигурация',
        'server.section.environment': 'Окружение',

        'server.field.version': 'Версия',
        'server.field.uptime': 'Время работы',
        'server.field.runtime_mode': 'Режим выполнения',
        'server.field.os': 'ОС',
        'server.field.arch': 'Архитектура',

        'server.metric.cpu': 'Загрузка ЦП',
        'server.metric.memory': 'Память',
        'server.metric.disk': 'Диск',
        'server.metric.cores_available': '{{n}} ядер доступно',
        'server.metric.memory_detail': '{{used}} из {{total}} используется',
        'server.metric.disk_detail': '{{free}} свободно из {{total}}',

        'server.env.empty': 'Переменные окружения не заданы',

        'server.config.boot_directory': 'Каталог загрузки',
        'server.config.data_directory': 'Каталог данных',
        'server.config.database_mode': 'Режим базы данных',
        'server.config.http_port': 'HTTP-порт',
        'server.config.iso_directory': 'Каталог ISO',
        'server.config.ldap_enabled': 'LDAP включён',
        'server.config.proxy_dhcp': 'Proxy DHCP',
        'server.config.windows_smb': 'Windows SMB',
        'server.config.windows_smb_patcher': 'Патчер Windows SMB',

        'stats.total_clients': 'Всего клиентов',
        'stats.active_clients': 'Активные клиенты',
        'stats.total_images': 'Всего образов',
        'stats.enabled_images': 'Активные образы',
        'stats.total_boots': 'Всего загрузок',

        'page.clients.title': 'Управление клиентами',
        'page.clients.intro': 'Управление клиентами PXE-загрузки. Обнаруженные клиенты появляются автоматически при подключении.',
        'page.client_groups.title': 'Группы клиентов',
        'page.client_groups.intro': 'Группируйте клиентов по местоположению, роли или парку. Включайте всех участников одновременно, задавайте образ для следующей загрузки и накладывайте назначенные образы.',
        'page.images.title': 'Управление образами',
        'page.images.intro': 'Загружайте, скачивайте и управляйте образами ОС, доступными для PXE-загрузки.',
        'page.public_files.title': 'Общие файлы',
        'page.public_files.intro': 'Файлы, доступные всем клиентам. Файлы для конкретного образа управляются на вкладке «Образы».',
        'page.users.title': 'Управление пользователями',
        'page.users.intro': 'Управление учётными записями и правами пользователей панели администратора.',
        'page.tools.title': 'Загрузочные инструменты',
        'page.tools.intro': 'Скачайте инструмент, чтобы он стал доступен, затем включите его для отображения в меню загрузки.',
        'page.profiles.title': 'Профили дистрибутивов',
        'page.profiles.intro': 'Профили дистрибутивов определяют, как ISO распознаются и загружаются. Встроенные профили обновляются из центрального репозитория. Пользовательские профили имеют приоритет.',
        'page.autoinstall.title': 'Файлы автоустановки',
        'page.autoinstall.intro': 'Устанавливайте операционные системы без ручного ввода. Windows использует autounattend.xml; Ubuntu — cloud-init; Red Hat и его семейство — kickstart; Debian — preseed. Прикрепите файл к образу для конфигурации по умолчанию или переопределите его для конкретного клиента.',
        'page.bootloaders.title': 'Управление загрузчиками',
        'page.bootloaders.intro': 'Управление наборами загрузчиков iPXE, отправляемых PXE-клиентам. Создавайте пользовательские наборы со своими файлами загрузчика.',
        'page.logs.title': 'Журналы сервера',
        'page.logs.intro': 'Живой вывод журналов сервера для TFTP, HTTP и PXE-загрузки.',

        'props.modal.title': 'Свойства образа',
        'props.tab.properties': 'Свойства',
        'props.tab.autoinstall': 'Автоустановка',
        'props.tab.files': 'Файлы',

        'props.field.display_name': 'Отображаемое имя',
        'props.field.description': 'Описание',
        'props.field.group': 'Привязка к группе',
        'props.field.group_unassigned': 'Не назначено',
        'props.field.order': 'Порядок',
        'props.field.boot_method': 'Метод загрузки',
        'props.field.distro': 'Профиль дистрибутива',
        'props.field.distro_auto': 'Автоопределение',
        'props.field.boot_params': 'Параметры загрузки',
        'props.field.boot_params_placeholder': 'Дополнительные параметры ядра',
        'props.field.boot_params_hint': 'Оставьте пустым, чтобы использовать значения дистрибутива по умолчанию.',
        'props.field.placeholders_label': 'Подстановки:',
        'props.field.public': 'Общий (доступен всем клиентам)',
        'props.field.default_autoinstall': 'Файл автоустановки по умолчанию',
        'props.field.autoinstall_none': '(Нет — установка вручную)',
        'props.field.autoinstall_hint': 'Показаны только файлы, соответствующие дистрибутиву этого образа. Файлы управляются в разделе «Автоустановка».',

        'props.boot_method.sanboot': 'SAN-загрузка',
        'props.boot_method.kernel': 'Ядро/Initrd',
        'props.boot_method.nbd': 'Монтирование NBD',

        'props.action.extract': 'Извлечь',
        'props.action.re_extract': 'Извлечь заново',
        'props.action.extracting': 'Извлечение…',
        'props.action.extract_now': 'Извлечь сейчас',
        'props.action.patch_smb': 'Патч SMB',
        'props.action.re_patch_smb': 'Перепатчить SMB',
        'props.action.patching': 'Применение патча…',
        'props.action.save_and_repatch': 'Сохранить и перепатчить',
        'props.action.redetect': 'Повторное определение',
        'props.action.download_iso': 'Скачать ISO',
        'props.action.download_netboot': 'Скачать файлы netboot',
        'props.action.download_netboot_tooltip': 'Скачать комплект ядра/initrd для netboot с зеркала дистрибутива',
        'props.action.open_settings': 'Открыть настройки',
        'props.action.save_properties': 'Сохранить свойства',

        'props.notify.patch_success': 'boot.wim пропатчен для автоустановки по SMB',
        'props.notify.patch_failed': 'Не удалось применить патч',

        'props.warn.not_extracted': 'Образ ещё не извлечён. Без извлечения эта ОС, скорее всего, не загрузится надёжно — ядро и initrd должны выдаваться напрямую.',
        'props.warn.netboot_required': 'Требуются файлы netboot. Этот DVD-ISO Debian/Ubuntu содержит установщик, которому нужен отдельный комплект ядра/initrd с зеркала дистрибутива — он не может загрузиться непосредственно из содержимого ISO.',
        'props.warn.smb_disabled': 'SMB-доступ сервера отключён. Для установки Windows он необходим. Включите Windows SMB в настройках.',
        'props.warn.wimlib_missing': 'wimlib-imagex не установлен на сервере. Без него boot.wim не может быть пропатчен. Установите wimtools (или wimlib-imagex) на хост, где запущен bootimus, и перезапустите.',
        'props.warn.repatch': 'Требуется перепатч. Параметры автоустановки изменились с момента последнего встраивания в boot.wim — сохраните и перепатчите SMB, чтобы изменения вступили в силу.',

        'lang.label': 'Язык',
    },
};

const I18N_LANGS = [
    { code: 'en-GB', name: 'English (UK)', flag: '🇬🇧' },
    { code: 'de',    name: 'Deutsch',      flag: '🇩🇪' },
    { code: 'fr',    name: 'Français',     flag: '🇫🇷' },
    { code: 'es',    name: 'Español',      flag: '🇪🇸' },
    { code: 'it',    name: 'Italiano',     flag: '🇮🇹' },
    { code: 'nl',    name: 'Nederlands',   flag: '🇳🇱' },
    { code: 'ru',    name: 'Русский',      flag: '🇷🇺' },
];

let _currentLang = 'en-GB';

function _detectInitialLang() {
    const stored = localStorage.getItem('bootimus_lang');
    if (stored && TRANSLATIONS[stored]) return stored;
    const browserFull = (navigator.language || 'en-GB').toLowerCase();
    if (browserFull.startsWith('en')) return 'en-GB';
    const browserShort = browserFull.slice(0, 2);
    return TRANSLATIONS[browserShort] ? browserShort : 'en-GB';
}

function t(key, params) {
    const dict = TRANSLATIONS[_currentLang] || {};
    const fallback = TRANSLATIONS['en-GB'] || {};
    let str = dict[key];
    if (str === undefined) str = fallback[key];
    if (str === undefined) str = key;
    if (params) {
        for (const k of Object.keys(params)) {
            str = str.replace(new RegExp('\\{\\{' + k + '\\}\\}', 'g'), params[k]);
        }
    }
    return str;
}

const _COMMON_BUTTON_MAP = {
    'Cancel': 'common.cancel',
    'Save': 'common.save',
    'Close': 'common.close',
    'Delete': 'common.delete',
};

function applyTranslations(root) {
    root = root || document;

    root.querySelectorAll('button').forEach(b => {
        if (b.hasAttribute('data-i18n')) return;
        const key = _COMMON_BUTTON_MAP[b.textContent.trim()];
        if (key) b.setAttribute('data-i18n', key);
    });

    root.querySelectorAll('[data-i18n]').forEach(el => {
        const key = el.getAttribute('data-i18n');
        const text = t(key);
        let lastText = null;
        for (let i = el.childNodes.length - 1; i >= 0; i--) {
            const n = el.childNodes[i];
            if (n.nodeType === Node.TEXT_NODE && n.nodeValue.trim()) {
                lastText = n;
                break;
            }
        }
        if (lastText) {
            lastText.nodeValue = lastText.nodeValue.replace(/\S.*\S|\S/, text);
        } else {
            el.textContent = text;
        }
    });
    root.querySelectorAll('[data-i18n-title]').forEach(el => {
        el.title = t(el.getAttribute('data-i18n-title'));
    });
    root.querySelectorAll('[data-i18n-placeholder]').forEach(el => {
        el.placeholder = t(el.getAttribute('data-i18n-placeholder'));
    });
    root.querySelectorAll('[data-i18n-aria-label]').forEach(el => {
        el.setAttribute('aria-label', t(el.getAttribute('data-i18n-aria-label')));
    });
}

function setLanguage(lang) {
    if (!TRANSLATIONS[lang]) lang = 'en-GB';
    _currentLang = lang;
    localStorage.setItem('bootimus_lang', lang);
    document.documentElement.lang = lang;
    ['lang-select', 'login-lang-select'].forEach(id => {
        const s = document.getElementById(id);
        if (s && s.value !== lang) s.value = lang;
    });
    applyTranslations();
}

function currentLanguage() {
    return _currentLang;
}

function _populateLangSelect(sel) {
    if (!sel) return;
    sel.innerHTML = '';
    for (const l of I18N_LANGS) {
        const opt = document.createElement('option');
        opt.value = l.code;
        opt.textContent = (l.flag ? l.flag + '  ' : '') + l.name;
        if (l.code === _currentLang) opt.selected = true;
        sel.appendChild(opt);
    }
    sel.addEventListener('change', e => setLanguage(e.target.value));
}

function _initI18n() {
    _currentLang = _detectInitialLang();
    document.documentElement.lang = _currentLang;

    _populateLangSelect(document.getElementById('lang-select'));
    _populateLangSelect(document.getElementById('login-lang-select'));

    applyTranslations();
}

if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', _initI18n);
} else {
    _initI18n();
}
