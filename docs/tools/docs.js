// Les schémas animés des documents de docs/ : le protocole, le verrou et
// le modèle de menace. Même boîte à outils que ceux du README (svg.js).
//
//   node docs/tools/docs.js
//   node docs/tools/planche.js docs/schemas/protocole/poignee.svg   # pour les vérifier
const path = require('path');
const S = require('./svg');
const { K, MONO, SANS, t, entete, fondu, quand, carte, boite, trace, fil, trace_anime, bille, pastille, zone, P, picto } = S;

const ecrire = (dossier) => (nom, l, h, corps, titre) => S.svg(path.join(__dirname, '..', 'schemas', dossier), nom, l, h, corps, titre);

/// Une puce de texte posée en (x, y) (centre), pour les jetons de Noise.
function jeton(x, y, s, couleur = 'CYAN') {
  const c = K[couleur];
  const l = S.largeur(s, 14, MONO) + 22;
  return `<rect x="${S.r3(x - l / 2)}" y="${y - 15}" width="${S.r3(l)}" height="30" rx="8" fill="${c}" fill-opacity="0.12" stroke="${c}" stroke-opacity="0.7"/>` +
    t(x, y + 5, s, { taille: 14, couleur: c, police: MONO, poids: 700, ancre: 'middle' });
}

/// Une rangée de jetons centrée en x, qui apparaissent l'un après l'autre
/// de [de] à [a], et restent jusqu'à [fin].
function jetons(C, x, y, liste, de, a, fin, couleur) {
  const larg = liste.map((s) => S.largeur(s, 14, MONO) + 22);
  const total = larg.reduce((m, l) => m + l, 0) + 10 * (liste.length - 1);
  let cx = x - total / 2;
  return liste
    .map((s, i) => {
      const c = cx + larg[i] / 2;
      cx += larg[i] + 10;
      const debut = de + ((a - de) * i) / liste.length;
      return quand(C, debut, fin, jeton(c, y, s, couleur), 0.012);
    })
    .join('');
}

