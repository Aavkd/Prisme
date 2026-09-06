# ADR 0001 — Hébergement et périmètre de phase 0

Date : 6 septembre 2026. Décision d'implémentation révisable.

## Contexte

Le [diagnostic existant](../../DIAGNOSTIC_TOUR_WINDOWS.md) suffit : WSL2/Docker
fonctionnent, mais le disque D présente des erreurs, l'OS et le pare-feu nécessitent
une remise à niveau, la sauvegarde et le démarrage autonome ne sont pas démontrés.
La tour est en NO-GO production. Le diagnostic n'a pas été refait ni ses correctifs exécutés.

## Décision

**WSL2 via Docker Desktop pour le développement. VM Linux dédiée comme cible
d'exploitation de la tour, après levée du NO-GO.** La VM doit démarrer au boot sans
session Windows et conserver son disque virtuel sur un stockage sain dédié.
Il s'agit d'un choix d'architecture ; aucune VM n'est installée par la phase 0.

La VM favorise une frontière système claire, un démarrage en service et une chaîne
de sauvegarde indépendante du profil Docker Desktop. Son coût mémoire impose un
dimensionnement après remise à niveau de l'hôte. Le maintien de WSL2 en production
nécessiterait une ADR remplaçante et la preuve d'un démarrage autonome équivalent.

Le Compose de développement comprend uniquement API, migration, PostgreSQL et
agent optionnel. Alloy, Loki, VictoriaMetrics et les intégrations attendent la
phase 2 et leurs usages réels. Limites mémoire : API 256 Mio, PostgreSQL 384 Mio,
agent 64 Mio ; la compilation est distincte de ces limites.

## Frontière d'authentification

Le backend exige `PRISME_MODE=development` et refuse tout autre mode. Un secret
aléatoire local de 256 bits ouvre une session de 60 minutes. La session reste en
mémoire du client, son empreinte SHA-256 est conservée dans PostgreSQL. La
déconnexion révoque immédiatement cette empreinte. Ce mode n'est pas une passkey.
Passkeys, appareils durables, récupération et authentification renforcée sont en phase 1.

L'API est publiée uniquement sur `127.0.0.1:8443`. Aucun tunnel n'est configuré.
TLS 1.3 obligatoire, CA locale explicitement approuvée, aucun bypass TLS.
PostgreSQL n'a pas de port hôte dans le Compose de développement.

L'agent reçoit un certificat client de 24 heures et seulement `self:heartbeat`.
Renouvellement, politique de révocation administrable, file durable et exécution de
plans restent en phase 1 et suivantes ; les endpoints correspondants sont absents.

## Validation et retour arrière

Vérifier le scénario Flutter → TLS → Go → PostgreSQL, le scénario agent → mTLS,
et les refus dans les tests d'intégration. `tool/dev.ps1 -Stop` arrête uniquement
le projet Compose `prisme-dev`, en préservant volumes et secrets. Aucun service
Windows, VPS, Caddy, pare-feu ou projet pilote n'est modifié.

Avant production : stockage sain, OS maintenu, pare-feu, sauvegarde restaurée,
onduleur/reprise secteur, boot de la VM et mesures de capacité. Ces conditions
matérielles ne sont pas neutralisées par le succès des tests locaux.
