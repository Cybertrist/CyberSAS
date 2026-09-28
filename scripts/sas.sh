#!/bin/bash
# CyberSAS : tout ce qu'on fait sur la pile passe par ce script.
#
# Sur le serveur :
#   sas.sh init                      secrets, autorité du labo, configurations
#   sas.sh google <id>               le client OAuth de Google Cloud (le secret est demandé)
#   sas.sh membre <email> <groupe>   donne l'accès à un compte Google (admins ou equipe)
#   sas.sh retirer <email>           le lui retire, et coupe ses appareils
#                                    (l'admin fait de même depuis l'appli, écran Équipe)
#   sas.sh invitation <email>        un lien cybersas:// pour que son appareil rejoigne le réseau
#   sas.sh invitations               les invitations pas encore utilisées
#   sas.sh annuler <id>              en annule une
#   sas.sh demarrer                  construit et lance la pile
#   sas.sh etat                      les appareils du VPN
#   sas.sh arreter                   arrête tout, sans rien effacer
#
# Le verrou (dans le labo, tout se fait sur la même machine ; en vrai, sur
# l'ordinateur de l'admin, voir docs/verrou.md) :
#   sas.sh signer <clé> [<clé>...]   signe ces appareils-là, et eux seuls
#   sas.sh politique                 signe politique/politique.json
#   sas.sh revoquer <clé>            bannit un appareil (volé, perdu)
#
# Le labo seulement (TLS=labo) :
#   sas.sh labo                      lance la fausse maison et le poste d'essai
#   sas.sh essai                     vérifie que tout répond comme prévu
#
# Tout ce qui est secret ou propre à une installation s'écrit dans etat/,
# qui n'est jamais versionné. etat/equipe/equipe.txt est la seule liste
# des personnes autorisées : le VPN et les services web la lisent tous
# deux, et sasd la réécrit quand l'admin change l'équipe depuis l'appli.
set -euo pipefail

# Tout ce que ce script crée n'est lisible que par son utilisateur : clés,
# secrets, liste de l'équipe.
umask 077

# Sous Git Bash, sans cela, « /CN=... » et « /secrets » deviendraient des
# chemins Windows.
export MSYS_NO_PATHCONV=1
# Pour la même raison, curl sous Windows ne connaît pas /dev/null.
NUL=/dev/null; [ -n "${MSYSTEM:-}" ] && NUL=NUL

RACINE="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$RACINE"
# Relatif : openssl sous Git Bash ne lit pas les chemins /c/...
ETAT="etat"
EQUIPE="$ETAT/equipe/equipe.txt"
EMAILS="$ETAT/equipe/emails.txt"
VERROU_EQUIPE="$ETAT/equipe/.verrou"
GOOGLE="$ETAT/secrets/google"
VERROU="$ETAT/verrou"
LABO=(docker compose -f labo/maison.yaml)

dit ()    { printf '\033[1;36m==>\033[0m %s\n' "$*"; }
# ok réussit toujours : dans « test && ok ... || rate ... », rate ne
# s'exécute donc que si le test a échoué.
ok ()     { printf '  \033[32mok\033[0m  %s\n' "$*" || true; }
rate ()   { printf '  \033[31mKO\033[0m  %s\n' "$*"; ECHECS=$((ECHECS+1)); }
meurt ()  { printf '\033[31merreur :\033[0m %s\n' "$*" >&2; exit 1; }

charger_env () {
  [ -f .env ] || meurt ".env manquant : copier .env.exemple en .env et le remplir."
  set -a; . ./.env; set +a
  : "${DOMAINE:?DOMAINE manque dans .env}"
  # Pas de valeur par défaut : oublier TLS ne doit pas faire tourner la
  # production en mode labo.
  case "${TLS:-}" in
    labo) ;;
    letsencrypt)
      # Un domaine dédié : le cookie de session vaut pour tous ses
      # sous-domaines. Sur exemple.fr, il partirait aussi vers
      # blog.exemple.fr, hébergé ailleurs.
      [ "$(echo "$DOMAINE" | tr -cd . | wc -c)" -ge 2 ] || meurt "DOMAINE doit être un sous-domaine dédié (sas.exemple.fr), pas $DOMAINE" ;;
    *) meurt "TLS doit valoir labo ou letsencrypt dans .env" ;;
  esac
  PORT_HTTPS="${PORT_HTTPS:-443}"
  # oauth2-proxy tourne sous notre utilisateur : ses secrets restent en 0600.
  export SAS_UID SAS_GID
  SAS_UID="$(id -u)"; SAS_GID="$(id -g)"
}

labo_seulement () {
  [ "$TLS" = labo ] || meurt "réservé au labo : cette commande inscrit, retire et écoute des appareils"
}

en_route () { docker compose ps --status running --services 2>/dev/null | grep -qx "$1"; }

# --- init -------------------------------------------------------------------

