# Anchored OSS — Plano de Lançamento v1.0

> Status: draft de planejamento (2026-05-28). Decisões fixadas: embeddings ONNX local (mesmo modelo do CLI, 384d) + Postgres/pgvector, provedores externos pluggáveis; v1.0 = pacote completo (Fases 0–5); install.sh default Postgres.

## Visão do produto

Times compartilham memórias do **mesmo projeto** em vez de markdown (que consome tokens e é menos eficiente que o Anchored). Cada dev tem memória **local + remota** sincronizando automaticamente, **exceto** memórias sensíveis e pessoais (scope=user). O `anchored_oss` é o servidor self-hosted que recebe esse sync e adiciona, por cima: busca semântica, grafo de conhecimento, auditoria, guardrails customizáveis, curadoria por workers e um **chat de IA opcional** (pré-contexto, não o objetivo central).

- Repo `anchored_oss`: **privado** (fonte do servidor).
- Repo `../anchored-oss` (`github.com/jholhewres/anchored-oss`): **público**, só distribuição (README, VERSION.md, install.sh). `curl | sh` baixa o binário e sobe com pm2.

---

## Estado atual (resumo do review crítico)

**Sólido:** arquitetura handler/store/middleware/policy, queries parametrizadas, auth API key sha256, shutdown graceful, migrations com advisory lock, audit parametrizado, curation worker (5s/batch 100/Jaccard near-dup), sync filtrando scope=user/secrets/paths, release multi-arch (linux/darwin/windows × amd64/arm64), design system próprio maduro, grafo SVG force-directed.

**Lacunas que o plano fecha:**
- 3 `panic()` em geração de ID; zero rate limiting; audit sem retenção.
- **Sem embeddings no servidor** → busca é substring (bloqueador-raiz).
- **Sem abstração de LLM/chat.**
- **Guardrails hardcoded** (`policy/filter.go:49`), sem config por-org nem CRUD admin.
- Frontend: busca semântica inexistente, grafo trava em 200 triples, guardrails só-leitura, audit com filtros não-funcionais, sem responsivo/skeletons/confirmações, modais duplicados.
- 3 cópias do `install.sh`; install defaulta SQLite, sem TLS/CORS/docs de env.
- Auto-sync no CLI inexistente (hoje `anchored remote sync` é manual).

---

## Fase 0 — Hardening pré-lançamento  ✅ IMPLEMENTADA (verificada, sem commit)

**Objetivo:** base segura e operável; fonte única de install.

**Status (verde em `go build` + `go vet` + `go test -race ./...`):**
- ✅ 3 panics removidos (`sync/engine.go`, `handler/memory.go`, `store/knowledge_graph.go`) — fallback time+counter infalível.
- ✅ Rate limiting token-bucket sem dependência (`internal/middleware/ratelimit.go`): bucket global por cliente + bucket estrito em `/v1/auth/login` e `/register`. Config `rate_limit.*` + env `RATE_LIMIT_*`. Testes: `ratelimit_test.go` (5 casos).
- ✅ Retenção de auditoria: `Store.PurgeAuditOlderThan` (Postgres+SQLite) + goroutine `runAuditPurge` (default 90d / sweep 6h). Teste: `audit_purge_test.go`.
- ✅ Install unificado: removido `scripts/install.sh` órfão; `make sync-install`/`verify-install` garantem `install/install.sh` == cópia embedada.
- ✅ Config documentada (`config.example.yaml` + `.env.example`).
- ⏭️ Request log já existia (`middleware/logging.go`) — não duplicado.
- 🔎 Achado: `mode:` em `config.example.yaml` não é lido pelo `config.go` (resquício da variante cloud) — deixado como está.

- Corrigir 3 panics → retornar erro: `sync/engine.go:372`, `store/knowledge_graph.go:261`, `handler/memory.go:294` (helper `newID() (string, error)`).
- Rate limiting (middleware token-bucket por IP+key): `/login`, `/register`, `/v1/sync`, `/v1/memories`. Config em `config.go`.
- Retenção de audit: migration com índice em `created_at` + job de purge (TTL configurável, default 90d) + paginação já existe.
- Unificar `install.sh`: **uma** fonte canônica (gerar as cópias `internal/web/install/` e `../anchored-oss/` no build/release a partir dela). Remover divergência entre `scripts/`, `install/`, `internal/web/install/`.
- Observabilidade mínima: middleware de request log (method/path/status/duração) reusando o recovery existente.

