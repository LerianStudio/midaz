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
| 4 | Pipeline 2-planos migrado para Huma 3.1 nativo; swaggo aposentado; `pkg.HTTPError` morto deletado; identidade total de schema `Error`; paridade pristine travada; `make ci` verde | 4.1, 4.2 | Complete (Epics 4.1 + 4.2 Done; gate final PASS) |

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
**Goal:** `make generate-docs` produz `postman/specs/midaz.openapi.yaml` a partir das specs Huma 3.1, sem swag nem Docker; swaggo aposentado (annotations + runtime + gerados) nos 2 planos; `pkg.HTTPError` morto deletado; `make ci` verde.
**Scope:** `postman/generator/{generate-docs,check-docs}.sh` + `redocly.yaml`; handlers dos 2 planos (annotations); `components/*/cmd/app/main.go` (general-info); wiring de runtime swaggo (`ledger/internal/bootstrap/{unified-server,swagger}.go`, `tracer/internal/adapters/http/in/swagger.go`); gerados `components/*/api/{docs.go,swagger.json,swagger.yaml,openapi.yaml}`; `go.mod`; `tracer/scripts/verify-api-docs.sh` + Makefiles; `pkg/errors.go` + 5 testes.
**Dependencies:** Task 4.0. ✅
**Status:** Done — ondas 4.1a + 4.1b completas (pipeline Huma 3.1 nativo + swaggo aposentado + `pkg.HTTPError` deletado). Gate do supervisor PASS nas duas.

**Fatos do recon-epic41 que moldam a fase:**
- **Tooling presente** (node 26 / npm 11 / redocly 2.32 / docker 29; `postman/generator/node_modules` existe) → o workflow **roda o pipeline e se auto-verifica**.
- **`generate-docs.sh` (435 linhas):** REPLACE `resolve_swag_bin`(57-81)/`generate_openapi_spec`(86-110, swag init)/`generate_openapi_yaml`(113-142, openapi-generator Docker); MODIFY `publish_specs`(145-160) e `consolidate_openapi`(165-270) — trocar os inputs do `redocly join`(207-216, ledger-first) de `api/openapi.yaml`→`api/openapi.huma.yaml`; PRESERVE version-parity assert(183-203), security jq guard(235-247, os 2 schemes já existem no dump), orphan-ref guard(249-264), npm/postman/verify.
- **`check-docs.sh` (238 linhas):** `parity_check`(125-134) e `security_coverage_check`(140-166, `SECURITY_COVERAGE_COMPONENT=ledger`) lêem o `swagger.json` swaggo (seam `read_field*`/`assert_*`, 60-123, hardcode 63/76) → retarget p/ `openapi.huma.yaml`. **`.schemes`(131) não existe em OAS 3.1** → dropar/reescrever p/ `servers`. `.info.version ^4\.0\.0$`(132) — ver Task 4.1.1. `drift_check`(201-218, `CHECK_DOCS_REGEN=1`) roda generate-docs + git-diff `components/*/api` `postman/specs` — preservar.
- **swaggo tem WIRING DE RUNTIME (não é só comment):** ledger `unified-server.go:20` blank-import `_ ".../ledger/api"` (dispara `SwaggerInfo` via `docs.go init()`), `:26` `fiberSwagger`, `:99-103` rotas `/swagger`+`/swagger/*`, `bootstrap/swagger.go` (`initSwaggerFromEnv` lê `SWAGGER_*`→`api.SwaggerInfo`); tracer `in/swagger.go` (rota swaggo). **A UI de docs migra p/ o Huma `/v1/docs` (ServeSpec).**
- **`tracer/api/types.go` SOBREVIVE** (hand-written `ReadyzResponse`/`ReadyzCheck`, LIVE em `readyz.go:157,162,+sigs`). Deletar só os gerados do pacote, não o pacote.
- **Epic 3.3 = 5 compile-breakers (não 4):** o 5º é `pkg/errors_test.go:278-306` `TestHTTPError_Error` que **constrói** o struct (deletar a função inteira); os 4 de fees são branches `if _, ok := err.(*pkg.HTTPError)` mortos (`validate-package-range-amount_test.go:267`, `feeshared/nethttp/httputils_test.go:217,319`, `feeshared/model/package_test.go:2190`).