// =========================================================== le protocole
{
  const svg = ecrire('protocole');

  // --------------------------------------------------------- les couches
  // Deux tuyaux bleus, un par appareil, jusqu'au serveur ; un tuyau cyan,
  // de bout en bout, qui traverse le serveur sans s'ouvrir.
  {
    const C = 8;
    const y = 150, h = 150;
    const a = carte(40, y, 250, h, 'fold8-tristan', 'appareil A', K.CYAN, { icone: P.telephone, enHaut: true });
    const s = carte(515, y, 250, h, 'sasd', 'le serveur', K.BLEU, { icone: P.serveur, enHaut: true });
    const b = carte(990, y, 250, h, 'maison', 'appareil B', K.VERT, { icone: P.maison, enHaut: true });
    // Le texte des cartes est en haut : on le remonte en les redessinant.
    const yt = y + h - 36; // l'axe des tuyaux, sous le texte des cartes
    const tuyau = (x1, x2, hauteur, couleur, remplissage) =>
      `<rect x="${x1}" y="${yt - hauteur / 2}" width="${x2 - x1}" height="${hauteur}" rx="${hauteur / 2}" fill="${couleur}" fill-opacity="${remplissage}" stroke="${couleur}" stroke-opacity="0.75" stroke-width="1.6"/>`;
    const paquet = (debut) => [
      bille(`M ${a.x + a.l - 30} ${yt} H ${s.x + 30}`, C, [[debut, 0], [debut + 0.2, 1]], { contenu: env('BLEU') }),
      bille(`M ${s.x + 30} ${yt} H ${s.x + s.l - 30}`, C, [[debut + 0.2, 0], [debut + 0.3, 1]], { contenu: env(null) }),
      bille(`M ${s.x + s.l - 30} ${yt} H ${b.x + 30}`, C, [[debut + 0.3, 0], [debut + 0.5, 1]], { contenu: env('BLEU') }),
    ].join('');
    function env(ext) {
      return `${ext ? `<rect x="-26" y="-19" width="52" height="38" rx="10" fill="${K.FOND}" stroke="${K[ext]}" stroke-width="2"/>` : ''}
<rect x="-15" y="-10" width="30" height="20" rx="6" fill="#0A2A33" stroke="${K.CYAN}" stroke-width="1.8"/>
<g transform="translate(-6,-7) scale(0.44)">${P.cadenas(K.CYAN)}</g>`;
    }
    const corps = `
${entete(1280, 'DEUX COUCHES, TOUTES DEUX EN NOISE IK', 'Chaque appareil a une session avec le serveur ; chaque paire d’appareils, une session à elle, que le serveur ne peut pas ouvrir.')}
${a}${s}${b}
${tuyau(a.x + a.l - 20, s.x + 20, 50, K.BLEU, 0.07)}
${tuyau(s.x + s.l - 20, b.x + 20, 50, K.BLEU, 0.07)}
${tuyau(a.x + a.l - 60, b.x + 60, 24, K.CYAN, 0.1)}
${pastille((a.x + a.l + s.x) / 2, y - 6, 'session A ↔ serveur', 'BLEU', { taille: 12 })}
${pastille((s.x + s.l + b.x) / 2, y - 6, 'session serveur ↔ B', 'BLEU', { taille: 12 })}
${paquet(0.05)}${paquet(0.45)}
${quand(C, 0.25, 0.35, pastille(s.cx, y + h + 34, 'le serveur ouvre la trame, lit « vers B », relaie', 'BLEU', { taille: 12 }))}
${quand(C, 0.65, 0.75, pastille(s.cx, y + h + 34, 'le serveur ouvre la trame, lit « vers B », relaie', 'BLEU', { taille: 12 }))}
<g transform="translate(0,${y + h + 92})">
  <rect x="250" y="-13" width="34" height="22" rx="11" fill="${K.BLEU}" fill-opacity="0.07" stroke="${K.BLEU}" stroke-opacity="0.75" stroke-width="1.6"/>
  ${t(296, 3, 'la couche transport : toujours ouverte, en UDP direct', { taille: 13 })}
  <rect x="690" y="-8" width="34" height="14" rx="7" fill="${K.CYAN}" fill-opacity="0.1" stroke="${K.CYAN}" stroke-opacity="0.75" stroke-width="1.6"/>
  ${t(736, 3, 'la couche de bout en bout : ouverte au premier paquet', { taille: 13 })}
</g>`;
    svg('couches.svg', 1280, y + h + 124, corps, "Les deux couches du protocole. Le téléphone fold8-tristan (A) et la maison (B) ont chacun une session Noise avec le serveur sasd, la couche transport, toujours ouverte en UDP direct. À l'intérieur, A et B ont leur propre session, de bout en bout, ouverte au premier paquet. Un paquet de A vers B voyage dans les deux : le serveur ouvre la trame de relais, lit le destinataire, et la remet à B sans pouvoir ouvrir la session A-B.");
  }

  // ------------------------------------------------- la poignée de main
  {
    const C = 14;
    const gx = 250, dx = 1030;
    const ini = carte(gx - 170, 120, 340, 76, 'initiateur', 'connaît d’avance la clé de l’autre', K.CYAN, { icone: P.telephone });
    const rep = carte(dx - 170, 120, 340, 76, 'répondeur', 'ne répond qu’à une clé connue', K.VERT, { icone: P.maison });
    const bas = 560;
    const y1 = 270, y2 = 390;
    const m1 = `M ${gx + 8} ${y1} H ${dx - 8}`;
    const m2 = `M ${dx - 8} ${y2} H ${gx + 8}`;
    const fin = 0.94;
    const corps = `
${entete(1280, 'LA POIGNÉE DE MAIN : NOISE IK, UN SEUL ALLER-RETOUR', 'Noise_IK_25519_ChaChaPoly_BLAKE2s, prologue « CyberSas tunnel v2 ». Deux messages, deux clés de session.')}
<path d="M ${gx} ${ini.y + ini.h} V ${bas}" stroke="${K.FIL}" stroke-width="2" stroke-dasharray="4 6"/>
<path d="M ${dx} ${rep.y + rep.h} V ${bas}" stroke="${K.FIL}" stroke-width="2" stroke-dasharray="4 6"/>
${ini}${rep}
${t(40, y1 + 5, '1', { taille: 13, couleur: K.DISCRET, police: MONO })}
${quand(C, 0.03, fin, `<circle cx="${gx}" cy="${y1}" r="5" fill="${K.CYAN}"/>`, 0.01)}
${trace_anime(m1, C, 0.03, 0.18, fin, { couleur: 'CYAN', fantome: false })}
${jetons(C, 640, y1 - 36, ['e', 'es', 's', 'ss', 'horodatage TAI64N'], 0.06, 0.26, fin, 'CYAN')}
${quand(C, 0.2, fin, t(640, y1 + 30, 'initiation · 148 octets · mac1, mac2', { taille: 13, couleur: K.TEXTE, police: MONO, ancre: 'middle' }), 0.01)}
${quand(C, 0.28, fin, pastille(dx, y1 + 62, 'clé inconnue ? pas de réponse', 'VERT', { taille: 11.5 }), 0.01)}
${t(40, y2 + 5, '2', { taille: 13, couleur: K.DISCRET, police: MONO })}
${quand(C, 0.38, fin, `<circle cx="${dx}" cy="${y2}" r="5" fill="${K.VERT}"/>`, 0.01)}
${trace_anime(m2, C, 0.38, 0.53, fin, { couleur: 'VERT', fantome: false })}
${jetons(C, 640, y2 - 36, ['e', 'ee', 'se', 'charge vide'], 0.41, 0.58, fin, 'VERT')}
${quand(C, 0.55, fin, t(640, y2 + 30, 'réponse · 92 octets', { taille: 13, couleur: K.TEXTE, police: MONO, ancre: 'middle' }), 0.01)}
${quand(C, 0.64, fin, `${pastille(gx, bas - 50, 'clé d’envoi → · clé de réception ←', 'CYAN', { taille: 12 })}${pastille(dx, bas - 50, '← clé d’envoi · clé de réception →', 'VERT', { taille: 12 })}${pastille(640, bas - 50, 'renouvelées toutes les deux minutes', 'BLEU', { taille: 12 })}`, 0.015)}
${t(640, bas + 30, 'e : clé éphémère · s : clé statique, envoyée chiffrée · es, ss, ee, se : échanges X25519 mêlés au hachage', { taille: 13, ancre: 'middle' })}`;
    svg('poignee.svg', 1280, bas + 60, corps, "La poignée de main Noise IK, en un aller-retour. Message 1, de l'initiateur au répondeur : e, es, s, ss et un horodatage TAI64N chiffré, 148 octets avec mac1 et mac2. Le répondeur ne répond pas à une clé inconnue. Message 2, en retour : e, ee, se et une charge vide chiffrée, 92 octets. Chaque côté en tire deux clés de session, une par sens, renouvelées toutes les deux minutes. e est la clé éphémère, s la clé statique envoyée chiffrée ; es, ss, ee et se sont des échanges X25519 mêlés au hachage.");
  }

  // ---------------------------------------------------- la vie d'une session
  {
    const C = 12;
    const x0 = 140, x1 = 1140;
    const px = (min) => x0 + ((x1 - x0) * min) / 4; // 0 à 4 minutes
    const y = 250;
    const barre = (de, a, yb, couleur, texte, e) =>
      `<rect x="${px(de)}" y="${yb}" width="${px(a) - px(de)}" height="34" rx="9" fill="${couleur}" fill-opacity="0.14" stroke="${couleur}" stroke-opacity="0.7"/>` +
      t(px(de) + 14, yb + 22, texte, { taille: 13, couleur, police: MONO, ...e });
    const graduations = [0, 1, 2, 3, 4]
      .map((m) => `<path d="M ${px(m)} ${y + 120} V ${y + 128}" stroke="${K.DISCRET}" stroke-width="2"/>${t(px(m), y + 150, `${m} min`, { taille: 12.5, couleur: K.DISCRET, police: MONO, ancre: 'middle' })}`)
      .join('');
    const curseur = `<g>${`<animateTransform attributeName="transform" type="translate" dur="${C}s" repeatCount="indefinite" values="0 0;${px(4) - px(0)} 0" keyTimes="0;1"/>`}
  <path d="M ${px(0)} ${y - 60} V ${y + 124}" stroke="${K.CYAN}" stroke-width="2"/><circle cx="${px(0)}" cy="${y - 60}" r="5" fill="${K.CYAN}" filter="url(#halo)"/></g>`;
    const corps = `
${entete(1280, 'LA VIE D’UNE SESSION', 'L’initiateur renouvelle toutes les deux minutes. Passé trois minutes, une session ne chiffre plus rien.')}
${barre(0, 3, y - 40, K.CYAN, 'session 1')}
<rect x="${px(2)}" y="${y - 40}" width="${px(3) - px(2)}" height="34" rx="9" fill="${K.FOND}" fill-opacity="0.55"/>
${barre(2, 4, y + 10, K.VERT, 'session 2')}
${barre(0, 4, y + 60, K.BLEU, 'maintien : un paquet vide si rien n’est reparti en 10 s')}
${graduations}
<path d="M ${px(0)} ${y + 124} H ${px(4)}" stroke="${K.FIL}" stroke-width="2"/>
${curseur}
${quand(C, 0.49, 0.74, pastille(px(2), y - 88, '2 min : nouvelle poignée de main', 'VERT', { taille: 12 }))}
${quand(C, 0.74, 0.99, pastille(px(3), y - 88, '3 min : la session 1 ne sert plus', 'ROUGE', { taille: 12 }))}
${quand(C, 0.02, 0.49, pastille(px(1), y - 88, 'le compteur de chaque paquet sert de nonce', 'CYAN', { taille: 12 }))}
${t(640, y + 196, 'Le répondeur n’adopte la session 2 qu’au premier paquet chiffré avec elle : la preuve que l’autre a les mêmes clés.', { taille: 13.5, ancre: 'middle' })}`;
    svg('sessions.svg', 1280, y + 230, corps, "La vie d'une session sur quatre minutes. La session 1 sert de 0 à 3 minutes ; à 2 minutes, l'initiateur fait une nouvelle poignée de main et la session 2 prend le relais ; à 3 minutes, la session 1 ne chiffre ni ne déchiffre plus rien. En continu, un pair qui n'a rien renvoyé en 10 secondes envoie un paquet vide. Le compteur de chaque paquet sert de nonce. Le répondeur n'adopte la session 2 qu'au premier paquet chiffré avec elle.");
  }

  // ------------------------------------------------------------ le filtre
  {
    const C = 12;
    const arr = carte(40, 190, 250, 76, 'paquet entrant', 'déjà déchiffré', K.CYAN, { icone: P.onde });
    const q1 = carte(400, 190, 330, 76, 'une règle ?', 'pour ce pair, ce protocole, ce port', K.BLEU, { icone: P.document });
    const q2 = carte(400, 360, 330, 76, 'une réponse ?', 'à un flux que j’ai ouvert vers lui', K.BLEU, { icone: P.onde });
    const ok = carte(900, 190, 300, 76, 'il entre', 'remis à l’appareil', K.VERT, { icone: P.bouclier });
    const non = carte(900, 360, 300, 76, 'jeté', 'en silence', K.ROUGE, { icone: P.croix });
    const d0 = trace(arr.ancre('d'), q1.ancre('g'));
    const dOui1 = trace(q1.ancre('d'), ok.ancre('g'));
    const dNon1 = trace(q1.ancre('b'), q2.ancre('h'));
    const dOui2 = trace(q2.ancre('d', 0.3), ok.ancre('g', 0.75), 'courbe');
    const dNon2 = trace(q2.ancre('d', 0.7), non.ancre('g', 0.7), 'courbe');
    // Trois paquets : 443 permis par une règle, 22 sans règle, 53 qui
    // répond à une question posée par l'appareil lui-même.
    const trajet = (debut, etapes) => etapes.map(([d, a, b]) => bille(d, C, [[debut + a, 0], [debut + b, 1]], { couleur: etapes.coul, rayon: 6 })).join('');
    const p1 = [[d0, 0, 0.06], [dOui1, 0.08, 0.16]]; p1.coul = 'VERT';
    const p2 = [[d0, 0, 0.06], [dNon1, 0.08, 0.12], [dNon2, 0.14, 0.22]]; p2.coul = 'ROUGE';
    const p3 = [[d0, 0, 0.06], [dNon1, 0.08, 0.12], [dOui2, 0.14, 0.22]]; p3.coul = 'VERT';
    const etiquette = (debut, fin, texte, couleur) => quand(C, debut, fin, pastille(arr.cx, arr.y - 30, texte, couleur, { taille: 12 }));
    const corps = `
${entete(1280, 'LE FILTRE, CHEZ CELUI QUI REÇOIT', 'Le serveur ne voit plus les paquets entre appareils : chaque appareil décide lui-même de ce qui entre.')}
${fil(d0, { pointe: true })}${fil(dOui1, { couleur: 'VERT', pointe: true })}${fil(dNon1, { couleur: 'ROUGE', pointe: true })}
${fil(dOui2, { couleur: 'VERT', pointe: true })}${fil(dNon2, { couleur: 'ROUGE', pointe: true })}
${arr}${q1}${q2}${ok}${non}
${t((q1.x + q1.l + ok.x) / 2, q1.cy - 12, 'oui', { taille: 12, couleur: K.VERT, police: MONO, ancre: 'middle' })}
${t(q1.cx + 12, (q1.y + q1.h + q2.y) / 2 + 4, 'non', { taille: 12, couleur: K.ROUGE, police: MONO })}
${t(q2.x + q2.l + 14, q2.y + q2.h * 0.3 - 10, 'oui', { taille: 12, couleur: K.VERT, police: MONO })}
${t(q2.x + q2.l + 14, q2.y + q2.h * 0.7 + 22, 'non', { taille: 12, couleur: K.ROUGE, police: MONO })}
${etiquette(0.02, 0.2, 'TCP 443 · une règle l’autorise', 'VERT')}${trajet(0.02, p1)}
${etiquette(0.34, 0.58, 'TCP 22 · aucune règle', 'ROUGE')}${trajet(0.34, p2)}
${etiquette(0.66, 0.9, 'UDP 53 · réponse à ma question', 'VERT')}${trajet(0.66, p3)}
${t(640, 500, 'Suivi des flux : cinq minutes en TCP, une en UDP, trente secondes pour un écho. Chaque pair a son propre budget.', { taille: 13.5, ancre: 'middle' })}`;
    svg('filtre.svg', 1280, 530, corps, "Le filtre, chez l'appareil qui reçoit. Un paquet entrant, déjà déchiffré, passe deux questions : une règle l'autorise-t-elle pour ce pair, ce protocole et ce port ? Sinon, répond-il à un flux que l'appareil a lui-même ouvert vers ce pair ? Oui à l'une : il entre. Non aux deux : il est jeté en silence. Trois exemples : TCP 443 autorisé par une règle entre ; TCP 22 sans règle est jeté ; UDP 53, réponse à une question de l'appareil, entre. Les flux sont suivis cinq minutes en TCP, une en UDP, trente secondes pour un écho, avec un budget par pair.");
  }

  // ------------------------------------------------------- l'inondation
  // Une voie, trois barrières. Chaque barrière est reliée à la carte qui
  // dit ce qu'elle vérifie ; un paquet refusé s'arrête devant elle.
  {
    const C = 12;
    const yl = 300; // la voie
    const src = carte(40, yl - 40, 230, 80, 'inondation', 'un flot d’initiations', K.ROUGE, { icone: P.crane });
    const fin = carte(1050, yl - 40, 190, 80, 'sasd', 'répond enfin', K.VERT, { icone: P.serveur });
    const etages = [
      carte(300, 130, 230, 80, 'mac1', 'vise-t-il ce serveur ?', K.BLEU, { icone: P.cle }),
      carte(545, 130, 230, 80, 'cookie, mac2', 'sous charge, reçoit-il ?', K.BLEU, { icone: P.onde }),
      carte(790, 130, 230, 80, '10 par seconde', 'par /32 ou /64', K.BLEU, { icone: P.horloge }),
    ];
    const voie = `M ${src.x + src.l} ${yl} H ${fin.x}`;
    const barrieres = etages.map((e) => `<rect x="${e.cx - 4}" y="${yl - 30}" width="8" height="60" rx="4" fill="${K.BLEU}"/>`).join('');
    const liens = etages.map((e) => fil(`M ${e.cx} ${e.y + e.h} V ${yl - 30}`, { couleur: 'BLEU', opacite: 0.7 })).join('');
    // [barrière où il s'arrête (-1 : passe), instant de départ]
    const paquets = [[0, 0.02], [0, 0.1], [0, 0.18], [1, 0.26], [0, 0.34], [1, 0.42], [2, 0.5], [0, 0.58], [-1, 0.66]]
      .map(([b, de]) => {
        const xArret = b < 0 ? fin.x : etages[b].cx - 16;
        const d = `M ${src.x + src.l + 16} ${yl} H ${xArret}`;
        const duree = 0.06 + ((xArret - src.x - src.l) / 900) * 0.12;
        return bille(d, C, [[de, 0], [de + duree, 1], [de + duree + 0.04, 1]], { couleur: b < 0 ? 'VERT' : 'ROUGE', rayon: 5 });
      })
      .join('');
    const corps = `
${entete(1280, 'UNE POIGNÉE DE MAIN COÛTE CHER : ON TRIE AVANT', 'Repris de WireGuard. Chaque barrière coûte moins cher que la suivante, et arrête ce qui ne passe pas.')}
${fil(voie, { couleur: 'FIL', pointe: true, depart: false })}
${liens}${barrieres}
${src}${fin}${etages.join('')}
${paquets}
${pastille(etages[0].cx, yl + 66, 'jeté : un hachage', 'ROUGE', { taille: 12 })}
${pastille(etages[1].cx, yl + 66, 'un cookie, rien d’autre', 'ROUGE', { taille: 12 })}
${pastille(etages[2].cx, yl + 66, 'au-delà : attendra', 'ROUGE', { taille: 12 })}
${t(640, yl + 130, 'Les poignées de main relayées échappent aux cookies : le serveur les limite par couple d’appareils, le client par pair.', { taille: 13.5, ancre: 'middle' })}`;
    svg('inondation.svg', 1280, yl + 162, corps, "Trois barrières contre l'inondation, de la moins chère à la plus chère. Un flot d'initiations arrive. Première barrière, mac1 : ce qui ne vise pas ce serveur (scan à l'aveugle, paquet au hasard) est jeté pour le prix d'un hachage ; la clé du serveur n'est pas secrète, cette barrière n'arrête pas un attaquant décidé. Deuxième, sous charge : le serveur exige un mac2 calculé avec un cookie, qui prouve que l'expéditeur reçoit bien les paquets envoyés à son adresse ; sinon il ne répond qu'un cookie. Troisième : dix poignées de main par seconde au plus, par réseau /32 en IPv4 et /64 en IPv6. Ce qui passe arrive à sasd. Les poignées de main relayées sont limitées par couple d'appareils chez le serveur et par pair chez le client.");
  }
}

