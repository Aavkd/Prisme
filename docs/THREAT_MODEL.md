# Modèle de menaces initial — phase 0

Date : 6 septembre 2026. Périmètre : environnement local, utilisateur de développement
de confiance, client Flutter, API Go, PostgreSQL, agent et CA de développement.

## Actifs et frontières

Secrets bootstrap/session/enrôlement, clés privées CA/serveur/agent, inventaire et
audit. Frontières : client → API TLS ; agent → API TLS/mTLS ; API → PostgreSQL sur
réseau Docker interne ; fichiers privés `.dev` → processus utilisateur/conteneurs.
Un administrateur Windows ou Docker peut lire ces actifs : cette phase ne prétend
pas protéger l'utilisateur contre son propre administrateur local.

| Menace | Contrôle livré | Preuve / limite |
|---|---|---|
| Écoute ou faux serveur | TLS 1.3, CA approuvée, vérification hostname, aucun redirect ni bypass | Tests mTLS + smoke Dart réel ; aucune CA globale installée |
| Vol de la base | Tokens/session uniquement sous forme d'empreintes ; secret bootstrap absent de la base | Vérification de la persistance ; chiffrement du volume de production à prévoir |
| Force brute / saturation | Secrets 256 bits ; 120 requêtes/min globales, corps 16 Kio, timeouts serveur/client, pools bornés | Limiteur local global : un processus local peut provoquer un déni de service ; revoir avant publication |
| Rejeu d'enrôlement | Token de 10 minutes, transaction et verrou de ligne, liaison CSR/nom | Rejeu identique récupère seulement le certificat public ; une autre clé est refusée |
| CSR malveillante | Signature vérifiée, P-256 ; sujet, SAN, extensions ignorés ; certificat client seul | Test de CSR prétendant être administrateur |
| Escalade agent | Certificat enregistré + série active vérifiés à chaque heartbeat ; capacité fixe | Certificat agent sans session incapable de lire la flotte ; aucun shell, socket Docker, mount hôte ou API d'exécution |
| Identité expirée/révoquée | Vérification TLS + vérification SQL à chaque heartbeat ; expiration des sessions | Tests sur connexion TLS déjà ouverte ; interface de révocation/rotation à construire |
| Fuite dans les journaux | Aucun corps, token, DSN ou clé journalisé ; erreurs publiques génériques | Test d'audit ; messages SQL/transport non propagés au client |
| Demi-mutation | Migration et mutations d'identité atomiques avec audit | Migration relançable et concurrence d'enrôlement testées |
| Cache trompeur | Pas de cache disque ; date de lecture et erreur explicites ; aucun bouton de mutation d'infrastructure | Tests Flutter de perte de connexion et session révoquée |
| Mauvaise exposition | Port API IPv4 loopback seul ; DB/agent sans port ; mode production refusé | Inspection Compose ; tout tunnel de phase 0 serait hors périmètre |
| Contenu non fiable | Noms, logs, prompts et sorties ne sont jamais des commandes | API structurée, agent sans exécution ; conserver ce principe pour les connecteurs |
| Supply chain | Dépendances et versions SDK verrouillées, CI, images minimales sans shell | Signatures d'artefacts, scans et pinning de toutes images par digest à durcir avant production |

## Dérogations locales explicites

Une CA locale unique signe serveur et agents ; elle n'est pas le coffre final.
Le secret bootstrap est réutilisable et n'est pas un mot de passe utilisateur.
L'émission de session peut créer une autre session après une réponse perdue ; la
session perdue expire en une heure. Aucun retry automatique de cette opération.
Heartbeat est une observation idempotente sans journal d'audit par battement ;
les mutations d'identité réussies sont auditées. Le journal de tentatives refusées,
les rôles, authentification forte et politiques d'audit finales sont en phase 1.

