// Les ports que le moteur calcule depuis la politique signée, et le bouton
// « Ouvrir dans le navigateur » qui en découle.
//   flutter test test/ports_test.dart
import 'package:cybersas/donnees.dart';
import 'package:flutter_test/flutter_test.dart';

Appareil _pair(String nom, {List<String>? ports, String etiquette = '', bool moi = false, bool connus = true}) =>
    Appareil.duMoteur({
      'nom': nom,
      'adresse': '10.77.0.2',
      'proprietaire': etiquette.isEmpty ? 'tristan@exemple.fr' : '',
      'etiquette': etiquette,
      'systeme': etiquette.isEmpty ? 'android' : '',
      'moi': moi,
      'signe': true,
      if (ports != null) 'ports': ports,
    }, portsConnus: connus);

void main() {
  test('la maison ouverte en web : http, le tunnel chiffre déjà', () {
    final a = _pair('maison', etiquette: 'maison', ports: ['tcp:80', 'tcp:443', 'icmp']);
    expect(a.web.toString(), 'http://maison.sas.internal');
  });

  test('443 seul : https', () {
    expect(_pair('nas', etiquette: 'maison', ports: ['tcp:443']).web.toString(), 'https://nas.sas.internal');
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
