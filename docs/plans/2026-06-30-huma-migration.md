# Migração swaggo→Huma + Envelope de Erro Unificado (RFC 9457) no midaz

> **Para implementadores:** documento rolling-wave (`ring:writing-plans`).
> Fases completas ficam colapsadas em registros Done + as lições que ainda
> vinculam o trabalho restante; a fase corrente é a única detalhada a nível de
> task. Execução via `ring:dispatching-workflows` (um workflow por onda:
> implement → review → contrarian embutidos, self-heal, gate do supervisor).
> Esta é a fonte viva de verdade; o histórico gate-a-gate vive no git.
>
> **Local:** worktree dedicado `/Users/fredamaral/repos/lerianstudio/midaz-huma`
> (repo `midaz`, módulo `github.com/LerianStudio/midaz/v4`, branch
> `feat/monorepo-consolidation` — **não** `main`; consentimento explícito do
> Fred em 2026-06-30). Comandos Go exigem `-buildvcs=false`.

**Goal:** Migrar os dois planos HTTP do midaz (ledger + tracer) de swaggo/Fiber-nativo para o wrapper Huma do lib-commons (`commons/net/http/{openapi,problem}`), adotando `problem.Detail` (RFC 9457) como o **único** envelope de erro, com **identidade total de schema `Error`** entre os planos e **preservação byte-a-byte** de todos os códigos de erro e seu mapeamento code→status. Entregável final: specs OAS 3.1 pristine e em paridade que o `midaz-sdk-golang` v4 vai consumir.

**Architecture:** O envelope de erro é a espinha money-path — vem primeiro e sozinho (Fase 1), guardado por um golden test que varre toda a tabela code→status e trava antes e depois do swap de dispatcher. Depois a reescrita de assinatura dos handlers, plano a plano (tracer piloto → ledger), preservando auth como middleware Fiber e declarando `Security` por-op só no spec. Por fim (Fase 4) a troca do pipeline de geração para emissão Huma 3.1 nativa com trava de paridade. Fiber permanece v2.52.13 (`humafiber.NewV2WithGroup`; **sem upgrade de framework**).

**Tech Stack:** Go 1.26.x · Fiber v2.52.13 (mantido) · `lib-commons/v5 v5.8.0` (`commons/net/http/{openapi,problem}`) · `huma/v2 v2.38.0` (via `humafiber`) · OpenAPI 3.1.0 · `@redocly/cli join` (mantido) · `testify` + `app.Test`.

**Superfície verificada:** tracer = **28 ops protegidas** migradas para Huma + **3 públicas** (health/version/readyz) que ficam Fiber cru fora do group `/v1`. ledger = **113 ops** em Huma. DSL multipart de transação (`.casl`) **não migra** (sunset 2026-08-01) — fica Fiber, fora do spec Huma. Total Huma: **141 ops**.

## Phase Overview

| Phase | Milestone | Epics | Status |
|-------|-----------|-------|--------|
| 1 | Envelope RFC 9457 no runtime dos 2 planos; golden code→status verde antes/depois do swap; zero código/status alterado | 1.1, 1.2, 1.3 | Complete |
| 2 | Tracer 100% Huma: 28 ops re-tipadas, spec OAS 3.1 nativa ADITIVA (swaggo intacto), auth Bearer+ApiKey por-op | 2.1, 2.2, 2.3 | Complete |
| 3 | Ledger 100% Huma: 113 ops re-tipadas, auth route-chain → Security por-op nos 3 namespaces (midaz/routing/plugin-fees) | 3.1, 3.2 | Complete |
| 4 | Pipeline 2-planos migrado para Huma 3.1 nativo; swaggo aposentado; `pkg.HTTPError` morto deletado; identidade total de schema `Error`; paridade pristine travada; `make ci` verde | 4.1, 4.2 | Detailed (Task 4.0 Done) |

---

## Decisões travadas (contexto vinculante para todos os implementadores)

1. **Shape canônico = `problem.Detail` (RFC 9457)** — adotar o lib-commons de verdade (terceiro-trilho), não um alias de doc. Superset local `Detail{ problem.Detail; EntityType string }`.
2. **Paridade = identidade total**, incluindo `@name` unificado (`Error` nos dois planos). Não basta mesmo conjunto de campos.
3. **Branch = direto na `feat/monorepo-consolidation`** no `../midaz`. Consentimento explícito dado.
4. **Money path é terceiro-trilho:** o envelope pode mudar de **shape**; **códigos, semântica e status HTTP não**. Toda mudança que toca `pkg/errors.go`, `pkg/constant/errors.go`, `pkg/net/http/errors.go` passa pelo golden test.
5. **Motor de execução:** um workflow `ring:dispatching-workflows` por onda, reviewers **e** fixers embutidos (self-heal: fix → re-review dentro da onda). Rolling wave: uma fase/onda detalhada por vez; supervisor faz gate independente em cada retorno (nunca confia no status do harness).