**Verificação:** `go test ./...`, `go vet`, smoke de rate limit (429), restart pm2 sem perda.

---

## Fase 1 — Camada de embeddings (pedra angular)  ✅ IMPLEMENTADA (verificada, sem commit)

**Status (unit `-race` verde + integração contra pgvector real via Docker, idempotente):**
- ✅ `internal/ai/embeddings`: interface `Embedder` (espelha o CLI) + provider `local` (hashing dep-free, default 384d, L2-normalizado), `openai` (qualquer endpoint OpenAI-compatível: OpenAI/z.ai/OpenRouter), factory `New` (`none` desabilita). Unit tests: shape/normalização/determinismo/ordenação/factory.
- ✅ Migration 011 (pg): `CREATE EXTENSION vector` + `memories.embedding vector(384)` + `embed_model`/`embed_dims` + índice HNSW cosine. SQLite 011: colunas (embedding JSON, brute-force). Schema bump 10→11.
- ✅ Store: `UpdateMemoryEmbedding` + `SearchMemoriesByVector` (pg `<=>` / sqlite cosine em Go), na interface `Store`.
- ✅ Geração no **curation worker** (já carrega cada memória de `/v1/memories` E do sync) — best-effort, pula low_signal/dup; embedder opcional (nil = desligado).
- ✅ Endpoint `GET /v1/memories/search?mode=semantic` (opt-in; fallback texto se embedder ausente/erro).
- ✅ Config `embeddings.*` + chave via env (`api_key_env`, nunca no arquivo). Testes integração: `internal/store/embedding_integration_test.go`, `internal/curation/worker_integration_test.go` (build tag `integration`, `ANCHORED_TEST_DSN`).
- ✅ **Comando `reindex`** (`anchored-oss -reindex`): backfill de embeddings para memórias sem vetor (paginado por id, à prova de falha parcial). `store.MemoriesMissingEmbedding`. Testado (integração) + rodado em PROD: 11.378 embeddings, 0 falhas, ~10s → 11.536/11.536 (100%).
- ✅ **DEPLOY EM PROD verificado** (openclaw-gateway): instalou pgvector pg16, pré-criou extensão, schema v11, busca semântica via API retornando resultados vetoriais sobre o corpus real.
- ⚠️ **Pendente p/ produção:** porta do ONNX real (paraphrase-multilingual-MiniLM-L12-v2) como provider `local` — runtime indisponível no dev; o `local` atual é hashing (lexical, não semântico profundo). `CREATE EXTENSION vector` exige role com extensão disponível (provisionar no install).

### Bônus — bug pré-existente corrigido
- ✅ **Audit batch insert** (`AppendAudits`): `auditInsertCols` era 8 com 7 colunas → placeholder órfão `$8` → `SQLSTATE 42P18` derrubava todo o batch de auditoria no sync em massa. Corrigido para stride 7 + regression test pg (`TestAppendAuditsBatch_Postgres`). Deployado em prod.

**Objetivo:** vetores no servidor, espaço **compatível** com o CLI.

- `internal/ai/embeddings`: interface `Embedder { Embed(ctx, []string) ([][]float32, error); Dimensions() int; Model() string; Name() string }`. Espelhar `anchored/pkg/memory` (reutilizar ONNX `yalue/onnxruntime_go`, modelo `paraphrase-multilingual-MiniLM-L12-v2`, 384d, download no 1º run, `ort.SetSharedLibraryPath`).
- Providers: `local` (ONNX, default) + `openai`/`zai`/`openrouter` (HTTP). Registry compartilha credenciais com o chat (Fase 4).
- Postgres + **pgvector**: migration `CREATE EXTENSION vector`; coluna `embedding vector(384)` + `embed_provider`, `embed_model`, `embed_dims` por memória (dims variam por provider → reindex ao trocar).
- Geração: no upsert de memória (sync push) embeda assíncrono via fila (reusar `curation_queue` ou nova `embedding_queue`); backfill command `anchored-oss reindex`.
- **Importante:** se o CLI já manda o vetor (mesmo modelo), aceitar e pular re-embed; senão re-embeda server-side.

**Verificação:** backfill de N memórias, KNN `<=>` retorna vizinhos coerentes PT-BR/EN; troca de provider dispara reindex.

---

## Fase 2 — Busca semântica + view de memórias  🟡 PARCIAL (frontend verde via tsc)

