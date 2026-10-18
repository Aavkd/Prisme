# Phase 4 — Déploiements Docker Compose

Implémentation locale du 8 septembre 2026. Schéma PostgreSQL **6**, OpenAPI **0.5.0**.
Le critère logiciel est vérifié sur une stack jetable réelle. Le critère de sortie
**opérationnel Sounding Lines reste ouvert** : ses images immuables, son schéma,
ses dépendances, ses sauvegardes, ses checks et son accès renforcé restent à qualifier.

## Livré

- Manifeste JSON v1 strict, [schéma](../contracts/project-v1.schema.json),
  [contrat](../contracts/project-v1.md) et validation CLI sans mutation.
- [Import Sounding Lines](../deploy/deployment/sounding-lines.import.json) désactivé,
  identifié comme historique. Aucun ID GitHub ni digest réel n'est inventé. Les
  ports proposés et la séparation des services applicatifs sont à qualifier.
- Plans persistants, versionnés, de 15 minutes ; provenance phase 3, images par
  digest, branche autorisée, comparaison avec la dernière release saine, références
  de secrets sans valeurs, préconditions, stratégie, étapes et retour arrière.
- Préparation idempotente ; approbation par cible exacte et révision, contrôle du
  baseline, annulation avant démarrage, audit atomique. L'approbation production
  est désormais raccordée aux preuves renforcées de [phase 5](PHASE_5.md) ; les
  préconditions et qualifications des cibles restent obligatoires.
- Worker local séparé de l'API, file PostgreSQL durable, verrou par cible entre
  processus, une seule opération active, heartbeat, timeout, journal borné par étape.
- Génération Compose bornée : aucune commande utilisateur ni build, image immuable,
  ports loopback, réseau de dépendance externe facultatif, fichiers d'environnement
  privés. Les fichiers sont liés au plan par empreinte et recontrôlés avant exécution.
- Vérification de l'espace disponible du workspace **et** des données Docker,
  ownership des conteneurs, validation Compose, pull, démarrage, checks de chaque
  service, observation puis promotion atomique en release saine.
- Recréation contrôlée et blue-green pour services explicitement stateless, une
  entrée HTTP gérée. Caddy change uniquement les upstreams d'un handler préexistant
  possédé par Prisme, avec comparaison ETag/If-Match. La route historique reste hors
  périmètre. Les deux slots sont conservés ; aucun prune global n'est exécuté.
- Restauration approuvée vers une release déjà prouvée saine sur le même manifeste.
  Une migration incompatible, une sauvegarde obligatoire non vérifiée ou une
  configuration de secrets différente bloque l'action.
- Écran Déploiements FR/EN Windows/Android : sélection cible/release, plan,
  confirmation, historique paginé, étapes/preuves, fraîcheur du worker, annulation
  sûre et état incertain. La fermeture de session efface les données et dialogues.

## Exécuter et importer

```powershell
# Lecture et validation uniquement, fonctionne sur Windows et Linux.
go run ./cmd/deployer -manifest deploy/deployment/sounding-lines.import.json

# Checks locaux avec PostgreSQL/TLS dans des schémas jetables.
./tool/test-integration.ps1

# Exercice Docker/Caddy réel, limité aux ressources prisme-phase-four-live-*.
./tool/test-deployments.ps1
```

Pour enregistrer des cibles, créer un fichier de configuration privé contenant
`version: 1`, `root` absolu et `targets` (liste de manifestes v1). Un import legacy
peut être enregistré sans GitHub connecté : il reste visible et bloqué. Fournir
ce fichier à l'API via `PRISME_DEPLOYMENT_CONFIG` après la migration explicite
`api -migrate`. Les paramètres TLS et PostgreSQL existants restent nécessaires.

L'API enregistre les manifestes, mais ne lance jamais de commandes Docker. Installer
le worker **sur l'hôte Linux du moteur Docker**, avec accès local à Docker/Compose,
au répertoire de données Docker (contrôle disque) et aux fichiers privés référencés :

```text
deployer -config /chemin/prive/deployments.json
```

Le worker utilise la configuration PostgreSQL existante (`PRISME_DATABASE_URL` ou
les fichiers de secrets supportés par `internal/config`), avec le schéma migré.
API et worker partagent le même manifeste et la même vue des fichiers privés.
Un DockerRootDir inaccessible bloque l'exécution : ne pas assimiler l'espace libre
Windows à celui du disque Docker Desktop. Le test utilise une fixture Linux dans
Docker Desktop, le réseau de l'hôte Linux et le montage du répertoire de données
en lecture seule. Aucun service ni worker distant n'est installé implicitement.