**Scrub de 5xx (ACEITO — Fred, 2026-06-30):** `problem.Install()` sanitiza o **texto** de 500/503 (`title`→"Internal Server Error", `detail`→"internal error"). **`code` e `status` sobrevivem verbatim** — invariante money-path mantida. O golden afere só code+status, então continua verde.

---

## Fase 1 — Fundação do envelope de erro (money-path) · **Complete**

**Entregue (2026-07-01):** os dois planos emitem `application/problem+json` (RFC 9457) em runtime; `WithError` reescrito para construir/serializar `*Detail` preservando code+status byte-a-byte; `mmodel.Error` limpo + política de tags; golden test auto-gerador (`pkg/net/http/errors_golden_test.go`, `TestGolden`) varre toda a tabela code→status e fica **verde antes e depois** do swap. Nenhum handler tocado. Commits-âncora: `4793eca44` (golden), `270bc45ef` (swap WithError→problem), `7ca117f9a` (mmodel.Error), `88ab870aa`+`b5cf781d8` (reconciliação de 264 testes de handler, 0 asserções de code/status removidas).

**Lições que vinculam o trabalho restante:**
- **Golden money-path é a rede permanente:** afere só `(code, status)`, sobrevive ao swap de shape e a toda fase seguinte. É a trava do terceiro-trilho.
- **Estado interino spec↔runtime:** ao fim da Fase 1 o runtime era `problem+json` mas as specs ainda eram swaggo. Resolvido por-plano nas Fases 2–3; o resíduo `http.BadRequest` (validação de campo do decode wrapper) fechou na Fase 3 (`withBody.go` → `DecodeAndValidate` fonte única Fiber+Huma).
- **Harness read-only:** reviewers **e** contrarians não mutam a fonte no worktree compartilhado (mutar gerou um falso 409→400 na onda 1).

---

## Fase 2 — Tracer → Huma (piloto) · **Complete**

**Entregue (2026-07-01):** 28 ops protegidas 100% Huma; `openapi.New` + `problem.Install()` nos paths de runtime e spec-gen; 2 security schemes (Bearer + ApiKey) declarados com requirement por-op; `ServeSpec` gated em `SwaggerEnabled` serve OAS 3.1 nativa **em paralelo** ao swagger 2.0 (aditivo, swaggo intacto). Registro extraído por-arquivo (`RegisterXxxRoutes`). Commits-âncora: `5d3a4307e` (bootstrap+referência), `28ddb4ee5`+`c9f89b2a5` (rule 8 ops), `dffa59704`/`ac663569a`/`c51e2fba6`/`3869eda67`/`5e1f711ef`/`f4f2c0537` (fan-out 20 ops + wiring), `61b4b0366`/`0dcf9248b`/`5bb314b6c`/`ca1a0ee82` (auth+spec-lock).

**Padrão de referência (replicado no ledger):** In struct com `RawBody []byte \`contentType:"application/json"\`` + `huma.Operation{SkipValidateBody:true}` → decode/validação imperativos → code canônico, **zero 422 nativo**; path/query com tags `path:`/`query:`/`doc:` **sem** tags de validação; erro via `humaProblem(err)` → `*Detail` (`problem+json`); DELETE 204 = Out sem `Body` + `DefaultStatus:204`; HEAD-count = 204 + `X-Total-Count` + `Content-Length:0`; auth = middleware Fiber, `Security` do Huma é só spec.

**Lições que vinculam o trabalho restante:**
- **Body de sucesso NÃO é byte-idêntico ao Fiber** (`\n` final + HTML-escaping). Invisível a qualquer parser JSON (incl. o SDK gerado). Garantia real = field/status/code/type/entityType-identical, guardada pelo golden. **Não alinhar encoders.**
- **Query params: present-but-empty e repeated-key.** `?x=` (vazio) e `?x=a&x=b` divergiam Huma↔Fiber (gorilla-schema do Fiber é **last-wins**). Binder de list usa `url.Values.Has` (gating presença) + helper `last(key)` (`vs[len-1]`). Copiado inline por arquivo, **não** extraído (serializaria as tasks).
- **flat-401 do tracer preservado:** `pkgHTTP.Unauthorized` emite `{code,title,message}` flat (não `problem+json`); auth é MW pré-Huma → 401 nunca passa pelo Huma. Unificar 401→problem+json é blast-radius alto → **decisão do Fred, fora de escopo.**
- **Follow-up p/ Fred:** pushar `DeclareApiKeyAuth` pra lib-commons (simetria com `DeclareBearerAuth`; hoje o ApiKey scheme é declarado local nos 2 planos).

