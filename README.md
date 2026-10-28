# FRS Remote

Remote control personal: Mac (la lucru) → PC Windows (acasă).

- **Stream** ecran + sunet: motorul Sunshine (pe PC) și Moonlight (pe Mac), incluse în installere.
- **Conexiune** de oriunde, criptată, fără port forwarding: rețea privată (Tailscale, inclusă în app).
- **Clipboard comun** (text și imagini), în ambele sensuri.
- **Fișiere**: Mac → PC din meniu sau din folderul `Desktop/Trimite pe PC`;
  PC → Mac din folderul `Desktop\Trimite pe Mac`. Ajung în `De pe Mac` (Desktop PC) / `Downloads/De pe PC` (Mac).
- **Monitoare**: alegi din meniu ce monitor al PC-ului vezi.
- **PC**: blocare, repornire stream, repornire PC.

## Instalare

1. Ia installerele din [Releases → latest](../../releases/tag/latest).
2. **PC**: rulează `FRS-Remote-Setup-*.exe`. La final se deschide browserul: intră cu contul tău (Google merge).
3. **Mac**: deschide `.dmg`, trage `FRS Remote` în Applications. Prima dată: click dreapta → Open.
   Se deschide browserul: intră cu **același** cont.
4. Pe Mac, din iconița din bara de sus: **Împerechează cu PC-ul (prima dată)**.
5. Gata: **Conectează la PC**.

## Structură

- `agent/` – aplicația (Go): aceeași sursă pentru Mac și Windows.
- `packaging/` – installer Windows (Inno Setup) și app/DMG Mac.
- `.github/workflows/build.yml` – build automat la fiecare push pe `main`.

Uz personal. Sunshine și Moonlight sunt GPL-3.0.
