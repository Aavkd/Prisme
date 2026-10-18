# Phase 1 — Flotte, identité et inventaire

État du 7 septembre 2026. Prisme reste un nom provisoire. Les composants logiciels
de phase 1 étendent la fondation existante ; l'hébergement de production reste
soumis au diagnostic de la tour et au choix du domaine de l'API.

## Livraisons

| Domaine | Comportement disponible |
|---|---|
| Passkeys | WebAuthn avec vérification utilisateur obligatoire ; première inscription, connexion, ajout via ticket, récupération |
| Sessions/appareils | Identité persistante par installation, expiration à une heure, rotation sans prolongation, révocation, verrouillage local à cinq minutes |
| Récupération | Dix codes à usage unique, empreintes SQL seulement ; récupération remplaçant les anciens authentifiants et sessions |
| Agent | Enrôlement borné, capacités explicites, renouvellement automatique de certificat, reprise avec identité persistante, révocation administrable |
| Inventaire | Hôte/architecture/CPU/capacité mémoire et disque ; conteneurs/état/health ; Compose, images/tags/digests, ports, réseaux, volumes |
| Découverte réelle | Collecte SSH ponctuelle sur `ovh-mantara`, sans installation ni mutation de production ; import dans PostgreSQL local |
| Client | Serveurs, détail et groupes Compose, appareils/récupération, audit, sélection d'accueil propre à chaque appareil, thèmes/locale persistés |
| Audit | Mutations d'identité/inventaire atomiques avec audit ; contexte acteur/appareil/origine/résultat/corrélation ; refus enregistrés sans corps ni secrets ; protection append-only logique |
| Contrats | OpenAPI 0.2.0, génération Dart, migrations SQL 002/003 conservant 001, agent v1 compatible heartbeat phase 0 |

L'inventaire est **découvert**, sans import déclaratif ou adoption de gestion d'un
projet. Le détail montre les labels Compose qui justifient le regroupement. Les
associations manuelles/manifests gérés, la lecture de dépôts Git et services système,
les métriques temporelles et le pilotage restent dans les phases métier suivantes.

## Reprendre localement

```powershell
./tool/dev.ps1
go run ./cmd/inventory -ssh ovh-mantara -api https://localhost:8443 -dev-secret-file .dev/bootstrap-secret -name 'OVH Mantara' -output .dev/vps-inventory.json
./tool/run-client.ps1
```

Le collecteur transmet son script Python fixe par stdin SSH. Il utilise l'alias
et la clé hôte déjà approuvés, en mode non interactif. Aucune commande de mutation
Docker, aucun contenu de `.env`, aucune sortie de healthcheck n'est lu. Les erreurs
Docker deviennent des avertissements structurés ; les snapshots sont limités à
2 Mio et 1 000 objets par famille. Les observations de plus de 24 h sont refusées.

Le résultat est une photographie : réexécuter la commande pour actualiser. La
fraîcheur devient ancienne après 90 secondes et reste distincte de la connectivité
API. Les 13 conteneurs observés le 6 septembre à 23:10 heure locale étaient regroupés
en `sounding-lines` (4), `arpent` (4), `mantara` (3), `edge` (1),
`monsieur-vincent` (1), avec **zéro avertissement** après vérification du collecteur.
Ces nombres ne sont pas des fixtures intégrées à l'application.

`-output` est facultatif ; les snapshots locaux restent dans `.dev/` ignoré par
Git et protégé par ACL. `-session-file` permet un import avec une session passkey
courte existante ; `-dev-secret-file` ne fonctionne qu'en mode développement.

## Utiliser les passkeys

Le mode d'identité est distinct du mode de développement :

```text
PRISME_MODE=identity
PRISME_AUTH_ORIGIN=https://DOMAINE_API
PRISME_IDENTITY_SECRET_FILE=/chemin/prive/initialisation
```

Il conserve les paramètres DB, CA et TLS de `cmd/api`, et exige la migration
explicite `api -migrate`. Il ne configure pas DNS, Tunnel ou Access. Le navigateur
et Flutter doivent faire confiance au certificat HTTPS. La CA de développement
n'est jamais ajoutée automatiquement au magasin Windows ou Android.

Le client se construit avec `PRISME_IDENTITY=true` et `PRISME_API_URL` ; le bouton
passkey ouvre la page de connexion dans le navigateur système. À la première
connexion, « Créer ou récupérer » demande le secret privé d'initialisation. Ensuite,
seules les passkeys, un code de secours ou un ticket d'ajout autorisent les parcours
correspondants. Conserver les codes de secours hors de l'appareil. Un code est
consommé dès le début d'une récupération, même si le parcours est abandonné.

La page navigateur ne reçoit pas le token de session native. L'application possède
un secret de polling distinct ; la récupération de session est à usage unique.
Le client ne persiste ni sessions, ni passkeys, ni codes. Les préférences d'apparence
et l'identifiant de l'installation sont locaux ; les sélections de ressources sont
en base, rattachées à cet identifiant. Retirer une carte n'arrête pas sa collecte.

