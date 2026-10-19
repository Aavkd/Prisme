# Étape 6 — Alertes sur le S24 et veilleurs : procédure pas à pas

Pour Alexy. Les parties marquées **Alexy** se font sur ses comptes ou ses machines ; les
parties marquées **agent** sont lancées depuis la tour, après son « oui » sur les commandes
exactes. Aucun secret (clé, jeton, code) n'est copié dans le chat, le dépôt ou un journal.

Ordre imposé : chaque partie a besoin de la précédente.

| Partie | Qui | Besoin |
|---|---|---|
| A. Première passkey | Alexy (portable) | — |
| B. Comptes Firebase limités | Alexy (navigateur) | — |
| C. Application sur le S24 | Alexy (portable + S24) | A |
| D. Clé de l'API sur le VPS | agent | B |
| E. Veilleur de la tour | Alexy (PowerShell administrateur) | B, C |
| F. Veilleur du portable | Alexy | B, C |
| G. Essais (critère de l'étape) | Alexy + agent | C, D, E |
| H. Révocation de l'ancienne clé complète | Alexy | G réussi |

## A. Première passkey (portable) — Alexy

1. Lire le secret d'initialisation soi-même, sans le copier ailleurs :
   `ssh ovh-mantara 'sudo cat /opt/projects/prisme/secrets/identity-secret'`.
2. Depuis `apps/prisme_client` :
   `flutter run -d windows --dart-define=PRISME_IDENTITY=true --dart-define=PRISME_API_URL=https://prisme.mantara.fr`
3. Bouton passkey → page de connexion dans le navigateur → « Créer ou récupérer » → secret
   d'initialisation → passkey (Windows Hello). Noter les **codes de secours** hors de l'appareil.

## B. Comptes de service Firebase limités à l'envoi — Alexy

Projet Google Cloud du Firebase existant (`prisme-49202`), sur
<https://console.cloud.google.com/>, compte propriétaire du projet.

1. **Rôle** : *IAM et administration → Rôles → Créer un rôle*. Titre `Prisme - envoi FCM`,
   ID `prismeFcmSend`, étape « Disponibilité générale », **une seule autorisation** :
   `cloudmessaging.messages.create`. Si la console refuse cette autorisation dans un rôle
   personnalisé, utiliser à la place le rôle prédéfini **Firebase Cloud Messaging API Admin**
   (`roles/firebasecloudmessaging.admin` : envoi, abonnements aux sujets, statistiques de
   livraison ; aucun accès au reste de Firebase).
2. **Compte de l'API** : *IAM et administration → Comptes de service → Créer*. Nom
   `prisme-api-fcm`, description « Prisme API, envoi FCM seul » ; rôle `Prisme - envoi FCM`
   sur le projet ; aucun utilisateur autorisé. Puis *Clés → Ajouter une clé → JSON* : le fichier
   se télécharge. Le garder dans `Téléchargements` (il sera envoyé au VPS en D puis supprimé).
3. **Compte des veilleurs** : même chose avec `prisme-watchdog-fcm` (« Veilleurs Prisme, envoi
   FCM seul »), puis **deux clés JSON** : une pour la tour, une pour le portable (noter l'ID de
   clé affiché pour chacune ; une machine perdue = une seule clé à supprimer).
4. Si la création de clé est interdite (« iam.disableServiceAccountKeyCreation »), s'arrêter
   et le signaler : c'est une règle d'organisation à lever pour ce projet.

Ces clés ne servent **qu'à envoyer** des notifications. L'actuel fichier
`prisme-*-firebase-adminsdk-*.json` de `D:\PrismeSecrets` (accès complet à Firebase) ne sert
plus après les essais : voir H.

## C. Application Prisme sur le S24 — Alexy

1. Sur le portable (le SDK Android y est installé, pas sur la tour) : placer le
   `google-services.json` de `dev.mantara.prisme_client` dans `apps/prisme_client/android/app/`
   (ignoré par Git), puis depuis `apps/prisme_client` :
   `flutter build apk --release --dart-define=PRISME_IDENTITY=true --dart-define=PRISME_API_URL=https://prisme.mantara.fr`
   Cet APK contient aussi le titre des veilleurs (`Prisme · tour`). Il est signé avec la clé de
   débogage du portable : l'installer par-dessus l'ancien peut demander de désinstaller d'abord.
2. Installer `build/app/outputs/flutter-apk/app-release.apk` sur le S24.
3. Sur le **portable connecté** : *Ajouter une passkey* → copier le **ticket d'ajout** (valable
   5 minutes). Sur le S24 : bouton passkey → « Créer ou récupérer » → ticket → passkey.
4. Sur le S24, écran **Notifications** : autoriser les notifications Android, *Enregistrer la
   clé biométrique*, puis horaires **Silence dès 22:00 — Jusqu'à 07:00**, fuseau
   `Europe/Paris`, *Enregistrer les horaires*. Seules les deux alertes urgentes (connexion SSH
   inconnue, fail2ban arrêté) traverseront ces horaires.
5. Toujours sur le S24 : vérifier que l'écran Serveurs et les incidents s'affichent.

Reste aussi l'agent d'inventaire de l'étape 4 (jeton depuis la vue Serveurs, voir
`deploy/production/LISEZMOI.md`, « Agent »).

## D. Clé de l'API sur le VPS — agent (après « oui »)

Depuis la tour (le fichier est lu par `ssh`, jamais affiché), avec `<clé-api.json>` le fichier
téléchargé en B.2 :