**Status:**
- ✅ Backend: endpoint `GET /v1/memories/search?mode=semantic` (Fase 1).
- ✅ API client `searchMemories(projectId, q, mode, limit)` + tipo `Memory.metadata`.
- ✅ `ProjectDetailPage`: toggle **Text/Semantic**, busca server-side (Enter/botão), estado de resultados + clear, paginação escondida durante busca.
- ✅ **Drawer de detalhe de memória** (`MemoryDetail`): conteúdo completo + metadata (curation_status, quality_score, scope, pinned, canonical_of, curation_rule, keywords, source, timestamps) + dump do resto.
- ✅ DS estendido: `Btn.disabled`, `Input.onKeyDown` (verificado `tsc --noEmit`).
- ⏭️ Pendente: rerank híbrido BM25+vetor no servidor; busca global multi-projeto no dashboard; highlight de score.

**Objetivo:** valor central pro time, no dashboard.

- Backend: `GET /v1/memories/search?q=&project=&k=` → embeda query + KNN pgvector, híbrido com BM25/trigram (rerank). Filtros por categoria/projeto/curation_status.
- Frontend: busca semântica real em `ProjectDetailPage` (substituir substring) + busca global no dashboard. Resultados com score, highlight, category.
- View rica de memória: drawer/modal com metadata completa, lifecycle, curation_status, canonical_of (near-dup), histórico de sync, autor.
- Deep-link de busca (querystring na URL).

**Verificação:** consulta semântica PT-BR encontra memória escrita em EN sobre o mesmo conceito; latência < 200ms p/ projeto típico.

---

## Fase 3 — Guardrails customizáveis  ✅ IMPLEMENTADA (verificada em prod)

**Status (unit + integração pgvector verdes; deployado v0.2.6-fase3, schema 12):**
- ✅ Migration 012 `org_policies` (pg array + sqlite JSON) — overrides por-org de blocked_categories + quality/near_dup thresholds; ausência = defaults.
- ✅ `policy.NewContentFilterWithConfig(blocked, threshold)` — filtro deixa de ser hardcoded (secrets/paths/user-scope continuam always-on, não-configuráveis).
- ✅ Store `GetOrgPolicy`/`UpsertOrgPolicy` (defaults quando sem row). Sync engine (`handlePushes`) carrega a policy do org e aplica o filtro per-org no gate de sync.
- ✅ API admin `GET/PUT /v1/policies` (verificado em prod: GET defaults+always_on, PUT round-trip).
- ✅ UI: aba Policies agora tem painel **editável** (`OrgGuardrails`: blocked categories, thresholds, save) + seção always-on read-only (`tsc` verde).
- ✅ Testes: `policy/config_test.go` (filtro custom) + `store` integração `TestOrgPolicy_Postgres`.
- ⏭️ Pendente: read-path SQL (`qualityFilterSQL`) ainda usa a const global; tornar per-org é refactor separado. Worker de curadoria ainda usa filtro default (o gate de sync é o per-org).

**Objetivo:** tirar o hardcode; defaults + CRUD admin.

- Migration `org_policies` (por org): `blocked_categories`, `quality_threshold`, `near_dup_threshold`, regras custom (regex/keywords), enabled flags. Seed com os defaults atuais de `policy/filter.go`.
- `policy.ContentFilter` passa a carregar config por-org (cache com invalidação) em vez de defaults fixos. Worker e sync push leem a mesma config.
- UI: aba **Policies** do `ProjectDetailPage`/org settings vira editável (admin) — toggle de categorias, sliders de threshold, CRUD de regras custom, preview "isto seria bloqueado?".
- Secrets/paths continuam como guardrail **não-desativável** (defesa em profundidade).

**Verificação:** admin adiciona regra custom → worker re-cura e demota memória que casa; threshold ajustado muda aceitação no próximo sync.

---

## Fase 4 — Chat IA opcional (pré-contexto)  ✅ IMPLEMENTADA (verificada em prod)