Les fichiers de développement sont protégés par ACL Windows dans `tool/dev.ps1`.
Les modes Unix seuls ne suffisent pas sur Windows. Ne pas copier ces secrets dans
un dossier partagé ; ne pas rendre les clés lisibles globalement pour résoudre
un problème Docker. Le Compose a été qualifié sur Docker Desktop Windows ; sur
Linux, préparer des ACL de lecture pour UID 10001 sur les fichiers montés avant usage.

## Extension phase 1 — 7 septembre 2026

L'ADR 0003 et `PHASE_1.md` remplacent les limites de phase 0 ci-dessus pour les
fonctions livrées : identité WebAuthn, appareils, récupération, renouvellement,
révocation, capacités et audit contextualisé. Les frontières restent locales
jusqu'à qualification du domaine, d'Access et de l'hébergement.

- WebAuthn exige RP/origine/challenge/signature et vérification utilisateur ;
  tests de mauvaises origines, UV absente, signature forgée et rejeu.
- Le secret d'initialisation cesse d'autoriser une inscription après le premier
  authentifiant. Le mode identité refuse les anciennes sessions de développement.
- Parcours navigateur, secret de polling et session sont distincts et bornés.
  Codes de secours et tokens sont conservés sous empreinte ; aucun secret en URL.
- La récupération remplace anciennes passkeys, sessions, codes et parcours.
  La révocation d'appareil n'efface pas une passkey synchronisée chez son fournisseur.
- Le collector est un processus séparé : accès Docker puissant mais commandes
  fixes en lecture seule. Ni env ni commandes ni sorties de healthchecks ne sont
  exportées. La collecte SSH ne modifie pas le VPS.
- L'audit des mutations fait partie de leur transaction. Un trigger interdit la
  réécriture ordinaire ; les refus sont enregistrés par route, sans corps/URL brute.
- Rotation du certificat avec dix minutes de recouvrement, puis refus de l'ancienne
  série. Révocation et observation s'excluent par verrou SQL.
- Le verrouillage client invalide les réponses tardives ; les dialogues de secrets
  sont fermés et effacés quand la session se verrouille. Le cache reste en mémoire.

Les authentifiants virtuels des tests ne prouvent pas Windows Hello, les passkeys
synchronisées du S24 ou la biométrie des futures actions de production. Le limiteur
reste global et peut causer un déni de service entre appelants ; durcir l'admission
avant exposition. Un administrateur de la base ou du groupe Docker reste privilégié.

## Phase 3 — GitHub

Les jetons App sont courts, en mémoire et limités aux IDs explicitement configurés,
avec permissions de lecture. La clé privée et le secret webhook sont fournis par
fichiers secrets en lecture seule ; le coffre produit n'est pas encore implémenté.
Les webhooks signés vérifient installation et dépôt, dédupliquent les livraisons et
déclenchent une relecture canonique. Un rejeu ne peut pas rétablir un ancien build.

Les réponses fournisseur sont bornées et projetées sur des champs autorisés. Les
URLs signées ne sont pas exposées ; leur téléchargement HTTPS n'emporte aucun
jeton et ne suit pas de nouvelle redirection. Les fichiers sont décodés avant
rédaction. Logs et sources restent des données non fiables, jamais des instructions.
La rédaction ne garantit pas de détecter tous les secrets possibles.

Une release exige le workflow configuré, une réussite hors PR/fork, le même dépôt,
commit, run/tentative et environnement, et des références d'images par digest.
Elle est immuable et auditée. La compromission du workflow autorisé reste une
frontière de confiance : le manifeste n'est pas une attestation cryptographique,
une vérification de registre ou une preuve de santé du déploiement. Aucun accès
d'écriture GitHub ni commande de production n'est accordé par l'intégration.

## Extension phase 4 : exécution Compose locale

Les requêtes clientes ne contiennent ni commande, ni fichier Compose, ni URL de
commande distante. Un manifeste enregistré lie la cible, les services, la branche,
les images et les routes autorisées. Approbation et audit sont transactionnels ;
une opération incertaine réserve la cible. Les secrets restent dans des fichiers
privés dont les empreintes lient le plan. Le worker ne journalise pas les sorties
Docker brutes. Caddy est modifié uniquement sur un handler possédé avec ETag.

