# Phase 6 — Terminal, fichiers, coffre et secours

Livraison locale du 9 septembre 2026. PostgreSQL **8**, OpenAPI **0.7.0**.
Les fonctionnalités sont désactivées tant que leurs cibles, clés, capacités et
politiques ne sont pas configurées. Aucun service, compte, tunnel ou fichier du
VPS de production n'a été modifié pour cette livraison.

## Fonctionnement livré

- Véritable terminal SSH avec PTY, transport WebSocket natif, redimensionnement,
  onglets, division sur grand écran, sélection/copie, collage multiligne confirmé,
  recherche en mémoire et touches mobiles. L'identité et l'environnement restent
  visibles. Ouverture possible depuis une fiche d'incident ; ouverture/fermeture
  sont corrélées à son audit. Aucune transcription persistante.
- Huit sessions maximum réparties sur des cibles indépendantes. Une session
  interactive réserve son hôte : une seconde mutation du même hôte est refusée.
  Déconnexion WebSocket distincte de l'arrêt du shell, reprise explicite avec
  preuve récente, aucun rejeu de saisie, tampon borné et indication de sortie perdue.
- SFTP : navigation, mode/propriétaire, aperçu texte, téléchargement/téléversement
  par blocs reprenables dans la session, révision du fichier, diff masqué,
  sauvegarde atomique et copie privée avant remplacement. Taille maximale 1 Mio.
  Refus des chemins système, traversées, liens symboliques et fichiers de clés.
- Coffre AES-256-GCM par enveloppe : clé de données aléatoire par révision, clé
  maîtresse indépendante de PostgreSQL, métadonnées authentifiées, rotation et
  expiration, destination explicite, historique sans ancienne valeur en clair.
  Révélation de 30 secondes après preuve. Les fichiers `.env` sont assemblés sur
  le serveur à partir de variables littérales et références de secrets, puis
  remplacés avec mode 0600 et copie de sécurité.
- Preuves à usage unique liées à la session, l'appareil et la requête complète.
  WebAuthn exige la vérification utilisateur ; Android réutilise la signature
  biométrique de phase 5. Nouvelle clé biométrique = anciennes preuves invalides.
- Passerelle native de secours indépendante : registre local de passkeys et
  appareils, révocations, enrôlements et codes de récupération à usage unique,
  sessions, clé biométrique Android et audit propres. Aucun accès au coffre central.
  Le client propose cette connexion avant la connexion normale et dans la barre
  d'outils. Il ne change jamais automatiquement le transport d'une action.
- Diagnostics SSH bornés : uptime, espace disque, conteneurs, logs de conteneurs
  explicitement configurés. Source/date visibles et résultat partiel explicite.
- Garde de mutation sur l'hôte : verrou système partagé, époque persistante,
  blocage après résultat incertain, suspension des automatismes après secours,
  réconciliation opérateur explicite et invalidation des anciens plans.
- Audit de secours append-only, signé Ed25519 à l'export, import PostgreSQL
  dédupliqué avec curseur acquitté. L'import ne reprend pas les automatismes.

Le [contrat détaillé](../contracts/operations-v1.md) décrit les limites,
messages WebSocket, droits et charges utiles. `internal/operations` contient le
transport et le coffre ; `internal/coordination` le verrou partagé ;
`internal/recovery` l'identité indépendante. Les migrations antérieures restent
inchangées. Les clients réutilisent exclusivement les composants publics Prisme UI.

## Configuration normale

1. Créer une clé maîtresse **binaire de 32 octets** dans un fichier privé extérieur
   à la base et aux sources. Sur Windows :

   ```powershell
   ./tool/new-vault-key.ps1 -Path D:/private/prisme-vault-master
   ```

   Conserver une copie hors ligne indépendante avant d'ajouter des secrets. Ne
   jamais remplacer silencieusement cette clé : une base sans sa clé est illisible.
   Sous Linux, un gestionnaire de secrets peut provisionner 32 octets aléatoires
   dans un fichier mode 0600, lisible uniquement par le service.
2. Copier [config.example.json](../deploy/operations/config.example.json) dans un
   fichier privé. Indiquer l'adresse SSH, le compte, le fichier de clé privée,
   l'empreinte hôte obtenue par un canal fiable et les racines autorisées. Les
   booléens `terminal`, `files`, `diagnostics` sont activés séparément.
