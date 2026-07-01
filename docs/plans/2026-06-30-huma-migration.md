# Migração swaggo→Huma + Envelope de Erro Unificado (RFC 9457) no midaz

> **Para implementadores:** Use ring:executing-plans (rolling wave), ou
> ring:dispatching-workflows para rodar cada fase como workflow multi-agente
> revisado. **Motor recomendado desta migração:** a skill
> `ring:adopting-lib-commons-huma-wrapper` (playbook de 11 gates, TDD, despacha
> `ring:backend-go`) — ela já codifica os landmines desta migração. Este
> documento é a fonte viva de verdade; a elaboração de tasks das fases
> posteriores é escrita de volta aqui durante a execução.
>
> **Local:** `docs/plans/2026-06-30-huma-migration.md` no worktree dedicado
> `/Users/fredamaral/repos/lerianstudio/midaz-huma` (repo `midaz`, branch
> `feat/monorepo-consolidation`). Aprovado por Fred em 2026-06-30.

**Goal:** Migrar os dois planos HTTP do midaz (ledger + tracer) de swaggo/Fiber-nativo para o wrapper Huma do lib-commons (`commons/net/http/{openapi,problem}`), adotando `problem.Detail` (RFC 9457) como o **único** envelope de erro, com **identidade total de schema** entre os planos e **preservação byte-a-byte** de todos os ~585 códigos de erro e seu mapeamento code→status. Entregável final: specs OAS 3.1 pristine e em paridade que o `midaz-sdk-golang` v4 vai consumir.

**Architecture:** Introdução greenfield do wrapper Huma (zero adoção hoje). O envelope de erro é a espinha money-path, então ele vem primeiro e sozinho (Fase 1), guardado por um golden test que varre toda a tabela code→status e trava antes de qualquer swap de dispatcher. Só depois vem a reescrita de assinatura dos 146 handlers, plano a plano (tracer piloto → ledger), e por fim a troca do pipeline de geração de spec para emissão Huma 3.1 nativa com trava de paridade. Fiber permanece v2.52.13 (o wrapper usa `humafiber.NewV2WithGroup`; **sem upgrade de framework**).

**Tech Stack:** Go 1.26.4 · Fiber v2.52.13 (mantido) · `github.com/LerianStudio/lib-commons/v5 v5.8.0` (`commons/net/http/openapi` + `commons/net/http/problem`) · `github.com/danielgtaylor/huma/v2 v2.38.0` (via `humafiber`) · OpenAPI 3.1.0 · `@redocly/cli join` (mantido) · `testify` + `app.Test` (não `humatest`).

## Phase Overview

| Phase | Milestone | Epics | Status |
|-------|-----------|-------|--------|
| 1 | Envelope RFC 9457 no runtime dos dois planos; golden test code→status verde antes e depois do swap; zero código/status alterado | 1.1, 1.2, 1.3 | **Complete** |
| 2 | Tracer 100% Huma: 31 ops re-tipadas, spec OAS 3.1 nativa, `Install()` nos dois paths, auth (Bearer+ApiKey) declarada; served body == spec | 2.1, 2.2, 2.3 | **Detailed** |
| 3 | Ledger 100% Huma: 115 ops re-tipadas, auth route-chain → Security por-op preservando granularidade resource/verb, `pkg.HTTPError` fundido | 3.1, 3.2, 3.3 | Epic-level |
| 4 | Pipeline 2-planos migrado para Huma 3.1 nativo, `redocly join` + guard preservados, identidade total de schema, paridade pristine verificada | 4.1, 4.2 | Epic-level |

---

## Decisões travadas (contexto para todos os implementadores)

1. **Shape canônico = `problem.Detail` (RFC 9457)**, não a Opção B minimalista. Decisão do Fred no checkpoint Wave 1a: adotar o lib-commons de verdade (terceiro rail), não um alias de doc.
2. **Paridade = identidade total**, incluindo `@name` unificado (`Error` nos dois planos). Não basta mesmo conjunto de campos.
3. **Branch = direto na `feat/monorepo-consolidation`** no `../midaz`. Não é `main`/`master`; consentimento explícito dado.
4. **Money path é terceiro rail:** o envelope pode mudar de **shape**; **códigos, semântica e status HTTP não**. Toda mudança que toca `pkg/errors.go`, `pkg/constant/errors.go`, `pkg/net/http/errors.go` passa pelo golden test.
5. **Motor de execução:** **um workflow `ring:dispatching-workflows` por fase**, com reviewers **e fixers embedded** (self-heal: fix → re-review dentro da própria onda), usando o conhecimento da skill `ring:adopting-lib-commons-huma-wrapper` (modo migração-de-swaggo; Gate 6 "deletar wrapper local" pulado — não existe wrapper Huma local hoje) como disciplina de implementação. Rolling wave: uma fase detalhada por vez; checkpoint com Fred entre fases.

### Decisão aprovada — Scrub de 5xx (ACEITO)

Adotar `problem.Install()` faz respostas 500/503 terem `title`→`"Internal Server Error"` e `detail`→`"internal error"` (texto sanitizado). **`code` e `status` sobrevivem verbatim** — invariante money-path mantida —; muda só o **texto** dos corpos 5xx. É fechamento deliberado de vazamento de causa interna e o comportamento canônico do lib-commons. **Fred aprovou 100% em 2026-06-30.** O golden test afere só code+status, então continua verde; asserção de texto em 5xx iria — corretamente — a vermelho (não a incluímos).

---

## Fase 1 — Fundação do envelope de erro (money-path)

**Milestone:** Os dois planos passam a emitir corpos de erro no shape RFC 9457 em runtime. O golden test varre toda a tabela code→status e está **verde antes e depois** do swap. Nenhum código, semântica ou status HTTP muda. Handlers **não** são tocados nesta fase.

**Nota de estado interino (bounded, documentada):** ao fim da Fase 1 o runtime emite `problem+json` mas as specs ainda são swaggo (shape antigo). Essa divergência runtime↔spec é intra-branch, não-liberada, e é resolvida por-plano nas Fases 2–3 quando a geração Huma 3.1 entra. O golden test guarda as invariantes money-path durante todo o interino. Este staging é explicitamente endossado pela mecânica do `MapError` (retorna um `*Detail` concreto, independente do transporte Huma).

### ✅ Fase 1 CONCLUÍDA (2026-07-01)

Executada em duas ondas `ring:dispatching-workflows`. **PASS** verificado pelo supervisor:
- **Onda 1** (golden + swap + mmodel): commits `4793eca44` (golden net code→status), `270bc45ef` (swap `WithError`→`problem`/RFC 9457), `7ca117f9a` (`mmodel.Error`), `e9ce63c24` (guarda do braço `libCommons.Response` no golden).
- **Onda 2** (fix-wave de reconciliação): o flip de envelope deixou 264 testes de handler vermelhos (todos shape-drift, zero regressão de code/status). Reconciliados pro RFC 9457 preservando code+status byte-a-byte: commits `88ab870aa` (ledger, 128 subtests) + `b5cf781d8` (tracer, 46). Auditoria de diff: 0 linhas removendo asserção de code/status.
- **Gate independente do supervisor:** `go test ./...` verde, golden net verde (`-count=10 -race` limpo), só `_test.go` tocado na onda 2.