// ============================================================== le verrou
{
  const svg = ecrire('verrou');

  // ---------------------------------------------- la chaîne de confiance
  {
    const C = 12;
    const cle = carte(465, 120, 350, 80, 'la clé du verrou', 'Ed25519, jamais sur le serveur', K.VERT, { icone: P.sceau, allume: [0.02, 0.3], cycle: C });
    const docs = [
      carte(80, 270, 330, 76, 'certificats', 'clé, adresse, groupe, 90 jours', K.VERT, { icone: P.document }),
      carte(475, 270, 330, 76, 'politique', 'qui atteint quoi, versionnée', K.VERT, { icone: P.document }),
      carte(870, 270, 330, 76, 'révocations', 'les clés bannies, versionnées', K.VERT, { icone: P.document }),
    ];
    const srv = carte(340, 420, 600, 80, 'sasd', 'les transmet, sans pouvoir y toucher', K.BLEU, { icone: P.serveur, allume: [0.3, 0.6], cycle: C });
    const apps = [
      carte(80, 570, 330, 76, 'fold8-tristan', 'vérifie, puis applique', K.CYAN, { icone: P.telephone }),
      carte(475, 570, 330, 76, 'laptop-lea', 'vérifie, puis applique', K.CYAN, { icone: P.portable }),
      carte(870, 570, 330, 76, 'maison', 'vérifie, puis applique', K.CYAN, { icone: P.maison }),
    ];
    const hauts = docs.map((d) => trace(cle.ancre('b', 0.5), d.ancre('h'), 'courbe', { vertical: true }));
    const milieux = docs.map((d, i) => trace(d.ancre('b'), srv.ancre('h', [0.15, 0.5, 0.85][i]), 'courbe', { vertical: true }));
    const bas = apps.map((a, i) => trace(srv.ancre('b', [0.15, 0.5, 0.85][i]), a.ancre('h'), 'courbe', { vertical: true }));
    const sceau = `<circle r="13" fill="${K.FOND}" stroke="${K.VERT}" stroke-width="2"/><g transform="translate(-9,-9) scale(0.64)">${P.sceau(K.VERT)}</g>`;
    const corps = `
${entete(1280, 'LA CHAÎNE DE CONFIANCE', 'L’admin signe, le serveur transporte, chaque appareil vérifie. Un octet changé en route, et la signature ne vaut plus rien.')}
${hauts.map((d) => fil(d, { couleur: 'VERT', pointe: true })).join('')}
${milieux.map((d) => fil(d, { couleur: 'BLEU', pointe: true })).join('')}
${bas.map((d) => fil(d, { couleur: 'BLEU', pointe: true })).join('')}
${cle}${docs.join('')}${srv}${apps.join('')}
${hauts.map((d) => bille(d, C, [[0.05, 0], [0.2, 1]], { contenu: sceau })).join('')}
${milieux.map((d) => bille(d, C, [[0.26, 0], [0.36, 1]], { contenu: sceau })).join('')}
${bas.map((d) => bille(d, C, [[0.5, 0], [0.62, 1]], { contenu: sceau })).join('')}
${apps.map((a) => quand(C, 0.64, 0.95, pastille(a.x + a.l - 70, a.y, 'signature juste', 'VERT', { taille: 11.5 }))).join('')}
${quand(C, 0.36, 0.5, pastille(srv.x + srv.l + 135, srv.cy, 'un octet changé ? tous refusent', 'ROUGE', { taille: 11.5 }))}
${t(640, 690, 'Chaque signature porte son contexte (« CyberSas certificat v2 », « politique v1 », « revocations v1 ») : l’une ne sert jamais pour une autre.', { taille: 13.5, ancre: 'middle' })}`;
    svg('confiance.svg', 1280, 720, corps, "La chaîne de confiance du verrou. La clé du verrou, Ed25519, ne touche jamais le serveur. Elle signe trois sortes de documents : les certificats d'appareil (clé, adresse, groupe, 90 jours), la politique versionnée, et la liste des révocations versionnée. Le serveur sasd les transmet sans pouvoir y toucher : un octet changé et tous les refusent. Chaque appareil, fold8-tristan, laptop-lea et la maison, vérifie la signature puis applique. Chaque signature porte son contexte, pour qu'une signature faite pour l'une ne serve jamais pour une autre.");
  }

  // --------------------------------------------------------- le coffre
  {
    const C = 11;
    const y = 170, l = 212, g = 33;
    const x0 = (1280 - (5 * l + 4 * g)) / 2;
    const etapes = [
      ['empreinte', 'invite biométrique', P.empreinte, K.VERT],
      ['clé AES', 'StrongBox, une fois', P.puce, K.BLEU],
      ['verrou.chiffre', 'déchiffré en mémoire', P.cadenas, K.BLEU],
      ['moteur Go', 'signe le document', P.sceau, K.CYAN],
      ['effacée', 'tableaux remis à zéro', P.croix, K.DISCRET],
    ];
    const tr = 0.8 / etapes.length;
    const cartes = etapes.map(([ti, so, ic, co], i) => carte(x0 + i * (l + g), y, l, 80, ti, so, co, { icone: ic, allume: [0.04 + i * tr, 0.04 + (i + 1) * tr], cycle: C, taille: 13.5 }));
    const liens = cartes.slice(1).map((c, i) => trace(cartes[i].ancre('d'), c.ancre('g')));
    const chemin = `M ${cartes[0].cx} ${y + 124} H ${cartes[4].cx}`;
    const pts = cartes.map((_, i) => [0.04 + i * tr + 0.03, i / 4]);
    const corps = `
${entete(1280, 'LE COFFRE DE LA CLÉ DU VERROU, SUR LE TÉLÉPHONE', 'Une empreinte ouvre le coffre pour une seule opération. La clé déchiffrée ne vit que le temps de signer.')}
${liens.map((d) => fil(d, { couleur: 'BLEU', pointe: true })).join('')}
${cartes.join('')}
<path d="${chemin}" stroke="${K.FIL}" stroke-width="2" stroke-dasharray="4 6"/>
${bille(chemin, C, pts, { couleur: 'VERT', rayon: 5 })}
${t(640, y + 170, 'Une empreinte ajoutée au téléphone invalide la clé AES : il faut ranger le verrou à nouveau.', { taille: 13.5, ancre: 'middle' })}
${t(640, y + 194, 'Le fichier chiffré, copié ailleurs, ne vaut rien : la clé AES ne quitte jamais la puce.', { taille: 13.5, ancre: 'middle' })}`;
    svg('coffre.svg', 1280, y + 224, corps, "Le coffre de la clé du verrou sur le téléphone de l'admin, en cinq temps : l'empreinte, par l'invite biométrique forte d'Android ; la clé AES du Keystore, dans la puce StrongBox, utilisable pour cette seule opération ; le fichier verrou.chiffre, déchiffré en mémoire ; le moteur Go, qui signe le document ; puis la clé est effacée, ses tableaux remis à zéro. Une empreinte ajoutée au téléphone invalide la clé AES. Le fichier chiffré copié ailleurs ne vaut rien.");
  }
}