#### Onda 4.1a — Pipeline → Huma 3.1 (ADITIVO, swaggo intacto) + de-risk join/lint · **Done**

**Commits:** `936e99eab` (4.1.1 pin version→4.0.0) → `2bd6f3e4e` (4.1.2 rewire generate-docs, drop swag+Docker) → `637730d4b` (4.1.3 reforma check-docs) → `2619b1b90` (4.1.4 pipeline e2e + commit specs) → `4ca895fbe` (self-heal: determinismo da collection) → `0662dedeb` (**fix do supervisor: leak de título**).

**Gate do supervisor (harness voltou `HEALED_NEEDS_REVERIFY` — reverificado + fechado por mim):**
- Review (logic+commons) e Contrarian (4 lentes) **ambos rodaram** → workflow compliant.
- **L1 determinismo (defeito real do contrarian):** curado na raiz — `convert-openapi.js` troca `new Date()` por `EXAMPLE_DATE_TIME` fixo (3 sites); `workflow-processor.js` troca `uuidv4()` por `uuidv5()` content-seeded. Reverifiquei: `make generate-docs` 2× → árvore limpa; `CHECK_DOCS_REGEN=1 make check-docs` verde ("no drift").
- **Medium (leak de título) — órfão do self-heal, fechado por mim:** `buildUnifiedHumaAPI` (contract_spec_routes_test.go:116) usava `Title: "contract-spec"`, divergindo da produção (`unified-server.go:127` = "Midaz Ledger API"). Como a 4.1a ligou o pipeline no dump do fixture, o placeholder vazava pro spec publicado (`info.title` + 22 tags `contract-spec_` via `--prefix-tags-with-info-prop title`, ledger-first). Alinhei o fixture ao runtime → `info.title: Midaz Ledger API`, tags `Midaz_Ledger_API_*` (idêntico ao baseline swaggo), zero `contract-spec` residual; re-habilitei a asserção `^Midaz` no `parity_check` (verde). contact/license/tos/schemes seguem descartados honestamente (Huma emite só `info.{title,version}`; OAS 3.1 não tem `.schemes`).
- Determinismo pós-fix: árvore limpa; swaggo canônico byte-idêntico base→HEAD (8 blobs); redocly lint RODOU (não-skip) + passou; joined = OAS 3.1, 113 ledger + 28 tracer ops.

**Lições:** (1) `--prefix-tags-with-info-prop title` faz o título do plano ledger-first virar prefixo de TODAS as tags do joined — título errado polui título + tags de uma vez. (2) `HEALED_NEEDS_REVERIFY` = self-heal cura as refutações do contrarian, não necessariamente os findings do reviewer; o gate do supervisor pega o órfão. (3) O bloco `info` do Huma é enxuto por decisão documentada (check-docs.sh:14-17) — restaurar contact/license faria o dump mentir vs o runtime servido.

Prova que o `redocly join` + guards + `redocly lint` passam nas 2 specs Huma **antes** de aposentar swaggo (o join nunca foi rodado — Task 4.0 só provou determinismo + paridade Error). Swaggo fica 100% presente; só o pipeline troca de fonte.

##### Task 4.1.1: Promover a versão do dump `test`→`4.0.0` (2 planos) + regen goldens
- [x] Done
**Context:** o dump Huma usa `Version: "test"` (tracer `openapi_spec_dump_test.go:56` pós-hermetização `d4f44b84e`; ledger `contract_spec_routes_test.go:116`). O `parity_check`(check-docs.sh:132) exige `.info.version ^4\.0\.0$` — o `@version 4.0.0` do swaggo main.go retirado. O artefato publicado + committed precisa da versão de contrato.
**Implementation vision:** trocar `Version` dos 2 builders de dump p/ `"4.0.0"` (fixo/hermético — não `os.Getenv`, preserva drift-determinismo), regen os 2 goldens via `go test -run TestOpenAPISpecDump ./components/{plane}/... -update`. `info.version` fica `4.0.0` nos 2 `openapi.huma.yaml`. (Versão dinâmica de build, se um dia desejada, é no path servido em runtime, não no golden committed.)
**Files:** Modify `components/tracer/internal/adapters/http/in/openapi_spec_dump_test.go:56`, `components/ledger/internal/adapters/http/in/contract_spec_routes_test.go:116`, os 2 `components/*/api/openapi.huma.yaml` (regen).
**Verification:** `go -C <wt> test -buildvcs=false -run TestOpenAPISpecDump ./components/{ledger,tracer}/internal/adapters/http/in/` verde `-count=2` (determinístico); `grep 'version: 4.0.0'` nos 2 goldens; parity Error (`tests/openapi`) segue verde.
**Done when:** os 2 goldens carregam `version: 4.0.0`, determinístico, paridade intacta.

