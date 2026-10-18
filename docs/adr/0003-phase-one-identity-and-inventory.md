# ADR 0003 — Identité et inventaire en phase 1

Date : 7 septembre 2026. Statut : implémenté localement ; hébergement final à qualifier.

La phase 1 étend le monolithe Go, PostgreSQL, le protocole agent v1 et le client
Flutter existants. Prisme reste provisoire. Le NO-GO de la tour n'est pas levé.

## Authentification

`PRISME_MODE=identity` désactive l'accès par secret de développement, y compris
les anciennes sessions de ce type. Il exige un domaine HTTPS explicite et un
secret d'initialisation privé. Ce secret n'autorise que la première inscription
de passkey, puis l'état SQL `initialized` interdit son réemploi.

Le navigateur système réalise WebAuthn : cela permet d'utiliser les authentifiants
Windows/Android sans reproduire un protocole cryptographique dans Flutter. La page
`/auth` sert uniquement la connexion, sans tableau de bord web. Le backend utilise
`go-webauthn/webauthn` v0.17.4, vérifie l'origine, le RP ID, le challenge, la
signature et la vérification utilisateur obligatoire. Les clés synchronisées
peuvent utiliser une confirmation par PIN ; l'UV WebAuthn ne constitue pas une
preuve de biométrie Android pour les futures opérations de production de phase 5.

L'application conserve un ID aléatoire par installation et reçoit deux valeurs
différentes : identifiant de parcours navigateur, secret de polling natif.
La page ne reçoit pas le secret de polling ni de jeton applicatif. Le parcours
expire après cinq minutes et ne peut produire qu'une session. Un challenge est
consommé même après une réponse invalide. Les tokens ne sont stockés qu'en mémoire
côté client et sous empreinte côté serveur. Rotation sans prolongation de la
durée maximale d'une heure, verrouillage client après cinq minutes d'inactivité.

Chaque appareil possède ses préférences et peut être révoqué. La révocation ferme
ses sessions et ses parcours en attente ; les sessions sont contrôlées à chaque
requête. Les actions d'identité exigent une passkey datant de moins de cinq minutes
(exception explicite du mode de développement local). Une passkey peut être
ajoutée avec un ticket à usage unique. Dix codes de secours de haute entropie sont
révélés une fois puis conservés sous empreinte. La récupération inscrit une nouvelle
passkey, révoque les anciens authentifiants, sessions, codes et parcours. Un code
est consommé au début : après abandon, utiliser un autre code.

Références : [API go-webauthn](https://pkg.go.dev/github.com/go-webauthn/webauthn@v0.17.4/webauthn),
[vérification WebAuthn](https://www.w3.org/TR/webauthn-2/#sctn-rp-operations).

## Identité agent et inventaire

Les certificats restent individuels, P-256, valables 24 heures. L'agent renouvelle
dans les huit dernières heures, en conservant sa clé. La série précédente reste
acceptée dix minutes pour récupérer une réponse perdue. Le remplacement local du
certificat est atomique. La révocation SQL bloque aussi une connexion TLS déjà
ouverte. Une identité expirée après une longue coupure exige un nouvel enrôlement.

Le cœur agent n'obtient pas de socket Docker. Un collecteur séparé produit un
snapshot borné, atomique et lisible par le groupe autorisé. Les unités systemd
fournies sont des modèles non installés. L'accès du collecteur au groupe Docker
reste un privilège puissant : ses opérations sont une liste fixe de lectures,
mais ce groupe ne devient pas une permission Docker en lecture seule au niveau OS.

Pour vérifier le critère de sortie sans installer un service en production,
`cmd/inventory -ssh ovh-mantara` exécute un script Python fixe par l'accès SSH
existant, avec vérification stricte de clé hôte. Il ne copie aucun exécutable,
n'installe rien et ne modifie ni service, conteneur, Compose, fichier, DNS ni Caddy.
Seule la base locale reçoit l'observation. La source est `ssh-readonly`, distincte
d'un agent enrôlé ; l'ID est stable pour le couple nom déclaré/hostname.

Les champs transmis sont une liste positive : hôte, capacité, conteneurs, état,
santé (sans sorties de sondes), labels Compose de regroupement et chemins,
images/tags/digests, ports, réseaux, volumes. Env, commandes, labels arbitraires,
contenu Compose et fichiers secrets ne sont pas collectés. Les noms restent des
données non fiables. Aucune ressource découverte n'est adoptée comme « gérée ».

La fraîcheur d'inventaire est indépendante du heartbeat. Après 90 secondes,
elle devient ancienne, même si l'API répond. Une collecte partielle conserve
ses avertissements et les mesures indisponibles restent nulles. Un import SSH
est une photographie ponctuelle, pas un flux de supervision permanent.

## Persistance et retour arrière

Migrations SQL 002 (identité/flotte) et 003 (contexte/protection de l'audit),
transactionnelles et relançables, sans modifier la migration 001. Le binaire
exige la version 3. Restaurer une sauvegarde antérieure avec le binaire associé
ou faire une migration corrective ; aucun down destructif automatique.

Un trigger interdit UPDATE/DELETE sur l'audit. Cela protège le journal contre les
opérations applicatives ordinaires, sans promettre une résistance à l'administrateur
de la base. Le contexte inclut acteur/appareil, origine, résultat, corrélation,
environnement et adresse directe ; les en-têtes de proxy ne sont pas crus.

## Qualifications externes restantes

Domaine/RP ID définitif, certificats TLS reconnus par les navigateurs, transport
Cloudflare Access des clients natifs et authentifiants réels Windows/S24 doivent
être qualifiés sur l'hébergement choisi. Le pont navigateur ne contourne pas Access
et ne configure aucun tunnel. Les tests cryptographiques utilisent des authentifiants
virtuels P-256, sans enregistrer de passkey personnelle à la place de l'utilisateur.