**Lição de harness (aplicada às fases seguintes):** contrarians são **read-only** (proibido mutar fonte no worktree compartilhado — foi o que gerou um falso 409→400 na onda 1) e reviewers **rodam a suíte downstream inteira** (não só o diff).

**⚠️ RESÍDUO bounded → fecha na Fase 3:** existe um **terceiro** caminho produtor de status que a análise r3 não pegou: `pkg/net/http/withBody.go → response.BadRequest()` (JSON cru), usado pela **validação de campo do decode wrapper** dos handlers `WithBody`. Ele NÃO foi trocado na Fase 1, então ~7 respostas 400 de validação de campo (holder/instrument CreateXxx, códigos 0009/0050/0051) ainda emitem o envelope legado `{code,title,message,fields}`. **Não é money-path** (code+status intactos) e **se aposenta sozinho na Fase 3**, quando os handlers ledger migram pra Huma e o `withBody.go` decode wrapper é substituído por In-structs tipadas + 400/422 nativos do Huma. Não swapar agora = evitar trabalho jogado fora + não abrir superfície money-path nova sem golden. **A Fase 3 (Epic 3.1) deve fechar isto explicitamente e o golden/parity deve verificar.**

### Epic 1.1: Golden test da tabela code→status (a rede money-path)

**Goal:** Um teste auto-gerador que varre todos os sentinels de `pkg/constant/errors.go` + os caminhos de helper + os dois braços de status explícito, afere `(code, status)` e está verde contra o `WithError` atual.
**Scope:** `pkg/net/http/` (novo `errors_golden_test.go`), leitura de `pkg/errors.go` + `pkg/constant/errors.go`.
**Dependencies:** nenhuma.
**Done when:** teste verde no código atual; prova de RED deliberado documentada (perturbar um braço → vermelho → reverter); cobre os 3 casos ambíguos (0485→405, 0143→413, 0094→status 94).
**Status:** Pending

#### Task 1.1.1: Golden test auto-gerador sobre toda a tabela code→status

- [ ] Done

**Context:** Os testes de contrato existentes (`components/ledger/internal/adapters/http/in/{crm,fee,mainline}_error_contract_test.go` = 45 casos escolhidos a dedo, `tracer_error_contract_test.go` = 24) são spot-checks, **não** varredura. A tabela real vive em `WithError` (`pkg/net/http/errors.go:26-111`) + os helpers de `pkg/net/http/response.go`, e o status é função do **tipo Go** escolhido em `ValidateBusinessError` (`pkg/errors.go:391`), nunca do código numérico. Há **dois** dispatchers de status: `WithError` e `CanonicalFiberErrorHandler`/`renderCanonical` (`pkg/net/http/fiber_error_handler.go:28-75`), este último emite 405/413 com status explícito.

