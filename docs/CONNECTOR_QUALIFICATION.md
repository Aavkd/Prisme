# Qualification initiale Codex / Claude Code

Relevé du 6 septembre 2026, lecture seule. Les capacités ci-dessous ne sont pas
activées dans Prisme. Aucun compte, session, daemon ou configuration de ces outils
n'a été modifié et aucune requête de modèle n'a été lancée.

## Éléments locaux vérifiés

- **Codex CLI 0.144.1** : aide `app-server`, `daemon` et génération locale du schéma
  JSON réussie. Le schéma exporté contient `ThreadReadParams`, `ThreadResumeParams`,
  `TurnSteerParams`, `TurnInterruptParams`, `ThreadTokenUsageUpdatedNotification`
  et `GetAccountRateLimitsResponse`. `TurnSteerParams` exige `expectedTurnId`.
- **Claude Code 2.1.207** : aides `claude --help` et `claude agents --help` lues.
  Sont présents `--print`, `--input-format stream-json`, `--output-format stream-json`,
  `--resume`, `--continue`, `--background`, `--remote-control` et `agents --json`.
  Cette aide ne démontre pas un canal public de pilotage d'une session interactive tierce.
- Les fichiers de credentials, historiques de tâches et contenus de conversations
  n'ont pas été collectés. Le schéma Codex temporaire reste dans `.dev/codex-schema`.

## Matrice d'activation

| Mode | Observation | Lancer / consigne / interruption | Décision phase 0 |
|---|---|---|---|
| Codex, app-server possédé par le futur relais | Protocole documenté ; événements et tokens présents localement | Méthodes start/steer/interrupt documentées, précondition de turn vérifiée dans le schéma | Candidat pour un adaptateur ; test de session isolée nécessaire |
| Codex CLI interactif déjà ouvert | Historique lisible ne prouve pas un abonnement aux événements vivants | Rattachement au processus propriétaire non qualifié | Capacité indisponible |
| Codex application Windows déjà ouverte | Le binaire CLI n'est pas une preuve d'accès au serveur de l'application | Contrat d'attachement et contrôle concurrent non qualifiés | Capacité indisponible |
| Claude Code, processus possédé par le futur relais | Flux JSON documenté et options locales confirmées | Entrées stream-json / reprise candidates ; interruption structurée à tester sur le processus possédé | Candidat ; non activé |
| Claude Code interactif déjà ouvert | Les options agents/remote-control sont présentes ; pas de test d'accès à une session existante | Pas de preuve d'un canal tiers supporté dans cette qualification | Capacité indisponible |

Le protocole Codex documente `thread/read`, `thread/resume`, `turn/start`,
`turn/steer` et `turn/interrupt`. Reprendre un historique et piloter une exécution
déjà chargée par un autre serveur restent deux cas distincts.
[Documentation officielle App Server](https://learn.chatgpt.com/docs/app-server).

Codex distingue connexion ChatGPT et clé API ; le futur relais doit confirmer le
mode ChatGPT avant activation et ne doit pas ajouter de clé API de substitution.
[Authentification officielle](https://learn.chatgpt.com/docs/auth).

La documentation Claude expose les modes print et flux JSON, la reprise et leurs
options. La page en ligne peut décrire une version plus récente que 2.1.207 ;
l'aide locale est donc le plafond de ce relevé.
[Référence CLI officielle](https://code.claude.com/docs/en/cli-reference).

Le régime des abonnements/Agent SDK a évolué ; les pages officielles consultées
comportent des annonces et dates de transition. Aucun droit de facturation ou
quota propre au compte de l'utilisateur n'a été testé. L'adaptateur devra
vérifier le mode réel et présenter toute consommation additionnelle avant usage.
[Information officielle sur les plans et l'Agent SDK](https://support.claude.com/en/articles/15036540-use-the-claude-agent-sdk-with-your-claude-plan).

## Contrat de capacité pour la suite

Chaque adaptateur annonce `provider`, `mode`, `version`, `observed_at`,
`connection_state`, et pour chaque capacité `available` ou `unavailable` avec
raison. Les données tâche, progression, tokens, quotas, CPU/RAM/GPU et coût sont
nullable, avec source et date. Une estimation monétaire n'est jamais une facture
et le quota global n'est pas attribué à une tâche. Aucun calcul inventé de GPU/coût.

Les commandes futures doivent porter machine + provider + session + run,
identifiant de commande, `expected_run_id`, expiration et précondition de capacité.
Une déconnexion n'autorise pas à rejouer une consigne ou approbation périmée.
Les approbations restent humaines selon les droits de l'outil ; aucun auto-approve.

Tests requis avant activation : session isolée avec abonnement identifié ; lancement,
événements partiels, demande utilisateur, consigne concurrente avec mauvais run ID,
interruption acquittée, coupure/reconnexion, processus terminé et refus de permissions.
Reprendre séparément le cas d'une session existante dans **chacun** des trois modes.
Cette liste est un reste à qualifier, pas une promesse de capacités livrées.
