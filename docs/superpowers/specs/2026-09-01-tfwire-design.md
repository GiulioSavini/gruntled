# tfwire — Design

Data: 2026-09-01
Stato: approvato, pronto per roadmap

> Nota sulla lingua: questo documento di design è in italiano perché è interno.
> Tutti gli artefatti rivolti all'utente — README, output della CLI, testi delle
> diagnostiche, messaggi di commit — sono in inglese.

## 1. Problema

Su un repository Terragrunt di dimensioni reali, scopri che il codice è rotto solo
alla fine di un comando lento, **un errore alla volta**. Correggi, riparti, aspetti
di nuovo. Il ciclo si misura in minuti per errore.

La causa non è il cloud: è il **parsing**. Terragrunt documenta la complessità O(n²)
nella valutazione dei `locals`, e il fatto che gli `include` non vengono parsati una
volta e condivisi ma rivalutati nel contesto di ogni unit che li include. Un `run_cmd`
in una root config inclusa da cento unit viene eseguito cento volte. Il risultato
documentato è `run-all plan` a 8+ minuti su 30-50 moduli.

Riferimenti: [Terragrunt Performance](https://terragrunt.gruntwork.io/docs/troubleshooting/performance),
[issue #2806](https://github.com/gruntwork-io/terragrunt/issues/2806).

## 2. Posizionamento

> **tfwire tiene il tuo repository Terragrunt indicizzato a caldo e ti dice,
> nell'istante in cui salvi, chi hai rotto e chi hai impattato.**

I check non sono il prodotto: l'**indice caldo** è il prodotto. Il costo di parsing
si paga una volta all'avvio; ogni salvataggio successivo costa solo il delta.

### Perché è difendibile

Analisi del panorama esistente (2026-09-01):

| Strumento | Cosa copre | Perché non chiude il problema |
|---|---|---|
| `terragrunt hcl validate --inputs --strict` | variabili required mancanti, inputs non dichiarati | one-shot, riparte da zero ogni volta |
| Terragrunt module-output probe | `dependency.X.outputs.Y` | in sviluppo e difettoso ([#5811](https://github.com/gruntwork-io/terragrunt/issues/5811)) |
| Terramate | change detection su git, anche Terragrunt-aware | rileva *cosa è cambiato*, non *cosa si rompe*; delega comunque a tool che riparsano |
| tflint, `tofu validate` | correttezza dentro il modulo | per-directory, richiedono `init`, ignorano il livello Terragrunt |
| `entr` / `watchexec` | watch generico | rilanciano il comando lento: nessun indice caldo |

**Nessuno mantiene un indice caldo.** Terragrunt è una CLI stateless *per design* e non
lo diventerà. È una difesa strutturale, non una rincorsa su feature che l'upstream può
assorbire.

## 3. Scope

**Dentro:** il livello Terragrunt — unit, `dependency`, `include`, `inputs`, il grafo
fra unit, e la lettura dei file `.tf` **limitata all'estrazione della superficie
pubblica** dei moduli (nomi e tipi delle `variable`, nomi degli `output`).

**Fuori:** validazione del Terraform in sé. Niente schemi provider, niente controllo
di attributi o risorse, niente valutazione di espressioni, nessuna delega a `tofu
validate` o `tflint`.

**Conseguenza architetturale che vale la pena rendere esplicita:** cadono tutte le
dipendenze da binari esterni. tfwire è un singolo binario Go che non lancia processi
e non apre connessioni di rete. Le garanzie di idempotenza e air-gap diventano
dimostrabili invece che dichiarate.

## 4. Funzionalità

### 4.1 Blast radius semantico (funzionalità di punta)

Due query distinte sullo stesso grafo. **È la distinzione a essere il prodotto:**

- **Broken** — nodi che hanno una diagnostica di severità error: non funzionano.
- **Impacted** — chiusura transitiva a valle di un cambiamento: funzionano, ma vanno riapplicati.

Terramate dice quali stack sono cambiati *secondo git*. tfwire dice che cancellando
`output "subnet_id"` rompi dodici unit a valle, quali sono, e le separa da quelle che
devono solo essere riapplicate — nell'istante in cui salvi.

### 4.2 Analisi

Tutti gli analyzer sono funzioni pure sul modello di dominio:

| Codice | Cosa rileva |
|---|---|
| `TFW001` | `dependency.X.outputs.Y` dove `Y` non è un output del modulo target |
| `TFW002` | `config_path` che non punta a una unit esistente |
| `TFW003` | ciclo di dipendenze fra unit |
| `TFW004` | output rimosso da un modulo ma ancora referenziato a valle |
| `TFW005` | chiave in `inputs` che non corrisponde a nessuna `variable` del modulo |
| `TFW006` | `variable` senza default che nessuna unit valorizza |
| `TFW100` | errore di sintassi HCL |

`TFW005` e `TFW006` duplicano `terragrunt hcl validate --inputs --strict`. Sono inclusi
perché i dati sono già in memoria e il costo marginale è nullo: il valore aggiunto è
che arrivano in millisecondi anziché a fine scansione.

### 4.3 Superficie CLI

| Comando | Descrizione |
|---|---|
| `tfwire check` | one-shot, senza daemon. Pre-commit e CI gratis. |
| `tfwire watch` | avvia il daemon |
| `tfwire report` | interroga il daemon (`--json`, `--sarif`) |
| `tfwire blast <path>` | blast radius a richiesta |
| `tfwire graph --json` | esporta il grafo come dato riusabile |
| `tfwire stop` | ferma il daemon |

`graph --json` non è un accessorio: è il dato su cui altri costruiscono (output e
variabili morte, unit orfane, generazione di config Atlantis o di job CI). Un tool che
produce un dato è più difficile da rendere obsoleto di uno che produce solo diagnostiche.

## 5. Architettura

### 5.1 Bounded context

**Core:** *Terragrunt Wiring* — unit, dependency, include, inputs, e il grafo fra unit.

**Supporting:** *Module Surface* — estrae da un modulo Terraform i nomi e i tipi delle
`variable` e i nomi degli `output`. Non conosce risorse, provider o espressioni.

I due contesti comunicano solo attraverso il modello di dominio. La libreria HCL non
compare mai nel dominio, nemmeno come import.

### 5.2 Layout

```
cmd/tfwire/                    composition root: l'unico punto che collega gli adapter
internal/
  domain/                      zero dipendenze esterne
    repograph/                 Unit, Module, Surface (VO), Reference (VO)
                               RepositoryGraph = aggregate root, custodisce gli invarianti
    diagnostic/                Diagnostic (VO), DiagnosticKey stabile, Set con Diff puro
    blast/                     domain service: Broken / Impacted
    analysis/                  domain service: gli analyzer
  application/                 use case; conosce le porte, non gli adapter
    indexing/  watching/  querying/
    ports/                     ConfigParser · SurfaceReader · FileWatcher
                               Notifier · IndexStore · Clock
  infrastructure/              adapter: qui e solo qui vivono hcl, fsnotify, socket
    terragrunt/                ACL: terragrunt.hcl + include condivisi -> dominio
    tfsurface/                 ACL: .tf -> Surface
    fswatcher/  notifier/  indexstore/  ipc/
  interfaces/
    cli/                       cobra
    presenter/                 human · json · sarif
```

Blast engine e analyzer sono funzioni pure su strutture dati: si testano senza
filesystem, senza HCL e senza mock.

### 5.3 Ubiquitous language

Fissato ora perché poi non si cambia più.

- **Unit** — directory contenente `terragrunt.hcl` che invoca un modulo
- **Module** — directory contenente file `.tf`
- **Surface** — l'insieme di `variable` e `output` pubblici di un Module
- **Reference** — uso da parte di una Unit di un output di un'altra Unit
- **Broken** — nodo con almeno una diagnostica di severità error
- **Impacted** — nodo a valle di un cambiamento: compila, ma va riapplicato
- **Blast radius** — Broken ∪ Impacted per un dato delta
- **Index** — la proiezione persistita del RepositoryGraph

### 5.4 Il nucleo tecnico: include parsati una volta

Terragrunt rivaluta la root config nel contesto di ogni unit che la include. tfwire
parsa ogni file di `include` **una sola volta** e ne condivide il risultato fra tutte
le unit che lo includono, risolvendo per costruzione l'O(n²) documentato.

È ciò che rende il daemon possibile, e rende `tfwire check` più veloce di
`terragrunt hcl validate` anche in modalità one-shot. Il benchmark riproducibile che
lo dimostra è insieme test di regressione e argomento di adozione.

### 5.5 Flusso

```
salvi -> debounce 300ms -> riparsa SOLO i file cambiati
      -> ricalcola SOLO i nodi che li referenziano -> propaga a valle
      -> analyzer (funzioni pure) -> diff vs stato precedente
      -> notifica solo i nuovi errori
```

Dopo l'avvio non avviene mai più una scansione completa.

## 6. Idempotenza

Sei garanzie, ognuna con la sua verifica.

**① Indicizzazione deterministica.** Indicizzare due volte lo stesso repository produce
un indice identico byte per byte. Nessun timestamp, nessun path assoluto, ordinamento
esplicito di ogni collezione — in Go l'iterazione delle map è randomizzata di proposito,
quindi senza ordinamento esplicito l'output non è stabile.
*Verifica:* indicizza due volte, confronta i byte.

**② Incrementale ≡ full rescan.** L'invariante centrale del progetto:
`reindex(index, Δ)` deve produrre esattamente ciò che produrrebbe
`index_from_scratch(repo_dopo_Δ)`.
*Verifica:* property-based test — applica N modifiche casuali, confronta incrementale
contro full, fallisci se divergono. Senza questo test il daemon accumula stato errato e
diventa peggio che inutile.

**③ Output stabile.** Stesso stato in ingresso, stesse diagnostiche, stesso ordine,
stesso exit code. Serve alla CI per diff puliti e al differ per non generare rumore.
*Verifica:* golden test.

**④ Notifiche at-most-once.** Lo stesso errore non notifica due volte finché persiste.
*Verifica:* test end-to-end con notifier fake.

**⑤ Daemon idempotente.** Avviarlo mentre già gira non ne crea un secondo: si attacca a
quello esistente tramite lock file e socket. Fermarlo due volte non è un errore.
Riavviarlo ricostruisce lo stesso stato dalla cache.
*Verifica:* test end-to-end su start/start/stop/stop.

**⑥ Zero effetti collaterali sul repository.** tfwire non scrive mai dentro il repository
analizzato: niente `.terraform/`, niente lock file, nessun `init`. Tutto lo stato risiede
in `$XDG_CACHE_HOME/tfwire/<hash-repo>/`.
*Verifica:* test che confronta l'hash dell'albero prima e dopo un run completo.

## 7. Robustezza

**Un falso positivo e l'utente disinstalla.** Regola non negoziabile: se un analyzer non
è certo, tace. Source remoto non risolvibile, espressione dinamica non valutabile,
`include` con logica condizionale — l'unit viene marcata `unknown` e si saltano i check
che dipendono da lei. Meglio dieci falsi negativi che un falso positivo.

Inoltre:
- un file HCL salvato a metà produce una diagnostica di sintassi, mai un panic
- se l'indice risulta corrotto viene rigenerato da zero, mai usato parzialmente
- lo schema dell'indice è versionato: al cambio di versione, invalidazione automatica

## 8. Requisiti enterprise

**Air-gapped by design.** Zero chiamate di rete a runtime, zero telemetria, nessun
processo esterno lanciato. Non è una feature aggiunta: il tool non ha ragione di parlare
con nessuno. Lo rende deployabile in ambienti chiusi dove la maggior parte degli
strumenti DevOps non entra.

- configurazione a livelli con precedenza documentata (default → `.tfwire.hcl` del repo
  → variabili d'ambiente → flag), ma **nessuna configurazione obbligatoria**
- logging strutturato `slog` su stderr, separato dall'output utente su stdout
- exit code stabili e documentati
- build riproducibile, SBOM, artifact firmati
- licenza Apache-2.0, per allineamento con OpenTofu e Terragrunt

## 9. Testing

Il valore si concentra nei **golden test**: repository di esempio in `testdata/` — uno
multi-unit con include annidati, uno con dipendenze cicliche, uno con source remoti
irrisolvibili — ciascuno con le diagnostiche attese in un file `.golden`.

Oltre a questi:
- unit test per analyzer: HCL minimale in ingresso, diagnostiche attese in uscita
- property-based test sull'incrementalizzatore (garanzia ②) — il test più importante
- test end-to-end sul daemon con notifier fake
- benchmark riproducibile: repository generato con N unit, `tfwire check` confrontato con
  `terragrunt hcl validate`

**Nessun test richiede rete o credenziali cloud.**

## 10. Milestone

| | Contenuto | Perché è già rilasciabile |
|---|---|---|
| **M1** | Indexer con include condivisi, Module Surface reader, `check`, `graph --json`, benchmark | Utile da solo; il benchmark è la dimostrazione |
| **M2** | Daemon, watcher, incrementalizzatore (garanzia ②), notifier, `report` | Il ciclo di feedback scompare |
| **M3** | `blast`, distinzione Broken / Impacted | La funzionalità di punta |
| **M4** | SARIF, pre-commit hook, integrazione CI, config a livelli | Adozione in team |

## 11. Stack

Go 1.24. Dipendenze: `hashicorp/hcl/v2`, `fsnotify/fsnotify`, `spf13/cobra`.
Nessuna dipendenza cloud, nessun binario esterno.

## 12. Decisioni aperte

- **Nome.** `tfwire` è libero su GitHub alla data di stesura. `tgwire` sarebbe più
  onesto rispetto allo scope Terragrunt-only. Decisione da chiudere prima del rilascio
  di M1, perché dopo cambia il module path e il nome del binario.
