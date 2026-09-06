# Diagnostic de la tour Windows — phase 0

- **Date du relevé :** 6 septembre 2026, à partir de 18 h 11 CEST
- **Périmètre :** diagnostic local en lecture seule de la future tour d'hébergement de Prisme
- **Référence projet :** [spécification de la plateforme](OPS_INFRASTRUCTURE_PLATFORM_SPEC.md), sections 4.2, 10.13, 17 et 22
- **Décision WSL2 ou VM Linux :** volontairement laissée ouverte ; ce document ne couvre que le diagnostic demandé

## Conclusion

**Avis de diagnostic : NO-GO temporaire pour un hébergement de production 24/7 en l'état.**

La tour sait déjà exécuter des conteneurs Linux sous WSL2, dispose d'un lien Ethernet à
1 Gbit/s, ne se met pas automatiquement en veille et fait tourner `cloudflared` comme
service Windows. Ces éléments rendent la machine exploitable pour des essais et la suite
de la phase 0.

Elle ne doit cependant pas recevoir les données ou services de production de Prisme avant
traitement des blocages suivants :

1. le disque dur de 2 To associé à `D:` a produit **112 événements disque ID 7 « bad
   block »** entre le 11 juin et le 31 août 2026 ;
2. `C:` et `D:` n'ont respectivement que **7,44 Gio (6,8 %)** et **106,14 Gio (5,7 %)**
   libres ;
3. les trois profils du pare-feu Windows sont désactivés ;
4. Windows 10 Pro 22H2 est hors support standard et la machine est restée sur la build
   `19045.6466` de novembre 2025 ; l'inscription ESU et la réception des mises à jour ESU
   ne sont pas démontrées ;
5. aucune sauvegarde restaurable, destination hors site ou protection par onduleur n'a
   pu être établie ;
6. les journaux montrent au moins six arrêts non propres en 90 jours ;
7. le démarrage autonome de Docker avant toute ouverture de session Windows n'est pas
   démontré ;
8. 16 Gio de RAM, dont environ 79 % étaient utilisés pendant le relevé, laissent peu de
   marge au futur ensemble PostgreSQL, Loki, VictoriaMetrics, Alloy et services Prisme.

Le premier risque à traiter est le disque `D:`. Les données utiles doivent être sécurisées
avant tout test long ou opération de réparation susceptible de solliciter davantage ce
disque.

## Méthode et confidentialité

Le relevé combine l'inventaire Windows, les journaux système sur 90 jours, `powercfg`, les
interfaces réseau, WSL, Docker, Defender, le pare-feu et les services Windows. Les tests
réseau sont courts : dix requêtes vers la passerelle, dix vers un résolveur public, cinq
résolutions DNS et une requête HTTPS Cloudflare.

Aucun stress test, redémarrage, arrêt, mise à jour, nettoyage Docker, réparation de disque,
modification du BIOS, changement réseau ou changement de configuration n'a été effectué.
Le nom de la machine, le compte Windows, les adresses IP et MAC, les numéros de série et les
identifiants Cloudflare sont exclus afin que le document puisse être versionné sans publier
d'identifiants d'infrastructure.

Le processus de collecte n'était pas élevé administrateur. Les résultats explicitement
marqués « non vérifié » devront donc être repris dans une session administrateur ou par
inspection physique.

## Synthèse par exigence de la spécification

