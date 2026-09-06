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