**Status (unit -race verde; deployado v0.2.6-fase4; chat OFF por default):**
- ✅ `internal/ai/chat`: interface `Provider` + `openai` (cobre OpenAI/z.ai/OpenRouter, /chat/completions) + `anthropic` (/messages) + factory (disabled=nil). Config `chat.*` (enabled/provider/model/base_url/api_key_env/max_tokens), chave via env.
- ✅ Endpoint **RAG** `POST /v1/chat`: embeda a pergunta → KNN das memórias do projeto (k=8) → contexto numerado → provider responde citando [n] → retorna {answer, sources}. Respeita authz de projeto (admin bypass).
- ✅ `GET /v1/chat/status` (enabled/model) p/ a UI mostrar/esconder.
- ✅ Frontend: aba **Chat** (`ChatTab`) — auto-esconde input se desabilitado; senão pergunta + resposta + fontes citadas.
- ✅ Testes: `chat/chat_test.go` (httptest fakes openai+anthropic, error-status, factory). Verificado em prod: status=disabled, POST→503 (gate correto sem chave).
- ⏭️ Pendente: streaming SSE; histórico por usuário; testar completion real (requer chave de provider — o usuário habilita).

**Objetivo:** ajudar o usuário com pré-contexto; **opt-in**, não o foco.

- `internal/ai/chat`: interface `ChatProvider` compartilhando registry/credenciais com embeddings. Providers: z.ai, OpenAI, Anthropic, OpenRouter. Config por-org, chaves cifradas em repouso, **nunca** em logs/audit body.
- RAG: chat recupera via busca semântica (Fase 2) memórias do projeto como contexto; cita as memórias usadas.
- UI: painel de chat (lazy-load, só aparece se habilitado pelo admin). Streaming SSE. Histórico opcional por usuário (scope=user, não compartilhado).
- Guardrail: chat **lê** memórias respeitando authz do usuário (não vaza memórias de projeto sem acesso).

**Verificação:** pergunta sobre o projeto retorna resposta citando memórias corretas; desabilitar provider esconde a UI; chave não aparece em audit/logs.

---

## Fase 5 — Polish UI/UX + grafo + auditoria  🟡 PARCIAL (lacunas funcionais fechadas, deployada)

**Status (tsc verde; deployada v0.2.6-fase5):**
- ✅ **Auditoria com filtros FUNCIONAIS** (antes decorativos): dropdown de kind (action), busca por actor (Enter), range de tempo (24h/7d/30d/all via `from`), botão Clear — tudo ligado ao backend `GET /v1/audit`.
- ✅ **Grafo com filtro**: busca de nó (subject/object) + filtro por predicado + contador "N of total edges (showing first 200)".
- ✅ DS: `Btn.disabled` + `Input.onKeyDown` (Fase 2).
- ⏭️ Pendente (cosmético, baixo risco): responsivo (sidebar/tabelas mobile), skeletons (vs "Loading…"), confirmações em ações destrutivas, dedup de modais, deep-link de tabs, paginação real do grafo no servidor (hoje cap 200 client-side).

**Objetivo:** acabamento de lançamento.

- Grafo: paginação/streaming além de 200 triples, busca de nó, filtro por predicado, foco/expand por nó.
- Auditoria: filtros funcionais (action/actor/project), time-range picker, export CSV, ligado à retenção da Fase 0.
- UX transversal: skeletons (substituir "Loading…"), confirmações em ações destrutivas (revoke key, delete project), dedup de modais (extrair `<Modal>` + `<FieldLabel>` compartilhados), `disabled` real em submits, `prefers-reduced-motion`, aria-labels em botões só-ícone, deep-link de tabs.
- Responsivo: sidebar colapsável, tabelas com scroll/cards em <1024px, modais mobile-safe.
- Páginas em "draft" priorizadas: ProjectDetail, Developers, Audit.

**Verificação:** Lighthouse a11y > 90; navegação mobile usável; nenhuma ação destrutiva sem confirmação.

---

## Fase 6 — Auto write-through no `anchored`  ✅ IMPLEMENTADA (verificada end-to-end em prod)

**Status (build+vet verde; testado contra prod openclaw-gateway):**
- ✅ `RemoteConfig.AutoSync` + `RemoteEntry.AutoSync` (`auto_sync` yaml), carregado também do bloco legacy `remote:`.
- ✅ `pkg/sync.ClassifyForAutoSync(m, root)` — classifica uma memória reusando a pipeline de preview; retorna **conteúdo redigido** (paths→`<user-home>`) e ok só se Syncable. Testes: `pkg/sync/autosync_test.go` (fact syncável; user-scope/preference/secret bloqueados; sem path home cru).
- ✅ Wiring **local-first write-through** em DOIS caminhos: MCP `toolSave` (quando `p.Remote==""`, push async best-effort) e CLI `runSave` (`autoSyncRemote`). Ambos miram o projeto remoto **linkado** (`entry.Projects[0]`, como o `remote sync`), não o id local.
- ✅ Fix correlato no CLI: `runSave` passa o cwd real (antes `""`) → memórias salvas dentro de um projeto pegam project scope e ficam elegíveis a sync (igual ao MCP).
- ✅ **Verificado em prod:** save scope=user → corretamente **bloqueado** (não vazou); save project-scoped → "Auto-synced to remote default" → memória no prod com `project_id` linkado + **embedded=t** (worker embedou). Memórias de teste limpas; `auto_sync` revertido a `false`.
- ⏭️ Pendente: search MERGE local+remoto; backoff/retry/fila offline; detecção de 401 (DB wipe).