##### Task 4.1.2: Rewire `generate-docs.sh` p/ dump Huma (drop swag+Docker, retarget join)
- [x] Done
**Context:** anchors no recon acima. Decisão (a): emit = `go test -run TestOpenAPISpecDump -update` (reusa o wiring dos contract-tests). O join precisa dos `openapi.huma.yaml`; o drift_check re-roda generate-docs → a etapa de spec-gen DEVE regenerar (não só consumir committed).
**Implementation vision:** substituir `resolve_swag_bin`/`generate_openapi_spec`/`generate_openapi_yaml` por uma etapa que roda `go test -run TestOpenAPISpecDump ./components/{ledger,tracer}/... -update -buildvcs=false` (regenera os 2 `api/openapi.huma.yaml`); `publish_specs` copia `openapi.huma.yaml`→`postman/specs/<c>/`; `consolidate_openapi` troca os 2 inputs do `redocly join`(207-216) p/ `openapi.huma.yaml` mantendo ledger-first + `--prefix-tags-with-info-prop title`. PRESERVAR os 3 guards (version-parity, security-scheme, orphan-ref) — ambos os dumps são `3.1.0` + declaram os 2 schemes. Atualizar o comentário stale "ApiKeyAuth (tracer)"(245) → tracer tem os 2.
**Files:** Modify `postman/generator/generate-docs.sh`.
**Verification:** `make generate-docs` produz `postman/specs/midaz.openapi.yaml` (openapi 3.1.0, 141 ops, `Error` unificado) SEM chamar swag nem Docker; rodar 2× → git-diff limpo (determinístico).
**Done when:** pipeline gera o joined 3.1 das specs Huma, guards verdes, zero swag/Docker.

##### Task 4.1.3: Reformar `check-docs.sh` (retarget seam + dropar `.schemes`)
- [x] Done
**Context:** seam `read_field*`/`assert_*`(60-123) + parity_check(125-134) + security_coverage_check(140-166) lêem `components/<c>/api/swagger.json`.
**Implementation vision:** retarget o path do seam p/ `components/<c>/api/openapi.huma.yaml`; remover a asserção `.schemes`(131, inexistente em 3.1) — se quiser paridade de servidor, comparar `.servers`; manter `.info.version ^4\.0\.0$`(agora satisfeito pela 4.1.1) + `.info.title ^Midaz`; security_coverage jq `.paths[].{verb}.security`(150-152) funciona igual em 3.1, só troca o path; manter drift_check.
**Files:** Modify `postman/generator/check-docs.sh`.
**Verification:** `CHECK_DOCS_REGEN=1 make check-docs` verde (parity + security-coverage + redocly lint + drift git-diff-clean).
**Done when:** check-docs valida a spec Huma 3.1, sem referência a `swagger.json`/`.schemes`.

##### Task 4.1.4 (integração/de-risk): rodar o pipeline ponta-a-ponta
- [x] Done
**Context:** tooling presente. Este é o de-risk: provar join+lint+guards nas specs Huma reais.
**Implementation vision:** rodar `make generate-docs` + `CHECK_DOCS_REGEN=1 make check-docs` end-to-end; confirmar `redocly lint` do joined NÃO é skip (existe) e passa com as regras atuais; commitar o `postman/specs/midaz.openapi.{yaml,json}` regenerado (o único evento de spec-regen). Swaggo permanece 100% presente (annotations, `/swagger/*`, gerados) — verificar que `swag init` ainda reproduz `api/swagger.json` byte-a-byte (aditivo).
**Files:** commit dos artefatos `postman/specs/`.
**Verification:** `make generate-docs` + `CHECK_DOCS_REGEN=1 make check-docs` verdes; joined = OAS 3.1, 141 ops, `Error` unificado; swaggo drift 0.
**Done when:** o pipeline Huma 3.1 roda limpo end-to-end com swaggo ainda intacto — bridge provada antes de queimar.