init_ca () {
  local ca="$ETAT/ca" tls="$ETAT/tls"
  mkdir -p "$ca/prive" "$ca/public" "$tls"
  if [ ! -f "$ca/public/cybersas-ca.pem" ]; then
    # nameConstraints : cette autorité ne peut signer que pour DOMAINE et
    # ses sous-domaines, et pour aucune adresse IP. Installée sur un
    # téléphone, elle ne pourrait usurper ni un autre site, ni l'interface
    # d'une box ou d'un NAS jointe par son adresse.
    openssl req -x509 -new -nodes -newkey ec -pkeyopt ec_paramgen_curve:P-256 \
      -keyout "$ca/prive/ca.key" -out "$ca/public/cybersas-ca.pem" -days 1825 \
      -subj "/O=CyberSAS/CN=CyberSAS Labo CA" \
      -addext "basicConstraints=critical,CA:TRUE,pathlen:0" \
      -addext "keyUsage=critical,keyCertSign,cRLSign" \
      -addext "nameConstraints=critical,permitted;DNS:$DOMAINE,excluded;IP:0.0.0.0/0.0.0.0,excluded;IP:0:0:0:0:0:0:0:0/0:0:0:0:0:0:0:0" 2>/dev/null
    ok "autorité du labo créée, limitée à $DOMAINE et à aucune adresse IP"
  fi
  if [ ! -f "$tls/fullchain.pem" ]; then
    openssl req -new -nodes -newkey ec -pkeyopt ec_paramgen_curve:P-256 \
      -keyout "$tls/privkey.pem" -out "$tls/requete.csr" -subj "/CN=$DOMAINE" 2>/dev/null
    printf 'subjectAltName=DNS:%s,DNS:*.%s\nextendedKeyUsage=serverAuth\nkeyUsage=critical,digitalSignature\n' \
      "$DOMAINE" "$DOMAINE" > "$tls/extensions.cnf"
    openssl x509 -req -in "$tls/requete.csr" -CA "$ca/public/cybersas-ca.pem" -CAkey "$ca/prive/ca.key" \
      -CAcreateserial -out "$tls/serveur.pem" -days 397 -extfile "$tls/extensions.cnf" 2>/dev/null
    cat "$tls/serveur.pem" "$ca/public/cybersas-ca.pem" > "$tls/fullchain.pem"
    rm -f "$tls/requete.csr" "$tls/extensions.cnf"
    # Nginx lit le certificat en root, puis n'en a plus besoin.
    chmod 644 "$tls/fullchain.pem" "$ca/public/cybersas-ca.pem"
    ok "certificat *.$DOMAINE signé"
  fi
}

init_secrets () {
  mkdir -p "$GOOGLE"
  # La clé qui chiffre le cookie de session. Lue dans un fichier,
  # oauth2-proxy la prend octet pour octet : 32 caractères, sans retour à
  # la ligne, soit une clé AES-256.
  [ -f "$GOOGLE/cookie_secret" ] || { printf '%s' "$(openssl rand -hex 16)" > "$GOOGLE/cookie_secret"; ok "clé des cookies"; }
  # Tant que le vrai client Google n'est pas là, des valeurs de réserve
  # laissent la pile démarrer : tout marche jusqu'à la page de Google.
  [ -f "$GOOGLE/client_id" ]     || { echo "a-remplir.apps.googleusercontent.com" > "$GOOGLE/client_id"; ok "client Google provisoire"; }
  [ -f "$GOOGLE/client_secret" ] || echo "a-remplir" > "$GOOGLE/client_secret"
}

# La clé du verrou : Ed25519, générée par openssl. Dans le labo seulement.
# En vrai, elle ne doit jamais toucher le serveur : elle se crée sur
# l'ordinateur de l'admin avec « sas verrou creer », et seule sa moitié
# publique est posée ici, dans etat/verrou/publique.
init_verrou () {
  mkdir -p "$VERROU"
  [ -f "$VERROU/publique" ] && return
  [ "$TLS" = labo ] || meurt "pas de verrou : créer la clé sur l'ordinateur de l'admin (sas verrou creer), puis poser sa moitié publique dans $VERROU/publique"
  openssl genpkey -algorithm ed25519 -out "$VERROU/cle.pem" 2>/dev/null
  # En PKCS#8, les 32 derniers octets de la clé privée sont la graine, et
  # ceux de la clé publique, la clé elle-même.
  openssl pkey -in "$VERROU/cle.pem" -outform DER 2>/dev/null | tail -c 32 | base64 > "$VERROU/cle"
  openssl pkey -in "$VERROU/cle.pem" -pubout -outform DER 2>/dev/null | tail -c 32 | base64 > "$VERROU/publique"
  rm -f "$VERROU/cle.pem"
  [ "$(base64 -d < "$VERROU/publique" | wc -c)" -eq 32 ] || meurt "clé du verrou mal formée"
  ok "verrou du réseau créé (labo : la clé privée est sur cette machine)"
}

