# ADR 0002 — Architecture de secours indépendante

Date : 6 septembre 2026. Choix de conception pour la phase 6, non déployé.

## Décision

Retenir une **passerelle de secours native systemd sur le VPS**, HTTPS/WebSocket,
avec tunnel et application Access propres au secours. Elle ne dépend ni de la tour,
ni de PostgreSQL Prisme, ni de Docker/Caddy pour démarrer. Les clients Flutter
partagent la présentation du terminal ; le fournisseur de transport est distinct.

La passerelle conserve son propre registre d'appareils et ses passkeys, ses
révocations et codes de récupération à usage unique. Les enrôlements sont préparés
depuis une session SSH administrateur du VPS. Cloudflare Access reste une barrière
supplémentaire ; la session normale Prisme n'est jamais utilisée comme autorité.
Chaque opération sensible exige une preuve locale récente, indépendante de la tour.

Le service utilise un compte sans privilèges généraux. Le terminal prend une
identité distante explicitement sélectionnée ; son pouvoir réel dépend de ce
compte et ne se réduit pas aux boutons visibles. Les secrets de projets et clés
du coffre central ne sont pas répliqués. Déploiements et rollbacks dépendant du
control plane sont absents de cette interface initiale.

Une connexion SSH directe par réseau privé constitue une voie opérateur externe
en cas de panne Cloudflare ; son intégration dans Flutter est différée. La
passerelle ne garantit donc pas la disponibilité si le VPS ou Cloudflare est hors ligne.

## Coordination à implémenter

Un verrou local **unique, commun à l'agent normal et à la passerelle**, arbitre
les mutations par cible, avec numéro d'époque/fencing monotone. Un bail expiré ne
prouve pas l'arrêt d'une commande : la reprise nécessite son acquittement ou une
prise de contrôle explicite qui incrémente l'époque et invalide les anciens plans.
Au retour de la tour, les automatismes restent suspendus jusqu'à la réconciliation.

L'audit append-only local comporte event_id, origine, acteur, cible, époque,
horodatage et résultat, sans contenu de terminal par défaut. L'import central a
une contrainte unique `(origin,event_id)` et un curseur acquitté ; les retries
ne dupliquent pas les événements. La passerelle refuse une mutation si elle ne
peut plus écrire l'audit. Les horloges et pertes de réseau restent explicites.

## Scénarios de sortie de phase 6

Tour arrêtée et session normale expirée : ouvrir une session de secours ; rejeter
un appareil révoqué ; diagnostiquer un conteneur sans dépendance Caddy ; refuser
deux mutations concurrentes ; interrompre puis reprendre la liaison sans rejouer
une consigne ; revenir à la tour, importer l'audit une seule fois et bloquer les
automatismes avant réconciliation. Simuler aussi une panne Cloudflare et un disque
d'audit plein. Ces scénarios ne sont pas déclarés testés en phase 0.