Le worker est une frontière de confiance privilégiée, distincte de l'API : accès
local Docker et PostgreSQL explicites, sans transport distant nouveau. Il ne fournit
pas encore le protocole de plans signés, le fencing indépendant ni la réparation
forcée d'un état incertain. L'approbation production reste refusée par l'API tant
que l'authentification renforcée des clients n'est pas disponible.

## Phase 5 : transport mobile et preuves d’action

Les messages FCM sont des entrées non fiables : routes strictes, IDs opaques,
aucune action automatique. Les détails exigent une session applicative, même
après ouverture d'une notification. Une révocation empêche les nouvelles
livraisons du control plane ; elle ne retire pas une notification déjà acceptée
par FCM. Le watchdog indépendant possède une liste de tokens à tenir à jour.

L'approbation production exige une preuve liée à session, appareil, révision et
travail. Le challenge expire, le serveur vérifie la signature, et la preuve est
consommée dans la transaction d'approbation. Une phrase ou un booléen client
n'est pas une preuve. Android protège la clé avec une biométrie forte par
signature ; l'attestation matérielle distante n'est pas vérifiée. L'enrôlement
requiert une session passkey récente. Windows impose UV WebAuthn sans prétendre
distinguer PIN et biométrie.

Le cache opt-in sélectionne des résumés bornés, sépare origines/appareils et
utilise le stockage chiffré natif. Une vue dédiée indique les dates et interdit
les mutations. Une révocation hors ligne ne peut être apprise avant reconnexion ;
la protection locale demeure celle du terminal et de son déverrouillage.

## Phase 6 : opérations SSH, coffre et secours indépendant

Les cibles SSH, empreintes hôtes et racines SFTP sont configurées côté serveur.
Les tickets WebSocket sont courts, à usage unique et transmis en en-tête ;
l'attachement exige la même session et la même cible que la preuve. Le client
refuse les redirections pendant le handshake TLS. Les preuves sensibles lient
action, charge utile, cible, session et appareil ; une nouvelle clé Android
invalide les preuves créées avec l'ancienne. Les entrées terminal ne sont jamais
rejouées automatiquement après une déconnexion.

Le terminal donne les droits réels du compte SSH configuré. Il faut donc un compte
restreint : un shell peut contourner les protections offertes par l'éditeur SFTP.
Les liens symboliques et chemins protégés sont refusés par cet éditeur ; une
course avec un processus privilégié qui modifie le système de fichiers reste une
limite du protocole SFTP. Les écritures exigent révision, backup privé, fsync et
renommage atomique. Les valeurs du coffre utilisent des clés de données distinctes
enveloppées par une clé maîtresse hors base, avec métadonnées authentifiées.
La perte de cette clé rend la restauration impossible ; conserver sa copie privée
avec un dispositif de sauvegarde indépendant. La rédaction des aperçus et diffs
reste heuristique et ne constitue pas une classification exhaustive des secrets.

Le verrou de coordination est local à l'hôte et commun au worker et aux opérations
SSH. Une mutation incertaine laisse une réservation persistante. Le secours
suspend l'automatisation jusqu'à réconciliation opérateur et incrémente l'époque
qui invalide les anciens plans. La coopération de tous les exécutants et l'identité
Unix commune du garde restent nécessaires ; ce mécanisme ne neutralise pas root.

La passerelle native possède ses propres passkeys, appareils, sessions et clés
d'audit. L'API normale et PostgreSQL ne participent pas à son authentification.
Les lots d'audit signés sont importés avec curseur contigu et déduplication, sans
reprendre automatiquement l'automatisation. L'indépendance réelle du tunnel, de
l'alimentation, du démarrage systemd et de la biométrie physique reste à qualifier
sur l'infrastructure cible. Les limites et preuves locales figurent dans PHASE_6.