| Exigence | État relevé | Statut |
|---|---|---|
| Édition et version de Windows | Windows 10 Pro 22H2 x64, build `19045.6466` | **Bloquant : maintenance OS** |
| CPU, mémoire et architecture | Core i7-6700K, 4 cœurs/8 threads, 16 Gio, x64 | **Risque de capacité** |
| Capacité, type, santé et espace disque | SSD 120 Go + HDD 2 To ; espace très faible ; 112 erreurs ID 7 sur le HDD | **Critique** |
| Fonctionnement 24/7 et veille | Veille/hibernation indisponibles ou désactivées ; uptime 7,22 jours | **Partiel** |
| Réseau, NAT et stabilité | Ethernet USB 1 Gbit/s ; échantillon sans perte ; NAT amont non audité | **Partiel** |
| WSL2, Hyper-V ou virtualisation | Hyperviseur présent, WSL2 et moteur Linux Docker fonctionnels | **Validé techniquement** |
| Docker existant | Docker Desktop et 14 conteneurs actifs | **Fonctionnel, démarrage autonome à qualifier** |
| Sauvegarde physique | Aucune chaîne de sauvegarde/restauration démontrée | **Bloquant** |
| Onduleur | Aucun UPS détecté par Windows ; présence physique non vérifiée | **Non démontré** |
| Redémarrage après coupure | Redémarrage après crash activé ; reprise après perte secteur non vérifiée dans le BIOS | **Non vérifié** |
| Cloudflare Tunnel | Service automatique, actif, récent et configuré pour redémarrer sur échec | **Satisfaisant localement** |
| Antivirus et pare-feu | Defender actif ; durcissements partiels ; tous les profils pare-feu désactivés | **Bloquant : sécurité** |
| Wake-on-LAN | Cartes compatibles et armées ; aucun essai réel depuis les états d'alimentation cibles | **Partiel** |

## Inventaire de la machine

### Système et matériel

| Élément | Valeur relevée |
|---|---|
| Système | Microsoft Windows 10 Professionnel, 22H2, x64 |
| Version | `10.0.19045.6466` |
| Installation Windows | 26 mai 2024 |
| Dernier démarrage | 30 août 2026 à 12 h 51 |
| Uptime au relevé | 7,22 jours |
| Fabricant / modèle | MSI `MS-7A12` |
| Carte mère | MSI Z170A GAMING PRO CARBON |
| BIOS | version `1.30`, datée du 12 mai 2016 |
| CPU | Intel Core i7-6700K à 4,00 GHz, 4 cœurs / 8 threads, x64 |
| Mémoire | 15,96 Gio utilisables ; un module DDR4 de 16 Gio à 2400 MT/s |
| Mémoire au relevé | 3,33 Gio libres, soit environ 79,1 % utilisés |
| GPU | NVIDIA GeForce RTX 3070 Ti ; télémétrie détaillée NVIDIA inaccessible sans élévation |
| Virtualisation | hyperviseur Windows présent ; WSL2 en fonctionnement |

Un seul module mémoire a été inventorié. La carte mère possède quatre emplacements et prend
en charge une architecture double canal, mais le relevé ne permet pas de confirmer le
canal effectif. La charge mémoire est une photographie de la machine déjà utilisée, pas un
benchmark de Prisme.

### Cycle de vie Windows et capacité d'évolution

Windows 10 Home et Pro ont atteint leur fin de support standard le 14 octobre 2025. La
page de suivi Microsoft consultée le 6 septembre 2026 donne `19045.7663` (11 août 2026)
comme build ESU la plus récente, alors que la tour est sur `19045.6466`.

Cette build installée correspond à KB5071959, une mise à jour hors cycle du 11 novembre
2025 destinée notamment à réparer l'inscription ESU des appareils grand public non encore
inscrits. Aucun objet de licence ESU n'a été retourné pendant le relevé. Cela ne prouve pas
formellement l'absence de licence, mais **la couverture ESU et le niveau de correctifs ne
sont pas démontrés**.

Le Core i7-6700K est un processeur Intel de 6e génération. La liste Microsoft actuelle des
familles Intel prises en charge par Windows 11 commence, pour les Core grand public de
cette période, à la 8e génération. Une migration Windows 11 prise en charge ne peut donc
pas être retenue pour ce matériel sans validation explicite du constructeur et de
Microsoft. Windows 11 exige en outre UEFI/Secure Boot et TPM 2.0.

Constats de sécurité de démarrage :

- la carte mère est documentée par MSI comme équipée d'un BIOS UEFI et d'un connecteur
  pour module TPM ;
- les deux disques utilisent GPT ;
- la valeur Windows de Secure Boot est `0`, donc Secure Boot n'est pas actif ;
- aucun périphérique de sécurité TPM utilisable n'a été retourné et `tpmtool` n'a pas pu
  obtenir d'informations ; la présence physique d'un module ou un réglage désactivé dans
  le firmware restent possibles ;
- le statut BitLocker n'a pas pu être lu sans droits administrateur.

### Stockage

