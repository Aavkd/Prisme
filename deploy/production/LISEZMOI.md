# Prisme en production (`ovh-mantara`)

Supervision et alertes seulement, sur `https://prisme.mantara.fr`. Journal complet,
décisions et preuves : [docs/PHASE_10_VPS.md](../../docs/PHASE_10_VPS.md).

| Fichier | Rôle |
|---|---|
| `compose.yml` | Pile `prisme` : PostgreSQL 17, migration, API (mode `identity`), VictoriaMetrics (30 j), Loki (30 j), Alloy ; outil `pki` à la demande. Aucun port publié. |
| `ovh-mantara.env` | Réglages **non secrets** : origine WebAuthn, groupes des sockets Docker (986) et containerd (0). |
| `config.alloy`, `observability.json` | Collecte (hôte, conteneurs, sites) et règles : voir « Collecte de l'hôte et des sites ». |
| `install-host.sh`, `host/` | Collecteurs installés sur l'hôte (inventaire pour l'agent, état des conteneurs et menaces SSH pour Alloy) et leurs unités systemd. |
| `init-secrets.sh` | Crée **une fois** `/opt/projects/prisme/secrets` (`root:10001`, `0640`) : mots de passe, CA privée, certificat de l'API. Refuse de remplacer. |
| `renew-server-cert.sh` | Réémet le certificat interne de l'API depuis la même CA et redémarre l'API. |
| `edge-bloc.caddy` | Bloc à ajouter au Caddy mutualisé, avec le certificat **public** de la CA (`pki ca-base64`). |
| `compose.notifications.yml`, `notifications.json` | Notifications FCM de l'API (étape 6) : complément de `compose.yml`, à n'utiliser qu'avec `secrets/fcm-api.json` en place ; règles notifiées et règles urgentes. |

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

## Menaces SSH (étape 5)

- `prisme-ssh-metrics.timer` (30 s) lance `ssh_metrics.py` sous `prisme-collector`, **sans
  Docker** et avec le groupe `adm` pour cette seule unité (journal de `ssh.service`,
  `/var/log/fail2ban.log`), et écrit `/var/lib/prisme-metrics/ssh.prom`. Aucune adresse n'entre
  dans les mesures : une connexion inconnue est décrite dans
  `journalctl -u prisme-ssh-metrics` (adresse, compte, clé).
- **Adresses connues** : `/etc/prisme/ssh-known-ips` (`root:prisme-observation`, `0640`), une
  adresse ou un réseau par ligne, `#` pour les commentaires ; **hors dépôt**, validé par Alexy.
  Sans ce fichier, le collecteur ne démarre pas. Toute connexion réussie depuis une autre adresse
  (mobile, déplacement, déploiement GitHub Actions) ouvre `VPS-SSH-UNKNOWN-LOGIN`.
- Règles : connexion réussie depuis une adresse inconnue (critique, se résout 30 min après),
  fail2ban arrêté (critique, après 1 min), plus de 150 échecs sur 10 min maintenus 10 min
  (avertissement), fail2ban actif mais ne détectant aucun de 20 échecs ou plus (avertissement,
  après 15 min), collecte SSH arrêtée (avertissement). Les deux critiques seront `urgent` dans la
  configuration des notifications.
- **Exercices** (n'ajoutent que des alarmes, n'en masquent aucune) : `sudo touch` puis `sudo rm`
  de `/etc/prisme/drill-ssh-unknown-login`, `drill-fail2ban-down`, `drill-ssh-failures`
  (incident après 10 min) ou `drill-fail2ban-blind` (après 15 min).
- Essai à blanc sans rien écrire (sortie à l'écran) :
  `sudo setpriv --reuid=prisme-collector --regid=prisme-observation --groups=prisme-observation,adm --inh-caps=-all python3 -I /usr/local/lib/prisme/ssh_metrics.py /etc/prisme/ssh-known-ips -`.

## Agent d'inventaire (après la première passkey)

L'agent tourne en conteneur (profil `agent`), sans socket Docker : il lit l'inventaire écrit par
`prisme-inventory.timer` et joint l'API directement (`https://prisme-api:8443`, mTLS).

1. Depuis la vue Serveurs de Prisme, générer un jeton d'enrôlement (valable dix minutes).
2. Sur le VPS, le coller soi-même, puis Entrée et Ctrl-D :
   `sudo install -d -m 0750 -o root -g 10001 secrets/agent && sudo sh -c 'umask 027; cat > secrets/agent/token' && sudo chgrp 10001 secrets/agent/token`
3. `sudo $P --profile agent up -d --build --wait agent`, puis supprimer `secrets/agent/token`
   une fois l'agent visible (l'identité reste dans le volume `agent_identity`).

## Notifications (étape 6)

- Clé d'un compte de service **limité à l'envoi FCM** dans `secrets/fcm-api.json`
  (`root:10001`, `0640`), écrite sans être affichée (procédure :
  [docs/ETAPE_6_PROCEDURE.md](../../docs/ETAPE_6_PROCEDURE.md), partie D).
- Une fois la clé en place, `P` inclut **toujours** le second fichier (un `up` sans lui
  recréerait l'API sans notifications ; avec lui mais sans clé, l'API refuse de démarrer) :
  `P="docker compose --env-file deploy/production/ovh-mantara.env -f deploy/production/compose.yml -f deploy/production/compose.notifications.yml"`.
- `notifications.json` : les 67 règles notifient ; seules `VPS-SSH-UNKNOWN-LOGIN` et
  `VPS-FAIL2BAN-DOWN` sont `urgent` (traversent les horaires silencieux réglés sur chaque
  appareil) ; aucune relance. Un test Go vérifie qu'aucune règle n'est oubliée. Toute nouvelle
  règle d'`observability.json` y reçoit sa ligne.
- L'API joint `fcm.googleapis.com` par le réseau `edge` (le réseau `control` n'a pas de sortie).

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