**Implementation vision:** Criar `pkg/net/http/errors_golden_test.go` (package `http`). Estratégia auto-gerada (drift-proof): enumerar todos os sentinels `constant.Err*` exportados de `pkg/constant/errors.go`; para cada um, chamar `pkg.ValidateBusinessError(sentinel, entityType)` e dirigir o erro pelo dispatcher real (`http.WithError`) via um `fiber.New()` + `app.Test(httptest.NewRequest(...))`; aferir **apenas** `resp.StatusCode` (invariante #1) e `body["code"]` (invariante #2). Aferir só code+status faz o mesmo teste ficar verde antes (`{code,title,message}`) e depois (`{code,title,detail,type,...}`) do swap. Derivar o status esperado classificando o tipo Go retornado com a mesma cascata `errors.As` da tabela §1 do `r3-moneypath-swap-spec.md` — o classificador do teste É o `statusOf` congelado que a produção terá de casar. Unir três fontes: (a) todo o mapa `ValidateBusinessError`; (b) os ~18 sentinels definidos-mas-não-mapeados alcançados por `ValidateInternalError`/`ValidateBadRequestFieldsError`/`ValidateUnmarshallingError` (aferir code+status explicitamente, incluindo 0094→94 via `strconv.Atoi`); (c) os dois braços de status explícito — dirigir `*fiber.Error{Code:405}` e `{Code:413}` por `CanonicalFiberErrorHandler` e travar 0485→405, 0143→413. Casos-limite nomeados: `FailedPreconditionError`→**500** (não 412); `ResponseError`/0094→status 94 (status-in-Code, ramo próprio); fallthrough sem match→500/code 0046.

**Files:**
- Create: `pkg/net/http/errors_golden_test.go`
- Test: o próprio arquivo (é o teste)

**Verification:** `go test ./pkg/net/http/ -run TestErrorEnvelopeGolden -v` — verde no código atual, todos os sentinels varridos. Prova de teeth: perturbar temporariamente um braço (ex.: `0007`→400) e confirmar vermelho, depois reverter.

**Done when:** o teste enumera todos os ~585 códigos mapeados + os 18 não-mapeados + os 2 de status explícito, está verde no `WithError` atual, e a prova de RED deliberado está no PR.

### Epic 1.2: Swap do dispatcher `WithError` → `problem` (shape RFC 9457)

**Goal:** `WithError` passa a construir e serializar um `*Detail` (RFC 9457 + superset `entityType`) preservando code+status; ambos os planos emitem `application/problem+json`. Assinatura de `WithError` inalterada (só as entranhas mudam).
**Scope:** `pkg/net/http/` (`errors.go`, novo `problem` wiring, `Detail` superset), dependência `commons/net/http/problem`.
**Dependencies:** Epic 1.1 (o golden test tem de existir e estar verde antes).
**Done when:** golden test **continua verde** pós-swap; corpos <500 = `{type,title,status,detail,code,entityType?,errors?}`; 5xx sanitizados (pendente ack); os dois braços de status explícito preservados fora da tabela; `ResponseError`/0094 tratado em ramo próprio.
**Status:** Pending

*(Tasks detalhadas na entrada em execução; visão travada em `r3-moneypath-swap-spec.md §2`: `codeOf` cascata sobre os 11 tipos na mesma ordem de declaração; `statusOf` = mapa `codeStatus` congelado construído do `ValidateBusinessError` (Opção (i) do r3 — reusa a lib, terceiro rail); `fallbackCode = "0046"`; `message`→`detail`; `entityType` via superset local `Detail{ problem.Detail; EntityType string }`; `fields`→`Errors[]huma.ErrorDetail` com `Location` congelado; os braços 405/413 do `renderCanonical` mantidos como override de status explícito.)*

### Epic 1.3: Limpeza do `mmodel.Error` + política de tags (preparação da paridade)

**Goal:** `mmodel.Error` deixa de emitir schema sujo (exemplo fictício + descrição corrompida) e ganha a política de `validate` que fará o `required[]` bater com o do tracer.
**Scope:** `pkg/mmodel/error.go` (só tags/doc, sem mudança de campo nem de runtime).
**Dependencies:** nenhuma (pode rodar paralela à 1.1/1.2).
**Done when:** exemplo do `code` = `"0147"` (código real); `@Description` limpa; `code`/`title`/`message` com `validate:"required"`, `entityType`/`fields` opcionais; sem mudança de campo ou de comportamento de runtime.
**Status:** Pending

*(Visão em `r3-moneypath-swap-spec.md §4.1/§4.2`. O fold efetivo do `pkg.HTTPError` e a unificação de `@name` são doc/spec e aterrissam com a geração Huma nas Fases 2–3, mas a política de tags é travada aqui.)*

---

## Fase 2 — Tracer → Huma (piloto)

**Milestone:** O plano tracer (31 ops, 8 arquivos) está 100% em Huma: assinaturas re-tipadas, `openapi.New` + `problem.Install()` nos paths de runtime **e** de spec-gen, schemas de segurança Bearer+ApiKey declarados com requirement por-op, spec OAS 3.1 nativa servida por `ServeSpec` (gated em `Swagger.Enabled`). O corpo servido bate com a spec committada (ambos `problem+json`). Piloto valida o padrão inteiro no plano menor antes do ledger.

**Por que tracer primeiro:** menor superfície (31 vs 115), auth mais simples de modelar (Bearer primário + X-API-Key fallback, confirmado em `auth_guard.go`), e é o plano cujo swagger estava mais divergente — validar aqui de-risca o ledger.

**Superfície real (recon `scratchpad/r4-tracer-phase2-surface.md`):** os **31 ops = 28 protegidos `/v1` + 3 públicos** (health/version/readyz). Só os **28 protegidos** migram pra Huma; os 3 públicos ficam rotas fiber cruas fora do group Huma (health é text/plain, os outros são probes de K8s pré-`/v1`). Registro hoje é **inline em `NewRoutes`** (`routes.go:335-401`): `api.Post("/rules", guard.With("rules","post",false), h.CreateRule)`. Handlers todos `func(c *fiber.Ctx) error`; path via `c.Params`, body via `c.BodyParser`, query via `c.QueryParser`; **nenhum handler lê header direto** (auth/tenant no middleware).

**Decisões travadas (recon + investigação do adapter humafiber):**
1. **ctx threading é NÃO-problema — sem middleware-ponte.** `humafiber_v2.go:207-214` constrói o ctx do handler Huma a partir de `c.UserContext()` (+ copia fasthttp locals via `VisitUserValuesAll`). O tenant MW injeta tenant-id/`*sql.DB` em `c.UserContext()` via `tmcore` (`routes.go:268-333`) → chega intacto no handler Huma. **Única condição de fiação:** as 28 ops Huma montam no MESMO group `f.Group("/v1")` (`routes.go:259`) que carrega o tenant MW, pra a ordem middleware→handler se manter.
2. **Auth continua MIDDLEWARE fiber.** `guard.With(resource, verb, forceAPIKey)` (`auth_guard.go:108`) permanece como middleware fiber na rota/group; o `Security` por-op do Huma é **só spec**. Preservar: Bearer-primeiro-depois-APIkey (`Protect:85`), o gate 401 `UNAUTHORIZED_MISSING_SUB` quando falta `sub` (`auth_guard.go:163-172`), a tupla `(resource,method)`, e o `forceAPIKeyAuth=true` **só** em `POST /v1/validations` (`routes.go:370`, `cfg.APIKeyOnlyValidation`).
3. **Validação imperativa FICA no service/handler.** `ListTransactionValidations.Validate()` (`transaction_validation_handler.go:256-291`: cursor/data/UUID) e `Validate.NormalizeAndValidate(now)` (clock-based) **não viram struct-tag** — traduzir mudaria o 422. Struct-tags Huma só onde a validação de campo já produz 400/422 hoje (**não introduzir 422 novo** — muda o contrato de erro).
4. **Registro extraído pra funcs por-arquivo** `RegisterRuleRoutes(api huma.API, h *Handler)` etc., pra os arquivos ficarem independentes (habilita fan-out paralelo com worktree isolation). `NewRoutes` passa a chamar cada `RegisterXxxRoutes`.

### Epic 2.1: Bootstrap Huma + handler de referência (de-risk do padrão)
**Goal:** `openapi.New(app, group, cfg)` monta a API Huma sobre o `*fiber.App` do tracer no group `/v1`; `problem.Install()` roda **antes** de qualquer `huma.Register` (runtime **e** spec-gen); UM handler de referência (rule CreateRule + GetRule) migrado ponta-a-ponta estabelece o padrão (In/Out structs, func `RegisterRuleRoutes`, auth middleware preservado, ctx threading verificado, teste via `app.Test`).
**Scope:** `components/tracer/cmd/app/main.go`, `components/tracer/internal/adapters/http/in/routes.go`, `rule_handler.go` (parcial), bootstrap.
**Dependencies:** Fase 1 completa. ✅
**Done when:** app tracer sobe com Huma montado no `/v1`; `Install()` provado nos 2 paths; CreateRule+GetRule respondem via Huma com body idêntico ao atual (`problem+json` no erro), auth middleware inalterado, tenant/DB lido do ctx OK; teste `app.Test` verde; padrão de registro por-arquivo documentado no PR pros demais handlers seguirem.
**Status:** Complete

#### Task 2.1.1: Montar Huma no `/v1` + `problem.Install()` + migrar CreateRule/GetRule como referência
- [ ] Done

**Context:** `NewRoutes` (`routes.go:165`) cria o `*fiber.App` (`fiber.New` `:186`, `ErrorHandler: CanonicalFiberErrorHandler`) e registra as rotas inline no group `api := f.Group("/v1")` (`:259`). O tenant MW roda no group (`:268-333`). O erro já é RFC 9457 (Fase 1). lib-commons expõe `openapi.New(app *fiber.App, group fiber.Router, cfg Config) huma.API` (wraps `humafiber.NewV2WithGroup`), `openapi.ServeSpec(app, api, logger, prefix, title)`, `problem.Install()`. `huma/v2 v2.38.0` + `lib-commons v5.8.0` já no `go.mod`.

**Implementation vision:** Chamar `problem.Install()` uma vez no bootstrap antes de qualquer Register. Criar a `huma.API` via `openapi.New(f, api, cfg)` passando o group `/v1` (garante que o tenant MW roda antes do handler Huma — ver decisão #1). Extrair `RegisterRuleRoutes(hAPI huma.API, h *Handler)` e migrar CreateRule (`rule_handler.go:70`, POST /rules, body `CreateRuleInput`+`Validate()`, 201 `model.Rule`) e GetRule (`:193`, GET /rules/:id, 200) pra assinatura Huma `func(ctx, *In)(*Out,err)` — In com `Body CreateRuleInput` + tags (só as constraints que já existem), path param `ID string \`path:"id"\``; Out com `Body model.Rule` + `Status`. Manter `guard.With("rules",verb,false)` como middleware fiber na rota (auth = middleware, Security do Huma é spec — decisão #2). Verificar que o handler lê tenant/DB do ctx (decisão #1). As outras 6 rotas de rule ficam inline no estilo antigo por enquanto (migradas na Epic 2.2).

**Files:**
- Modify: `components/tracer/internal/adapters/http/in/routes.go`, `rule_handler.go`, `components/tracer/cmd/app/main.go`, bootstrap (onde `problem.Install()` mora)
- Test: `rule_handler_test.go` (ou novo `*_huma_test.go` via `app.Test`)

**Verification:** `go -C <wt> build -buildvcs=false ./...`; `go -C <wt> test -buildvcs=false ./components/tracer/internal/adapters/http/in/ -run Rule`; probe manual/teste: CreateRule 201 + GetRule 200 com tenant resolvido do ctx; erro de validação mantém code+status+shape RFC 9457.

**Done when:** Huma montado, `Install()` nos 2 paths, CreateRule+GetRule via Huma verdes, ctx threading confirmado, padrão de `RegisterXxxRoutes` documentado.

### ✅ Fase 2a CONCLUÍDA (2026-07-01) — PASS (Epic 2.1)

Huma montado no `/v1` do tracer; `problem.Install()` no path de runtime (spec-gen ainda não existe — ver obrigação diferida); CreateRule+GetRule migrados ponta-a-ponta como referência. Commits: `5d3a4307e` (bootstrap + 2 handlers + `RegisterRuleRoutes`), `edccf8d65` (drop `format:"uuid"` do path param → preserva 400/0065 canônico, não o 422 nativo da Huma), `f5119b6a7` (deflake do harness + guard de `$schema` ausente + claim de byte-identidade honesta).

**Gate do supervisor (PASS):** golden net money-path verde (`pkg/net/http` ok), pacote `http/in` verde em `-count=3` single-process, tree limpa, escopo confirmado. **Padrão de referência** no header de `rule_handler_huma.go`: In/Out com `RawBody []byte`+`contentType` + `SkipValidateBody:true` (malformed/validação de campo caem no `json.Unmarshal`+`Validate()` imperativo → code canônico, sem 422 novo); core transport-agnóstico compartilhado com o método Fiber legado; erro via `humaProblem(err)` → `*pkgHTTP.Detail` (satisfaz `huma.StatusError`+`ContentTypeFilter` → `problem+json`); `RegisterXxxRoutes` por-arquivo; auth `guard.With` como middleware fiber; ctx via `c.UserContext()` do adapter humafiber, sem ponte.

**Correção de claim (relevante pro fan-out):** o body de sucesso NÃO é byte-idêntico ao Fiber — diverge por um `\n` final (Huma usa `json.Encoder.Encode`) e HTML-escaping (`SetEscapeHTML(false)` vs default Fiber). Invisível pra qualquer parser JSON (ambos decodificam idêntico, incl. o SDK gerado); só consumidor de byte-cru/hash/ETag observaria — esta API não tem nenhum. Garantia real = **field/status/code/type/entityType-identical**, guardada pelo golden net. NÃO alinhar encoders.

**Lição de harness (custou um falso High):** a "flakiness" reportada (422 fantasma intermitente) é quase certamente **artefato de worktree compartilhado** — `re:test-reviewer` perturbava a fonte (re-adicionava `format:"uuid"` pra verificar RED) DENTRO da janela em que `re:security-reviewer` rodava a suíte em laço no MESMO worktree → o 422 induzido foi mis-atribuído como flakiness. Não-reproduzível em 165+ runs, incluindo binário single-process com concorrência máxima (a condição real de corrida de estado global). `t.Parallel()` removido mesmo assim (zero benefício em testes <1s; fecha janela latente antes de copiar 28×). **Regra nova pro 2b:** READ-ONLY vale pra REVIEWERS também, não só contrarians — verificação de RED não pode perturbar o worktree compartilhado (cópia fora do worktree, ou confiar no RED do implementador + spot-check determinístico do supervisor).

**Obrigação diferida:** `problem.Install()` antes do path de spec-gen só morde quando `ServeSpec`/emissão OAS 3.1 existir — Epic 2.3. Hoje o tracer serve swagger 2.0 (swaggo, `/swagger/*`), independente da Huma.

### Epic 2.2: Re-tipar os 26 handlers protegidos restantes (fan-out por arquivo)
**Goal:** Cada handler protegido `func(c *fiber.Ctx) error` vira `func(ctx, *In)(*Out, error)` com I/O tipada + `huma.Operation`, seguindo o padrão da Epic 2.1, agrupado por arquivo (`RegisterXxxRoutes`).
**Scope:** `rule_handler.go` (6 restantes), `limit_handler.go` (9), `transaction_validation_handler.go` (2), `validation_handler.go` (1), `reservation_handler.go` (5), `audit_event_handler.go` (3).
**Dependencies:** Epic 2.1 (padrão estabelecido). ✅
**Done when:** 26 ops via `huma.Register`; landmines endereçados — dual-status Validate 200/201 (`validation_handler.go:149-154`) via responses explícitas; UpdateLimit raw-body probe de campo imutável (`limit_handler.go:272-285`) preservado; 204 no-body deletes (`DeleteRule`/`DeleteLimit`) com Out vazio + `DefaultStatus:204`; list com query-param + validação imperativa sem 422 novo; reservation = 2 helpers `terminate`/`terminateByTransaction` → 4 shells. Testes `app.Test` (harness não-paralelo) verdes; golden net verde; sem 422 novo.
**Status:** Done — 2b-1 ✅ (rule, 8 ops); 2b-2 ✅ (20 ops + wiring). **28 ops protegidas do tracer em Huma.**

**Fatiamento (de-risk das sub-shapes que a 2a NÃO tocou):** a 2a provou body-POST + path-param + error seam + ctx + auth + harness. Os 26 restantes trazem: **list com query-param + validação imperativa** (risco 422, mesma classe do `format:"uuid"`), **204 no-body**, **dual-status** (Validate 201/200), **POST id-only** (actions), **413 guard** (Validate). Fanning out 26 numa shape de list não-validada = risco de retrabalho. Então divide em duas waves:
- **2b-1 (wave detalhada, gated — Task 2.2.1 abaixo):** completar `rule_handler.go` (6 ops) — exercita 4 sub-shapes novas (PATCH+body, list+query, POST id-only, 204) num arquivo só, vira o exemplar completo + prova o wiring em `routes.go`.
- **2b-2 (epic-level, detalhada após o gate da 2b-1):** fan-out paralelo dos outros 5 arquivos, cada um copiando a sub-shape provada; + task serial de integração (Epic 2.3).

#### Task 2.2.1: Completar `rule_handler.go` em Huma — UpdateRule, ListRules, Activate/Deactivate/Draft, DeleteRule
- [x] Done — commits `28ddb4ee5` (6 ops) + `312847d60` (present-but-empty self-heal) + `c9f89b2a5` (repeated-key last-wins). Gate PASS.

**Context:** A 2a migrou CreateRule (`rule_handler.go:70`) e GetRule (`:193`) e criou `rule_handler_huma.go` (padrão + `RegisterRuleRoutes`) + `rule_handler_huma_test.go` (harness `buildHumaRuleApp`, NÃO-paralelo). As 6 ops restantes ainda estão inline fiber em `NewRoutes` (`routes.go`). Como é UM arquivo (não fan-out), o wiring em `routes.go` pode ser feito direto aqui — isso prova o padrão de wiring (list/action/delete + guard.With) antes do fan-out 2b-2.

**Implementation vision:**
- **UpdateRule** (PATCH `/rules/{id}`, body `UpdateRuleInput`+`Validate()`/`IsEmpty()`, 200 `model.Rule`): igual CreateRule mas PATCH + path param `ID`. `RawBody []byte`+`SkipValidateBody:true`; core `updateRule(ctx, id, rawBody)` compartilhado com o método Fiber legado.
- **ListRules** (GET `/rules`, query `ListRulesInput`+`Validate()`/`SetDefaults()`, 200 `ListRulesResponse`): **sub-shape nova (query-param).** In struct com os campos de query taggeados `query:"..."` MAS **sem tags de validação** (min/max/enum/required) — senão Huma emite 422 nativo antes do handler. Preferir tipos que Huma coage sem 422 (string; se `int`/`bool` forçar tipo, aceitar e validar imperativamente no core). O core `listRules` roda o equivalente ao `QueryParser` + `input.Validate()`/`SetDefaults()` imperativo → 400 canônico em erro. Out = `ListRulesResponse` (cursor DTO próprio) serializado verbatim. **Teste obrigatório:** query param inválido (ex. cursor malformado / limit fora de range) → 400 canônico, NÃO 422 nativo.
- **ActivateRule/DeactivateRule/DraftRule** (POST `/rules/{id}/activate|deactivate|draft`, id-only, sem body, 200 `model.Rule`): In só com path param `ID` (SEM `RawBody`, não há body → sem `SkipValidateBody`); Out `model.Rule` 200. Core compartilhado por ação.
- **DeleteRule** (DELETE `/rules/{id}`, 204 no-body): Out **sem campo `Body`** + `huma.Operation{DefaultStatus:204}`; core `deleteRule(ctx, id)`. Confirmar que Huma emite 204 sem corpo (não um 200 com `null`).
- Cada op: core transport-agnóstico em `rule_handler.go` (extrair do método Fiber atual, mantendo o Fiber como wrapper fino pros testes diretos existentes) + método `XxxHuma` em `rule_handler_huma.go` + entrada no `RegisterRuleRoutes`. Em `routes.go`, trocar as 6 rotas inline por `api.<Verb>(path, guard.With("rules",verb,false))` (middleware-only) — o `RegisterRuleRoutes` já registra o handler Huma no mesmo path/verb. Preservar a tupla `(resource="rules", verb, forceAPIKey=false)` byte-a-byte.
- Testes no `buildHumaRuleApp` (NÃO-paralelo — ver lição 2a): por op, sucesso (status + campos via `json.Unmarshal` + tenant capturado + `$schema` ausente), erro canônico (code+400/status+`problem+json`, service não alcançado onde aplicável, **sem 422 novo**), e pro Delete o 204 sem corpo.

**Files:**
- Modify: `components/tracer/internal/adapters/http/in/rule_handler.go` (extrair cores), `rule_handler_huma.go` (In/Out + methods + `RegisterRuleRoutes` expandido), `routes.go` (6 rotas inline → guard.With middleware + registro Huma)
- Test: `components/tracer/internal/adapters/http/in/rule_handler_huma_test.go`

**Verification:** `go -C <wt> build -buildvcs=false ./...`; `go -C <wt> test -buildvcs=false -count=1 ./components/tracer/internal/adapters/http/in/` (verde); `go -C <wt> test -buildvcs=false -run TestGolden ./pkg/net/http/` (money path verde); teste de query-param inválido em ListRules → 400 canônico (prova zero-422 da sub-shape de list).

**Done when:** as 8 ops de `rule_handler.go` estão em Huma (2 da 2a + 6 aqui), wired em `routes.go` com auth preservada; 4 sub-shapes novas (PATCH+body, list+query sem 422, POST id-only, 204 no-body) provadas por teste; harness não-paralelo; golden net verde.

##### Gate 2b-1 (2026-07-01) — ISSUES→fix wave, depois PASS

Workflow `wf_37e2511c-823` rodou o harness completo (implement→5 reviewers→4 contrarian→self-heal 5+4). Resultado e adjudicação:
- **Implement `28ddb4ee5`:** 6 ops migradas; 4 sub-shapes provadas. Zero-422 confirmado (query = string sem tags de validação; body = RawBody+SkipValidateBody; path sem `format`); DeleteRule 204 bodiless verificado no mecanismo (huma.go `outBodyIndex==-1`).
- **Contrarian LENS 3 REFUTOU** o claim de paridade de query → defeito money-path REAL: query param **present-but-empty** (`?status=`, `?limit=`) colapsava pra `nil` → Huma devolvia 200 onde Fiber devolve 400 canônico (0082/0331). Mesma classe do `format:"uuid"` da 2a. **Self-heal `312847d60`** corrigiu na raiz: `ListRulesInputHuma` implementa `huma.Resolver` (captura `ctx.URL().Query()`, retorna nil→sem 422) + `bindListRulesInput` usa `url.Values.Has` pra reproduzir present-vs-absent do Fiber.
- **High remanescente (re:nil-reviewer) → fix wave `a…`:** `bindListRulesInput` lê `url.Values.Get` (**first**-wins) mas o gorilla-schema do Fiber é **last**-wins → chaves repetidas (`?status=ACTIVE&status=garbage`) FLIPAM status/code (400↔200). Terceiro-trilho (identidade byte-a-byte Fiber→Huma) + este arquivo é o **exemplar copiado 5× no 2b-2** → fechar aqui. Fix = helper `last(key)` lendo `q[key][len-1]` em todos os call sites; `q.Has` fica pro gating de presença. Irmão direto do bug present-but-empty; last-wins subsume o fix anterior.

##### Fix wave 2b-1 — repeated-key last-wins parity ✅ PASS (`c9f89b2a5`)
- [x] Done — helper `last(key)` (`vs[len-1]`, nil-safe) em todos os call sites; `q.Has` mantido pro gating. Teste `TestHuma_ListRules_RepeatedKeyParity` prova o flip nos 2 sentidos (status/limit/sort_by→400 canônico; `?limit=101&limit=25`→200 limit=25). Suites verdes (http/in 4.751s, golden 0.471s) verificados pelo supervisor independentemente. **Gate 2b-1 = PASS. Task 2.2.1 completa (8 ops rule em Huma). Exemplar limpo pro fan-out.**

**Context:** `bindListRulesInput` (`rule_handler_huma.go:188-250`) usa `q.Get`/`optStr` (first value) pra `status`(221)/`action`(226)/`limit`(231)/`sort_by`(217)/`sort_order`(218)/`cursor`(216) + scope fields (209-215 via optStr:204). Fiber v2.52.13 QueryParser (gorilla-schema) é last-wins. Divergência confirmada empiricamente por 3+ agentes contra o fiber real.

**Implementation vision:** Introduzir `last := func(key string) string { vs := q[key]; if len(vs)==0 { return "" }; return vs[len(vs)-1] }` e trocar TODO `q.Get(key)`→`last(key)` (inclusive dentro de `optStr`). `q.Has` permanece pro gating present-vs-absent (Has-true garante slice não-vazio → `vs[len-1]` seguro). Last-wins subsume o present-but-empty: `?status=A&status=`→last=""→0082; `?status=&status=A`→last="A"→200 (ambos Fiber-idênticos). NENHUM 422 novo (Resolve segue retornando nil; validação segue imperativa).

**Files:**
- Modify: `components/tracer/internal/adapters/http/in/rule_handler_huma.go` (binder → last-wins)
- Test: `components/tracer/internal/adapters/http/in/rule_handler_huma_test.go` (parity test de chave repetida)

**Verification:** RED = teste `?status=ACTIVE&status=garbage`→400/0082 e `?limit=101&limit=25`→200 FALHA no first-wins atual; GREEN passa no last-wins. `go test -buildvcs=false -count=1 ./components/tracer/internal/adapters/http/in/` verde; `go test -buildvcs=false -run TestGolden ./pkg/net/http/` verde. Probe fiber empírico FORA do worktree cobrindo TODOS os campos afetados.

**Done when:** chaves repetidas produzem o mesmo code/status que o Fiber pra todos os campos de query; parity test no harness não-paralelo; golden net verde; exemplar limpo pro fan-out 2b-2.

#### 2b-2 — Fan-out dos 5 arquivos restantes (20 ops) + wiring (elaborado 2026-07-01 do recon-2b2)

**Divergências vs exemplar rule (recon confirmou):** cada arquivo tem SEU handler struct (`LimitHandler`/`TransactionValidationHandler`/`ValidationHandler`/`ReservationHandler`/`AuditEventHandler`), NÃO o `Handler` do rule → shells são métodos desses structs. NENHUM core extraído ainda → cada task EXTRAI o core (span+parse+Validate+service+log inline → core puro), deixa o método Fiber como wrapper fino, cria o shell Huma. `humaProblem` (`rule_handler_huma.go:482`) é package-level → reusar verbatim, não redefinir. Split `handleXxxServiceError` (render via `http.WithError` Fiber-bound) em `classifyXxxError` (sem render) + render no edge, como rule fez. **Binder de list (`last()`/`optStr`) copiado INLINE por arquivo — NÃO extrair helper compartilhado (serializaria as tasks de list).** Teste: `buildHumaXxxApp` copiado de `rule_handler_huma_test.go:102`, NÃO-PARALELO (`problem.Install()`+pools globais Huma), `tenantSpyService` pra ctx-threading.

Execução: **5 tasks de implement SERIAIS no worktree compartilhado** (disjuntas mas serializadas — compile/test/commit no mesmo tree colidiriam em paralelo; worktree-isolation forçaria merge-back frágil no money-path; correctness>tempo) + 1 task serial de integração. Review + contrarian paralelos (read-only).

##### Task 2.2.2: `limit_handler.go` → Huma (9 ops)
- [ ] Done
**Ops:** CreateLimit(POST /limits, body+Validate, 201), GetLimit(GET /limits/{id}, 200), ListLimits(GET /limits, 13 query, 200 — **Resolver+last-wins inline**), UpdateLimit(PATCH /limits/{id}, body+Validate+IsEmpty, 200 — **raw-body immutable probe**), Activate/Deactivate/Draft(POST /limits/{id}/{action}, id-only, 200), DeleteLimit(DELETE /limits/{id}, **204 no-body**), GetLimitUsage(GET /limits/{id}/usage, 200).
**Landmine raw-body (`:270-285`):** UpdateLimit faz `json.Unmarshal(body,&map)` e rejeita se `limitType`/`currency` presentes (`ErrLimitImmutableField`) ANTES de BodyParser. Core `updateLimit(ctx,id,rawBody)` preserva o map-probe verbatim; shell passa `in.RawBody`. **204:** copiar `DeleteRuleOutputHuma struct{}`+`DefaultStatus:204`. **Query** (`ListLimitsInput`,`limit_validation.go:159`): dropar tags `enums:`, só `doc:`; `.Validate()`+`.SetDefaults()` imperativos.
**Files:** Modify `limit_handler.go`,`limit_validation.go`; Create `limit_handler_huma.go`,`limit_handler_huma_test.go`. NÃO tocar routes.go.
**Verification:** build+`go test -count=1 ./components/tracer/internal/adapters/http/in/` verde; golden net verde; teste query inválido→400 canônico (não 422), raw-body probe→erro imutável, repeated-key last-wins, 204 sem corpo.

##### Task 2.2.3: `transaction_validation_handler.go` → Huma (2 ops)
- [ ] Done
**Ops:** ListTransactionValidations(GET /validations, 12 query, 200 — **Resolver+last-wins**, só `limit` é pointer), GetTransactionValidation(GET /validations/{id}, 200).
**Files:** Modify `transaction_validation_handler.go`,`transaction_validation_validation.go` (se o binder morar lá); Create `_huma.go`,`_huma_test.go`.
**Verification:** idem 2.2.2 (list sem 422, repeated-key).

##### Task 2.2.4: `validation_handler.go` → Huma (1 op)
- [ ] Done
**Op:** Validate(POST /validations, body `ValidationRequest`+`NormalizeAndValidate(now)`, **dual-status 200/201**).
**Landmine dual-status (`:149-154`):** `IsDuplicate`→200, senão 201. Out carrega `Status int` que o shell define via `IsDuplicate`; `huma.Register` declara responses 200+201 explícitas, SEM `DefaultStatus` fixo. Core `validate(ctx,rawBody)(*ValidateResult,error)`. **Landmine payload-size (`:90`):** `len(body)>maxPayloadSize`(100KB,`:30`)→`ErrPayloadTooLarge` — checar no core sobre `len(rawBody)`. `handleValidationError`(`:158`) é método → core também método. forceAPIKey (`cfg.APIKeyOnlyValidation`) é do wiring (Task 2.2.7), não do shell.
**Files:** Modify `validation_handler.go`; Create `_huma.go`,`_huma_test.go`.
**Verification:** build+suite+golden; teste dual-status (duplicate→200, novo→201), payload >100KB→ErrPayloadTooLarge (não 422 nativo).

##### Task 2.2.5: `reservation_handler.go` → Huma (5 ops)
- [ ] Done
**Ops:** Reserve(POST /reservations, body+`NormalizeAndReserveValidate`, 201, payload-size), Confirm(POST /reservations/{id}/confirm, 200), Release(POST /reservations/{id}/release, 200), ConfirmByTransaction(POST /reservations/transaction/{transaction_id}/confirm, 200), ReleaseByTransaction(.../release, 200).
**Landmine 2 helpers→4 shells:** os 4 lifecycle ops delegam a `h.terminate(...)`(`:279`, param `id`) / `h.terminateByTransaction(...)`(`:234`, param `transaction_id`) com `(operation,terminalStatus,action)`. Refatorar em cores puros `terminate(ctx,id,op,status,action)(*ReservationActionResponse,error)` e `terminateByTransaction(ctx,txID,...)(*TransactionActionResponse,error)`. 4 shells explícitos: Confirm→terminate+StatusConfirmed+service.Confirm; Release→terminate+StatusReleased+service.Release; ConfirmByTransaction→terminateByTx+StatusConfirmed; ReleaseByTransaction→terminateByTx+StatusReleased. Ops 4-5: In com `TransactionID string \`path:"transaction_id"\``.
**Files:** Modify `reservation_handler.go`,`reservation_validation.go`; Create `_huma.go`,`_huma_test.go`.
**Verification:** build+suite+golden; teste os 4 lifecycle + Reserve (payload-size), path param transaction_id resolve.

##### Task 2.2.6: `audit_event_handler.go` → Huma (3 ops)
- [ ] Done
**Ops:** ListAuditEvents(GET /audit-events, **18 query — o mais pesado, Resolver+last-wins**), GetAuditEvent(GET /audit-events/{id}, 200), VerifyHashChain(GET /audit-events/{id}/verify, 200).
**Query** (`ListAuditEventsInput`,`audit_event_validation.go:159`): typed pointers (`*AuditEventType`/`*AuditAction`/`*AuditResult`/`*ResourceType`/`*ActorType`) + strings + `limit`(`*int`); dropar `validate:`/`enums:`, só `doc:`; binder converte string→typed pointer via `q.Has`+`last`. `.Validate()`+`.SetDefaults()`+`toAuditEventFilters`(retorna error).
**Files:** Modify `audit_event_handler.go`,`audit_event_validation.go`; Create `_huma.go`,`_huma_test.go`.
**Verification:** idem list (sem 422, repeated-key, typed-pointer binding).

##### Task 2.2.7: Integração — wiring routes.go (serial, após 2.2.2–2.2.6)
- [ ] Done
**Context:** trocar as 20 rotas inline (`routes.go:371-431`) por middleware-only + 5 `RegisterXxxRoutes`, espelhando o padrão rule (`:359-367`). /v1 group `:261`, huma.API mount `:345` (após `problem.Install():343`), tenant MW `:270-335` roda antes → cobre Huma.
**Preservar byte-a-byte:** tuplas `guard.With(resource,verb,forceAPIKey)` exatas; **reservation = DOIS middlewares** (`resTenantMW`+`guard.With`, ordem transaction-first `:421-424` antes de `:id`); **validation = `cfg.APIKeyOnlyValidation`** como 3º arg (NÃO literal `true`). Rotas Fiber middleware-only mantêm `:id`/`:transaction_id`; `RegisterXxxRoutes` usa `{id}`/`{transaction_id}`.
**Flat-401 (`pkgHTTP.Unauthorized`,`pkg/net/http/response.go:16`):** emite envelope FLAT `{code,title,message}`, não problem+json; auth é middleware pré-Huma → 401 nunca passa pelo Huma. **PRESERVAR (paridade) + documentar a divergência.** Unificar 401→problem+json é mudança de contrato em todo o tracer (blast radius alto) → decisão explícita do Fred, FORA desta wave.
**Files:** Modify `routes.go`.
**Verification:** build+`go test -count=1 ./components/tracer/internal/adapters/http/in/` verde; golden net verde; grep confirma 0 rotas inline dessas 5 resources restantes; auth tuplas idênticas ao pré-wiring (diff só troca handler terminal por middleware-only+Register).
**Done when (Epic 2.2):** 26 ops via huma.Register (8 rule+18 aqui... na verdade 20 aqui, 28 protegidas totais), landmines endereçadas, wired em routes.go com auth preservada, suites+golden verdes. Epic 2.3 (per-op Security spec+ServeSpec+Unauthorized decision) é wave seguinte.

##### Gate 2b-2 (2026-07-01) — PASS (Epic 2.2 completa)
Workflow `wf_f70f2f3a-4b0`: 6 implement serial → 5 reviewers → 5 contrarian, **PASS clean** (0 blocking, 0 refutado, 0 self-heal). Tasks 2.2.2–2.2.7 todas Done. Commits: `dffa59704`(limit 9), `ac663569a`(tx-val 2), `c51e2fba6`(validation 1), `3869eda67`(reservation 5), `5e1f711ef`(audit 3), `f4f2c0537`(wiring routes.go). Suites verdes verificadas por mim (build EXIT0, http/in 4.75s, golden 0.50s, vet limpo). Contrarians provaram empíricamente (fiber v2.52.13/huma v2.38.0): no-422 nos 3 lists + typed-pointer, landmines (probe→0380, dual-status 200/201, payload 100KB, reservation 4-shells), auth byte-a-byte, repeated-key last-wins, wiring completo sem órfãos.
**Nit `time_of_day.go` (meu catch, resolvido):** `dffa59704` appendou `MarshalText`/`UnmarshalText` (+32, 0 del) num arquivo pré-existente — faz o gerador OpenAPI do Huma renderizar `TimeOfDay` (campos unexported) como string `"HH:MM"` no schema de resposta de `model.Limit`; runtime byte-idêntico (encoding/json prefere MarshalJSON). Migration-necessário, não-breaking.
**Deferidos pra Epic 2.3 (nenhum bloqueia):** (1) `@Failure 413` stale em reserve/validate (mentira swaggo pré-existente — ErrPayloadTooLarge→code canônico, não 413; some quando swaggo aposenta); (2) `enums:` removido só de ListLimitsInput (metadata morto, spec vem de *InputHuma); (3) flat-401 decisão (preservado+documentado; unificar→problem+json é decisão do Fred fora da wave).

### Epic 2.3: Auth declarada (Bearer + ApiKey) + spec 3.1 nativa + paridade served==spec
**Goal:** Declarar os 2 security schemes no shape correto (`BearerAuth` `type:http,scheme:bearer` — não o hack apiKey-in-header do swagger atual; `ApiKeyAuth` `type:apiKey,in:header,name:X-API-Key`); requirement por-op nas 28 ops (`POST /validations` reflete o `forceAPIKeyAuth`); spec OAS 3.1 servida por `ServeSpec` (gated em `Swagger.Enabled`); remover `@securityDefinitions` swaggo do `main.go`.
**Scope:** `components/tracer/cmd/app/main.go`, `routes.go`, `ServeSpec` wiring, `components/tracer/api/` (artefatos gerados).
**Dependencies:** Epic 2.2.
**Done when:** spec anuncia os 2 schemes corretos (fecha o gap Bearer-faltando do recon §4); requirement por-op bate com o middleware; `POST /v1/validations` mostra ApiKey; served body == spec (ambos `problem+json`); `/openapi.yaml`+`/docs` gated; swagger 2.0 velho aposentado.
**Status:** Pending

---

## Fase 3 — Ledger → Huma

**Milestone:** O plano ledger (115 ops, 27 arquivos) está 100% em Huma, com o redesign de auth preservando a granularidade `resource/verb`, e o `pkg.HTTPError` fundido no envelope canônico. Padrão idêntico ao da Fase 2, em escala maior + a complexidade de auth do ledger.

### Epic 3.1: Bootstrap Huma do ledger + re-tipar 115 handlers
**Goal:** `New`+`Install` no ledger; 115 handlers re-tipados (39 usam hoje o form `DecodeHandlerFunc` via `http.WithBody` — perdem o wrapper de decode; 73 são fiber-plain). A maquinaria `WithBody`/`ParseUUIDPathParameters`/`WithBodyLimit`/`ProtectedRouteChain` de `pkg/net/http/` é substituída por input structs Huma + middleware.
**Scope:** `components/ledger/internal/adapters/http/in/` (27 arquivos), `components/ledger/internal/bootstrap/unified-server.go`, `pkg/net/http/` (aposentar decorators).
**Dependencies:** Fase 2 (padrão validado).
**Done when:** 115 ops via `huma.Register`; decorators aposentados sem regressão; build + `-race` verdes.
**Status:** Pending

#### ⚠️ Decisão de design a resolver na elaboração (não é port mecânico)
Auth do ledger hoje é `protectedMidaz(auth, resource, verb, ...)` por rota (`pkg/net/http/protected_routes.go:24`), com granularidade `auth.Authorize(midazName, resource, verb)`. Migrar para Huma exige `Security` por-operação **+ um middleware Huma que lê resource/verb e chama `auth.Authorize`**, preservando a granularidade. Perder essa granularidade seria **regressão de segurança** — o plano preserva explicitamente. A modelagem concreta (middleware único parametrizado vs metadata por-op) é decidida na entrada em execução da Epic 3.2.

### Epic 3.2: Redesign de auth do ledger (route-chain → Security por-op)
**Goal:** Substituir a cadeia `ProtectedRouteChain` por Security Huma por-op + middleware que preserva `auth.Authorize(resource, verb)`.
**Scope:** `pkg/net/http/protected_routes.go`, routes do ledger, middleware Huma.
**Dependencies:** Epic 3.1.
**Done when:** toda rota protegida mantém a mesma decisão de autorização resource/verb de hoje; nenhuma rota fica acidentalmente pública (verificado contra o mapa de rotas atual).
**Status:** Pending

### Epic 3.3: Fold do `pkg.HTTPError` no envelope canônico
**Goal:** Os 10 refs `@Failure {object} pkg.HTTPError` (`audit.go:86,87`; `encryption.go:36-40,129-131`) repontam para o schema unificado; o campo vazado `err:{}` some do contrato. O tipo Go `pkg.HTTPError` permanece (é erro tipado usado por clientes); só seu papel de DTO documentado acaba.
**Scope:** `components/ledger/internal/adapters/http/in/{audit,encryption}.go`.
**Dependencies:** Epic 3.1.
**Done when:** spec regenerada não tem mais `definitions."pkg.HTTPError"`; os 10 endpoints referenciam o único schema de erro; `err` não aparece no contrato.
**Status:** Pending

---

## Fase 4 — Pipeline de spec 2-planos + trava de paridade

**Milestone:** O pipeline de geração (`postman/generator/generate-docs.sh`) é migrado de swag→openapi-generator-Docker→3.0.1 para emissão Huma 3.1 nativa por plano; o `redocly join` + o guard de security-scheme sobrevivem; `check-docs.sh` é reformulado; as duas specs atingem **identidade total** de schema de erro (`@name Error` unificado, `required[]` idêntico). Entregável: OAS 3.1 pristine + paridade — o insumo do remodel do SDK.

### Epic 4.1: Trocar o pipeline de geração para Huma 3.1 nativo
**Goal:** Substituir os estágios `generate_openapi_spec`/`generate_openapi_yaml` por dump da spec Huma; preservar o `redocly join` (ledger-first) + o jq guard que assere `BearerAuth`+`ApiKeyAuth`; reformular `check-docs.sh` (lógica de drift).
**Scope:** `postman/generator/generate-docs.sh`, `postman/generator/check-docs.sh`, `components/*/api/`.
**Dependencies:** Fases 2 e 3.
**Done when:** `make generate-docs` produz `postman/specs/midaz.openapi.yaml` a partir das specs Huma 3.1 dos dois planos; guard verde; sem estágio Docker de conversão.
**Status:** Pending

### Epic 4.2: Trava de paridade + verificação pristine
**Goal:** Unificar `@name` do schema de erro para `Error` nos dois planos; `required[]` idêntico; nenhum outro envelope divergente; verificação final de que ledger e tracer emitem schema de erro byte-idêntico.
**Scope:** `components/tracer/api/types.go` (retirar `@name ErrorResponse`), refs `@Failure` do tracer, `components/tracer/internal/testutil/integration_helpers.go:770` (mirror duplicado).
**Dependencies:** Epic 4.1.
**Done when:** diff dos dois schemas de erro é vazio (nome, campos, required, facets); golden test money-path ainda verde; specs OAS 3.1 válidas; consumidor (SDK) tem um único shape de erro para gerar.
**Status:** Pending

---

## Self-review (checklist do ring:writing-plans)

- **Cobertura da spec:** os 5 buracos dos juízes da Wave 1a estão cobertos — auth mal-declarado (Epics 2.3/3.2), `pkg.HTTPError` (Epic 3.3), `required[]` divergente (Epic 1.3/4.2), `mmodel.Error` sujo (Epic 1.3), nome de schema divergente (Epic 4.2). ✅
- **Money path:** front-loaded na Fase 1 com golden test RED-first; invariante code+status guardada em todas as fases. ✅
- **Fronteiras de fase:** cada fase termina buildando + testável (Fase 1 com estado interino spec-drift documentado e guardado). ✅
- **Consistência de contrato:** o envelope canônico (`Detail` = `problem.Detail` + `entityType`) é definido uma vez (Fase 1) e referenciado por todas as fases. ✅
- **Sem tasks vagas na wave detalhada:** Fase 1 detalhada com file:line e edge cases nomeados; Fases 2–4 em epic-level (rolling wave). ✅
- **Snippets:** evitados no plano; a mecânica concreta vive nos relatórios `r{1,3}-*.md` referenciados. ✅