Pour une nouvelle cible blue-green, provisionner explicitement un handler Caddy
`reverse_proxy` avec `@id: prisme-…`, un seul upstream `127.0.0.1:1`, et un endpoint
HTTP de test. L'exercice automatique montre ce provisionnement sur une fixture.
Les services de données, migrations, réseaux historiques et secrets réels doivent
être préparés séparément ; le fichier importé n'autorise pas leur transformation.

Les IDs de cible et manifestes enregistrés sont immuables. Une modification de
configuration ne remplace jamais silencieusement celle des releases saines.
Préparer une évolution explicite d'ownership avant de migrer une cible existante.

## Reprise, erreurs et rollback

| Situation | Résultat et reprise |
|---|---|
| Rejeu de préparation/approbation | Même travail ; conflit si le contenu ou la révision diffère |
| Plan expiré, baseline modifié, cible occupée | Approbation refusée ; préparer un nouveau plan |
| Annulation avant exécution | `cancelled`, aucune commande ; refus après claim du worker |
| Précondition/pull échoue | `failed`, aucune promotion ; nouvelle demande explicite après correction |
| Check échoue après démarrage/bascule connue | `failed`, dernière release saine conservée ; rollback approuvé possible |
| Connexion perdue ou redémarrage pendant mutation | `uncertain`, cible réservée ; aucune répétition automatique de commande |
| Runtime candidat vérifiable après interruption | Relecture des images, checks, route et observation complète, puis promotion auditée |
| Runtime ne peut être prouvé | Reste `uncertain` ; diagnostic opérateur requis, aucune levée arbitraire du verrou |

Le worker ne réessaie pas une mutation incertaine et ne tue pas arbitrairement une
opération en cours. Les appels ont une durée bornée et le worker interroge sa connexion
de verrou entre étapes. La réconciliation est exclusivement en lecture. Si elle ne
peut pas établir l'état voulu, cette version ne propose pas de bouton « forcer » ou
de reprise avec mutation : la qualification d'une réparation opérateur reste ouverte.
Les événements contiennent des preuves choisies et codes d'erreur, jamais stdout,
stderr, fichier d'environnement ou sortie fournisseur brute.

## Preuves

- Tests Go et PostgreSQL/TLS : upgrade 005→006, préparation sans exécution,
  idempotence, phrase/révision, audit, concurrence de workers, verrou de cible,
  expiration, annulation sûre, déploiement/restauration, échec de santé, refus de
  rollback non sain, dérive du manifeste, reprise sans rejouer la mutation.
- Tests du driver : validation stricte et chemins refusés, fichiers secrets liés au
  plan, Compose sans valeurs de secrets ni commandes libres, ownership Caddy et CAS.
- Test réel `TestPhaseFourLiveComposeBlueGreen`, Docker Desktop Linux : Caddy →
  Nginx → Caddy, trois releases saines avec les vrais digests du registre. **551
  requêtes HTTP, zéro échec** pendant remplacement et restauration lors du contrôle
  intégrant la vérification du disque Docker et l'arrêt des sous-processus. Ce résultat concerne cette fixture,
  pas une promesse de continuité pour un pilote avec données et traitements actifs.
- Tests Flutter à **390 / 1 440 px** : ouverture du plan, approbation avec phrase,
  absence de mutation avant confirmation, effacement à la fermeture de session,
  absence d'action dangereuse sur résultat incertain.
- Suite client complète : **17 tests réussis**, analyse Flutter sans problème et
  build Windows debug compilé. Contrats OpenAPI et schéma du manifeste validés.

## Limites conservées

Le transport de plans signés vers l'agent distant, la révocation/fencing de secours,
la qualification d'une VM de production, les migrations exécutables et sauvegardes
vérifiées restent ouverts. Le worker actuel est un exécuteur local de confiance
connecté à PostgreSQL, pas un remplacement du protocole agent mTLS. L'API ne reçoit
pas la socket Docker.

Les politiques de retry automatique, fenêtres planifiées, nettoyage automatique,
rolling et plusieurs routes Caddy atomiques ne sont pas activées. Le premier
blue-green couvre un sous-ensemble stateless avec une entrée HTTP ; il ne duplique
pas PostgreSQL ni un worker audio. Les limites de ressources Compose actuelles
(512 MiB/1 CPU par service) doivent être adaptées lors de la qualification du pilote.
La qualification interactive native S24/Windows et l'authentification renforcée restent distinctes
des tests widget. Aucun tunnel, dépôt GitHub pilote, VPS, pare-feu ou Caddy historique
n'est modifié par cette livraison.