#### Onda 4.1b — Aposentar swaggo + Epic 3.3 (após gate da 4.1a) · **Done**

**Commits:** `5bedcb72e` (4.1.5 annotations ledger) → `1f835a3c9` (4.1.5 annotations tracer) → `eca60ed12` (4.1.6 runtime wiring + gerados deletados + go mod tidy + Makefiles) → `8e7f82323` (4.1.7 Epic 3.3 pkg.HTTPError + 5 compile-breakers) → `3fcf9eeb8` (self-heal: sweep completo dos 2 dialetos) → `f40a4da44` (**cleanup do supervisor: 3 Lows órfãos**).

**Gate do supervisor (harness voltou `HEALED_NEEDS_REVERIFY` — reverificado + fechado por mim):**
- Review (dead-code+logic+commons) e Contrarian (4 lentes) **ambos rodaram** → compliant.
- **L4 defeito (contrarian):** 49 `@Description` swaggo residuais FORA do escopo handler-only (em `postgres/{operation,transaction,assetrate}` + `pkg/feeshared/model` + `tracer/pkg/model`). **Descoberta:** o repo tinha DOIS dialetos — swaggo (`@Router` nos handlers) E go-swagger (`swagger:model`/`@name` nos model structs). Self-heal fez sweep completo dos dois (245 deleções, 27 arquivos, só comentários — verifiquei `} // @name X → }`, zero código tocado).
- **3 review Lows (não passados ao self-heal, fechados por mim `f40a4da44`):** (1+2) `api.ErrorResponse`/`api.VersionResponse` (tracer/api/types.go) órfãos — só consumidos pelas annotations+docs.go deletados; deletei os 2 structs (ReadyzResponse/ReadyzCheck ficam, vivos em readyz.go). (3) campo `errCode` em httputils_test.go órfão — o branch `err.(*pkg.HTTPError)` que o lia estava morto; re-apontei pro erro VIVO (`errors.As` → `pkg.ValidationError.Code`), reativando a cobertura por-código que a tabela sempre pretendeu (8/8 casos passam, prova que toda a validação é ValidationError uniforme).
- **Reverify independente:** build `./...` EXIT0; suites verdes (money-path golden + parity + fees); `CHECK_DOCS_REGEN=1 make check-docs` verde (parity ^Midaz + security-coverage 113 + redocly lint + drift determinístico); árvore limpa; grep-de-ausência dos 2 dialetos + go.mod + pkg.HTTPError todos 0. Invariantes A (`swaggerEnabled()`+`ServeSpec` vivos), B (`tracer/api/types.go`+readyz), C (`openapi.huma.yaml` intocados) verificadas.
- **Follow-up p/ Epic 4.2 (Info, não bloqueante):** o gate DC-3 (rota montada-vs-spec, comparava com `swagger.json` deletado) foi aposentado sem substituto Huma. O golden dump pega mudança de spec, mas NÃO "rota montada no Fiber mas ausente do contrato publicado" — invisível pro SDK do Plano B. Reinstaurar um diff montado-vs-`openapi.huma.yaml`.

Agora que o pipeline Huma está provado, remover swaggo por completo.

##### Task 4.1.5: Deletar annotations swaggo + general-info
- [x] Done
**Escopo:** todas as `@Router`/`@Security`/`@Success`/`@Failure`/`@Param`/`@securityDefinitions` dos handlers dos 2 planos (ledger ~114/137/570/565; tracer 31/28/123/83) + os blocos general-info em `components/{ledger,tracer}/cmd/app/main.go` (ledger:19-75, tracer:25-53). NÃO tocar código executável.
**Verification:** `grep -rn '@Router\|@Security' components/ --include='*.go'` = 0 (fora de `_test`); build EXIT0.

