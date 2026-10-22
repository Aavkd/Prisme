# Scripts d'exploitation (depuis un poste, par SSH)

Lancés depuis la racine du dépôt, sur la tour ou le portable, sans rien copier sur le VPS :

```sh
ssh -o BatchMode=yes ovh-mantara 'bash -s' < deploy/production/ops/<script>.sh
```

| Script | Effet | Accord d'Alexy |
|---|---|---|
| `etat.sh` | État général : pile, RAM, disque, passkeys, appareils, agents, règles, incidents, notifications envoyées (des nombres, aucun secret). | non (lecture seule) |
| `verif-collecte.sh` | Séries VictoriaMetrics : sites, disque, swap, conteneurs, collecte hôte. | non |
| `verif-ssh.sh` | Collecteur SSH, `ssh.prom`, séries SSH/fail2ban, état des règles. | non |
| `observation-ssh.sh [date]` | Faux positifs des règles SSH depuis une date (défaut : 24 h) : `'bash -s -- "AAAA-MM-JJ HH:MM"'`. | non |
| `codes-sites.sh` | Code HTTP de chaque site du Caddy mutualisé, joint en local (avant/après une modification de Caddy). | non |
| `avec-memoire.sh "<commande>"` | Lance une commande en relevant RAM disponible minimale et swap maximal. | selon la commande |
| `exercice-drill.sh` | Crée puis supprime `/etc/prisme/drill` : incident `PRISME-DRILL` (information) ouvert puis résolu. | **oui** |
| `exercice-ssh.sh` | Quatre drapeaux d'exercice SSH (≈ 25 min, à lancer en arrière-plan) : quatre incidents ouverts puis résolus ; deux sont **urgents** une fois les notifications actives. | **oui** |

Sous Git Bash (Windows), `ssh` reçoit le script par l'entrée standard : rien à convertir. Les
requêtes SQL passent par `docker compose exec` avec `compose.yml` seul, ce qui suffit pour lire
la base même quand les notifications sont actives (aucun conteneur n'est recréé).