---

## Fase 3 — Ledger → Huma · **Complete**

**Entregue (2026-07-01):** ledger 100% Huma (**113 ops**) = de-risk `asset.go` (6) + wave-1 CRUD core no-money (45) + wave-2 money-read+routing (23) + wave-3 aditivas open-source (28) + wave-4 money-write (11). Mount huma.API criado no `unified-server.go` (novo group `/v1`, `openapi.New` com `Servers:["/v1"]`, `problem.Install` antes dos Register, `ServeSpec` gated em `LEDGER_HUMA_DOCS_ENABLED`). Auth `ProtectedRouteChain`→Security por-op preservada byte-a-byte nos 3 namespaces (`protectedMidaz`="midaz" / `protectedRouting`="routing" / `protectedFees`="plugin-fees"). Helper compartilhado `pkg/net/http/huma_error.go` (`HumaProblem`) + `DecodeAndValidate` (fonte única Fiber+Huma, fecha o resíduo `http.BadRequest` da Fase 1). swaggo 0-drift em todas as waves.

**Aditivas = third-rail RESOLVIDO por evidência** (recon `aacaa9e243807dc5b`): TODAS as superfícies (CRM holder/instrument/encryption/audit; fees/billing; composition) são **open-source nativas** no monorepo — zero proxy/cliente para plugin closed. `plugin-fees` é namespace RBAC **legado** preservado verbatim (compat X1), não indício de código closed. Migrei tudo, rail avaliado e limpo.

**Lições que vinculam o trabalho restante (CRÍTICAS para Fase 4 e Plano B):**
- **⚠️ Contrato de header de idempotência money-path:** os headers de runtime são `X-Idempotency` / `X-TTL` (lib-commons/v5), **não** `X-Idempotency-Key`/`X-Idempotency-TTL` (o doc-comment CRM estava errado). A wave-4 pegou os shells Huma de create ligados aos nomes errados → chave do caller silenciosamente dropada → fallback pra payload-hash → **retry keyed executaria 2ª mutação de balance** (violação double-entry). Corrigido (`2f06a416c`) + revert-TTL 0→300 (`ParseIdempotencyTTL("")`) + regressão (`4c843f451`). **Lição: money-path exige verificar o CONTRATO de header, não só o helper de parse.** O SDK v4 (Plano B) DEVE mandar `X-Idempotency`/`X-TTL`.
- **Merge-patch RFC 7396** (operation_route/holder/instrument PATCH): campo-ausente vs `null` colapsaria sob decode tipado. O core Huma popula `*Raw` a partir de `in.RawBody` e reproduz a probe de unknown-key; `FindNilFields` compartilhado. Guardado por `TestHuma_UpdateOperationRoute_MergePatch`.
- **201-sempre** em transaction create/commit/cancel/revert (+ replay); read/update/patch = 200.
- **Migração é transport-only:** extrai core transport-agnóstico, wrapper Fiber fino, shell Huma delega ao **mesmo** `services/{command,query}`. Lógica de double-entry/balance nunca tocada.
- **Harness — schema contrarian:** `refuted` (dupla-negativa) gerou falso-positivos (agente que provava correção escrevia `refuted:true`). Trocado por `defectFound:boolean` (true SÓ com defeito real ou verificação inconclusiva). Aplicar nas ondas da Fase 4.
- **`swag init` reprodutível:** a wave que tocar `pkg/net/http` compartilhado quebra/regenera specs dos 2 planos — buildar o MÓDULO inteiro + rodar a suíte do tracer também em cada gate.

**Epic 3.3 (deletar `pkg.HTTPError` morto) → movido para Fase 4:** `pkg.HTTPError` (`pkg/errors.go:139-150`) é código morto (zero construtores runtime) carregando 10 annotations swaggo stale + type-asserts em 4 testes. Foi deliberadamente diferido para concentrar o **único evento de spec-regen** com o rework do pipeline. audit.go/encryption.go mantêm as annotations intactas até lá (swaggo 0-drift preservado).