La révocation d'un appareil ferme ses sessions et bloque son ID d'installation.
Elle ne détruit pas une passkey synchronisée dans le compte du fournisseur. En cas
de compromission de l'authentifiant lui-même, utiliser la récupération, qui remplace
toutes les anciennes passkeys. Le RP ID définitif doit être choisi avant de créer
des authentifiants durables ; un changement de domaine peut imposer un réenrôlement.

## Installer un agent ultérieurement

Les modèles dans `deploy/systemd/` ne sont **pas installés sur le VPS**. Préparer
les comptes `prisme-agent` et `prisme-collector`, le groupe `prisme-observation`,
les exécutables, les fichiers CA/token privés et remplacer les cibles des unités
avant tout déploiement. Le collecteur utilise Docker CLI et Python 3. Le groupe
Docker est un privilège administratif, même si les appels livrés sont uniquement
des lectures ; seul le processus collecteur le reçoit. Le cœur agent reçoit un
snapshot 0640 par fichier et ne possède ni socket Docker ni accès SSH.

Depuis la vue Serveurs, générer un jeton de dix minutes. L'agent reçoit initialement
`self:heartbeat`, `inventory:host`, `inventory:docker`. Le serveur vérifie ces
capacités à chaque observation. L'agent renouvelle son certificat dans les huit
dernières heures ; une série précédente est tolérée dix minutes pour récupérer
une réponse perdue. Une identité révoquée est refusée immédiatement. Une coupure
dépassant la validité du certificat exige un nouvel enrôlement explicite.

## Vérifications reproductibles

```powershell
./tool/verify-foundation.ps1 -Integration
./tool/verify.ps1
```

Tests Go : validation d'inventaire et réponses partielles, frontières SSH, migration
relançable, enrôlement concurrent, mTLS, réduction de capacités, renouvellement et
redémarrage, révocation des deux séries, import idempotent, champs secrets refusés,
authentifiant P-256 virtuel, origine/UV/signature invalides, rejeu de challenge et de
polling, récupération, rotation, révocation d'appareil et préférences isolées.

Les cinq tests Flutter couvrent connexion, hors connexion, révocation, flotte aux
largeurs 390/1 440, observation ancienne avec heartbeat récent, valeurs nulles,
épinglage et réponse tardive après verrouillage. Les tests du socle UI restent
dans leur package existant.

Le test natif `integration_test/local_backend_test.dart` se connecte au vrai
backend TLS local et cherche `OVH Mantara`. Sous Windows, définir
`PRISME_SMOKE_DIR` vers le chemin absolu `.dev`, puis exécuter depuis le client :

```powershell
flutter test integration_test/local_backend_test.dart -d windows
```

Sous Android, utiliser un émulateur de développement, `adb reverse tcp:8443
tcp:8443`, puis placer uniquement les fichiers requis dans le sandbox privé du
package debug via `run-as` (`files/phase1/bootstrap-secret` et
`files/phase1/pki/ca.pem`). Le test utilise ce répertoire. Ne jamais intégrer ces
secrets à l'APK ni les transmettre comme arguments `--dart-define`.

Le helper `python -X utf8 tool/test-native-android.py emulator-5554` automatise
ce parcours : il attend le sandbox du test, copie les deux entrées par stdin,
récupère la capture via `adb exec-out`, puis efface les entrées privées. Il refuse
les identifiants d'appareils physiques. Flutter désinstalle son application de
test à la fin ; les captures doivent être récupérées avant cette étape.

Résultats locaux : tests natifs **Windows et Android API 36.1 réussis** contre le
backend réel, ouverture de la fiche OVH et de sa découverte Compose ; builds
Windows debug/release et Android debug réussis. Les captures de validation sont conservées
dans `.dev/phase1-windows.png` et `.dev/phase1-android.png`, hors Git. Aucun appareil
Samsung physique n'était connecté pendant cette validation.

Les vérifications du socle ont également réussi : 13 tests du package UI et
9 tests du catalogue, palettes synchronisées, analyses statiques sans problème.
OpenAPI est validé avec `openapi-spec-validator` et la projection Dart est à jour.
La CI comprend désormais la compilation Android ; son exécution distante n'est
pas prétendue à partir des seuls résultats locaux.

## Limites d'exploitation

Les passkeys matérielles Windows/S24, le domaine final et la barrière Access restent
à qualifier sur l'hébergement réel. Les tests cryptographiques ne remplacent pas
une confirmation humaine Windows Hello/Android. L'UV peut être un PIN : la preuve
biométrique propre aux futures actions sensibles de production reste en phase 5.
La limite de 120 requêtes/minute est encore globale ; adapter l'admission avant
publication pour éviter qu'un acteur anonyme ne gêne la flotte. La mémoire de
l'hôte Windows peut être indisponible dans le collecteur actuel et reste `null`.

Le cache hors connexion est en mémoire, consultable pendant la session ; il est
effacé au verrouillage. Aucun cache disque chiffré ni contrôle de workers IA n'est
annoncé. `pinned_workers` prépare le contrat, sans inventer de workers observés.

Le retour arrière d'une migration demande sauvegarde/restauration ou migration
corrective, sans down destructif automatique. Supprimer une identité de la vue
Serveurs est une révocation, sans suppression de son historique ou de ses projets.

Voir [ADR 0003](adr/0003-phase-one-identity-and-inventory.md) pour les choix et
frontières de sécurité.
