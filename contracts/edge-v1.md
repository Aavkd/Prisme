# Edge v1 — Caddy et Cloudflare

Schéma SQL 10, OpenAPI 0.9.0. Module facultatif, désactivé sans configuration.

`GET /v1/edge?page=1` retourne les cibles publiques, les observations datées et
50 plans. `POST /v1/edge/jobs` reçoit `{target,value,rollback}` avec une clé
`Idempotency-Key`. `rollback` désigne un précédent plan réussi ou réconcilié de la même cible
et configuration ; dans ce cas `value` reste vide.

`GET /v1/edge/jobs/{job}` expose le plan, sa révision, sa phrase de confirmation
et les preuves. Les snapshots privés ne figurent pas dans cette réponse.
`POST /v1/edge/jobs/{job}/decision` reçoit
`{action,revision,phrase,proof}`. L'approbation requiert une preuve fraîche liée à
`{action:"edge_approve",target:job,payload:{revision}}`. La phase 6 fournit les
passkeys et signatures biométriques ; une session de développement ne suffit pas.

États : `preparing → planned → queued → running → succeeded|rolled_back|failed|uncertain`.
Un plan invalide termine `failed`, un refus explicite `cancelled`. Le délai de
15 minutes commence à la création. La configuration et les snapshots sont liés
au plan ; aucun champ exécutable ne peut être édité après validation.

L'API ne contacte aucun fournisseur et ne lit aucun jeton. `cmd/edge` collecte,
adapte, valide et applique sur son hôte Linux explicitement configuré. Le verrou
PostgreSQL sérialise les workers, le garde local partage l'identité d'hôte avec
déploiements, sauvegardes et secours. Les interruptions de mutation ne sont pas
rejouées. Une cible `uncertain` exige inspection, réconciliation du garde et
`prisme-edge -acknowledge <job>`, qui marque le travail échoué sans mutation.

Caddy : import exact d'un fichier possédé, bloc généré pour un seul domaine et
upstream autorisé, validation `caddy adapt --validate`, comparaison source/actif,
rechargement JSON via `POST /config/` avec `If-Match`, tests puis restauration
automatique conditionnelle si les tests échouent. Une nouvelle route absente
avant import est testée après application ; les autres checks servent aussi de
baseline. Un snapshot source/actif antérieur valide sert de retour arrière.

Cloudflare : inventaire paginé des zones explicites, DNS et tunnels/connecteurs ;
mutation bornée du champ `content` d'un enregistrement A, AAAA ou CNAME existant.
Le snapshot logique conserve le reste des propriétés. Lecture et écriture
utilisent des fichiers de jetons distincts. Une écriture ambiguë n'est jamais
réessayée automatiquement ; le plan inverse est proposé après succès vérifié ou réconciliation opérateur.
L'API Cloudflare DNS ne fournit pas ici de compare-and-swap : un écrivain externe
simultané reste un risque de concurrence ; la lecture avant/après ne l'élimine pas.

Les champs sensibles connus des configurations Caddy sont masqués dans les vues
publiques ; les snapshots d’exécution restent privés. La collecte DNS est bornée
à 100 pages et 4 Mio par liste, 8 Mio pour l’inventaire Cloudflare final.
Les observations deviennent anciennes après 90 secondes. Une erreur de collecte
ne rafraîchit pas la date du dernier état connu. Les erreurs de fournisseur sont
remplacées par des codes stables ; leurs corps, tokens et stderr ne sont pas logués.

Références : [Caddy API](https://caddyserver.com/docs/api),
[Cloudflare DNS PATCH](https://developers.cloudflare.com/api/resources/dns/subresources/records/methods/edit/),
[Cloudflare tunnels](https://developers.cloudflare.com/api/resources/zero_trust/subresources/tunnels/).