##### Task 4.1.6: Remover wiring de runtime swaggo + gerados (preservar `tracer/api/types.go`)
- [x] Done
**Escopo:** ledger — `unified-server.go:20` (blank import), `:26` (`fiberSwagger`), `:99-103` (rotas `/swagger`), `bootstrap/swagger.go` (`initSwaggerFromEnv`/`SWAGGER_*`/`WithSwaggerEnvConfig`); tracer — `in/swagger.go` + sua rota. Deletar gerados `components/{ledger,tracer}/api/{docs.go,swagger.json,swagger.yaml,openapi.yaml}` **mantendo `tracer/api/types.go`** (LIVE em `readyz.go`) — o pacote `tracer/api` sobrevive. `go mod tidy` (drop `swaggo/fiber-swagger` + `swaggo/swag`). Deletar `tracer/scripts/verify-api-docs.sh` + targets swag nos Makefiles (ledger:171-173, tracer:146-149+128+181-182). Docs UI passa a ser Huma `/v1/docs` (ServeSpec).
**Verification:** build módulo EXIT0; `readyz` compila (usa `api.ReadyzResponse`); `grep swaggo go.mod` = 0; tracer + ledger sobem sem `/swagger/*`.

##### Task 4.1.7: Epic 3.3 — deletar `pkg.HTTPError` + corrigir 5 compile-breakers
- [x] Done
**Escopo:** deletar struct `pkg/errors.go:139-150` (zero construtores runtime, confirmado). As 10 annotations `@Failure pkg.HTTPError` (audit.go:88-89, encryption.go:39-43,143-145) já morreram na 4.1.5. Corrigir ATÔMICO (senão build quebra): deletar os branches mortos `if _, ok := err.(*pkg.HTTPError)` em `validate-package-range-amount_test.go:267`/`feeshared/nethttp/httputils_test.go:217,319`/`feeshared/model/package_test.go:2190`; deletar a função inteira `TestHTTPError_Error` (`pkg/errors_test.go:278-306`, constrói o struct).
**Verification:** `grep -rn 'HTTPError' pkg/ components/ --include='*.go'` sem o struct morto; build + `go test ./pkg/... ./components/ledger/internal/services/fees/... ./components/ledger/pkg/feeshared/...` verdes.

##### Task 4.1.8 (verify): `make ci` verde
- [x] Done
**Verification:** `make ci` (lint → check-telemetry → proto-check → test-unit -race → `CHECK_DOCS_REGEN=1 check-docs`) verde end-to-end; `grep @Router` = 0; `pkg.HTTPError` inexistente; golden money-path verde.
**Done when:** swaggo aposentado, pkg.HTTPError deletado, `make ci` verde — Epic 4.1 completa.

### Epic 4.2: Trava de paridade + verificação pristine + `make ci`
**Goal:** travar no `check-docs.sh` a identidade total do schema `Error` entre os 2 planos; re-habilitar as regras redocly (decisão c); verificação final de OAS 3.1 pristine com UM shape de erro; `make ci` verde end-to-end.
**Scope:** `check-docs.sh` (parity_check do `Error` + reinstaurar o gate DC-3 rota-montada-vs-`openapi.huma.yaml`, aposentado na 4.1b), `redocly.yaml` (re-habilitar regras só-swaggo agora que os dumps são Huma nativos), specs consolidadas.
**Dependencies:** Epic 4.1. ✅
**Done when:** identidade do schema `Error` travada no CI (Go `tests/openapi/error_schema_parity_test.go` já trava o closure in-memory — decidir se check-docs adiciona trava spec-level); regras redocly só-swaggo re-habilitadas (as que sobram justificadas p/ Huma); gate rota-montada-vs-spec reinstaurado (protege o Plano B); golden money-path verde; `make ci` verde.
**Status:** Done — `916762ad0`, `0503bcc05`, `b08d376bc`, `7b6753b25` (impl) + `84233f7ca`, `a9871237b`, `b6298c521`, `eb4687998` (fixes do gate). Gate PASS (juiz LLM: CONDITIONAL PASS → PASS, zero blockers).

