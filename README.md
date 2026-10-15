# BINGO! 🎉

Application web de bingo multijoueur en temps réel, au style pop-art / bande
dessinée. Les joueurs rejoignent une partie avec un code et proposent des
situations communes. L’hôte valide la liste des propositions puis lance la partie :
chaque joueur reçoit une grille aléatoire. Quand une ligne, une colonne ou une
diagonale est complète, un énorme **BINGO!** explose à l’écran de tout le monde.

> ⚠️ **Projet 100 % vibecodé.** Aucune review : je n’ai aucune idée de ce à
> quoi ressemble le code. C’est marrant, ça fonctionne, et donc je m’en fiche.

## Stack

- **Backend** : Go (stdlib `net/http`, routeur 1.22+), WebSocket
  `github.com/coder/websocket`
- **Base** : SQLite pur Go (`modernc.org/sqlite`, sans CGO), WAL, migrations
  versionnées
- **Frontend** : HTML/CSS/JS vanilla, embarqué dans le binaire (`go:embed`)

Résultat : **un seul binaire** + un fichier `bingo.db`.

## Lancer

```bash
make run
```

Ou manuellement :

```bash
go build -o bin/bingo ./cmd/server
PORT=8080 DB_PATH=bingo.db ./bin/bingo
```

Variables d'environnement :

| Variable  | Défaut     | Rôle                    |
|-----------|------------|-------------------------|
| `PORT`    | `8080`     | Port d'écoute           |
| `DB_PATH` | `bingo.db` | Chemin du fichier SQLite|

## Docker

```bash
docker build -t bingo .
docker run -d --name bingo -p 8080:8080 -v bingo-data:/data bingo
```

L’application est accessible sur `http://localhost:8080`. Le volume
`bingo-data` conserve la base SQLite lors du remplacement du conteneur.
Le serveur tourne avec un utilisateur non privilégié ; le frontend et ses
images sont embarqués dans le binaire.

## Tester à plusieurs

Chaque onglet du navigateur a son propre cookie de session, donc le moyen le
plus simple :

1. Ouvre `http://localhost:8080` dans un onglet → crée une partie (note le
   code, ex. `K7QX`).
2. Ouvre un **onglet privé** (ou un autre navigateur / un autre téléphone sur
   le réseau) → rejoins avec le code + un pseudo.
3. Ajoutez des propositions communes. Avec au moins autant de propositions que de cases (9, 16 ou 25), l’hôte valide la liste complète et lance la partie : les grilles sont distribuées aléatoirement.

Astuce mobile : le serveur écoute sur toutes les interfaces, donc les
téléphones de la même machine peuvent jouer via `http://<ip-de-la-machine>:8080`.

## Fonctionnalités

- Création de partie (3×3, 4×4, 5×5, case centrale gratuite en impair),
  code court + lien d'invitation
- Rejoindre avec code + pseudo (pas de compte, token en cookie HttpOnly,
  reconnexion après rechargement)
- Propositions communes (40 caractères max), validation globale de la liste par l’hôte
- Lobby en temps réel, seuil de 9/16/25 propositions
- Distribution aléatoire des grilles au lancement, sans doublon dans une grille
- Mascottes à l’accueil, dans le lobby, pendant le jeu et en fin de partie
- Détection des bingos **côté serveur** (lignes, colonnes, diagonales)
- Explosion BINGO! plein écran, classement live, continuation ou fin par
  l'hôte
- Écran final : classement + grilles de tous les joueurs
- Reconnexion WebSocket automatique avec resynchronisation complète

## Tests

```bash
make test
```

Tests unitaires de la détection de bingo (toutes tailles, case gratuite,
diagonales) dans `internal/game`.

## Architecture

```
cmd/server/        point d'entrée (config, embed, HTTP)
internal/store/    accès SQLite + migrations SQL versionnées
internal/game/     règles pures : patterns gagnants et détection
internal/http/     handlers REST + hub WebSocket par partie
web/               frontend statique (index.html, style.css, app.js)
```

Messages WebSocket (serveur → client) : `state`, `player_joined`,
`proposals_updated`, `game_started`, `cell_toggled`, `bingo`, `game_finished`.