## Phase 7 : sauvegardes et restaurations

Le worker lit les credentials privés ; l'API expose uniquement des IDs et
métadonnées. Les demandes ne peuvent choisir ni chemin, ni SQL, ni exécutable.
Les backends exécutables Restic et les connexions S3 sans HTTPS sont refusés.
Le mot de passe du dépôt reste indépendant de PostgreSQL et des sauvegardes de
cette base. Un dépôt perdu avec sa seule clé n'est pas récupérable.

Restic vérifie le producteur pg_dump et le contenu chiffré avant publication d'un
artefact. Chaque restauration répète les vérifications et crée une cible nouvelle.
Le rôle PostgreSQL de restauration n'est pas superuser. Un dump reste un programme
SQL potentiellement hostile : ne restaurer que des dépôts/source de confiance
avec ce rôle restreint sur un cluster isolé. Les archives temporaires sont privées.
Le contrôle SQL est un fichier opérateur de confiance, jamais une entrée client.

Rétention et restauration manuelles exigent une preuve à usage unique liée au
travail, à sa révision et à la session. La rétention protège les copies vérifiées
contre leur éviction par des snapshots non enregistrés. Aucun rejeu après résultat
incertain ni déverrouillage automatique de Restic. Les gardes nécessitent la même
identité Unix et les mêmes fichiers de coordination sur l'hôte réel ; une simple
étiquette de serveur distant ne crée pas de verrou distribué.

La propriété hors site est déclarée par l'opérateur ; un succès de sauvegarde
ne prouve pas la localisation physique du stockage. Le mode volume suppose un
arbre arrêté ou un snapshot cohérent préparé hors du worker. La politique ne peut
pas rendre cohérent un système de fichiers modifié simultanément par un tiers.

## Phase 8 — plans edge

Les entrées client sélectionnent une cible et un upstream autorisés ou le contenu
d’un DNS A/AAAA/CNAME ; elles ne définissent ni URL fournisseur ni commande libre.
Les jetons de lecture/écriture sont séparés et montés uniquement dans le worker.
Les plans immuables, preuves par révision, snapshots et gardes bornent les mutations.
Caddy applique `If-Match` sur `/config/`. Cloudflare DNS ne fournit pas cette
condition dans le contrat retenu : la relecture avant/après ne supprime pas la
course avec un écrivain externe. Toute réponse ambiguë requiert réconciliation.
Les snapshots privés doivent être protégés avec PostgreSQL et ses sauvegardes.
Les fichiers historiques ne doivent pas contenir de secrets littéraux ; éviter
les écrivains échappant au garde. Voir le contrat edge v1 pour les limites.

## Phase 9 — automatisation bornée

La configuration fixe l’hôte, le projet, l’environnement, le service et la règle.
L’activation et les versions de politiques exigent une preuve fraîche ; l’arrêt
global est actif initialement. Une panne autorise une seule exécution bornée par
incident. Ni un changement de version ni un redémarrage ne réinitialise cette
limite. Les résultats incertains conservent le circuit et la cible occupée.

Le worker ne dispose d’aucune commande libre. Il vérifie le propriétaire et le
digest du conteneur avant restart, puis réutilise les contrôles de déploiement
pour rollback. Le garde d’hôte, le verrou PostgreSQL de cible et la liaison du job
au run empêchent un autre worker de contourner l’arrêt. La perte d’un verrou ou
d’une réponse ne justifie jamais le rejeu d’une mutation ; les écritures de
résultat sont conditionnées par la révision pour éviter un résultat tardif.

Une preuve d’étape expire après deux minutes avant dispatch. L’arrêt pendant une
commande est un résultat incertain, car Docker peut avoir accepté la demande.
La réconciliation vérifie le garde et le verrou local avant de retirer les jobs
liés, sans promotion ni replay. Le worker continue à appliquer les exigences de
migration, sauvegarde et secrets du manifeste. Les logs et titres externes ne
peuvent définir des actions ni des paramètres exécutables.