##### Task 4.2.1: Re-habilitar as regras redocly só-swaggo (empírico)
- [x] Done — impl `916762ad0`. 5 regras re-habilitadas (0 findings nos dumps Huma nativos: `no-server-trailing-slash`, `no-server-example.com`, `security-defined`, `no-unused-components`, `no-invalid-schema-examples`); 3 mantidas off com justificativa Huma-era reescrita (`no-empty-servers` = join artifact; `operation-4xx-response` = 71 ops sem 4xx explícito; `no-ambiguous-paths` = 2 sub-paths ledger). redocly lint verde.
**Context:** `redocly.yaml` relaxa 8 regras, TODAS justificadas como "inherited from swaggo/openapi-generator artifacts" ou "join artifact". Agora os dumps são Huma 3.1 nativos (servers=`/v1` relativo, sem host/trailing-slash; examples de struct tags; 28 tracer ops todas com Security). Muitas relaxações são obsoletas.
**Implementation vision:** flipar cada regra de volta (exceto `no-empty-servers` = join artifact real, KEEP off) e rodar `cd <wt> && CHECK_DOCS_REGEN=1 make check-docs` (roda `redocly lint` do joined). Re-habilitar as que passam; manter off SÓ as que genuinamente ainda trippam, com comentário RE-ESCRITO pra era Huma (não mais "swag emits"). Hipótese (verificar): re-enable `no-server-trailing-slash`, `no-server-example.com`, `security-defined`, `no-unused-components`; testar `no-invalid-schema-examples`, `operation-4xx-response`, `no-ambiguous-paths`.
**Files:** Modify `postman/generator/redocly.yaml`.
**Verification:** `CHECK_DOCS_REGEN=1 make check-docs` verde com as regras tightened; cada relaxação restante tem justificativa Huma-era verdadeira.
**Done when:** redocly.yaml só relaxa o que os dumps Huma genuinamente exigem, cada uma com justificativa atual; lint verde.

##### Task 4.2.2: Trava spec-level do `Error` no joined (complementa o Go test)
- [x] Done — impl `0503bcc05`. `error_schema_singleton_check` (jq sobre o joined json): exige UM `Error` canônico, sem irmãos sufixados pelo join (`^Error[-_]?[0-9]+$`, poupa `ErrorDetail`), + shape RFC 9457. Mutação provada pelo supervisor: injetar `Error2` → FALHA; remover campo `code` → FALHA; restore byte-clean.
**Context:** `tests/openapi/error_schema_parity_test.go` já trava o closure `Error` byte-idêntico entre os 2 dumps POR-PLANO (roda em `make ci`). GAP: o spec JOINED (`postman/specs/midaz.openapi.json`, o que o SDK do Plano B consome) é saída do `redocly join` — nada garante que o join não duplica/sufixa o `Error` (ex. `Error`+`Error2`).
**Implementation vision:** adicionar ao `check-docs.sh` uma asserção (jq sobre o joined json) de que existe EXATAMENTE UM schema `Error` canônico (RFC 9457 shape: `error`+`error_description`/`type`/etc. conforme o dump), sem duplicata induzida pelo join. NÃO reimplementar o closure em bash (o Go test cobre); só guardar o artefato publicado contra duplicação do join. Comentar apontando pro Go test como a trava primária.
**Files:** Modify `postman/generator/check-docs.sh`.
**Verification:** `CHECK_DOCS_REGEN=1 make check-docs` verde; a asserção FALHA se o joined tiver `Error` duplicado/sufixado (provar com mutação temporária).
**Done when:** o joined publicado é guardado contra duplicação do schema `Error` pelo join.

##### Task 4.2.3: Reinstaurar o gate DC-3 (rota-montada-vs-spec) contra o dump Huma
- [x] Done — impl `b08d376bc`. `TestContractSpecMatchesRoutes` lê `openapi.huma.yaml`, diff bidirecional montado-vs-spec, `canonicalizePath` colapsa `:param`↔`{param}`→`{}` (compara estrutura, não label), exempts=const (probes + DSL multipart). Mutação provada pelo supervisor: remover exempt `/health` → FALHA (mounted=114 vs spec=113). 113=113 no estado limpo.
**Context:** `eca60ed12` (4.1.6) removeu TODO o DC-3 de `contract_spec_routes_test.go` (`specPath=api/swagger.json`, `collectMountedRoutes`, `collectSpecRoutes`, `TestContractSpecMatchesRoutes`, lista de exempts) pois comparava com o `swagger.json` deletado. Sem substituto, "rota montada no Fiber mas ausente do contrato publicado" fica invisível — risco pro SDK do Plano B. A versão base é `git show 767133e9:components/ledger/internal/adapters/http/in/contract_spec_routes_test.go`.
**Implementation vision:** re-adicionar o gate usando a versão base como referência, com collectSpecRoutes lendo `../../../../api/openapi.huma.yaml` (YAML, não swagger.json). Normalizar: paths Huma usam `{param}` + prefixo `/v1`; Fiber usa `:param`. Montar via `buildUnifiedHumaAPI()` (retorna app+api; o app tem TODAS as rotas montadas). Exempts atualizados: `/health`,`/version`,`/readyz` (probes Fiber públicas, não no dump Huma) + a rota DSL `POST /v1/transactions/dsl` (Fiber intencional, sunset 2026-08-01, não migra) — **SEM** mais `/swagger*` (deletado). Lista de exempts = const (não pode crescer silenciosamente). O gate falha em qualquer divergência montado-vs-spec.
**Files:** Modify `components/ledger/internal/adapters/http/in/contract_spec_routes_test.go`.
**Verification:** `go -C <wt> test -buildvcs=false -run TestContractSpecMatchesRoutes ./components/ledger/internal/adapters/http/in/` verde; provar que pega divergência (mutação: montar uma rota fora do spec → falha).
**Done when:** o gate montado-vs-`openapi.huma.yaml` roda verde e pega divergência real.