**Objetivo original abaixo.** CORE do v1.0 — sem isso o time sincronizaria na mão.

**Objetivo:** fechar a visão "sincronizando automaticamente" no nível das **tools** (não num comando manual). É a proposta de valor central — sem isso, o time teria de rodar `anchored remote sync` na mão.

**Estado atual (já existe ~70%):** `anchored_save`/`anchored_search` já têm o parâmetro `remote` (opt-in por chamada), `RemoteConfig`+`RemoteEntry` (multi-remote, Default), `pkg/sync` client e o filtro de elegibilidade. Falta tornar **automático** quando há remoto configurado + projeto linkado.

**Modelo de escrita — LOCAL-FIRST write-through (NÃO remote-first):**
1. `anchored_save` grava **local primeiro** (fonte da verdade, instantâneo, offline-safe — invariante "local save always succeeds" já prometido nas tools).
2. Empurra pro remoto como **best-effort assíncrono** (fila + backoff/retry, não bloqueia a resposta da tool), aplicando o filtro automaticamente → scope=user/secrets/paths/event/preference **nunca saem**.
3. Resolve remoto+`project_id` a partir do link do projeto (multi-remote via `RemoteEntry.Default`).

**Search — MERGE (não substituição):**
- Hoje `remote` busca *no lugar* do local. Mudar para **merge**: local (pessoal) + remoto (time) com dedup, **timeout curto** e fallback para só-local se o servidor não responder.

**Config/gating:**
- Novo flag `auto_sync` (bool) em `RemoteConfig` (hoje só `Enabled/ServerURL/APIKey/Projects`). Auto write-through ativa só com `Enabled && projeto linkado && auto_sync`.
- Detectar 401 (DB wipe → key stale, gotcha já documentado) e orientar re-`anchored remote configure`.
- Status/observabilidade da fila de push (pendentes, último erro).

> **Decisão de escopo:** este track vive no repo `anchored` (privado, CLI/MCP), em paralelo ao `anchored_oss`. Puxado para o **core do v1.0** porque é o que entrega o "automático".

---

## Install.sh (transversal, fecha na Fase 0/5)

Pra experiência real "curl | sh" de admin de time:
- Default **Postgres + pgvector** (provisionar/local ou pedir DSN); SQLite só como opção `--dev`.
- Setup de CORS (origem do dashboard), TLS opcional (Caddy/reverse proxy documentado), env vars documentadas (`ANCHORED_OSS_PORT/HOME/VERSION`, `DATABASE_URL`, `CORS_ALLOWED_ORIGINS`).
- pm2 `ecosystem.config.cjs` (já gerado) + `pm2 save` + startup. Pós-install imprime comando de bootstrap do 1º admin (ou link de onboarding).
- ONNX: baixar runtime lib + modelo no 1º start (não no install) e documentar tamanho/cache.

---

## Riscos / decisões em aberto

- **Dims por provider:** trocar de local(384) p/ OpenAI(1536) invalida o índice → reindex obrigatório; bloquear busca cross-provider no mesmo projeto.
- **onnxruntime cross-platform:** mesma fragilidade do CLI (shared lib por OS/arch). Reusar o que o CLI já resolve.
- **`engine.authorize` admin=bypass total:** decidir se dev-level grants (SetAccountProjects) devem limitar o que admin-scope enxerga (follow-up #4 da memória) — provavelmente fora do v1.0.
- **Custo de tokens do chat:** opt-in + limites por org.

## Sequência de build sugerida

0 → 1 → 2 → **6** (auto write-through local-first no `anchored` — core do automático) → 3 → 4 → 5 → tag v1.0.

> Fase 6 movida para antes do polish: depende da busca semântica (Fase 2, pro merge de search) e é o que materializa "compartilham automaticamente". Roda no repo `anchored` em paralelo às fases de UI.