# Le dossier de l'équipe : le seul de etat/ où sasd écrit, quand l'appli
# de l'admin change l'équipe (voir compose.yaml). sasd est root dans son
# conteneur, sans DAC_OVERRIDE : il n'écrit que là où « les autres »
# peuvent écrire. D'où 0733 (écrire et traverser, pas lister) ; etat/,
# en 0700, ferme tout de même ce dossier aux autres comptes de l'hôte.
# Les fichiers qu'il écrit sont en 0644, pour que ce script les relise.
#
# Pour ce script, tout ce qui est dans ce dossier vient donc peut-être
# d'un sasd piraté : un equipe.txt en lien symbolique vers
# ~/.ssh/id_ed25519, un fichier temporaire posé d'avance en lien vers
# ~/.bashrc. D'où trois règles :
#   - rien n'y est écrit en place : chaque fichier s'écrit dans etat/,
#     où sasd n'entre pas (temporaire), puis y est renommé (mv -T, qui
#     remplace un lien sans le suivre et refuse un dossier) ;
#   - rien n'y est lu directement : lire_equipe refuse un lien, ou ce
#     qui n'est pas un fichier ordinaire, et ne lit que le fichier qu'il
#     a vérifié ;
#   - un verrou de fichier, que sasd prend aussi, empêche l'un d'effacer
#     le changement de l'autre (verrouiller_equipe).
#
# Pas de sticky bit (1733) : il interdirait à sasd, qui n'a pas
# CAP_FOWNER, de remplacer un equipe.txt renommé ici par ce script (il
# appartient à notre utilisateur), et l'appli ne pourrait plus changer
# l'équipe. Plus solide encore : faire tourner sasd sous un uid à lui, et
# ouvrir ce dossier à son seul groupe (0770) plutôt qu'à « tous ». Mais
# sasd a besoin de ses capacités (tun, nftables), que Docker ne donne
# qu'au root du conteneur (voir le Dockerfile) : cela passe par le
# remappage des utilisateurs de Docker (userns-remap), un réglage du
# démon, hors de ce script.
dossier_equipe () {
  mkdir -p "$ETAT/equipe"
  # Le 0733 ci-dessous ne protège que si etat/ reste fermé aux autres.
  chmod 700 "$ETAT"
  chmod 733 "$ETAT/equipe"
  fichier_ordinaire "$EQUIPE"
  fichier_ordinaire "$EMAILS"
  # Avant, la liste était dans etat/equipe.txt, recopiée pour sasd : on
  # la déplace une fois, et l'on efface les anciennes copies.
  if [ -f "$ETAT/equipe.txt" ] && [ ! -e "$EQUIPE" ]; then mv -f -T "$ETAT/equipe.txt" "$EQUIPE"; fi
  rm -f "$ETAT/sasd/equipe.txt" "$ETAT/oauth2-proxy/emails.txt"
  # Pas de touch : un fichier réécrit par sasd appartient à root. Et,
  # comme tout ici, créé à côté puis renommé.
  if [ ! -e "$EQUIPE" ]; then local t; t="$(temporaire)"; mv -f -T "$t" "$EQUIPE"; fi
}

# temporaire : un fichier neuf dans etat/, hors de portée de sasd.
temporaire () { mktemp "$ETAT/.equipe.XXXXXX"; }

# fichier_ordinaire : un fichier de etat/equipe doit être absent ou un
# fichier ordinaire. Un lien symbolique, un dossier ou un tube, c'est que
# sasd a été piraté : on s'arrête avant d'y toucher.
fichier_ordinaire () {
  if [ -L "$1" ] || { [ -e "$1" ] && [ ! -f "$1" ]; }; then
    meurt "$1 n'est pas un fichier ordinaire (lien symbolique ?) : refusé. sasd a peut-être été piraté, voir docs/menaces.md"
  fi
}

# meme_fichier <chemin> <fd> : le descripteur ouvert est bien ce chemin-là,
# un fichier ordinaire, et pas ce que désignait un lien posé entre la
# vérification et l'ouverture.
meme_fichier () {
  local ici ouvert
  ici="$(stat -c '%d:%i:%F' -- "$1" 2>/dev/null)" || return 1
  ouvert="$(stat -L -c '%d:%i:%F' "/dev/fd/$2" 2>/dev/null)" || return 1
  [ "$ici" = "$ouvert" ] && [[ "$ici" == *"regular"* ]]
}

# lire_equipe <copie> : recopie equipe.txt dans <copie>, un fichier de
# etat/. Le reste du script ne lit que cette copie.
lire_equipe () {
  fichier_ordinaire "$EQUIPE"
  fichier_ordinaire "$EMAILS"
  if [ ! -e "$EQUIPE" ]; then : > "$1"; return; fi
  exec 3< "$EQUIPE" || meurt "$EQUIPE illisible"
  if ! meme_fichier "$EQUIPE" 3; then
    exec 3<&-
    meurt "$EQUIPE a changé de nature pendant sa lecture (lien symbolique ?) : refusé"
  fi
  cat <&3 > "$1"
  exec 3<&-
}

# a_une_ligne <adresse> <fichier> : la même lecture que sasd (adresse en
# minuscules, groupe, # pour commenter).
a_une_ligne () {
  awk -v m="$1" 'NF >= 2 && $1 !~ /^#/ && tolower($1) == m {t = 1} END {exit !t}' "$2"
}

# dans_equipe <adresse> : la personne a une ligne dans equipe.txt.
dans_equipe () {
  local c r=0; c="$(temporaire)"
  lire_equipe "$c"
  a_une_ligne "${1,,}" "$c" || r=1
  rm -f "$c"
  return "$r"
}

# verrouiller_equipe : le verrou que sasd prend aussi avant de réécrire
# l'équipe (flock sur etat/equipe/.verrou), tenu jusqu'à
# deverrouiller_equipe. Ouvert en lecture seule, et seulement si c'est un
# fichier ordinaire. Sans flock (Git Bash), pas de verrou : c'est le labo
# sous Windows, où il ne traverserait de toute façon pas la machine
# virtuelle de Docker.
verrouiller_equipe () {
  command -v flock >/dev/null || return 0
  fichier_ordinaire "$VERROU_EQUIPE"
  if [ ! -e "$VERROU_EQUIPE" ]; then
    # ln ne suit pas un lien, et échoue si le fichier est apparu entre-temps.
    local t; t="$(temporaire)"
    ln -T "$t" "$VERROU_EQUIPE" 2>/dev/null || true
    rm -f "$t"
    fichier_ordinaire "$VERROU_EQUIPE"
  fi
  exec 9< "$VERROU_EQUIPE" || meurt "$VERROU_EQUIPE illisible"
  meme_fichier "$VERROU_EQUIPE" 9 || meurt "$VERROU_EQUIPE a changé de nature (lien symbolique ?) : refusé"
  flock -w 10 9 || meurt "l'équipe est en cours de modification (depuis l'appli ?) : réessayer"
}
deverrouiller_equipe () { exec 9<&-; }