##### Task 4.2.4 (verify): `make ci` verde end-to-end
- [x] Done — `make ci` verde e TRUSTWORTHY (exit 0, self-sufficient sem GOFLAGS externo): lint (4 escopos) → telemetry → proto → test-unit `-race` (**13.918 testes, 0 fail/race/panic**) → check-docs (no drift) → cross-plane spec locks. Verificado 5× (incl. 2× pelo juiz LLM independente).
**Verification:** `cd <wt> && make ci` — reportar cada etapa (lint → check-telemetry → proto-check → test-unit -race → check-docs). Falha só bloqueia se causada por esta onda; falha ambiental (protoc/lint infra) reportada e isolada, confirmando build+test+check-docs verdes.
**Done when:** `make ci` verde (ou só falhas ambientais isoladas documentadas) — Epic 4.2 completa, Fase 4 pronta pro gate final.

**Deviations / achados do gate 4.2.4 (o "verify make ci" fez o trabalho pra que existe):**
- `make ci` estava VERMELHO na base do Epic 4.2 por 2 defeitos latentes das waves de migração, invisíveis aos gates targeted-test (4.1a/4.1b nunca rodaram `make ci` lint completo). Corrigidos: `84233f7ca` (money-path: `createTransactionFiber` param `isRevert` sempre-false removido — behavior-preserving, revert vai por `createRevertTransaction`), `a9871237b` (`problem.go` wsl_v5, 10 blank lines no dispatch de erro).
- 3 false-greens de gate descobertos e neutralizados: (1) pipe `tee` mascarando exit do make; (2) cache de resultado do golangci-lint envenenado; (3) `test-unit` `go list ./...` fail-open em worktree (0 pacotes → exit 0, pulava 13.9k testes). Fix `b6298c521`: `test-unit` agora fail-CLOSED em descoberta vazia + self-sufficient (`GOFLAGS=-buildvcs=false`).
- Parity lock do SDK não gateava: `tests/openapi/error_schema_parity_test.go` (closure `Error` cross-plane por-plano) estava escrito pra rodar no gate mas o filtro `/tests` do test-unit o excluía. Fix `7b6753b25`: target `test-openapi-locks` (offline) wired no `ci` após check-docs.
- Hygiene: `eb4687998` removeu ~700 linhas de anotação swaggo/go-swagger morta em `pkg/mmodel`+`pkg/mtransaction` (comment-only, no-drift provado) — completa "swaggo fully retired".

**Gate final (Plano A) — juiz LLM independente: CONDITIONAL PASS → PASS.** Reproduziu `make ci` verde 2×, provou cada gate MORDE (mutação → RED → revert), confirmou money-path intacto, contagens exatas (ledger 113 + tracer 28 + 3 probes = 141), swaggo 100% aposentado. Única condição (o 4º false-green do test-unit) corrigida + provada. Zero blockers. Residuais aceitos (documentados, não-corrigíveis sem trade-off): `operation-4xx-response: off` (71 ops sem 4xx explícito — completude, não correção; SDK herda `Error` schema definido); `check-docs` fail-open standalone sem artefato (não sob `make ci`); `problemDetailPkgPath` string-match frágil (parity test pega quebra).

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