---

## Fase 4 — Pipeline de spec 2-planos + trava de paridade · **Detailed** (onda corrente)

**Milestone:** o pipeline de geração (`postman/generator/generate-docs.sh`) migra de swag→openapi-generator-Docker→3.0.1 para emissão Huma 3.1 nativa por plano; `redocly join` (ledger-first) + guard de security-scheme + orphan-ref guard sobrevivem; `check-docs.sh` reformulado; swaggo aposentado nos 2 planos; `pkg.HTTPError` morto deletado; as duas specs atingem **identidade total** do schema `Error`; `make ci` verde end-to-end. Entregável: OAS 3.1 pristine + paridade — o insumo do remodel do SDK (Plano B).

**Fatos do recon-f4 (`a059c23a70`) que reformam a fase:**
- A spec Huma-nativa **nunca era dumpada em disco** (só servida em runtime). `api.OpenAPI().YAML()` snapshota tudo após os `huma.Register`, antes do listen → dump offline trivial (sem server/DB/Docker). **Resolvido pela Task 4.0.**
- O schema de erro Huma-nativo chamava-se `Detail` (nome do tipo) nos 2 planos. Impor `Error` = trabalho novo (schema-namer), não reconciliação de diff. **Resolvido pela Task 4.0** (namer compartilhado `problem.Detail`→`Error`).
- **Version-lock do `redocly join`** recusa merge se os `openapi:` divergem → os 2 planos migram em LOCKSTEP 3.0.1→3.1 (ambos já Huma).

**Decisões do supervisor (2026-07-01, recon-f4 — todas judgment, sem gate do Fred):**
- **(a) Emit = test-golden com flag `-update`** (não `cmd/dump-spec`): reusa o wiring que os contract-tests já constroem, casa com o modelo regen+git-diff do drift.
- **(b) Nome `Error` via schema-namer COMPARTILHADO no repo** (`pkg/net/http/huma_schema_namer.go`), **não** empurrado pra lib-commons (outros consumidores podem não querer o erro chamado `Error`).
- **(c) redocly.yaml:** re-habilitar as regras relaxadas SÓ por artefato swag/openapi-generator (`no-invalid-schema-examples`, `no-server-trailing-slash`, `no-server-example.com`, `security-defined` com override per-path pros públicos do tracer).
- **(d) Epic 3.3 escopo COMPLETO:** struct + 10 annotations + **4 arquivos de teste com type-assertion que QUEBRAM compilação** (validate-package-range-amount_test.go, feeshared/nethttp/httputils_test.go ×2, feeshared/model/package_test.go). Não é seguro deixar os testes.
- **(e) De-risk primeiro** (espelha o de-risk da Fase 3 que valeu): dump + namer + prova de determinismo/OAS-3.1/paridade ANTES de reformar o pipeline.

### Task 4.0 (DE-RISK): dump offline das 2 specs Huma + schema-namer `Detail`→`Error` — **Done**

- [x] Done — impl `24196c4da` (namer compartilhado), `0e1a59def` (seam `registerTracerHumaRoutes`), `497394642` (golden `-update` dump por plano), `2b5575abf` (lock cross-plane de paridade), `72f53d32c` (self-heal: comentário 31→28), `d4f44b84e` (supervisor: dump do tracer hermético). **Gate PASS.**

**Entregue:** (1) schema-namer compartilhado renomeia `problem.Detail`→`Error` nos 2 planos (guarda `Name()=="Detail" && PkgPath()==problem`; `ledgerSchemaNamer` mantém os prefixos Fee/Operation/Transaction das waves 1-4 e cai no fallthrough compartilhado). (2) Seam `registerTracerHumaRoutes` extraído (análogo ao `humaMount` do ledger) — chamável de teste sem bootstrap/DB; produção e teste chamam a MESMA função (28 ops byte-a-byte vs base). (3) Dump golden `-update` por plano → `components/{ledger,tracer}/api/openapi.huma.yaml` (OAS 3.1), **nome distinto** do swaggo `api/openapi.yaml`. (4) `tests/openapi/error_schema_parity_test.go` trava a identidade do closure `Error` (computado, não hardcoded).