# Écrit les configurations de etat/ à partir des modèles et de equipe.txt.
# sasd relit les siennes toutes les cinq secondes, oauth2-proxy surveille
# sa liste : aucun redémarrage n'est nécessaire après un changement.
rendre () {
  mkdir -p "$ETAT/sasd" "$ETAT/oauth2-proxy"
  dossier_equipe
  local id; id="$(cat "$GOOGLE/client_id")"
  local copie emails; copie="$(temporaire)"; emails="$(temporaire)"
  verrouiller_equipe
  lire_equipe "$copie"
  # Le poste d'essai du labo s'inscrit avec une clé, sans compte Google.
  # Il est écrit dans la liste elle-même : sasd la réécrit, il ne doit
  # pas l'y perdre. equipe.txt d'abord, puis la liste web qui en découle.
  if [ "$TLS" = labo ] && ! grep -q '^essai@labo\.local ' "$copie"; then
    echo "essai@labo.local equipe" >> "$copie"
    awk 'NF>=2 && $1 !~ /^#/ {print $1}' "$copie" > "$emails"
    mv -f -T "$copie" "$EQUIPE"
  else
    awk 'NF>=2 && $1 !~ /^#/ {print $1}' "$copie" > "$emails"
    rm -f "$copie"
  fi
  mv -f -T "$emails" "$EMAILS"
  deverrouiller_equipe
  printf '%s\n' "$id" > "$ETAT/sasd/clients_google"
  cp "$VERROU/publique" "$ETAT/sasd/verrou.pub"
  sed -e "s|@DOMAINE@|$DOMAINE|g" -e "s|@GOOGLE_CLIENT_ID@|$id|g" oauth2-proxy/oauth2-proxy.cfg > "$ETAT/oauth2-proxy/oauth2-proxy.cfg"
}

sasd () { docker compose exec -T sasd sasd "$@"; }

cmd_init () {
  charger_env
  command -v docker >/dev/null || meurt "Docker est introuvable."
  dit "Autorité et certificat du labo"
  if [ "$TLS" = labo ]; then init_ca; else ok "TLS=$TLS : rien à faire ici"; fi
  dit "Secrets"
  init_secrets
  init_verrou
  dit "Accès"
  dossier_equipe
  if [ -n "${ADMIN_EMAIL:-}" ]; then
    local copie; copie="$(temporaire)"
    verrouiller_equipe
    lire_equipe "$copie"
    if [ ! -s "$copie" ]; then
      printf '# adresse Google        groupe (admins ou equipe)\n%s admins\n' "$ADMIN_EMAIL" > "$copie"
      mv -f -T "$copie" "$EQUIPE"
      ok "$ADMIN_EMAIL admin"
    fi
    deverrouiller_equipe
    rm -f "$copie"
  fi
  rendre
  ok "configurations écrites dans etat/"
  dit "Prêt. Suite : bash scripts/sas.sh demarrer"
}

# --- Google et accès ----------------------------------------------------------

