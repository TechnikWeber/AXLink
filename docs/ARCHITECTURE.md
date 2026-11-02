# AXLink — Technische Zielarchitektur (Entwurf)

## Leitprinzipien

1. **Interoperabilität vor Neuerfindung:** Bestehende AX.25-IP-Datagrammkonventionen beibehalten; KISS ist nur der Modemtransport.
2. **Schichten entkoppeln:** Hardwaretreiber, AX.25, IPv4, Mesh, Dienste und UI testbar separieren.
3. **Optionales Mesh** darf Standard-IP-Dienste nicht verhindern. Bestehende Packet-Radio-Stationen ohne AXLink sollen weiter nutzbar sein, soweit die verwendeten Protokolle kompatibel sind.
4. **Durchsatz respektieren:** Bei 1200 bit/s maximal 150 Byte/s brutto vor Overhead; ein Mehrhop-Relais auf demselben Kanal erhöht die Funkzeit erheblich.
5. **Sichere Standards:** Keine unbeabsichtigten Pakete im Amateurfunk senden; Sende-/Relay-Freigaben, Filter und Profile.

## Schichtenmodell

```text
   GUI: Netzwerkgraph | Stationsverzeichnis | Chat | Funkseiten | Status
                         |                 |
          Native Dienste (Discovery, Chat, Cache, Mailbox)
                         |                 |
              Mesh-/Routing-Control-Plane  |   Standard-Sockets
                         |                 v
                         |            TCP / UDP / HTTP
                         |                 |
                         |       OS IPv4 Stack / radio0
                         |                 |
                         |        pdn-net TUN/Wintun
                         |                 |
                         +----> pdn / Packet.NET via RHPv2
                                     |
                             AX.25 UI / NET-ROM*
                                     |
                              KISS Serial/TCP
                                     |
                              Quansheng / TNC

* NET/ROM als zu evaluierende Routing-Option, NICHT automatisch IPv4-Mesh.
```

### Erste Implementierungsform (Adaptersystem)

- `pdn`/Packet.NET läuft als separater Prozess und besitzt den direkten KISS-Port.
- `pdn-net` läuft als separater IP-Adapter und redet mit `pdn` via **RHPv2** (laut Upstream lokales TCP mit JSON-Framing).
- `AXLink` läuft als eigener Dienst, liest Stationsereignisse, richtet konfigurierte IP-Routen ein, führt Dienste/Discovery/Chat und UI; detaillierte RHPv2-Fähigkeiten erst im Code prüfen.
- Keine konkurrierenden seriellen Portzugriffe: **ein einziger Hardware-Eigentümer**. Für Reticulum oder andere Protokolle ggf. Frame-Multiplexing mit klarer Rückkanalzuordnung / eigene Funkports.

## IPv4 und Adressierung

### Erste Phase: statisch, privat

Beispiel **nur für Labortests**:

| Station | AX.25-Identität | IPv4-Hostadresse |
|---|---|---|
| A | TESTA-1 | `10.73.0.1/32` |
| B | TESTB-1 | `10.73.0.2/32` |
| C | TESTC-1 | `10.73.0.3/32` |

Für den 2-Knoten-Test kann man alternativ `10.73.0.0/30` verwenden. Einzelne `/32`-Routen vermeiden unter Umständen die falsche Vorstellung eines normalen Ethernet-Broadcast-Domains. Tests auf den vorgesehenen Interfaces/Routen sorgfältig konfigurieren.

Die Ziel-IPv4-Adresse bleibt bei Mehrhop über die Strecke erhalten. Die nächste **AX.25-Zieladresse** kann sich von der endgültigen IP-Zielstation unterscheiden. IPv4-Forwarding muss TTL reduzieren, Loop-Schutz und Routen unterstützen. `pdn-net` bietet bereits IP->Callsign-Mapping und ARP-Lernen, aber bislang kein komplettes automatisches Multi-Hop-IP-Routing.

