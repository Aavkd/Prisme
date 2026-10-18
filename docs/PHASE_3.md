# Phase 3 — GitHub, builds et releases

Implémentation locale du 8 septembre 2026. Schéma PostgreSQL **5**, contrat OpenAPI
**0.4.0**, écran Flutter GitHub partagé Windows/Android.

## Disponible

- GitHub App privée : JWT RS256, jetons d'installation courts limités aux IDs de
  dépôts configurés, cache mémoire, expiration et backoff sur refus/rate limits.
- Webhook HMAC SHA-256 sur les octets originaux, validation installation/dépôt,
  déduplication persistante, conflit de livraison rejeté, rétention de 30 jours.
  Les payloads ne modifient pas directement les builds : le polling relit GitHub.
- Synchronisation durable des runs, pagination historique avec curseur en base,
  protection contre les régressions de tentative/date, état de fraîcheur et erreurs.
- Workflows, branches, tags, commits, PR ouvertes/fermées et fichiers en lecture
  seule ; références et chemins encodés ; fichiers UTF-8 limités à 256 KiB.
- Jobs, étapes, runner, horodatages, logs de job, métadonnées d'artefacts et expiration.
  Pagination des jobs/artefacts ; logs bornés à 1 MiB, sans stockage en base.
- Releases immuables par build/tentative, images multi-services référencées par
  digest, environnement et commit exacts, provenance disponible, audit d'import.
  Le [contrat Release](../contracts/release-v1.md) décrit l'artefact de publication.
- Interface FR/EN avec les thèmes Prisme, recherche des dépôts, pages de builds et
  releases, navigateur de sources, états désactivé/chargement/vide/erreur/ancien.
  Réponses tardives et données privées sont éliminées à la fermeture de session.

## Activer une GitHub App existante

1. Créer une App **privée** et l'installer uniquement sur les dépôts choisis. Accorder
   `Metadata: read`, `Contents: read`, `Actions: read`, `Pull requests: read`.
   Aucun PAT du poste de développement n'est utilisé. Les accès d'écriture et les
   commandes de workflow ne sont pas activés par cette phase de consultation.
2. Copier `deploy/github/config.example.json` vers `.dev/github/config.json` et
   remplacer **tous** les IDs/noms d'exemple par ceux de l'App, installation, dépôts
   et workflows qui publient les images. Choisir l'environnement cible explicitement.
3. Placer la clé PEM RSA (2048 bits minimum) dans `.dev/github/app-key.pem` et un
   secret de webhook aléatoire d'au moins 32 caractères dans
   `.dev/github/webhook-secret`. Restreindre les ACL à l'utilisateur et au runtime.
   Les fichiers sont montés en lecture seule ; le coffre produit reste en phase 6.
4. Démarrer le Compose local avec l'extension :

   ```powershell
   docker compose -f deploy/compose.dev.yml -f deploy/compose.github.yml up --build --wait api
   ```

   Le processus natif accepte `PRISME_GITHUB_CONFIG` pointant vers le même schéma
   JSON, avec des chemins de secrets adaptés au poste.
5. Si un endpoint externe a été explicitement configuré, le webhook est
   `POST /v1/github/webhook`. Abonner `workflow_run`, `workflow_job`, `push`,
   `pull_request`, `release` et les événements d'installation. GitHub ne peut pas
   franchir une page de connexion Cloudflare Access : seule cette route signée
   nécessite un routage approprié ; les autres routes restent privées. Aucun
   tunnel, domaine, règle Access ni webhook distant n'est créé par ce changement.
   Le polling fonctionne aussi sans webhook.
6. Adapter le fragment du contrat Release aux vrais workflows après publication
   d'images ; vérifier un build réussi de chaque pilote dans le client.

La configuration d'un ID de dépôt ne peut pas réaffecter son historique à un autre
nom, environnement ou workflow. Une modification de ces liens demande une migration
explicite. Retirer un dépôt de la configuration le rend inaccessible aux routes du
client et aux nouveaux jetons, tout en conservant l'historique local.

## Fréquence, bornes et confidentialité

Le worker se réveille toutes les 30 secondes, lit immédiatement les dépôts marqués
par webhook et rafraîchit normalement toutes les deux minutes. La page récente est
toujours relue ; une page historique supplémentaire est lue par cycle avec reprise
du curseur après redémarrage. L'historique très volumineux converge progressivement,
dans les limites de pagination du fournisseur. Le client relit les builds toutes
les 30 secondes et considère une observation de plus de cinq minutes comme ancienne.

