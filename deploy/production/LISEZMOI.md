# Prisme en production (`ovh-mantara`)

Supervision et alertes seulement, sur `https://prisme.mantara.fr`. Journal complet,
décisions et preuves : [docs/PHASE_10_VPS.md](../../docs/PHASE_10_VPS.md).

| Fichier | Rôle |
|---|---|
| `compose.yml` | Pile `prisme` : PostgreSQL 17, migration, API (mode `identity`), VictoriaMetrics (30 j), Loki (30 j), Alloy ; outil `pki` à la demande. Aucun port publié. |
| `ovh-mantara.env` | Réglages **non secrets** : origine WebAuthn, groupes des sockets Docker (986) et containerd (0). |
| `config.alloy`, `observability.json` | Collecte (hôte, conteneurs, sites) et règles : voir « Collecte de l'hôte et des sites ». |
| `install-host.sh`, `host/` | Collecteurs installés sur l'hôte (inventaire pour l'agent, état des conteneurs pour Alloy) et leurs unités systemd. |
| `init-secrets.sh` | Crée **une fois** `/opt/projects/prisme/secrets` (`root:10001`, `0640`) : mots de passe, CA privée, certificat de l'API. Refuse de remplacer. |
| `renew-server-cert.sh` | Réémet le certificat interne de l'API depuis la même CA et redémarre l'API. |
| `edge-bloc.caddy` | Bloc à ajouter au Caddy mutualisé, avec le certificat **public** de la CA (`pki ca-base64`). |

Toutes les commandes se lancent depuis `/opt/projects/prisme`, avec `sudo` : le
dossier `secrets/` est fermé à l'utilisateur `ubuntu`. Abréviation utilisée ici :

```sh
P="docker compose --env-file deploy/production/ovh-mantara.env -f deploy/production/compose.yml"
```

## Installation (une fois)

1. Clone partiel en lecture seule (clé `~/.ssh/deploy_prisme`, alias `github-prisme`) :
   `git clone --filter=blob:none --sparse git@github-prisme:Aavkd/Prisme.git /opt/projects/prisme`
   puis `git sparse-checkout set deploy cmd internal`.
2. `sudo sh deploy/production/init-secrets.sh`
3. `sudo $P up -d --build --wait` : tous les services `healthy`, `migrate` terminé en 0.

## Collecte de l'hôte et des sites (étape 4)

- `sudo sh deploy/production/install-host.sh` : groupe `prisme-observation` (GID 10002),
  compte `prisme-collector` (groupes `docker` et `prisme-observation`), `prisme-inventory`
  compilé depuis ce dépôt, `host_metrics.py`, minuteurs `prisme-inventory.timer` (60 s,
  `/var/lib/prisme-observation/inventory.json`) et `prisme-host-metrics.timer` (30 s,
  `/var/lib/prisme-metrics/host.prom`). Relançable.
- Alloy (`config.alloy`) : hôte (`/` seulement), cAdvisor pour **tous** les conteneurs, logs
  des seuls conteneurs Prisme, et un check HTTPS par site toutes les 30 s par le réseau `egress`.
- Règles (`observability.json`) : RAM disponible < 1 Gio, disque `/` > 85 %, swap > 75 %,
  conteneur arrêté/en redémarrage/en mauvaise santé, collecte hôte arrêtée (> 3 min), et pour
  chaque site : injoignable ou en erreur (5xx, 404, TLS invalide) et certificat < 14 jours.
- **Nouveau site** : ajouter sa cible dans `config.alloy` et sa portée + ses deux règles dans
  `observability.json`, puis `sudo $P up -d --wait`.
- **Exercice** (aucune panne réelle) : `sudo touch /etc/prisme/drill` ouvre l'incident
  `PRISME-DRILL` en moins d'une minute ; `sudo rm /etc/prisme/drill` le résout.

## Agent d'inventaire (après la première passkey)

L'agent tourne en conteneur (profil `agent`), sans socket Docker : il lit l'inventaire écrit par
`prisme-inventory.timer` et joint l'API directement (`https://prisme-api:8443`, mTLS).

1. Depuis la vue Serveurs de Prisme, générer un jeton d'enrôlement (valable dix minutes).
2. Sur le VPS, le coller soi-même, puis Entrée et Ctrl-D :
   `sudo install -d -m 0750 -o root -g 10001 secrets/agent && sudo sh -c 'umask 027; cat > secrets/agent/token' && sudo chgrp 10001 secrets/agent/token`
3. `sudo $P --profile agent up -d --build --wait agent`, puis supprimer `secrets/agent/token`
   une fois l'agent visible (l'identité reste dans le volume `agent_identity`).

## Mise à jour

```sh
git pull --ff-only
sudo $P up -d --build --wait
sudo $P up -d --force-recreate --no-deps --wait alloy api
```

`config.alloy` et `observability.json` sont montés **fichier par fichier** : `git pull`
remplace le fichier et le conteneur continuerait de lire l'ancien. Recréer `alloy` et `api`
après chaque mise à jour de ces fichiers (troisième commande).

Ne jamais utiliser `down -v` : il supprime la base, les métriques et les logs.

## Certificat interne de l'API

Valable 397 jours (CA : 5 ans). Le renouveler avant l'échéance :
`sudo sh deploy/production/renew-server-cert.sh`. La date est visible avec
`sudo openssl x509 -enddate -noout -in secrets/pki/server.pem`.

## Bloc Caddy

Ne l'ajouter que lorsque `dig +short prisme.mantara.fr @1.1.1.1` **et** `@8.8.8.8`
donnent `51.255.163.21`. Remplacer `__PRISME_CA_DER_BASE64__` par la sortie de
`sudo $P --profile tools run --rm --no-deps -T pki -dir /secrets ca-base64`
(certificat public, pas un secret), sauvegarder le Caddyfile, `caddy validate`,
puis `caddy reload`.