```sh
ssh ovh-mantara "sudo sh -c 'umask 027; cat > /opt/projects/prisme/secrets/fcm-api.json && chgrp 10001 /opt/projects/prisme/secrets/fcm-api.json'" < <clé-api.json>
ssh ovh-mantara "sudo python3 -c \"import json; d = json.load(open('/opt/projects/prisme/secrets/fcm-api.json')); print(d['type'], d['project_id'], d['client_email'].split('@')[0])\"; sudo stat -c '%a %U:%G' /opt/projects/prisme/secrets/fcm-api.json"
```

Attendu : `service_account prisme-49202 prisme-api-fcm`, `640 root:10001`. Puis, depuis
`/opt/projects/prisme`, avec le fichier de notifications en plus :

```sh
git pull --ff-only
P="docker compose --env-file deploy/production/ovh-mantara.env -f deploy/production/compose.yml -f deploy/production/compose.notifications.yml"
sudo $P up -d --force-recreate --no-deps --wait api
```

À partir de là, **toujours** utiliser ce `P` (un `up` sans le second fichier recréerait l'API
sans notifications). Alexy supprime ensuite l'original de la clé dans `Téléchargements`.

## E. Veilleur de la tour — Alexy (PowerShell administrateur)

Prérequis : C fait (le S24 est abonné), clé « tour » de B.3 téléchargée. Clic droit sur
PowerShell → *Exécuter en tant qu'administrateur*, puis :

```powershell
cd D:\MANTARA\Prisme\deploy\watchdog\windows
.\install-watchdog.ps1 -Instance tour -Root D:\PrismeWatchdog -Binary D:\MANTARA\Prisme\.dev\bin\windows-amd64\watchdog.exe -CredentialsFile <clé-tour.json>
.\install-watchdog.ps1 -Root D:\PrismeWatchdog -Status
```

Le script affiche le compte de la clé (refuse `firebase-adminsdk`), lit le jeton du S24 sur le
VPS par SSH sans l'afficher, verrouille `D:\PrismeWatchdog`, crée la tâche planifiée
`\Prisme\Prisme watchdog` (compte SERVICE LOCAL, au démarrage + contrôle toutes les 5 min,
relance après 1 min) et la lance. Supprimer ensuite `<clé-tour.json>`. Détails :
[deploy/watchdog/windows/LISEZMOI.md](../deploy/watchdog/windows/LISEZMOI.md).

## F. Veilleur du portable — Alexy

Sur le portable, dépôt à jour, dans un PowerShell **administrateur** :

```powershell
cd <dépôt>\Prisme
go build -trimpath -ldflags="-s -w" -o $env:TEMP\watchdog.exe ./cmd/watchdog
cd deploy\watchdog\windows
.\install-watchdog.ps1 -Instance portable -Binary $env:TEMP\watchdog.exe -CredentialsFile <clé-portable.json>
.\install-watchdog.ps1 -Status
```

(Dossier par défaut `C:\ProgramData\PrismeWatchdog`.) Exige que `ssh ovh-mantara` fonctionne
depuis le portable. Supprimer ensuite `<clé-portable.json>` et `$env:TEMP\watchdog.exe`.

## G. Essais (critère de l'étape 6) — Alexy + agent, à un moment où Alexy regarde le S24

1. **Chaîne de l'API** : `sudo touch /etc/prisme/drill` → notification `PRISME-DRILL` sur le S24
   en ≈ 1 min, qui ouvre la bonne fiche ; `sudo rm /etc/prisme/drill` → résolution.
2. **Nuit simulée** : sur le S24, horaires réglés pour couvrir l'heure actuelle ; drapeaux
   `drill-ssh-unknown-login` (urgent) **et** `drill` (non urgent) → seule l'alerte urgente
   arrive tout de suite ; l'autre attend la fin des horaires. Retirer les drapeaux, remettre
   22:00–07:00.
3. **Veilleurs** : `sudo $P stop api` (seul le conteneur API de Prisme ; les sites ne sont pas
   touchés) pendant ≈ 3 min → « Prisme · tour » (et « Prisme · portable ») « injoignable » en
   ≈ 1 min 30 ; `sudo $P start api` → « rétabli » en ≈ 1 min 30. Pendant la coupure, Prisme
   lui-même n'envoie rien (c'est le rôle des veilleurs).
4. Noter ce qui n'a pas été vérifié (portable endormi, Doze, Ne pas déranger).

## H. Révocation de l'ancienne clé complète — Alexy

Une fois G réussi : console Google Cloud → *Comptes de service* → `firebase-adminsdk-…` →
*Clés* → supprimer la clé correspondant au fichier de `D:\PrismeSecrets` (date de création),
puis supprimer ce fichier. Le `google-services.json` (configuration publique de l'application)
reste utile pour construire l'APK.

## Limites à connaître

- Le **portable dort** : son veilleur ne voit rien pendant la veille. Après une coupure
  d'Internet de son côté, il peut envoyer un « rétabli » isolé ; une connexion instable peut
  produire une fausse alerte « injoignable » suivie d'un « rétabli ».
- La **tour** a eu 6 arrêts non propres en 90 jours : la tâche repart seule au démarrage, mais
  rien ne veille pendant qu'elle est éteinte.
- Une même panne vue par les deux veilleurs donne **deux notifications** : c'est normal.
- FCM peut retarder une notification ; le mode *Ne pas déranger* du S24 reste prioritaire.
- Les veilleurs gardent leur propre liste de jetons : après une réinstallation de l'application
  ou un changement de téléphone, relancer `install-watchdog.ps1 -RefreshTokens` sur chaque PC.