Les réponses fournisseur sont limitées à 4 MiB. Les routes de consultation exposent
uniquement une liste de champs ; elles ne renvoient pas les URLs signées, les URLs
de téléchargement ni les corps arbitraires de PR. Les téléchargements HTTPS sont
limités aux hôtes GitHub Actions/Azure attendus, sans transfert du jeton App et sans
redirection supplémentaire. La rédaction des secrets connus est appliquée aux logs
et aux fichiers **après** décodage base64 ; elle reste une défense complémentaire,
pas une garantie de détection de chaque secret possible. Ne pas publier de secrets
dans le dépôt ni dans les logs des workflows.

Le navigateur Contents est borné à la réponse du fournisseur (1 000 entrées maximum
par dossier). Les logs trop gros et les fichiers binaires sont refusés ; aucun
téléchargement intégral vers l'appareil ni fichier temporaire n'est créé. L'archive
de provenance doit apparaître dans les 100 premiers artefacts du run. Les builds
et releases sont conservés ; leur rétention de production reste à dimensionner.

## Vérifier

```powershell
./tool/verify-foundation.ps1 -Integration
cd apps/prisme_client
flutter analyze --no-pub
flutter test --no-pub
flutter build windows --release --no-pub --dart-define=PRISME_DEVELOPMENT=true
flutter build apk --debug --no-pub --dart-define=PRISME_DEVELOPMENT=true
```

Les tests Go vérifient les JWT, permissions et IDs du jeton, cache/backoff, refus des
redirections non autorisées, absence de credentials sur téléchargement, limites
d'archive et provenance, encodage des références et rédaction des fichiers.
Les tests PostgreSQL/TLS utilisent des schémas temporaires : upgrade depuis phase 0,
deux fixtures de pilotes avec leurs images exactes, replay, reprise sans webhook,
non-régression des tentatives, panne fournisseur, séparation des dépôts et audit.
Les tests Flutter couvrent les parcours à 390 et 1 440 dp, les logs, fichiers,
branches/commits/PR, l'état désactivé, la panne et la révocation en vol.

Résultats acquis le 8 septembre 2026 : vérification Go (formatage, vet, tests,
compilation), suite PostgreSQL/TLS avec les cinq scénarios phase 3, configurations
Compose, génération Dart et validation OpenAPI 3.1 **réussies**. Analyse Flutter
sans diagnostic et **14 tests client réussis**, dont quatre nouveaux tests GitHub.
Compilation Windows release, Android debug et image Linux de l'API **réussies**.
Les artefacts expirés, les archives dont le digest est altéré et les commits
incohérents sont refusés ; un artefact corrigé est importé au prochain passage.
Le moteur Docker de
test, initialement arrêté, a nécessité un redémarrage ; seuls des schémas PostgreSQL
temporaires ont servi aux tests. Aucun essai GitHub sur téléphone physique, aucune
session native contre l'App réelle ni validation du registre n'est revendiqué.

## Qualification opérationnelle restante

Le critère « chaque build pilote est relié à son commit, son image immuable et son
environnement » est exercé **sur fixtures locales**. Il n'est pas attesté sur les
vrais Sounding Lines/Arpent : aucune App réelle ni clé n'a été créée/configurée,
aucun dépôt pilote ou workflow distant n'a été modifié et aucun build distant n'a
été lancé. Sounding Lines nécessite encore la migration du build sur VPS vers une
publication d'images immuables ; le registre cible est à confirmer.

La liaison d'une release prouve la cohérence des métadonnées du workflow configuré,
pas l'existence actuelle du digest dans le registre, une attestation cryptographique
ou la santé de l'environnement. Elle ne déploie rien. Les mutations de code/PR,
dispatch/annulation/relance, déploiements et remédiation sont des capacités distinctes
à qualifier avec droits, confirmation et exécution durable. La tour reste en NO-GO.

Retour arrière local : retirer l'extension Compose et redémarrer l'API courante pour
désactiver l'intégration ; les routes affichent l'état désactivé. Ne pas lancer une
ancienne API contre le schéma 5 et ne pas supprimer les tables. Restaurer une
sauvegarde locale compatible si le binaire doit réellement revenir en arrière.

Références de protocole : [authentification GitHub App](https://docs.github.com/en/rest/apps/apps),
[signatures webhook](https://docs.github.com/en/webhooks/using-webhooks/validating-webhook-deliveries),
[artefacts GitHub Actions](https://docs.github.com/en/rest/actions/artifacts).