| Disque | Support | Volume | Taille utile | Libre | Santé synthétique | Signal complémentaire |
|---|---|---|---:|---:|---|---|
| Kingston SV300S37A120G | SSD SATA | `C:` | 109,74 Gio | 7,44 Gio (6,8 %) | Healthy / Online | aucune erreur ID 7 attribuée pendant la fenêtre |
| Seagate ST2000DM001-1ER164 | HDD SATA | `D:` | 1 862,89 Gio | 106,14 Gio (5,7 %) | Healthy / Online | **112 erreurs disque ID 7 « bad block »** |

Les 112 erreurs visent `\Device\Harddisk1\DR1`, qui correspond au disque Seagate et au
volume `D:` dans l'inventaire actuel. Elles s'étendent du 11 juin au 31 août 2026. Le
statut synthétique « Healthy » de l'API de stockage ne neutralise pas ce signal issu du
journal système.

Les compteurs SMART détaillés (température, heures de fonctionnement, usure et erreurs non
corrigées) n'étaient pas accessibles. Il n'y a pas de redondance visible entre les deux
volumes. Le fichier d'échange alloue environ 2,87 Gio sur `C:` et 15,67 Gio sur `D:`, ce
qui ajoute des écritures et une consommation potentielle d'espace au disque signalé.

Conséquence : le HDD ne doit pas devenir le stockage PostgreSQL, Loki, VictoriaMetrics ou
des sauvegardes de Prisme dans son état actuel. Sa qualification approfondie ne doit être
lancée qu'après mise en sécurité des données.

### Alimentation, disponibilité et reprise

- mode d'alimentation actif : **Performances élevées** ;
- délai de veille sur secteur et batterie : `0`, c'est-à-dire jamais ;
- hibernation désactivée ; les états S1, S2, S3 et Modern Standby sont indisponibles dans
  la configuration actuelle ;
- redémarrage automatique après crash Windows : activé ;
- reprise automatique après coupure secteur : non exposée par Windows et non vérifiée
  dans le BIOS ;
- aucun onduleur ni batterie n'est détecté par Windows ; un UPS sans liaison USB resterait
  invisible à ce contrôle ;
- les wake timers n'ont pas pu être lus sans élévation.

Sur 90 jours, le journal système contient onze entrées Kernel-Power 41 / EventLog 6008
réparties sur six dates : 11 et 19 juin, 28 juin, 20 et 24 août, puis 30 août 2026. Elles
représentent **au moins six arrêts ou redémarrages non propres**, sans permettre d'en
attribuer la cause à une coupure secteur, un reset manuel ou un autre incident. Aucun
BugCheck ni événement WHEA matériel n'a été trouvé sur la même fenêtre.

L'uptime courant de plus de sept jours et l'absence de veille sont favorables, mais ne
constituent pas encore une validation 24/7. Les arrêts non propres et l'absence d'UPS
démontré sont des risques directs pour une base et des métriques locales.

### Réseau et NAT

La liaison utilisée est un **Realtek USB GbE Family Controller**, connectée à 1 Gbit/s sur
un profil réseau privé. Elle possède une adresse IPv4 privée RFC 1918, une passerelle IPv4
et IPv6 et une connectivité Internet IPv4/IPv6. Le contrôleur Intel I219-V intégré à la
carte mère est présent mais déconnecté.

Échantillon ponctuel :

| Test | Résultat |
|---|---|
| Passerelle locale, 10 requêtes | 10/10, 0 % de perte, latence mesurée inférieure à 1 ms |
| Résolveur public, 10 requêtes | 10/10, 0 % de perte, moyenne 4 ms, maximum 9 ms |
| DNS, 5 résolutions | 5/5, moyenne 4,4 ms, maximum 18 ms |
| HTTPS Cloudflare | HTTP 200 en 185 ms |

Depuis le dernier démarrage, la carte active rapporte zéro erreur de réception ou
d'émission et 17 330 paquets reçus écartés sur environ 277,9 millions, soit environ
0,006 %. Le journal NetworkProfile compte quatorze transitions de déconnexion sur 90
jours, mais celles-ci peuvent inclure les redémarrages de la machine ; aucun test continu
n'a été effectué.