**Gate do supervisor (verificado independentemente, não o status do harness):**
- Diff `ba2ecc944..HEAD` toca só 11 arquivos; **zero** swaggo/pipeline (`swagger.json`/`generate-docs.sh`/`check-docs.sh`/`docs.go`/`@Router`/`@Security` intactos).
- Determinismo: `TestOpenAPISpecDump` verde `-count=2` nos 2 planos (golden reproduzível, drift=0).
- Paridade: `Error` byte-idêntico entre planos (closure `{Error, ErrorDetail}`); nenhum schema `Detail` cru; ledger 113 ops / tracer 28 ops.
- Money-path golden verde; módulo inteiro builda EXIT0; tracer http/in (4.75s) + ledger http/in + bootstrap verdes.
- **Medium fechado por mim:** o dump do tracer lia `Version` de `os.Getenv("VERSION")` (não-hermético; ledger já era `"test"`). Hardcoded `"test"` + regen do golden. Regressão confirmada: `VERSION=v9.9.9-CI go test -run TestOpenAPISpecDump` **agora passa** (antes falhava).
- **Residual Low (aceito):** `problemDetailPkgPath` casado por string — frágil a refactor de path do lib-commons, mas o parity test pega. Guardado, não bloqueia.

### Epic 4.1: Pipeline → Huma 3.1 nativo + aposentar swaggo + Epic 3.3
**Goal:** `make generate-docs` produz `postman/specs/midaz.openapi.yaml` a partir das specs Huma 3.1 (Task 4.0), sem estágio Docker; swaggo aposentado nos 2 planos; `pkg.HTTPError` morto deletado.
**Scope:** `postman/generator/{generate-docs,check-docs}.sh`, `redocly.yaml`, `components/*/api/`, annotations dos 2 planos, `components/*/cmd/app/main.go` (general-info), `tracer/scripts/verify-api-docs.sh`, Makefiles (swag targets), `pkg/errors.go`, 4 testes de fees.
**Dependencies:** Task 4.0. ✅
**Done when:** `make generate-docs` sem estágio Docker; `CHECK_DOCS_REGEN=1 make check-docs` verde; `grep @Router|@Security` = 0; `pkg.HTTPError` não existe; build+testes verdes.
**Status:** Pending — **elaborar em tasks dispatch-ready contra `generate-docs.sh`/`check-docs.sh`/`redocly.yaml` reais no lançamento da onda** (recon do pipeline primeiro).

### Epic 4.2: Trava de paridade + verificação pristine + `make ci`
**Goal:** travar no `check-docs.sh` a identidade total do schema `Error` entre os 2 planos; re-habilitar as regras redocly (decisão c); verificação final de OAS 3.1 pristine com UM shape de erro; `make ci` verde end-to-end.
**Scope:** `check-docs.sh` (parity_check do `Error`), `redocly.yaml`, specs consolidadas.
**Dependencies:** Epic 4.1.
**Done when:** diff dos 2 schemas `Error` vazio (travado no CI); redocly lint verde com regras re-habilitadas; golden money-path verde; `make ci` verde.
**Status:** Epic-level (detalhar após Epic 4.1).

---

## Handoff para o Plano B (SDK v4 remodel)

O entregável desta fase é o contrato que o `midaz-sdk-golang` v4 consome. Invariantes que o SDK DEVE respeitar (herdadas das lições acima):
- **Um único envelope de erro** = `Error` (RFC 9457, `problem.Detail` + `entityType`), idêntico nos 2 planos.
- **Idempotência:** mandar `X-Idempotency` / `X-TTL` (nomes de runtime), nunca `X-Idempotency-Key`.
- **Paginação tipada** + ambos os planos first-class (ledger + tracer).
- Specs OAS 3.1 pristine como insumo de codegen (hybrid codegen+facade, breaking em `/v4`, sem shim de compat).

---

## Self-review (checklist do ring:writing-plans)

- **Cobertura da spec:** os 5 buracos dos juízes originais cobertos — auth mal-declarado (Fases 2/3), `pkg.HTTPError` (Epic 4.1/3.3), `required[]` divergente (Fase 1 + Epic 4.2), `mmodel.Error` sujo (Fase 1), nome de schema divergente (Task 4.0 + Epic 4.2). ✅
- **Money path:** front-loaded na Fase 1 com golden RED-first; invariante code+status guardada em todas as fases; contrato de header de idempotência verificado na wave-4. ✅
- **Fronteiras de fase:** cada fase termina buildando + testável. ✅
- **Consistência de contrato:** envelope canônico (`Error` = `problem.Detail` + `entityType`) definido uma vez, referenciado por todas as fases + handoff do Plano B. ✅
- **Rolling wave:** Fases 1-3 colapsadas a Done + lições; Fase 4 corrente com Task 4.0 Done e Epics 4.1/4.2 a detalhar no lançamento da onda. ✅
