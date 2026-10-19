# Prisme en production (`ovh-mantara`)

Supervision et alertes seulement, sur `https://prisme.mantara.fr`. Journal complet,
décisions et preuves : [docs/PHASE_10_VPS.md](../../docs/PHASE_10_VPS.md).

| Fichier | Rôle |
|---|---|
| `compose.yml` | Pile `prisme` : PostgreSQL 17, migration, API (mode `identity`), VictoriaMetrics (30 j), Loki (30 j), Alloy ; outil `pki` à la demande. Aucun port publié. |
| `ovh-mantara.env` | Réglages **non secrets** : origine WebAuthn, groupes des sockets Docker (986) et containerd (0). |
| `config.alloy`, `observability.json` | Collecte (hôte, Alloy, conteneurs Prisme) et portées ; règles ajoutées aux étapes 4 et 5. |
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

## Mise à jour

```sh
git pull --ff-only
sudo $P up -d --build --wait
```

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