L'adressage privé indique un réseau local en amont. Le type exact de NAT, la réservation
DHCP, le routeur, le chemin Internet, le débit montant, la redondance et la stabilité sur
plusieurs jours ne peuvent pas être validés depuis la seule tour. Aucun objet NAT Windows
local n'a été trouvé. Cloudflare Tunnel limite le besoin d'une redirection entrante, mais
ne remplace pas la qualification de la liaison.

### Wake-on-LAN

Les deux cartes réseau filaires exposent des capacités de réveil :

- la carte Realtek USB utilisée a `Wake on Magic Packet`, `Wake on pattern match` et
  `Wake on link change` activés ; elle figure dans les périphériques armés pour réveiller
  Windows ;
- la carte Intel I219-V intégrée a Magic Packet, Pattern Match et PME activés et figure
  également dans les périphériques armés, mais elle est déconnectée ;
- les états de veille classiques ne sont actuellement pas disponibles et l'hibernation
  est désactivée.

Cela prouve la présence de réglages logiciels, pas un réveil effectif. Un test réel reste
nécessaire pour S5/arrêt, après coupure secteur et depuis le futur relais réseau. Il doit
également vérifier l'alimentation USB hors arrêt si la carte Realtek reste la liaison
principale. La carte Intel intégrée offre un chemin matériel à évaluer séparément.

## WSL2 et Docker

| Élément | État relevé |
|---|---|
| WSL | `2.7.12.0` |
| Noyau WSL | `6.18.33.2` |
| Distribution par défaut | `docker-desktop`, WSL 2, active |
| Distribution Linux généraliste | aucune autre distribution listée |
| `.wslconfig` | absent ; aucun plafond explicite relevé côté utilisateur |
| Docker Desktop | `4.77.0` |
| Docker Engine | `29.5.3`, serveur Linux `amd64` |
| Docker Compose | `v5.1.4` |
| Backend | contexte `desktop-linux`, `overlayfs`, cgroups v2 |
| Ressources visibles par Docker | 8 CPU logiques, 7,73 Gio de mémoire |
| Inventaire Docker | 21 conteneurs, dont 14 actifs ; 18 images |
| Live restore | désactivé |

L'utilisation Docker au relevé était déjà significative :

- images : 22,32 Go, dont 1,52 Go récupérables ;
- conteneurs : 1,03 Go, dont 0,94 Go récupérables ;
- volumes locaux : 3,57 Go, dont 0,74 Go récupérables ;
- cache de build : 12,52 Go, dont 9,66 Go récupérables ;
- états de santé enregistrés sur les 21 conteneurs : 11 `healthy`, 1 `unhealthy`,
  5 `starting` et 4 sans healthcheck ;
- politiques de redémarrage : 6 `always`, 9 `unless-stopped`, 6 sans redémarrage
  automatique.

Ces chiffres décrivent des charges existantes qui ne sont pas attribuées à Prisme dans ce
rapport. Aucun conteneur n'a été supprimé, redémarré ou inspecté fonctionnellement.

Docker Desktop possède une entrée de démarrage utilisateur. Le service
`com.docker.service` était en mode manuel et arrêté alors que le moteur répondait via
l'application Docker Desktop. Le redémarrage du moteur **au boot sans ouverture de
session interactive** n'est donc pas démontré. Ce point est déterminant pour un control
plane 24/7, indépendamment du futur arbitrage WSL2/VM.

WSL2 est techniquement opérationnel. Ce diagnostic ne valide toutefois pas encore le choix
WSL2 : une VM Linux dédiée améliorerait l'isolation et le démarrage en service, mais
consommerait une marge mémoire déjà faible ; WSL2 est plus léger, mais l'exploitation
actuelle dépend de Docker Desktop et de la session utilisateur. L'arbitrage doit suivre la
remise à niveau de l'hôte et le dimensionnement des services stateful.

## Cloudflare Tunnel

| Contrôle | Résultat |
|---|---|
| Service Windows | présent, `Running`, démarrage `Auto`, compte `LocalSystem` |
| Version | `cloudflared 2026.5.2` |
| Reprise sur échec | redémarrage du service après 20 secondes |
| Erreurs Service Control Manager | aucune liée à `cloudflared` sur 30 jours |
| Accès HTTPS à Cloudflare | réussi |

L'état local est satisfaisant. Ce contrôle ne prouve pas encore que le futur sous-domaine
Prisme, Cloudflare Access, les politiques d'identité ou la route jusqu'au backend final
sont corrects ; ces éléments ne sont pas définis à ce stade.

