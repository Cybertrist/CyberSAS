// Les ports que le moteur calcule depuis la politique signée, et le bouton
// « Ouvrir dans le navigateur » qui en découle.
//   flutter test test/ports_test.dart
import 'package:cybersas/donnees.dart';
import 'package:flutter_test/flutter_test.dart';

Appareil _pair(String nom,
        {List<String>? ports, String etiquette = '', bool moi = false, bool connus = true, String adresse = '10.77.0.2'}) =>
    Appareil.duMoteur({
      'nom': nom,
      'adresse': adresse,
      'proprietaire': etiquette.isEmpty ? 'tristan@exemple.fr' : '',
      'etiquette': etiquette,
      'systeme': etiquette.isEmpty ? 'android' : '',
      'moi': moi,
      'signe': true,
      'ports': ?ports,
    }, portsConnus: connus);

void main() {
  test('la maison ouverte en web : http, le tunnel chiffre déjà', () {
    final a = _pair('maison', etiquette: 'maison', ports: ['tcp:80', 'tcp:443', 'icmp']);
    expect(a.web.toString(), 'http://10.77.0.2');
  });

  test('443 seul : https', () {
    expect(_pair('nas', etiquette: 'maison', ports: ['tcp:443']).web.toString(), 'https://10.77.0.2');
  });

  // Le nom vient du serveur sans signature : un serveur piraté qui le
  // change n'envoie pas le navigateur ailleurs. Seule l'adresse, signée,
  // compte, et elle doit être dans le réseau.
  test('un nom piégé ne change pas l\'adresse ouverte', () {
    for (final nom in ['evil.example/p?', 'evil.example#', 'a@evil.example', 'x:1@evil.example']) {
      final a = _pair(nom, etiquette: 'maison', ports: ['tcp:80']);
      expect(a.web.toString(), 'http://10.77.0.2', reason: nom);
      expect(a.web!.host, '10.77.0.2');
    }
  });

  test('une adresse hors du réseau, ou qui n\'en est pas une : pas de bouton', () {
    for (final adresse in ['', '8.8.8.8', '10.78.0.2', '10.77.0.0', '10.77.0.255', '10.77.0.02', '10.77.0.2/p?',
        'evil.example', '10.77.0.2.evil.example', '::1', '10.77.0.256']) {
      expect(_pair('maison', etiquette: 'maison', ports: ['tcp:80'], adresse: adresse).web, isNull, reason: adresse);
    }
    expect(dansPlage('10.77.0.2', '10.77.0.0/24'), isTrue);
    expect(dansPlage('10.77.3.9', '10.77.0.0/16'), isTrue);
    expect(dansPlage('10.77.0.2', 'n\'importe quoi'), isFalse);
  });

  test('une plage qui couvre 80 compte', () {
    expect(_pair('nas', etiquette: 'maison', ports: ['tcp:1-1024']).ouvert(80), isTrue);
    expect(_pair('nas', etiquette: 'maison', ports: ['udp:80']).ouvert(80), isFalse);
  });

  test('« * » ouvre une machine, pas un téléphone', () {
    expect(_pair('maison', etiquette: 'maison', ports: ['*']).web, isNotNull);
    expect(_pair('pixel-lea', ports: ['*']).web, isNull);
  });

  test('ni soi-même, ni un pair sans port web', () {
    expect(_pair('fold8', ports: ['*'], moi: true).web, isNull);
    expect(_pair('maison', etiquette: 'maison', ports: ['icmp']).web, isNull);
    expect(_pair('maison', etiquette: 'maison').web, isNull);
  });

  test('ports inconnus : on ne conclut rien', () {
    final a = _pair('maison', etiquette: 'maison', connus: false);
    expect(a.portsConnus, isFalse);
    expect(a.ports, isEmpty);
  });
}
