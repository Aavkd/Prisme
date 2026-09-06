# Contrat agent v1 — périmètre phase 0

Les échanges JSON sont définis dans [OpenAPI](openapi.json). Version de protocole
`"1"` obligatoire ; toute autre valeur est refusée avant mutation (HTTP 409).

1. Le client humain authentifié émet un token : `POST /v1/enrollment-tokens`,
   objet vide et `Idempotency-Key` de 16–128 caractères. Même session + même clé →
   même token et même expiration. Le token expire en dix minutes, sans prolongation.
2. L'agent génère localement une clé P-256 et une CSR signée, qu'il conserve avant
   l'appel. `POST /v1/agent/enroll` envoie token, nom, CSR PEM, version. Le serveur
   attribue un ID opaque et signe un certificat client de 24 heures. La clé
   privée n'est jamais transmise.
3. La transaction consomme le token et lie CSR/nom/certificat. Un retry exact
   retourne HTTP 200 et le même certificat public ; l'initial renvoie 201.
   Un token consommé par une autre CSR/nom est refusé. Un certificat révoqué ou
   expiré n'est pas récupérable par rejeu.
4. `POST /v1/agent/heartbeat` utilise mTLS. Le sujet et la série doivent correspondre
   à une identité active en base ; le body contient seulement la version.
   Réponse : ID, heure serveur UTC, `capabilities: ["self:heartbeat"]`.
5. Toutes les 30 secondes, le binaire renouvelle cette observation. Une coupure
   ne met pas en file des battements anciens. Au redémarrage, il recharge la clé
   et le certificat et ne requiert plus de token tant que le certificat est valide.

## Privilèges et stockage

Compte non administrateur ; aucune écoute entrante ; aucune socket Docker, clé
SSH, clé CA privée, environnement API, montage de l'hôte ou exécution de commande.
Le conteneur a un filesystem en lecture seule, toutes capacités Linux retirées,
`no-new-privileges`, 32 PID max, 64 Mio RAM et uniquement un volume d'identité privé.

La CA publique est fournie par un canal local approuvé, pas téléchargée aveuglément
depuis l'endpoint d'enrôlement. Les fichiers d'identité utilisent O_EXCL, modes 0600
et répertoire 0700 (ACL Windows préparées par le script de développement).
Ne jamais partager le même répertoire d'identité entre deux processus agents.

## Limites contractuelles

Pas d'inventaire, de plans, de terminal, de renouvellement automatique ni de file
durable en phase 0. Après 24 heures, un nouvel enrôlement explicite dans un nouveau
répertoire est requis. Ces fonctions futures ne sont ni annoncées ni simulées.
Compatibilité N-1 : il n'existe pas de protocole v0 déployé. Une version suivante
devra tester v1 avant retrait ; le client ne déduit aucune capacité du numéro de version.