## Sécurité de l'hôte

### Points positifs

- Microsoft Defender est le produit antivirus déclaré ;
- antivirus, antispyware, protection comportementale, protection à l'accès, analyse des
  téléchargements et protection réseau au niveau moteur sont actifs ;
- les signatures ne sont pas périmées et avaient un âge de zéro jour ;
- une analyse rapide datait de deux jours ;
- l'UAC est actif et utilise le bureau sécurisé pour l'élévation ;
- aucun indicateur Windows Update ou Component Based Servicing de redémarrage n'était
  présent.

### Écarts

- **pare-feu Windows désactivé** sur les profils Domaine, Privé et Public ;
- protection antialtération Defender désactivée ;
- protection contre les applications potentiellement indésirables désactivée ;
- Controlled Folder Access désactivé ;
- Network Protection configurée à `0` ;
- un chemin, un processus et une extension sont exclus de Defender ; leurs valeurs ne sont
  volontairement pas publiées, mais doivent être revues avant production ;
- aucun scan complet récent n'est enregistré ;
- Secure Boot désactivé ; TPM non démontré ; BitLocker non vérifié ;
- un signal `PendingFileRenameOperations` est présent, ce qui justifie une revue de
  maintenance et du prochain redémarrage ;
- la build Windows installée ne contient pas les mises à jour ESU publiées après novembre
  2025.

Aucun pare-feu hôte compensatoire n'a été validé. La désactivation des profils Windows est
donc considérée bloquante, même si Cloudflare Tunnel évite d'ouvrir directement un port
entrant sur le routeur.

## Sauvegarde et restauration

Le relevé n'apporte aucune preuve d'une sauvegarde exploitable :

- aucun service tiers identifiable de type Veeam, Acronis, Macrium, Duplicati, Restic ou
  Borg n'a été trouvé ;
- les services Windows Backup sont arrêtés et en démarrage manuel ;
- seules des tâches planifiées Windows génériques sont présentes ; elles ne prouvent pas
  une sauvegarde utilisateur ;
- la clé de configuration File History existe, mais son journal actif ne contient aucun
  événement sur les 90 derniers jours ;
- `wbadmin`, l'inventaire des clichés et le statut BitLocker nécessitaient une élévation
  non disponible pendant ce relevé ;
- aucune destination locale, physique ou hors site et aucun test de restauration n'ont été
  identifiés.

Pour Prisme, la stratégie devra distinguer au minimum les données PostgreSQL, les
configurations et secrets chiffrés, les volumes Docker, les métriques/logs selon leur
rétention, ainsi que la copie hors site. Une sauvegarde placée uniquement sur `D:` ne serait
ni indépendante du défaut actuel ni hors site.

## Actions préalables recommandées — non exécutées

Ordre de traitement proposé pour préparer la suite de la phase 0 :

1. **Sécuriser immédiatement les données utiles de `D:`**, puis effectuer un diagnostic
   constructeur/SMART long hors charge ; remplacer le disque si le défaut est confirmé.
2. Définir et tester une sauvegarde indépendante, versionnée et hors site, avec une
   restauration réelle et des objectifs RPO/RTO.
3. Ajouter du stockage sain et réservé au control plane ; fixer les rétentions de Loki et
   VictoriaMetrics avant le dimensionnement.
4. Choisir une trajectoire Windows prise en charge : preuve d'inscription ESU et mise à
   jour immédiate comme mesure transitoire, ou renouvellement matériel permettant un OS
   Windows supporté. Ne pas compter sur une installation Windows 11 non prise en charge.
5. Réactiver et définir le pare-feu par profils, revoir les exclusions Defender et les
   protections désactivées, puis vérifier TPM, Secure Boot et BitLocker en administrateur.
6. Installer ou confirmer un onduleur avec arrêt propre, vérifier dans le BIOS le réglage
   de restauration après perte secteur, puis tester la séquence coupure → retour secteur →
   Windows → runtime Linux → control plane.
7. Démontrer un démarrage Docker/VM réellement autonome, sans connexion interactive, avec
   healthchecks, dépendances et politiques de redémarrage cohérentes.
