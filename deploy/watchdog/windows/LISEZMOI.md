# Veilleur Prisme sous Windows (tour et portable)

Le veilleur (`cmd/watchdog`) interroge `https://prisme.mantara.fr/readyz` toutes les 30 s et
prévient le S24 par FCM après 3 échecs (« Prisme · tour » / « Prisme · portable »), puis
après 3 réussites. Il est indépendant du VPS : il voit la panne de Prisme, pas celle d'un site.
Procédure complète de l'étape : [docs/ETAPE_6_PROCEDURE.md](../../../docs/ETAPE_6_PROCEDURE.md).

## Installation

Dans un PowerShell **administrateur** :

```powershell
.\install-watchdog.ps1 -Instance tour -Root D:\PrismeWatchdog -Binary <watchdog.exe> -CredentialsFile <clé.json>
```

| Élément | Valeur |
|---|---|
| Dossier | `-Root` (défaut `%ProgramData%\PrismeWatchdog`) : `bin`, `config`, `state` |
| Droits | héritage coupé ; SYSTEM et Administrateurs : contrôle total ; SERVICE LOCAL : lecture (écriture dans `state` seulement). L'utilisateur courant non élevé n'y a pas accès. |
| Tâche | `\Prisme\Prisme watchdog`, compte intégré **SERVICE LOCAL** (aucun mot de passe, droits minimaux), au démarrage + toutes les 5 min (une seule instance), relance 1 min après un échec, durée illimitée, sur batterie aussi |
| Configuration | `urgent: true` (traverse les horaires silencieux 22 h–7 h Paris), seuil 3, URL ci-dessus |
| Clé FCM | compte limité à l'envoi ; le script **refuse** une clé `firebase-adminsdk` |
| Jetons | lus sur le VPS par `ssh ovh-mantara` (appareils actifs, abonnés aux notifications), écrits dans la configuration sans être affichés |
| Réseau | aucune écoute ; sorties HTTPS vers `prisme.mantara.fr`, `oauth2.googleapis.com`, `fcm.googleapis.com` |

`-DryRun` vérifie tout (clé, programme, tâche, configuration avec un jeton fictif) sans rien
installer, sans droits administrateur et sans SSH.

## Exploitation

```powershell
.\install-watchdog.ps1 -Root <dossier> -Status            # tâche, processus, dernier contrôle
.\install-watchdog.ps1 -Root <dossier> -RefreshTokens     # nouveau téléphone ou jeton changé
.\install-watchdog.ps1 -Root <dossier> -Uninstall         # retire la tâche, garde le dossier
```

Mise à jour du programme : relancer l'installation avec le nouveau `-Binary`, sans
`-CredentialsFile` (la clé déjà en place est gardée). Ne jamais lancer deux veilleurs sur le
même dossier `state`.

## Limites

Le portable endormi ne veille pas ; sans Internet de son côté, il ne peut pas prévenir, puis
peut envoyer un « rétabli » isolé au retour du réseau. La tour éteinte ne veille pas non plus.
Deux notifications pour une même panne (tour + portable) sont normales.