3. Installer le binaire `cmd/host-guard` sur **l'hôte d'exécution** sous le nom
   `prisme-host-guard`. Le fichier est contrôlé par l'administrateur ; les comptes
   SSH ne doivent pas pouvoir le remplacer. Préparer le répertoire de coordination
   privé. Normal, secours et worker doivent accéder aux mêmes fichiers sous le
   **même UID opérationnel** (ou root pour le worker), jamais à des copies.
4. Si un worker de déploiement utilise le même hôte, son `coordination_root` doit
   correspondre à celui du garde, et son manifeste `server` au `lock_target` SSH.
   Arrêter les anciens workers sans garde avant d'activer cette cohabitation.
   Le moteur Compose de phase 4 reste local ; cela n'ajoute pas un worker distant.
5. Définir `PRISME_OPERATIONS_CONFIG`, appliquer `api -migrate`, puis redémarrer
   l'API. La configuration de développement ne permet pas de fabriquer des preuves
   sensibles : utiliser une session passkey. Android doit avoir enrôlé sa clé
   biométrique ; le navigateur Windows utilise une passkey avec UV.

Pour désactiver la fonctionnalité, retirer `PRISME_OPERATIONS_CONFIG` et
redémarrer. Les données chiffrées restent dans la migration 008. Ne pas lancer de
down migration destructive ; conserver base et clé lors d'un retour de version.

## Préparation du secours