// ======================================================== les menaces
{
  const svg = ecrire('menaces');

  // ---------------------------------------------- la surface d'attaque
  {
    const C = 11;
    const att = carte(40, 280, 260, 80, 'n’importe qui', 'sur Internet, qui scanne', K.ROUGE, { icone: P.crane });
    const vps = zone(420, 140, 440, 400, 'LE VPS', 'BLEU');
    const p1 = carte(450, 175, 380, 76, 'UDP 51820', 'muet sans mac1 valide', K.BLEU, { icone: P.onde });
    const p2 = carte(450, 280, 380, 76, 'TCP 443', 'vpn. auth. maison., le reste coupé', K.BLEU, { icone: P.mur });
    const p3 = carte(450, 385, 380, 76, 'TCP 80', 'redirige vers 443, rien d’autre', K.BLEU, { icone: P.lien });
    const mz = zone(960, 200, 280, 220, 'LA MAISON', 'VERT');
    const mai = carte(980, 270, 240, 80, 'maison', 'aucun port ouvert', K.VERT, { icone: P.maison });
    const d1 = trace(att.ancre('d', 0.3), p1.ancre('g'), 'courbe');
    const d2 = trace(att.ancre('d', 0.5), p2.ancre('g'));
    const d3 = trace(att.ancre('d', 0.7), p3.ancre('g'), 'courbe');
    const d4 = `M ${att.cx} ${att.y + att.h} C ${att.cx} 660 ${mai.cx} 660 ${mai.cx} ${mai.y + mai.h}`;
    const tir = (d, de) => bille(d, C, [[de, 0], [de + 0.1, 0.96]], { couleur: 'ROUGE', rayon: 5 });
    const corps = `
${entete(1280, 'CE QUI EST EXPOSÉ, ET CE QUI RÉPOND', 'Un seul point public, le VPS. Un scan n’y trouve presque rien à qui parler.')}
${vps}${mz}
${fil(d1, { couleur: 'ROUGE', pointe: true, opacite: 0.6 })}${fil(d2, { couleur: 'ROUGE', pointe: true, opacite: 0.6 })}${fil(d3, { couleur: 'ROUGE', pointe: true, opacite: 0.6 })}
${fil(d4, { couleur: 'ROUGE', pointe: true, opacite: 0.6 })}
${att}${p1}${p2}${p3}${mai}
${tir(d1, 0.03)}${tir(d2, 0.25)}${tir(d3, 0.47)}${bille(d4, C, [[0.69, 0], [0.84, 0.93]], { couleur: 'ROUGE', rayon: 5 })}
${quand(C, 0.13, 0.25, pastille(p1.x + p1.l - 100, p1.y, '… silence', 'ROUGE', { taille: 12 }))}
${quand(C, 0.35, 0.47, pastille(p2.x + p2.l - 100, p2.y, 'nom inconnu : coupé', 'ROUGE', { taille: 12 }))}
${quand(C, 0.57, 0.69, pastille(p3.x + p3.l - 100, p3.y, '301 vers 443', 'ROUGE', { taille: 12 }))}
${quand(C, 0.84, 0.97, pastille(mai.cx, mai.y + mai.h + 40, 'rien n’écoute', 'ROUGE', { taille: 12 }))}
${t(640, 646, 'L’API de sasd n’écoute que sur 127.0.0.1 : on ne l’atteint qu’à travers Nginx.', { taille: 13.5, ancre: 'middle' })}`;
    svg('surface.svg', 1280, 676, corps, "La surface d'attaque. Quelqu'un sur Internet scanne. Sur le VPS, seul point public : le port UDP 51820 du tunnel reste muet sans mac1 valide ; le port TCP 443 ne sert que vpn., auth. et maison., tout autre nom est coupé avant l'échange de certificat ; le port TCP 80 ne fait que rediriger vers 443. À la maison, aucun port ouvert : rien n'écoute. L'API de sasd n'écoute que sur 127.0.0.1, derrière Nginx.");
  }
}