### Später: automatische Adressvergabe

Ein klar dokumentierter Bereich innerhalb RFC1918, **nicht** frei gewählte öffentliche 44Net-Adressen. Ersten Adresskandidaten aus Station-ID ableiten, **Kollisionen** durch Announcement/Probe und Adresskonfliktdetektion behandeln. Alternative zu eigener Logik: IPv4LL oder zentral zugewiesene DHCP-Leases für spezielle Topologien evaluieren; diese Mechanismen sind nicht ohne Weiteres für Mehrhop-Funk geeignet. Routing darf keine Verbindung zwischen allen Stationen auf Layer 2 voraussetzen.

## Discovery und Diensteverzeichnis

### Protokollkonzept — nicht final!

- Eigene, dokumentierte AXLink-Discovery-Nachrichten über passende AX.25-UI-Frames, nicht mit APRS-Inhalten verwechseln.
- Minimale Nutzdaten: Protokollversion, Rufzeichen/Node-ID, Sequenz, Ablaufzeit/TTL, Fähigkeiten-/Dienstebitset, optionale IPv4-Adresse, optional Anzeigename, optionale Service-Abfrage.
- Seltene, **zufällig verzögerte** Bekanntmachungen; passives Mithören bevorzugen; Antworten auf gezielte Anfrage; veraltete Einträge verfallen.
- Anzeigen/Diagnose dürfen eine zuletzt gesehene Station nicht mit **derzeit zuverlässig erreichbar** gleichsetzen.
- Gesonderte **Routing-Announcements**, deren Reichweite begrenzt und dedupliziert ist.
- Künftiges Multi-Hop braucht explizite Überlegungen zu versteckten Stationen, geteilten Rufzeichen/SSIDs, Routing-Stabilität, verzögerter Zustellung und Kosten pro Hop.

## Mesh-Routing

**Weder vorhandenes NET/ROM noch einfache AX.25-Digipeater automatisch als transparentes IP-Mesh betrachten.** Zu prüfen:

- Nutzung von vorhandenen NET/ROM-Nachbarschafts- und Routeninformationen als Input einer IPv4-Forwarding-Schicht (Interoperabilität vs. Protokolloverhead).
- Alternativ einfaches, quell-/zielorientiertes **reaktives Routing** mit begrenzter Routensuche statt periodischem Flooding. Keine voreilige Festlegung auf AODV o.ä.; erst simulieren und evaluieren.
- Jeder Relay-Node: separate Routingtabelle, Loop-Schutz, Hop-Limit / IPv4-TTL, Kanalzugriff, Wiederholungs-/Retry-Limits und Warteschlange.
- Weiterleitung **freiwillig** und pro Transport/pro Frequenz absicherbar.
- Routingmetriken: Hops, geschätzte Erfolgsrate, Belegungszeit, RTT / gemessene Paketverluste; kein unkritisches RSSI-Äquivalent behaupten.
- Tests A↔B↔C, A nicht direkt C; danach Linkausfall B und alternative Route bei 4 Knoten.

## Native Messaging / Chat

- Separat von transparentem TCP/IP, für kurze Nachrichten effizient.
- Nachrichten-ID, Sender, Empfänger/Gruppe, Zeitinformation wenn vorhanden, Payload-Limit, ACK und Retransmission mit Backoff; dedup und Idempotenz.
- Chat/Anwendungs-Zustellbestätigung von RF/AX.25-Frame-ACK unterscheiden.
- Store-and-Forward später: Empfang durch Zwischenknoten, sichere Persistenz, maximale Lebensdauer/Queue-Größe, Authentizität/Spam-/Relay-Policy. Normale TCP-Verbindungen **nicht** store-and-forward-fähig.
- BPQ-Chat/Mailbox als separates Gateway evaluieren; keine vollständige native Kompatibilität annehmen.

## Landingpages & HTTP