cmd_google () {
  [ $# -eq 1 ] || meurt "usage : sas.sh google <client_id>  (le secret est demandé ensuite)"
  [[ "$1" == *.apps.googleusercontent.com ]] || meurt "un identifiant Google finit par .apps.googleusercontent.com"
  # Le secret est lu au clavier : en argument, il finirait dans
  # l'historique du shell et dans la liste des processus.
  local secret
  read -rsp "secret du client Google : " secret; echo
  [ -n "$secret" ] || meurt "secret vide"
  mkdir -p "$GOOGLE"
  printf '%s\n' "$1" > "$GOOGLE/client_id"
  printf '%s\n' "$secret" > "$GOOGLE/client_secret"
  rendre
  ok "client Google enregistré"
  # oauth2-proxy ne relit son identifiant qu'au démarrage.
  if en_route oauth2-proxy; then docker compose restart oauth2-proxy >/dev/null 2>&1 && ok "oauth2-proxy relancé"; fi
}

# Une invitation : le lien que l'appli ouvre pour rejoindre le réseau. Il
# porte tout ce qu'il faut : l'adresse du serveur, une clé d'inscription à
# usage unique (dix minutes), la clé publique du verrou, que l'appareil
# retient dès le départ, et, dans le labo, l'autorité qui a signé le
# certificat. La personne doit déjà être dans l'équipe.
cmd_invitation () {
  [ $# -ge 1 ] || meurt "usage : sas.sh invitation <adresse google> [durée, 10m par défaut]"
  local mail="$1" duree="${2:-10m}" cle verrou="" lien
  dans_equipe "$mail" || meurt "$mail n'est pas dans l'équipe : sas.sh membre $mail equipe"
  cle="$(sasd cle --utilisateur "$mail" --duree "$duree" | tr -d '\r')"
  [ -n "$cle" ] || meurt "pas de clé d'inscription"
  [ -f "$VERROU/publique" ] && verrou="$(tr -d '\r\n' < "$VERROU/publique")"
  lien="cybersas://rejoindre?serveur=$(url "https://vpn.$DOMAINE")&cle=$(url "$cle")&verrou=$(url "$verrou")"
  [ "$TLS" = labo ] && lien+="&autorite=$(url "$(base64 -w0 < "$ETAT/ca/public/cybersas-ca.pem")")"
  printf '%s\n' "$lien"
  echo "valable $duree, une seule fois (sas.sh annuler pour la retirer avant)" >&2
}

# Les invitations en cours, et l'annulation de l'une d'elles : une clé
# annulée ne fait plus entrer personne. Les mêmes que dans l'appli.
cmd_invitations () { sasd invitations; }
cmd_annuler () {
  [ $# -eq 1 ] || meurt "usage : sas.sh annuler <id> (voir sas.sh invitations)"
  sasd annuler "$1"
}

# url : encode ce qui ne passe pas tel quel dans un lien (base64 surtout).
url () { printf '%s' "$1" | sed 's/%/%25/g; s/+/%2B/g; s#/#%2F#g; s/=/%3D/g; s/:/%3A/g'; }

# changer_equipe <adresse> <groupe> : met la personne dans ce groupe, ou
# la retire (groupe vide). Le nouveau fichier est écrit en entier, puis
# renommé d'un coup : sasd, qui relit l'équipe toutes les cinq secondes,
# ne voit jamais un état où la personne manque, et ne purge pas ses
# appareils pour un simple changement de groupe. Échoue si l'on retire
# quelqu'un qui n'y est pas.
changer_equipe () {
  local mail="$1" groupe="$2" copie nouvelle r=0
  copie="$(temporaire)"; nouvelle="$(temporaire)"
  verrouiller_equipe
  lire_equipe "$copie"
  if [ -z "$groupe" ] && ! a_une_ligne "$mail" "$copie"; then
    r=1
  else
    {
      awk -v m="$mail" '!(NF >= 2 && $1 !~ /^#/ && tolower($1) == m)' "$copie"
      [ -z "$groupe" ] || printf '%s %s\n' "$mail" "$groupe"
    } > "$nouvelle"
    mv -f -T "$nouvelle" "$EQUIPE" || meurt "équipe non enregistrée"
  fi
  deverrouiller_equipe
  rm -f "$copie" "$nouvelle"
  return "$r"
}

cmd_membre () {
  [ $# -eq 2 ] || meurt "usage : sas.sh membre <adresse google> <admins|equipe>"
  local mail="${1,,}" groupe="$2"
  case "$groupe" in admins|equipe) ;; *) meurt "groupe inconnu : $groupe (admins ou equipe)";; esac
  [[ "$mail" =~ ^[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]+$ ]] || meurt "adresse invalide : $mail"
  dossier_equipe
  changer_equipe "$mail" "$groupe"
  rendre
  ok "$mail : $groupe, pris en compte d'ici cinq secondes"
}

cmd_retirer () {
  [ $# -eq 1 ] || meurt "usage : sas.sh retirer <adresse google>"
  local mail="${1,,}"
  dossier_equipe
  changer_equipe "$mail" "" || meurt "$mail n'est pas dans l'équipe"
  rendre
  # sasd purge lui-même les appareils d'une personne sortie de l'équipe.
  ok "$mail retiré : ses appareils sont coupés d'ici cinq secondes"
  echo "  S'ils étaient signés, révoquer aussi leurs clés : sas.sh revoquer <clé>"
}

# --- le verrou ------------------------------------------------------------------

# verrou_admin : lance « sas verrou » dans un conteneur jetable qui voit la
# clé privée du verrou. En production, on ne passe pas par ici : la clé
# n'est pas sur le serveur, et c'est l'admin qui lance sas sur son poste.
verrou_admin () {
  [ -f "$VERROU/cle" ] || meurt "pas de clé privée du verrou sur cette machine, et c'est normal hors du labo : signer depuis l'ordinateur de l'admin (docs/verrou.md)"
  [ "$TLS" = labo ] || meurt "une clé privée de verrou traîne dans $VERROU sur le serveur : la déplacer sur l'ordinateur de l'admin"
  # pwd -W donne le chemin Windows sous Git Bash ; ailleurs, pwd suffit.
  local v; v="$(cd "$VERROU" && { pwd -W 2>/dev/null || pwd; })"
  docker run --rm -i -v "$v:/verrou:ro" cybersas:dev sas verrou "$@" --fichier /verrou/cle
}

# Signe les appareils dont l'admin donne les clés, lues sur les appareils
# eux-mêmes (sas etat, ou l'écran de l'appli). La liste venue du serveur ne
# sert qu'à connaître les autres champs.
cmd_signer () {
  charger_env
  [ $# -ge 1 ] || meurt "usage : sas.sh signer <clé publique> [<clé>...]  (lue sur l'appareil : sas etat)"
  local cles; cles="$(IFS=,; echo "$*")"
  sasd appareils --json | verrou_admin signer --cle "$cles" | sasd signatures | sed 's/^/  /'
}

cmd_politique () {
  charger_env
  verrou_admin politique < politique/politique.json > politique/politique.sig.tmp
  mv politique/politique.sig.tmp politique/politique.sig
  chmod 644 politique/politique.sig
  ok "politique signée, version $(grep -o '"version": *[0-9]*' politique/politique.json | grep -o '[0-9]*$')"
}

cmd_revoquer () {
  charger_env
  [ $# -ge 1 ] || meurt "usage : sas.sh revoquer <clé publique> [<clé>...]"
  local cles; cles="$(IFS=,; echo "$*")"
  local actuelle="$ETAT/sasd/revocations.json"
  # On part de la liste en vigueur, qui peut venir de l'appli (plus récente
  # que celle du dossier) ; serveur arrêté, de celle du dossier.
  { docker compose exec -T sasd sasd revocations 2>/dev/null | grep . || { [ -f "$actuelle" ] && cat "$actuelle"; }; true; }     | verrou_admin revoquer --cle "$cles" > "$actuelle.tmp"
  mv "$actuelle.tmp" "$actuelle"
  ok "liste de révocation à jour : les appareils l'appliqueront d'ici dix secondes"
}

# --- démarrage ---------------------------------------------------------------

attendre_sasd () {
  local _
  for _ in $(seq 1 60); do
    docker compose exec -T sasd wget -qO- http://127.0.0.1:8080/api/v1/sante >/dev/null 2>&1 && return 0
    sleep 2
  done
  docker compose logs --tail 30 sasd nginx
  meurt "sasd ne répond pas après deux minutes."
}

cmd_demarrer () {
  charger_env
  [ -f "$EQUIPE" ] || [ -f "$ETAT/equipe.txt" ] || meurt "lancer d'abord : bash scripts/sas.sh init"
  rendre
  dit "Construction et démarrage de la pile"
  docker compose build -q
  if [ "$TLS" = labo ] && { [ ! -f politique/politique.sig ] || [ politique/politique.json -nt politique/politique.sig ]; }; then
    cmd_politique
  fi
  # Nginx ne relit ses modèles qu'en démarrant : l'empreinte force compose
  # à le recréer quand l'un d'eux a changé (voir compose.yaml).
  SAS_EMPREINTE_NGINX="$(find nginx -type f | LC_ALL=C sort | xargs cat | sha256sum | cut -c1-16)"
  export SAS_EMPREINTE_NGINX
  docker compose up -d
  attendre_sasd
  ok "sasd répond"
  dit "En ligne : https://vpn.$DOMAINE  https://auth.$DOMAINE  https://maison.$DOMAINE"
}

# Inscrit une machine du labo si elle ne l'est pas déjà. Le verrou lui est
# donné d'avance, comme l'admin le ferait : elle refusera un serveur qui en
# annoncerait un autre. La clé d'inscription passe par l'environnement.
inscrire () {
  local svc="$1"; shift
  if "${LABO[@]}" exec -T "$svc" sas etat >/dev/null 2>&1; then
    ok "$svc déjà dans le VPN"; return
  fi
  local cle; cle="$(sasd cle "$@" | tr -d '\r')"
  [ -n "$cle" ] || meurt "pas de clé pour $svc"
  "${LABO[@]}" exec -T -e SAS_CLE="$cle" "$svc" sas rejoindre --serveur "https://vpn.$DOMAINE" --nom "$svc" \
    --verrou "$(cat "$VERROU/publique")" | sed 's/^/  /'
}

# La clé publique d'une machine du labo, lue sur la machine elle-même :
# c'est ce que l'admin ferait avant de la signer.
cle_de () { "${LABO[@]}" exec -T "$1" sas etat 2>/dev/null | sed -n 's/^clé publique : \([^ ]*\) .*/\1/p' | tr -d '\r'; }

cmd_labo () {
  charger_env; labo_seulement
  dit "Démarrage du labo"
  "${LABO[@]}" up -d
  inscrire maison --etiquette maison --nom maison
  inscrire poste --utilisateur essai@labo.local
  dit "Signature des deux appareils, clés lues sur eux-mêmes"
  cmd_signer "$(cle_de maison)" "$(cle_de poste)"
}

# --- essais -----------------------------------------------------------------

# shellcheck disable=SC2015 # ok réussit toujours, voir sa définition.
cmd_essai () {
  charger_env; labo_seulement
  ECHECS=0
  # --ssl-no-revoke : le curl de Windows (Schannel) exige sinon une liste de
  # révocation que l'autorité du labo ne publie pas. Ignoré ailleurs.
  local CA="$ETAT/ca/public/cybersas-ca.pem"
  local c=(curl -s -o "$NUL" -w '%{http_code}' --max-time 10 --ssl-no-revoke --cacert "$CA")
  local code loc
  # L'adresse vers laquelle une page renvoie, sans la suivre.
  lieu () {
    curl -s -D - -o "$NUL" --max-time 10 --ssl-no-revoke --cacert "$CA" \
      --resolve "$1.$DOMAINE:$PORT_HTTPS:127.0.0.1" "$2" 2>/dev/null | tr -d '\r' | sed -n 's/^[Ll]ocation: //p' || true
  }
  # Code HTTP d'un appel à l'API, avec un corps JSON éventuel.
  api () {
    curl -s -o "$NUL" -w '%{http_code}' --max-time 10 --ssl-no-revoke --cacert "$CA" \
      --resolve "vpn.$DOMAINE:$PORT_HTTPS:127.0.0.1" -H 'Content-Type: application/json' "$@" || true
  }
  local V="https://vpn.$DOMAINE:$PORT_HTTPS"

  dit "Depuis Internet"
  code="$("${c[@]}" --resolve "vpn.$DOMAINE:$PORT_HTTPS:127.0.0.1" "$V/api/v1/sante" || true)"
  [ "$code" = 200 ] && ok "l'API du VPN répond" || rate "vpn.$DOMAINE/api/v1/sante : $code"

  code="$("${c[@]}" --resolve "auth.$DOMAINE:$PORT_HTTPS:127.0.0.1" "https://auth.$DOMAINE:$PORT_HTTPS/ping" || true)"
  [ "$code" = 200 ] && ok "le portail de connexion web répond" || rate "auth.$DOMAINE/ping : $code"

  loc="$(lieu maison "https://maison.$DOMAINE:$PORT_HTTPS/")"
  [[ "$loc" == "https://auth.$DOMAINE/oauth2/start?rd=https://maison.$DOMAINE"* ]] \
    && ok "maison.$DOMAINE renvoie vers la connexion" || rate "maison.$DOMAINE sans session : ${loc:-pas de renvoi}"

  loc="$(lieu auth "https://auth.$DOMAINE:$PORT_HTTPS/oauth2/start?rd=https://maison.$DOMAINE/")"
  [[ "$loc" == "https://accounts.google.com/"*"code_challenge_method=S256"* ]] \
    && ok "la connexion web part chez Google, avec PKCE" || rate "auth.$DOMAINE/oauth2/start : ${loc:-pas de renvoi}"

  code="$(curl -sk -o "$NUL" -w '%{http_code}' --max-time 5 --resolve "inconnu.test:$PORT_HTTPS:127.0.0.1" "https://inconnu.test:$PORT_HTTPS/" || true)"
  [ "$code" = 000 ] && ok "un nom inconnu n'obtient même pas de certificat" || rate "nom inconnu : $code, coupure attendue"

  code="$(curl -s -o "$NUL" -w '%{http_code}' --max-time 5 --resolve "vpn.$DOMAINE:${PORT_HTTP:-80}:127.0.0.1" "http://vpn.$DOMAINE:${PORT_HTTP:-80}/" || true)"
  [ "$code" = 301 ] && ok "le HTTP en clair est redirigé" || rate "HTTP : $code, 301 attendu"

  code="$(api -X POST -d '{"cle_inscription":"sas-inventee","cle_publique":"AAAA","nom":"x"}' "$V/api/v1/connexion")"
  [ "$code" = 400 ] || [ "$code" = 401 ] && ok "une inscription sans preuve est refusée ($code)" || rate "inscription sans preuve : $code"
  code="$(api "$V/api/v1/reseau")"
  [ "$code" = 401 ] && ok "l'état du réseau demande un jeton (401)" || rate "réseau sans jeton : $code"

  dit "Dans le VPN, de bout en bout"
  local maison poste
  maison="$("${LABO[@]}" exec -T poste sas appareils 2>/dev/null | awk '$1=="maison" {print $2}' | tr -d '\r' || true)"
  poste="$("${LABO[@]}" exec -T poste sas etat 2>/dev/null | awk 'NR==1 {print $2}' | tr -d '\r' || true)"
  [ -n "$maison" ] && ok "le poste voit maison ($maison) dans sa liste" || rate "le poste ne voit pas maison"

  # Si le serveur vient de redémarrer, les appareils ont perdu leur session
  # et doivent s'en apercevoir seuls. On leur laisse une minute.
  local t0=$SECONDS joint=""
  while [ $((SECONDS - t0)) -lt 60 ]; do
    if "${LABO[@]}" exec -T poste wget -qO- -T 2 "http://$maison/" >/dev/null 2>&1 \
       && docker compose exec -T sasd wget -qO- -T 2 "http://$maison:8081/" >/dev/null 2>&1; then joint=1; break; fi
    sleep 1
  done
  [ -n "$joint" ] && ok "tous les appareils sont joignables (en $((SECONDS - t0)) s)" || rate "des appareils restent injoignables après une minute"

  local nom
  nom="$(docker compose exec -T sasd nslookup maison.sas.internal 10.77.0.1 2>/dev/null \
         | sed -n 's/^Address: *\(10\.[0-9.]*\).*/\1/p' | head -1 || true)"
  if [ -n "$nom" ] && docker compose exec -T sasd wget -qO- -T 5 "http://$nom:8081/" 2>/dev/null | grep -q Hostname; then
    ok "le serveur trouve maison.sas.internal ($nom) et atteint le port publié"
  else rate "le serveur n'atteint pas maison.sas.internal:8081"; fi

  if "${LABO[@]}" exec -T poste wget -qO- -T 5 "http://$maison:8081/" >/dev/null 2>&1; then
    rate "le poste atteint maison:8081, le port réservé au serveur (X-Email y serait falsifiable)"
  else ok "le port publié de la maison est fermé au poste : X-Email n'y est pas falsifiable"; fi

  if "${LABO[@]}" exec -T poste wget -qO- -T 5 "http://$maison:8080/" >/dev/null 2>&1; then
    rate "le poste atteint maison:8080, que la politique ne lui donne pas"
  else ok "maison refuse elle-même le port 8080 au poste"; fi

  if "${LABO[@]}" exec -T maison wget -qO- -T 5 "http://$poste:80/" >/dev/null 2>&1 \
     || "${LABO[@]}" exec -T maison ping -c1 -W3 "$poste" >/dev/null 2>&1; then
    rate "maison atteint le poste : elle ne devrait rien pouvoir ouvrir"
  else ok "maison ne peut pas se retourner vers le poste"; fi

  # La preuve du bout en bout : on écoute tout ce qui passe sur le serveur,
  # interface du VPN comprise, pendant que le poste demande à la maison une
  # page dont l'adresse contient un marqueur. Le marqueur ne doit jamais
  # apparaître. Témoin : la même demande faite par le serveur lui-même
  # (Nginx publie la maison, le TLS se termine sur le VPS) doit, elle, se
  # voir en clair. Sans ce témoin, une capture vide ne prouverait rien.
  local cap; cap="$(mktemp)"
  trap 'rm -f "${cap:-}"' EXIT
  local porte; porte="$(docker compose ps -q porte)"
  docker run --rm --network "container:$porte" --cap-add NET_RAW --cap-add NET_ADMIN alpine:3.24 sh -c \
    'apk add -q tcpdump >/dev/null 2>&1; timeout 12 tcpdump -i any -A -s0 -U -n 2>/dev/null' > "$cap" &
  local espion=$!
  sleep 5
  "${LABO[@]}" exec -T poste wget -qO- -T 3 "http://$maison/SECRET-BOUT-EN-BOUT" >/dev/null 2>&1 || true
  docker compose exec -T sasd wget -qO- -T 3 "http://$maison:8081/TEMOIN-DU-SERVEUR" >/dev/null 2>&1 || true
  wait "$espion" 2>/dev/null || true
  if ! grep -q "TEMOIN-DU-SERVEUR" "$cap"; then
    rate "la capture sur le serveur n'a rien vu, même le témoin : test non concluant"
  elif grep -q "SECRET-BOUT-EN-BOUT" "$cap"; then
    rate "le serveur a vu en clair une requête du poste vers la maison"
  else ok "le serveur relaie sans rien lire : le témoin apparaît dans la capture, le secret du poste jamais"; fi
  rm -f "$cap"; trap - EXIT

  dit "Le verrou du réseau"
  # Un serveur piraté inscrit sa propre machine sous l'étiquette maison. Le
  # poste la voit annoncée, mais sans certificat du verrou : il la refuse.
  local intrus
  "${LABO[@]}" exec -T -e SAS_ETAT=/tmp/intrus -e SAS_CLE="$(sasd cle --etiquette maison --nom intrus | tr -d '\r')" maison \
    sas rejoindre --serveur "https://vpn.$DOMAINE" --nom intrus >/dev/null 2>&1 || true
  sleep 1
  intrus="$("${LABO[@]}" exec -T poste sas appareils 2>/dev/null | grep '^intrus' || true)"
  if [[ "$intrus" == *"REFUSÉ : pas signé par le verrou"* ]]; then
    ok "un appareil que le serveur annonce sans certificat est refusé par le poste"
  else rate "le poste ne refuse pas l'intrus non signé : ${intrus:-absent}"; fi
  sasd retirer intrus >/dev/null 2>&1 || true
  "${LABO[@]}" exec -T maison rm -rf /tmp/intrus

  dit "Retirer quelqu'un de l'équipe le coupe"
  # Une personne entre dans l'équipe, inscrit un appareil, puis en sort.
  # C'est ce qui ne marchait pas quand l'équipe était montée fichier par
  # fichier : le conteneur ne voyait jamais le fichier remplacé.
  cmd_membre partant@labo.local equipe >/dev/null
  sleep 6
  "${LABO[@]}" exec -T -e SAS_ETAT=/tmp/partant -e SAS_CLE="$(sasd cle --utilisateur partant@labo.local | tr -d '\r')" maison \
    sas rejoindre --serveur "https://vpn.$DOMAINE" --nom portable >/dev/null 2>&1 || true
  if sasd appareils | grep -q "partant@labo.local"; then
    cmd_retirer partant@labo.local >/dev/null
    sleep 7
    if sasd appareils | grep -q "partant@labo.local"; then
      rate "l'appareil d'une personne retirée de l'équipe est toujours inscrit"
    else ok "retirée de l'équipe, la personne perd ses appareils en quelques secondes"; fi
  else rate "l'appareil de la personne n'a pas pu s'inscrire"; cmd_retirer partant@labo.local >/dev/null; fi
  "${LABO[@]}" exec -T maison rm -rf /tmp/partant

  dit "Retirer un appareil le coupe"
  sasd retirer poste-essai >/dev/null
  sleep 7
  if "${LABO[@]}" exec -T poste wget -qO- -T 5 "http://$maison/" >/dev/null 2>&1; then
    rate "le poste retiré atteint encore la maison"
  else ok "le poste retiré n'atteint plus rien"; fi
  # On le remet, pour que l'essai puisse se relancer.
  "${LABO[@]}" exec -T poste rm -f /var/lib/sas/etat.json
  inscrire poste --utilisateur essai@labo.local >/dev/null
  cmd_signer "$(cle_de poste)" >/dev/null 2>&1

  echo
  if [ "$ECHECS" -eq 0 ]; then dit "Tout est conforme."; else meurt "$ECHECS vérification(s) en échec."; fi
}

# --- divers -----------------------------------------------------------------

cmd_etat ()      { charger_env; sasd appareils; }
cmd_arreter ()   { charger_env; "${LABO[@]}" down 2>/dev/null || true; docker compose down; }

commande="${1:-}"; shift || true
case "$commande" in
  init) cmd_init ;;
  google) charger_env; cmd_google "$@" ;;
  membre) charger_env; cmd_membre "$@" ;;
  invitation) charger_env; cmd_invitation "$@" ;;
  invitations) charger_env; cmd_invitations ;;
  annuler) charger_env; cmd_annuler "$@" ;;
  retirer) charger_env; cmd_retirer "$@" ;;
  demarrer) cmd_demarrer ;;
  signer) cmd_signer "$@" ;;
  politique) cmd_politique ;;
  revoquer) cmd_revoquer "$@" ;;
  labo) cmd_labo ;;
  essai) cmd_essai ;;
  etat) cmd_etat ;;
  arreter) cmd_arreter ;;
  *) sed -n '2,26p' "$0" | sed 's/^# \{0,1\}//'; exit 1 ;;
esac
