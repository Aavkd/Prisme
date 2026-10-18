# Contrat agent v1 — phases 0 et 1

Les échanges structurés sont définis dans [OpenAPI](openapi.json). La version
`"1"` reste acceptée ; les capacités ne sont jamais inférées de ce numéro.

1. Un client authentifié récemment émet `POST /v1/enrollment-tokens` avec
   `capabilities` et une clé d'idempotence de 16–128 caractères. L'objet vide
   historique reste accepté et accorde uniquement `self:heartbeat`.
   Même session/clé → même token/expiration ; capacités différentes → HTTP 409.
2. L'agent crée localement une clé P-256 et persiste sa CSR exacte avant l'appel
   d'enrôlement. Le token est valable dix minutes et consommé transactionnellement.
   Le serveur choisit l'ID et le sujet du certificat client, valable 24 heures.
   Un rejeu exact récupère le certificat public ; une autre CSR/nom est refusée.
3. `POST /v1/agent/heartbeat` exige mTLS, une identité SQL active et la version 1.
   Il renvoie les capacités actuellement accordées. La révocation est vérifiée
   même sur une connexion TLS déjà ouverte. Le binaire envoie un signal toutes
   les 30 secondes, sans file de heartbeats anciens.
4. Dans les huit dernières heures de validité, le client appelle
   `POST /v1/agent/renew` avec une CSR signée par sa clé existante. Il valide le
   nouveau certificat et remplace atomiquement le fichier avant de changer son
   transport. La série précédente reste acceptée dix minutes ; son rejeu de
   renouvellement récupère le certificat courant et ne prolonge pas la tolérance.
5. `POST /v1/agent/inventory` exige `inventory:host`, plus `inventory:docker`
   lorsque le snapshot contient des observations Docker. Le certificat désigne
   la cible : aucun agent ne peut choisir un autre ID dans le corps. Une réduction
   de capacités ou révocation sérialise les mutations par verrou SQL de l'agent.
6. `InventorySnapshot` contient une source `agent`, date UTC, hôte, capacités,
   conteneurs, images, réseaux/volumes et avertissements ; 2 Mio maximum,
   1 000 éléments par famille. Champs inconnus, doublons de conteneurs, caractères
   de contrôle, dates futures hors tolérance et observations de plus de 24 heures
   sont refusés. Un snapshot antérieur ou identique ne remplace pas le plus récent.

## Capacités

- `self:heartbeat` : obligatoire ; connexion et renouvellement.
- `inventory:host` : lecture de l'observation hôte.
- `inventory:docker` : nécessite `inventory:host` ; observations Docker/Compose.

Toute autre capacité est refusée. Aucune exécution de plan, commande shell,
terminal, modification Compose ou opération de déploiement n'est disponible.
La capacité historique `self:heartbeat` préserve le fonctionnement des agents
phase 0 ; ne pas étendre leurs capacités sans mettre à jour le binaire.

## Séparation et reprise

Le cœur agent fonctionne sans privilèges, sans écoute entrante, socket Docker,
clé SSH ou clé privée CA. Un collecteur séparé produit un fichier de snapshot
atomique ; `-inventory-file` autorise sa lecture. Les unités dans `deploy/systemd`
sont des modèles non installés. Les capacités Docker du collecteur ne sont pas
héritées par le cœur agent. Le collecteur seul dispose du groupe Docker, puissant
au niveau OS, et n'exécute que les lectures structurées livrées.

Les répertoires d'identité restent 0700 et les clés 0600 (ACL explicites Windows).
Ne pas partager un répertoire entre deux processus agents. Au redémarrage,
certificat/clé sont rechargés. La coupure conserve seulement le dernier snapshot
collecté ; il n'y a pas de file de commandes. Une identité expirée ou révoquée
nécessite un nouvel enrôlement explicite.

## Validation du VPS sans installation

`cmd/inventory -ssh` utilise une session SSH existante en lecture seule pour une
collecte ponctuelle, puis `POST /v1/inventory-imports` associe cette photographie
à une identité locale `ssh-readonly`. Ce n'est pas un certificat agent, et ce chemin
ne donne aucun contrôle distant au backend. La fraîcheur d'inventaire expire après
90 secondes, indépendamment de l'état de connexion de l'API.