- Standard-HTTP: Jeder Teilnehmer kann auf `10.x.x.x:PORT` einen bestehenden Webserver hosten. Bei 1200 bit/s nur **minimalistische** Seiten sinnvoll, keine großen JS-Bundles, Bilder oder zahlreiche Requests.
- Funk-Landingpage: optionales kompaktes format-/versionsdefiniertes Protokoll, cachebar, ggf. textorientiertes Markup mit maximalem Inhaltsbudget und Abruf auf Anfrage.
- Service-Discovery trennt `http` und `radio-page`; keine stillschweigende Übersetzung aller externen Internetdienste.
- Bewusst keine Verschlüsselung/Inhaltsverschleierung über deutschen Amateurfunk; außerdem Dienste whitelist-basiert und lokal begrenzter Webserver-Zugriff.

## Windows/Linux

| Thema | Linux | Windows |
|---|---|---|
| USB-KISS | `/dev/ttyACM*` oder anderer tty | `COM*` |
| IP-Interface | `/dev/net/tun` | Wintun (noch zu implementieren/prüfen) |
| Rechte | `CAP_NET_ADMIN`/privilegierter Helper | Admin/Treiberinstallation |
| Dienst | systemd (optional) | Windows Service (optional) |
| UI | localhost HTTP | localhost HTTP |

Hinweis: Die Upstream-`pdn-net`-Implementierung ist derzeit **Linux-only** hinsichtlich TUN; Windows darf nicht als fertig beschrieben werden.

## Reticulum/NomadNet

- Separater optionaler Adapter/Sidecar; Reticulum/LXMF **nicht** als IP-Stack-Ersatz.
- Möglichkeiten: Reticulum über einen dafür geeigneten Nicht-Amateurfunk-IP-Transport; direkter KISS-Austausch nur mit kontrolliertem gemeinsamem Modemzugriff; LXMF-Nachrichten-Bridge; NomadNet-Seitenrendering.
- Über Amateurfunk in Deutschland **keine gewöhnlich verschlüsselten Reticulum-Nutzdaten** aussenden.
- Lizenz/Compliance der aktuellen Reticulum-Referenzimplementierung vor Bundle/Weiterverteilung prüfen.

## Ressourcen- und Rechtsgrenzen

- §13 AFuV für automatisch arbeitende Stationen und §16 AFuV (u.a. offener Verkehr/Bestimmbarkeit, Verbot der Inhaltsverschlüsselung, technische Pflichten) ausdrücklich berücksichtigen.
- „ISM“ bedeutet nicht automatisch, dass Quansheng-Firmware, Leistung, Frequenz und Verschlüsselung frei wählbar sind. Konforme Funkhardware und Bedingungen der Allgemeinzuteilung sind Voraussetzung.
- Nur bewusst konfigurierte Aussendungen; „Simulation only“ ist Default bis Betreiber eine legale Funkkonfiguration erstellt hat.
- Keine globalen Default-Routen ins Funknetz, kein unkontrolliertes VPN/HTTPS, keine offenen transparenten Internet-Gateways.

## Erste zu definierende Software-Interfaces (Vorschläge)

```text
IKissTransport       Open/Close/SendFrame/ReceiveFrame; Diagnostic
IAx25Gateway         SendUiFrame/SubscribeFrames; Callsign
IIPNetworkAdapter    Open/SetMtu/AddAddress/AddRoute/Close
IStationRegistry     Upsert/Expire/List/OnStationChanged
IRouteResolver       ResolveNextHop(destination)/UpdateNeighbors
IServiceDirectory    Announce/Query/ListByStation
IMessageService      Send/Ack/Inbox/Outbox
ITransportPolicy     CanTransmit/CanRelay/EncryptionAllowed
```

Der echte RHPv2-/Packet.NET-Code bestimmt später die tatsächlichen Interfaces. Dies ist **kein fertiges API-Design**.