La [configuration](../deploy/operations/recovery.example.json) et
[l'unité systemd](../deploy/systemd/prisme-recovery.service) sont des modèles,
pas une installation effectuée. Le service s'exécute sous le compte opérationnel
explicitement retenu, sans sudo général. Il utilise SSH local, sa propre identité
TLS et sa propre origine HTTPS. Le démarrage ne dépend ni de Caddy ni de Docker.

Construire les deux exécutables sur/pour Linux :

```sh
go build -o prisme-recovery ./cmd/recovery
go build -o prisme-host-guard ./cmd/host-guard
```

Depuis une session SSH administrateur de l'hôte, préparer l'ID affiché dans le
client **Accès de secours**, avec un nouveau fichier privé de sortie :

```sh
prisme-recovery -identity-root /var/lib/prisme-recovery \
  -enroll-device <id-affiche> -name 'Appareil de secours' \
  -code-file /chemin-prive/enrollment-code
```

Le code est écrit dans le fichier et n'est jamais affiché par le programme.
Dans le navigateur ouvert par le client, choisir l'enregistrement d'une passkey
et fournir ce code. Il est consommé au début de la cérémonie ; en cas d'abandon,
l'administrateur génère un autre ticket. L'appareil révoqué ne peut pas réutiliser
son ID. `-recovery-code` génère un code indépendant qui permet de remplacer les
anciennes passkeys et sessions depuis un nouvel appareil.

Une fois connecté sur Android, **Enregistrer la clé biométrique de secours**
prépare une clé distincte pour cette origine. L'enrôlement exige une connexion
passkey de moins de cinq minutes. La clé normale n'est pas réutilisée comme
autorité de secours.

Le registre, la clé de signature d'audit et les fichiers TLS/SSH doivent rester
privés, appartenir au compte du service et être sauvegardés indépendamment de
la tour. Révoquer sur le VPS avec `-revoke-device <id>` ; les sessions actives sont
réévaluées. `-public-key` expose uniquement la clé publique à inscrire dans
`recovery_keys` côté normal, indexée par son SHA-256 (octets DER exclus : ce sont
les 32 octets de clé publique Ed25519). L'API refuse les exports non signés ou
signés par une clé non configurée.

Le tunnel Cloudflare et l'application Access doivent être préparés séparément,
avec vérification du TLS de l'origine. Leur publication et leur authentification
native n'ont pas été déployées/qualifiées ici. Un client peut préconfigurer
`PRISME_RECOVERY_URL` et, pour un laboratoire, `PRISME_RECOVERY_CA_BASE64`.
L'URL peut aussi être saisie à la connexion, avec validation TLS normale.

## Retour à l'exploitation normale

Une perte de liaison ne prouve pas l'arrêt du processus SSH. L'état actif reste
conservé après fermeture incertaine ou arrêt du service. Vérifier sur l'hôte les
processus, les conteneurs, les fichiers modifiés et les travaux interrompus.
Lire `<coordination_root>/<lock_target>.json`, puis acquitter l'époque exacte :

```sh
prisme-host-guard -root /var/lib/prisme-coordination -target vps \
  -actor operator -reconcile -epoch <epoque-observee>
```

Un verrou encore détenu est refusé : l'opérateur doit d'abord arrêter le processus
et constater sa fin. Ce mécanisme ne tue pas arbitrairement un ancien processus.
L'acquittement invalide les plans antérieurs ; les replanifier depuis l'état réel.
Dans le client, une connexion normale valide et une connexion de secours permettent
**Importer l'audit vers la tour**. Une interruption reprend via le curseur serveur ;
les doublons restent sans effet.

Récupération du coffre : arrêter les mutations, restaurer une sauvegarde cohérente
de PostgreSQL et le fichier exact de clé maîtresse, remonter ses permissions,
redémarrer puis vérifier une révélation autorisée sans l'inscrire dans les logs.
La rotation des valeurs est livrée ; le remplacement automatique de la clé
maîtresse et les connecteurs de coffres externes ne sont pas implémentés.

## Vérifications

```powershell
go test ./...
go vet ./...
./tool/test-integration.ps1
./tool/test-operations.ps1
dart run tool/generate_api.dart --check
cd apps/prisme_client
flutter analyze --no-pub
flutter test --no-pub
dart run tool/operations_transport_smoke.dart ../../.dev/pki
flutter build windows --debug
flutter build apk --debug
```

Preuves locales acquises :

- PostgreSQL : migration 008, stockage chiffré, métadonnées sans valeur, révélation,
  rotation/conflit, preuve WebAuthn, rejeu refusé, appareil révoqué, audit sans secret.
- Secours : enrôlement/passkey sur une autorité distincte, refus du bearer normal,
  session normale expirée puis API normale arrêtée, nouvelle connexion de secours,
  révocation persistante, redémarrage sans conservation du bearer, import signé
  dédupliqué et rejet d'un export modifié.
- OpenSSH isolé : véritable PTY, sortie `stty`, redimensionnement et reconnect,
  empreinte hôte erronée, SFTP réel, sauvegarde/backup 0600, conflit de révision,
  transferts rejoués/d'un autre acteur, exclusion de mutation, suspension/reprise
  explicite et échec d'écriture de l'audit. Aucun port n'est publié par la fixture.
- Dart : handshake WSS réel avec TLS local, transmission des frames, refus de
  redirection et de certificat non approuvé. Le SDK WebSocket suivait par défaut
  les redirections HTTP ; le transport effectue maintenant un handshake explicite.
- Flutter : écrans 390/1 440 px, sélection de chemins, refus de preuve sans
  exécution, secrets masqués, contexte de secours sans coffre, réponses tardives
  après fermeture et suppression du contenu sensible des diffs.
- Builds natifs de développement : Windows et APK Android compilés localement.

Dernière vérification du 10 septembre : `go test ./...`, `go vet ./...`, contrat
généré, analyse Flutter sans diagnostic, 29 tests Flutter, smoke WSS et builds
Windows/Android réussis. Le lancement final des fixtures Docker a été bloqué par
une erreur de démarrage de Docker Desktop (socket local du gestionnaire
d'inférence). Les suites PostgreSQL et OpenSSH avaient réussi auparavant dans
cette tâche ; elles restent à relancer après remise en service de Docker pour
qualifier les derniers ajustements de liaison cible/session et de validation de
chemin. Ces ajustements passent les tests unitaires locaux.

## Limites de qualification

L'implémentation et les scénarios locaux ne valent pas qualification d'exploitation
sur le VPS réel. Restent : installation systemd et redémarrage autonome de l'hôte,
tunnel/Access indépendant, biométrie physique du S24 pour ces nouvelles actions,
Windows Hello, panne effective Cloudflare et coupure électrique de la tour.
L'échec d'audit est injecté comme erreur d'écriture ; aucun disque utilisateur
n'est rempli pour le tester. Les garanties Android d'attestation restent celles
décrites dans PHASE_5.

Les reprises de transfert sont en mémoire, bornées et limitées à la session ; elles
ne survivent pas au redémarrage. La comparaison à Git, le glisser-déposer, les
transcriptions intégrales et les outils d'éditeur avancés restent des extensions.
Les sauvegardes privées SFTP nécessitent une politique de rétention opérateur ;
le chantier global de quotas/rétention et les sauvegardes automatiques relèvent de
la phase 7. Les fichiers déclaratifs des intégrations existantes ne sont pas migrés
automatiquement vers le coffre. La voie SSH privée de dernier recours reste une
voie opérateur externe. Le NO-GO production de la tour reste inchangé.