8. Mesurer la mémoire et les I/O avec la future stack Prisme sur stockage sain ; augmenter
   la RAM si la marge reste comparable aux 3,33 Gio observés.
9. Qualifier la liaison réseau sur 24 à 72 heures, la réservation DHCP, le débit montant et
   la carte filaire retenue ; tester Wake-on-LAN dans chaque état d'alimentation prévu.
10. Mettre à jour et vérifier le BIOS et les pilotes depuis les sources MSI/Intel/Realtek,
    avec sauvegarde et procédure de retour arrière avant toute mise à jour firmware.

Ces actions sont des prérequis proposés ; aucune n'a été appliquée dans le cadre de ce
diagnostic.

## Points restant à vérifier

Une session administrateur et/ou une inspection physique devront confirmer :

- les attributs SMART complets des deux disques et le résultat d'un test constructeur ;
- l'état BitLocker et les clés de récupération ;
- la présence/version du TPM et le mode UEFI effectif ;
- le niveau et les réglages exacts du BIOS, notamment `Restore on AC Power Loss`, la
  virtualisation et le Wake-on-LAN ;
- la présence, la puissance, l'autonomie et la liaison USB/réseau d'un éventuel UPS ;
- le statut réel de File History/Windows Backup et toute sauvegarde externe non visible ;
- la licence ESU Windows 10 et le fonctionnement de Windows Update ;
- les causes des six arrêts non propres ;
- la santé fonctionnelle du conteneur marqué `unhealthy` ;
- l'accès Cloudflare de bout en bout une fois le sous-domaine Prisme défini ;
- le comportement après redémarrage sans ouverture de session ;
- le Wake-on-LAN réel depuis arrêt et après perte secteur ;
- la stabilité réseau et thermique sur une période longue.

## Références externes consultées

- [Windows 10 — informations de publication et builds ESU](https://learn.microsoft.com/en-us/windows/release-health/release-information)
- [KB5071959 — Windows 10 build 19045.6466](https://support.microsoft.com/en-us/servicing/os/windows-10/2025/11/november-11-2025-kb5071959-windows-10-version-22h2-os-build-19045-6466-out-of-band)
- [Configuration minimale requise pour Windows 11](https://support.microsoft.com/en-us/windows/experience/compatibility/windows-11-system-requirements)
- [Processeurs Intel pris en charge par Windows 11](https://learn.microsoft.com/en-us/windows-hardware/design/minimum/supported/windows-11-supported-intel-processors)
- [Spécifications MSI Z170A GAMING PRO CARBON](https://www.msi.com/Motherboard/Z170A-GAMING-PRO-CARBON/Specification)
- [Liste MSI des cartes mères compatibles avec un module TPM 2.0](https://download.msi.com/archive/mnu_exe/pdf/2021/msi-tpm-2-0-motherboard-list.pdf)
- [Comportement Wake-on-LAN sous Windows](https://learn.microsoft.com/en-us/troubleshoot/windows-client/setup-upgrade-and-drivers/wake-on-lan-feature)

## Commandes et sources locales principales

Les familles de commandes suivantes ont été utilisées en lecture seule :

- `Get-CimInstance` pour l'OS, le matériel, le BIOS, la mémoire, les disques et les
  services ;
- `Get-PhysicalDisk`, `Get-Disk`, `Get-Volume`, `Get-Partition` et les journaux `disk` ;
- `Get-WinEvent` pour les événements disque, alimentation, WHEA, BugCheck, réseau,
  File History et `cloudflared` ;
- `powercfg /a`, `/query`, `/lastwake`, `/devicequery wake_armed` et
  `/devicequery wake_programmable` ;
- `Get-NetAdapter`, `Get-NetConnectionProfile`, `Get-NetAdapterStatistics` et les
  propriétés Wake-on-LAN ;
- `Test-Connection`, `Resolve-DnsName` et une requête HTTPS Cloudflare ;
- `wsl --version`, `wsl --status`, `wsl --list --verbose` ;
- `docker version`, `docker info`, `docker system df` et agrégation des états/restart
  policies ;
- `Get-MpComputerStatus`, `Get-MpPreference`, `Get-NetFirewallProfile` et lecture UAC ;
- inventaire des services/tâches de sauvegarde et du service `cloudflared`.
