# Loom Content Authoring and Editorial Operations Direction

**Status:** Directional architecture draft

**Date:** 2026-08-22
**Scope:** Desired product boundaries and architecture; not an implementation plan

## Purpose

This document describes the intended place of Loom in the Hollis Labs
portfolio as it assumes the content-authoring and editorial-operations role
previously explored in Glyph. It evaluates authoring, campaigns, ideas, briefs,
source packets, generation, compilation, directives, review, revision lineage,
wiki and other output forms, scheduling, publication, extensions, credentials,
persistence, observability, and portfolio composition against the shared
engineering boundaries.

It deliberately does not define phases, estimates, task breakdowns, migration
steps, or compatibility sequencing. Existing code and `docs/architecture.md`
remain the current implementation truth until explicitly superseded. This
document describes the desired state for a later architecture and planning
session.

The earlier Glyph direction remains useful architectural context, but it is not
the target ownership split. In the desired portfolio state:

- Loom owns the content-authoring and editorial-operations product boundary.
- Loom's current wiki compiler becomes one mature output profile within that
  broader product.
- Glyph is not a second active authority for the same editorial records.
- The target model integrates editorial intent and compilation rather than
  transplanting Glyph's tables beside Loom's wiki tables.

## Portfolio axiom

> Hollis tools own execution and operational state, but not the business
> definitions or business data they operate on.

For Loom, this requires a precise distinction between semantic authority,
operational authority, and custody:

- A user, team, project, publication, or source application owns the meaning of
  content, campaigns, audiences, channels, voice, constraints, and editorial
  policy.
- Source authorities own the facts and current values referenced by content.
- Blueprint, generator, template, content-type, voice, review-policy,
  publication-policy, and output-profile publishers own those definitions.
- A participant owns the intent of an editorial decision; Loom owns the fact
  that it captured, validated, and applied that decision.
- Publication destinations own their live rendition and destination lifecycle.
- Credential authorities own secrets, keys, tokens, and provider identity.
- Loom owns the operational record required to turn editorial intent and source
  material into governed outputs: identities, bindings, immutable revisions,
  compilation runs, candidates, lineage, review records, lifecycle facts,
  materialization manifests, delivery attempts, receipts, provenance, and
  audit history.

Loom may be the custodial system of record for authored content and its
editorial pipeline without becoming the semantic owner of the content. A
project entrusting a draft to Loom grants custody and operational authority;
it does not transfer ownership of the project's message, policy, or facts.

The central boundary is:

> Loom owns the editorial work record and content-production execution, not
> editorial truth in the abstract.

## Product definition

Loom is a local-first content authoring, editorial operations, and content
compilation control plane. It:

1. Captures or accepts content intentions as ideas, briefs, direct authoring
   requests, or versioned source material.
2. Binds content work to explicit collections, campaigns, audiences, channels,
   sources, voice guidance, constraints, and publication targets.
3. Resolves source pointers into provenance-aware source packets without
   claiming authority over the sources.
4. Coordinates deterministic and model-assisted research, synthesis,
   generation, transformation, compilation, linting, and audit as attributed
   runs.
5. Maintains stable content identities, immutable revisions, lineage, working
   projections, links, attachments, and review context.
6. Compiles adopted content revisions into declared output profiles such as an
   OKF-shaped wiki page, Markdown bundle, site input, or destination package.
7. Captures typed editorial feedback, requested changes, approvals, blockers,
   attestations, and waivers with actor and evidence.
8. Maintains an operational editorial queue and publication calendar.
9. Materializes or delivers approved outputs through registered adapters and
   records manifests, attempts, destination receipts, and reconciliation
   observations.
10. Exposes the same semantics through GUI, CLI, HTTP, MCP, events, and narrow
    portfolio-native integrations.

Loom is not:

- a universal CMS, website builder, or page-hosting platform;
- the canonical portfolio knowledge or memory store;
- a general fragment capture and triage engine;
- a workflow language or durable general-purpose workflow runner;
- a task and work coordination system;
- an agent runtime, conversation host, or LLM gateway;
- a general scheduler or message broker;
- a provider credential store;
- the authority for external source material;
- the authority for a destination's live published state;
- an analytics warehouse;
- an infrastructure or local-daemon control plane.

The product remains independently useful with local files, deterministic
compilation, and SQLite. Models, agents, workflows, gateways, knowledge stores,
fragment sources, and remote publishers are optional attached capabilities.

## Core product boundary: authoring plus compilation

Loom's product boundary is broader than a compiler but narrower than a general
content platform.

The editorial side answers:

- What content work exists?
- Which brief, sources, constraints, and policies govern it?
- Which immutable revision is under review or adopted?
- Who decided what, against which revision and policy?
- What is intended for which destination and when?

The compilation side answers:

- Which exact inputs and definitions were resolved?
- What deterministic or model-assisted process ran?
- Which candidate and output artifacts were produced?
- Which checks passed or failed?
- What was materialized or delivered, and what receipt was observed?

These are one coherent product because content production requires both.
They remain separate lifecycle domains so a successful compile never silently
means approved, adopted, published, or true.

The wiki is one content collection and output profile. `wiki_page` should not
remain the universal internal type onto which ADRs, reminders, articles,
campaign copy, documentation, and every future artifact are flattened. A wiki
page may be the correct adopted form for any of those, but the decision is
explicit in the output profile and content-type binding.

## Architecture sketch

```text
Business and editorial authorities
project · operator · strategist · writer · reviewer · publisher
        |
        | editorial intent + versioned definition references
        v
+------------------------+       Source authorities
| publisher definitions  |       Tesseract · Fragments · git · web · files
| type · blueprint       |                    |
| voice · review · output|                    | immutable refs/observations
+-----------+------------+                    v
            |                          +---------------------------+
            +------------------------->|           Loom            |
                                       |                           |
 UI / CLI / HTTP / MCP -------------->| editorial application     |
                                       | source-packet resolver     |
                                       | content/revision catalog   |
                                       | compile + validation runs  |
                                       | review + decision ledger   |
                                       | calendar + delivery        |
                                       +-------------+-------------+
                                                     |
                                  adopted revision + output binding
                                                     |
                         +---------------------------+-------------------+
                         |                           |                   |
                         v                           v                   v
                  OKF/wiki bundle              Loom/site input      channel API
                  Markdown/files/git           or CMS package       publisher
                         |                           |                   |
                         +---------------------------+-------------------+
                                                     |
                                      manifest / receipt / observation

Hadron ------ optional complex content workflow and durable run
Nanite ------ optional author/research/curator agent session
Tether ------ optional model/MCP gateway, messaging, or federation
Torque ------ optional managed work; not the editorial record
Tangent ----- optional rich review interaction; not decision authority
Cerberus ---- deployment and attached resources; no content semantics
```

An idea, brief, content item, content revision, compile run, generated
candidate, review, agent session, workflow run, calendar intent,
materialization, publication attempt, and destination object are distinct
identities even if one local interaction creates several of them.

## Responsibility boundaries

| Concern | Authoritative owner | Loom responsibility |
|---|---|---|
| Content strategy and campaign meaning | Project, operator, or campaign owner | Maintain the entrusted operational record and bind exact revisions |
| Idea meaning and priority | Originating user, agent, or project | Capture, correlate, queue, and preserve disposition history |
| Brief requirements | Brief author or project | Validate, version, bind, present, and track fulfillment |
| External source facts | Tesseract, repository, Fragments Engine, URL, or other source | Resolve, snapshot what was used, and preserve provenance |
| Native authored content | User or project | Provide custodial storage, revision lineage, collaboration, and retrieval |
| Content-type and output definitions | Definition publisher | Register, validate, resolve, cache, materialize, and bind exact revision |
| Blueprint, template, and voice | Definition publisher | Apply a pinned definition and record outputs and conformance |
| Directive vocabulary | Directive publisher | Parse, validate, plan, authorize, and execute supported operations |
| Compilation execution | Loom | Bind exact inputs, run, validate, emit candidate outputs, and record provenance |
| Model and provider behavior | Provider and selected model | Preserve disclosure, parameters, usage, and observed output |
| Agent session | Nanite or another launching application | Request by contract and correlate returned artifacts when composed |
| Review input | Authenticated reviewer | Capture exact decision, scope, evidence, and actor |
| Review application | Loom | Validate authority and policy, then append operational facts |
| Editorial queue and calendar | Loom as operational custodian | Maintain desired editorial state and attention projections |
| Publication policy | Project or destination publisher | Resolve and enforce a pinned policy through an adapter |
| Materialization | Loom | Produce a manifest-bound projection from an adopted revision |
| Publication attempt | Loom | Dispatch idempotently and record the exact request and receipt |
| Published rendition and status | Destination system | Retain destination identity and reconciliation observations |
| General work and commitments | Torque | Link or create work without duplicating its lifecycle |
| General workflows | Hadron | Invoke a workflow and correlate its run |
| Portfolio knowledge and memory | Tesseract namespace authority | Consume references; promote only explicitly selected outcomes |
| Fragment intake and provenance | Fragments Engine | Consume selected fragment revisions or routed delivery bundles |
| Rich interaction surface | Tangent | Publish a review interaction and consume its typed resolution |
| Infrastructure and local service | Cerberus and the OS | Expose health/readiness and consume attached resources |
| Secrets and provider credentials | External credential authority | Store references and consume narrow runtime grants |

## Authority and custody model

### Native content

Content deliberately authored in or adopted by Loom is native content. Loom may
be its durable custodian and operational source of truth while the user,
project, or publication remains its semantic authority.

```text
ContentItem
  stable identity
  owning scope and semantic authority
  content kind and media type
  title and descriptive metadata
  current adopted revision reference
  editorial-state reference
  custody, sensitivity, and retention policy
  external correlation references
  creation and archival facts
```

Changing content creates a revision. Mutable editing state may exist for user
experience, but accepted checkpoints and every revision used by a compile,
review, export, or delivery are immutable and addressable.

### External source content

A source URI or Fragments Engine ID alone is not enough to explain an authored
result. When Loom uses an external source, it binds what it observed:

```text
SourceBinding
  source authority and connector
  source item identity and locator
  requested and resolved revision
  observed and source-modified times
  digest and media type
  excerpt, snapshot, or immutable content reference
  transformation and retrieval provenance
  sensitivity, use restrictions, and custody class
  availability and freshness observations
```

Refreshing creates another observation or source-packet revision. It does not
silently change the evidence behind an existing content revision, compile,
review, or publication.

### Definitions

Content schemas, bundle profiles, generators, templates, prompts, voice
profiles, directives, validation rules, review policies, publication mappings,
and export layouts are publisher-owned definitions.

Loom may discover, register, validate, resolve, materialize, cache, execute,
compare, and report on them. Registration and caching do not transfer
ownership. Every run binds an immutable definition revision and digest;
historical runs are never reinterpreted by a later update.

### Derived content

Research summaries, generated drafts, compiled pages, extracted metadata,
links, scores, conformance findings, suggested titles, and rendered packages
are derived outputs. Each records producer, definition revisions, exact inputs,
provider/model disclosure where applicable, parameters, grants, timestamps,
and provenance.

A derived output is a candidate until an authorized adoption rule accepts it.
Successful computation alone does not make it the current authored revision.

### Published content

Loom is authoritative for what it materialized or attempted to publish and
what response it observed. The destination owns its current live object.

```text
desired publication
  -> validated package
  -> dispatch accepted
  -> destination object created or updated
  -> destination reports scheduled or live
  -> optional independent verification observed
```

Each arrow records a separate fact. Removing a Loom calendar item or content
record does not imply permission to delete an external publication.

## Core domain model

### Scope and publication space

Every durable record belongs to an explicit user, project, brand,
publication, or organization scope. Scope controls identity, authorization,
definition resolution, defaults, retention, and queries.

A Loom publication space groups related content and output policy:

```text
PublicationSpace
  stable identity and owning scope
  title, purpose, and audience references
  content-type and output-profile allowances
  default policy and definition references
  destination registrations
  custody and retention defaults
```

The current `wiki_bundle` is a useful publication-space projection for an OKF
wiki. `project|meta|personal` are useful classifications, but they are not by
themselves authorization boundaries or globally unique authority identities.

### Campaign

A campaign groups editorial work around publisher-owned intent:

```text
Campaign
  stable identity and owning scope
  purpose, audience, and outcome references
  publication-space and channel references
  strategy-definition revision
  active window and editorial lifecycle
  related business-object references
  provenance and audit metadata
```

Loom can be the operational system of record for campaign content without
becoming the authority for product strategy, budget, audience taxonomy, or
analytics.

### Idea

An idea is a captured content opportunity, not a production commitment:

```text
Idea
  stable identity and owning scope
  originating principal and source binding
  title, summary, and angle hypotheses
  campaign, target, and related-item references
  triage state and disposition
  creation and decision history
```

Deduplication proposes relationships or merges and preserves provenance. It
does not silently discard independently captured intent.

### Brief

A brief is a versioned editorial contract:

```text
BriefRevision
  brief identity and immutable revision
  campaign and idea references
  title, purpose, angle, audience, and channel
  constraints and acceptance criteria
  content-type, voice, generator, review, and output references
  source-packet revision
  target dates
  author, provenance, digest, and supersession
```

Every generated candidate and adopted revision binds the exact brief revision.
A mutable “current brief” is a convenience projection, never the only evidence
for how a prior output was produced.

### Source packet

A source packet is a curated, immutable evidence set for a content item:

```text
SourcePacketRevision
  packet identity and revision
  ordered source bindings
  curator and rationale
  required, optional, and excluded classifications
  stale or unresolved observations
  sensitivity and use restrictions
  digest and creation facts
```

It may contain references, excerpts, or retained snapshots according to source
and custody policy. The packet preserves enough evidence to explain a compile,
review, or publication without pretending Loom is the current source authority.

### Content revision

Content identity and content revision are separate:

```text
ContentRevision
  content item identity
  immutable revision identity and sequence
  parent or merge-parent revisions
  body or immutable artifact reference
  media type and structural metadata
  brief and source-packet bindings
  producer and run reference
  digest, timestamps, and provenance
  adoption and supersession facts
```

Revision number alone is not lineage. Branches, agent proposals, manual edits,
imports, and merges are explicit. Working, submitted, reviewed, adopted, and
superseded are roles or lifecycle facts, not destructive mutations of history.

### Output profile and compiled artifact

An output profile maps an adopted content revision into a target contract:

```text
OutputProfile
  publisher-qualified identity and immutable revision
  supported content kinds and target formats
  required metadata and validation rules
  renderer/compiler reference
  path and naming policy
  destination capabilities
  compatibility and trust evidence

CompiledArtifact
  immutable artifact identity and digest
  input content and definition bindings
  output profile and compiler revision
  media type, metadata, and payload reference
  validation and conformance reports
  producing run and provenance
```

`wiki_page` plus OKF-style Markdown is one output profile. Other profiles do
not require overloading the wiki page table or pretending every output is a
page.

### Review and verification

Automated checks and actor attestations are distinct:

```text
VerificationObservation
  exact subject artifact or revision
  checker identity and version
  rule-definition revision
  finding, severity, evidence, and timestamp

ReviewRecord
  exact subject revision or artifact
  reviewer principal and represented role
  review-policy revision
  annotations and evidence
  typed decision
  supersession or revocation facts
```

Loom owns the observation that a checker produced a result. The checker
publisher owns rule meaning. A human or agent owns its attestation. Loom owns
policy-authorized application of the decision.

### Editorial and publication state

Operational projections remain simple for users while their causes stay
explicit:

```text
EditorialState
  subject and current adopted revision
  state-vocabulary and policy revision
  current state and blocking conditions
  attention reasons
  actor, cause, evidence, and timestamps

PublicationIntent
  content item and selected revision
  destination and output-profile references
  desired window, embargo, and expiry
  publication-policy revision
  desired state and operator notes
```

`blocked` is an orthogonal condition rather than a state that erases whether
content was drafting, under review, approved, or scheduled.

### Run, candidate, and materialization

Every research, generation, compile, lint, export, or packaging action is a
first-class run:

```text
ContentRun
  stable run identity and operation kind
  requested-by principal and owning scope
  input revisions and source-packet bindings
  definition and policy revisions
  execution target and external run/session references
  capability and credential-grant references
  lifecycle timestamps and attempts
  candidate and artifact references
  diagnostics, usage, and outcome

Materialization
  adopted content and compiled-artifact bindings
  target root or destination registration
  manifest of paths and digests
  authority mode and reconciliation policy
  creation, verification, and replacement facts
```

This model supports local deterministic functions, provider-backed generation,
a Nanite agent, or a Hadron workflow without making their runtime mechanics the
editorial domain.

## Lifecycle separation

Loom should keep these lifecycles independent:

| Lifecycle | Example states | Owner |
|---|---|---|
| Idea disposition | captured, triaged, adopted, declined, archived | Loom record under editorial policy |
| Brief | drafting, approved, superseded, withdrawn | Loom record under brief authority |
| Content revision | working, submitted, adopted, superseded | Loom custodial record |
| Content run | queued, running, succeeded, failed, cancelled | Loom or delegated executor, correlated by Loom |
| Candidate | produced, staged, rejected, adopted | Loom under editorial policy |
| Review | open, resolved, superseded, revoked | Loom capture under reviewer authority |
| Verification | pending, passed, warned, failed, stale | Loom observation under checker definition |
| Agent session | requested, active, suspended, completed | Nanite or launching runtime |
| Workflow run | accepted, running, waiting, completed | Hadron |
| Calendar intent | proposed, scheduled, cancelled, missed | Loom |
| Materialization | rendered, staged, verified, applied, stale | Loom |
| Publication attempt | pending, dispatched, acknowledged, failed | Loom |
| Destination object | draft, scheduled, live, removed | Destination authority |

One lifecycle can trigger or project another but cannot impersonate it:

- generation success produces a candidate, not an adopted revision;
- compilation success produces an artifact, not an editorial approval;
- passing structural conformance does not attest factual correctness;
- a high text-similarity score does not establish content quality or trust;
- approval does not create a calendar slot unless policy composes those acts;
- a calendar item is intent, not a durable timer execution;
- a successful HTTP response is not proof that an output is live;
- an agent session ending does not determine whether its output was adopted.

## Authoring and compilation model

Compilation must become a pure candidate-producing operation at its semantic
boundary:

```text
immutable inputs
  + resolved definition revisions
  + validated policy and grants
  -> compile or generation run
  -> immutable candidate revision and artifacts
  -> validation and comparison
  -> explicit adoption decision
  -> materialization or delivery
```

A run may persist its candidate and diagnostics atomically. It must not
overwrite the currently adopted revision before the caller can decide whether
to stage or accept it.

Comparison signals should be named for what they measure:

- line or structural similarity measures change size;
- collision risk measures path or identity conflict;
- validation results measure declared conformance;
- model confidence, when available, is attributed provider output;
- editorial confidence is a policy decision based on evidence;
- factual trust requires source and reviewer evidence.

The current line-similarity score is useful as a change-risk input. Calling a
brand-new page `confidence=1.0` only means it cannot overwrite an existing
body; it says nothing about quality, factuality, safety, or readiness. The
target model must not overload one confidence field with those meanings.

Deterministic compilation should be reproducible from immutable inputs and
tool versions. Model-assisted generation should preserve exact recorded inputs
and provider disclosure without promising byte-for-byte replay where the
provider cannot guarantee it.

## Content formats and OKF

OKF-shaped Markdown is a strong portable output contract for Loom's wiki
profile. It should remain a supported standard-compatible profile rather than
the entire internal domain model.

The canonical content representation should preserve:

- stable content identity and immutable revision;
- media type and structured blocks where required;
- title, summary, descriptions, tags, and type references;
- source bindings and provenance;
- lifecycle and trust observations;
- links, attachments, annotations, and embedded assets;
- publisher extensions without invalidating the standard core;
- digest and schema/profile revisions.

CommonMark is an excellent textual denominator. Rich content may retain a
lossless structured form or source attachment so tables, embeds, annotations,
media, and destination-specific capabilities are not destroyed during
normalization.

Flat frontmatter may remain an export profile. Internal storage should not be
forced to mirror one frontmatter layout, and an export timestamp must not
change the content digest when content has not changed.

Content type is open for interoperability but should resolve to an optional
publisher-qualified definition when behavior, validation, or authorization
depends on it. A bare string remains descriptive metadata; it must not silently
activate privileged code.

## Directives and executable content

Embedded directives are executable intent and must be treated as code-like
input, not decorative prose.

```text
source syntax
  -> structurally parsed directive
  -> namespaced command and definition revision
  -> typed parameters and scoped context
  -> policy and capability validation
  -> deterministic execution plan
  -> content run or delegated execution
  -> attributed candidate/result
```

Target rules:

- Unknown directives remain visible and inert or fail usefully; they never
  fall back to executing a generic content write.
- A directive hash identifies exact syntax and context, not semantic identity
  across handler revisions.
- Definitions declare effects such as source reads, model calls, tool use,
  artifact writes, external delivery, or publication.
- Loom distinguishes declared, granted, and actually used capabilities.
- Every execution binds exact handler, policy, and definition revisions.
- Retry is run policy and does not create ambiguous duplicate content.
- Directives from external or untrusted content are inert until authorized.
- Authors can express intent without implicitly granting side effects.

Generic parsing belongs in `go-directives`. Loom owns its editorial and
compilation interpretation. A `reminder` may compile into a content artifact,
but reminder scheduling belongs to the application that owns the commitment.
An ADR may be a content type without making Loom the authority for a project's
decision process.

## Generators, templates, voice, and policy

A generator is versioned executable definition, not only a string such as
`wiki_page`:

```text
GeneratorDefinition
  publisher-qualified identity and immutable revision
  supported input and output kinds
  schema and required definitions
  deterministic or model-assisted classification
  implementation or adapter reference
  template/prompt assets
  declared capabilities and effects
  validation contract
  compatibility and trust evidence
```

Templates, blueprints, voice profiles, review policies, status vocabularies,
channel mappings, and publication rules are also publisher-owned definitions.
Loom may provide authoring and registration UX, but a record stored in Loom is
not therefore Loom-owned business logic.

Local directories, Git repositories, embedded packages, and remote registries
are possible sources. Organization and filenames are semantic preferences;
the functional requirements are stable publisher identity, immutable revision,
digest, provenance, trust, compatibility, and deterministic resolution.

Mutable template upsert is convenient for current operation. Runs in the target
model bind immutable template revisions so editing a template never changes the
meaning of a historical compile.

## Source packets, ingestion, and research

Loom accepts deliberate content input; it does not absorb broad capture,
fragment provenance triage, or universal source crawling.

Direct file and text ingest remains useful for:

- importing an explicitly selected authored corpus;
- compiling repository documents into a declared output space;
- local standalone use without Fragments Engine;
- administrative migration and recovery;
- controlled batch authoring.

When composed, Fragments Engine owns broad capture, normalization, enrichment,
triage, and routing. It delivers immutable fragment references or source
bundles. Loom decides how accepted material participates in a source packet or
content item and records the delivery binding.

Source resolution should support pinned Tesseract records, Fragments Engine
revisions, repository commits and paths, URLs with observation metadata, local
files under explicit roots, application objects, and user-provided excerpts.

Research is a content run that produces findings with sources, claims,
uncertainty, and provenance. It may enrich a source packet or propose sources;
it does not overwrite Tesseract, Fragments Engine, or external authority.

Refreshing source material creates a new packet revision. Existing compiles,
reviews, and publications retain the evidence they actually used.

## Model and agent execution

Model execution is one adapter class, not Loom's core abstraction. Human-only
authoring and deterministic compilation remain first-class.

Loom owns:

- the editorial request and exact inputs;
- prompt and definition materialization;
- provider/model selection policy when delegated by the owner;
- data-egress and redaction policy application;
- the run record;
- result validation and candidate creation;
- usage and diagnostics supplied by the adapter.

Loom does not own provider credentials, a universal LLM gateway, the
provider's session, a Nanite conversation, or acceptance of model output.

When composed with Nanite, Loom requests a bounded curator, writer, researcher,
or reviewer session with exact content and source references. Nanite owns the
session, agent process, tool loop, and conversation. Loom correlates returned
artifacts to a content run.

Curator and Weaver are roles, not hidden ownership transfers:

- Curator may classify source material and propose or compile candidates; Loom
  applies editorial and adoption policy.
- Weaver may search and answer over Loom content; Loom remains the content
  custodian while Tesseract remains the general knowledge/memory authority.
- Neither agent obtains ambient permission to overwrite adopted content,
  publish, or promote knowledge.

Tether may provide model or MCP gateway mechanics when composed. Loom owns
editorial settings and egress policy; Tether owns gateway settings and
transport mechanics. Tether is not required for core operation.

## Review, verification, and trust

The existing distinction between automated verification and actor attestation
is a strong foundation.

Automated checks should record:

- exact subject revision or artifact;
- checker and rule-set revisions;
- deterministic or model-produced classification;
- finding severity, message, evidence, and affected location;
- creation and invalidation conditions.

Review should:

- bind every note and decision to an immutable revision or artifact;
- authenticate the reviewer or explicitly identify a local principal;
- distinguish personal identity from represented role;
- bind the review-policy revision granting authority;
- use stable structural anchors where possible;
- preserve supersession and revocation;
- distinguish advice, requested change, blocker, approval, rejection, and
  waiver;
- invalidate or re-evaluate approval when its subject or governing inputs
  change;
- apply transitions idempotently and auditably.

Structural conformance means an artifact matches a declared shape. Content
quality means it meets editorial requirements. Factual trust means claims are
supported under source and review policy. Publication readiness means all
required gates are satisfied. These are separate axes.

Tangent may host rich comparison, annotation, or approval interactions. Tangent
owns the surface and resolution delivery; the reviewer owns the decision; Loom
validates and applies it.

## Materialization, export, scheduling, and publication

The existing rule that exported directories are regenerated projections is
sound. A materialization manifest should make it enforceable:

```text
MaterializationManifest
  identity and target root
  authority mode
  input revision and output-profile bindings
  ordered files with paths, digests, and ownership class
  generator and release provenance
  applied and verified timestamps
  prior manifest or supersession reference
```

Managed projections are regenerated from Loom's adopted records and should not
be hand-edited. Scaffold or transferred outputs explicitly move future editing
authority to a consumer. Authoritative external files are ingested as sources;
Loom does not overwrite them through a managed-export path.

Writes are staged, root-safe, collision-aware, and atomic where possible.
Cleanup is manifest-based and never broad recursive deletion. Divergence is
reported rather than silently clobbered.

Loom's calendar is editorial planning, not a general durable scheduler. Generic
timers can be delegated to Hadron, an OS scheduler, or another registered
scheduler. Destination-native scheduling remains with the destination adapter.

Publication adapters declare supported operations, idempotency, update/delete
capabilities, draft/scheduled/live states, media constraints, credentials,
rate limits, and reconciliation. Loom prefers official Go clients, SDKs, CLIs,
and provider APIs before implementing protocols directly.

`published` is an observed destination fact attached to an exact content and
artifact revision. Exported, delivered, acknowledged, scheduled, live,
verified, and removed remain distinct.

## Search, recall, and answer generation

Loom owns search and relationship projections over the content entrusted to
it. That supports authoring, review, reuse, and Weaver-style questions without
making Loom a general memory or knowledge substrate.

Search results should distinguish:

- adopted content from candidates and superseded revisions;
- current materializations from stale ones;
- authored assertions from source observations and model suggestions;
- structural links from inferred relationships;
- local text match from semantic retrieval;
- source freshness and trust observations.

Large-result caching is a derived performance projection. It must be scoped to
an authenticated caller or capability, bounded by TTL, redacted under the same
policy as the original result, and safe to discard.

Answers generated from Loom content cite immutable content revisions and their
sources. An answer is a derived result, not a mutation of the corpus. Discoveries
become proposed content revisions or explicit Tesseract promotions rather than
silent writes.

## Interfaces and parity

Loom follows “one core, several doors.”

- **GUI:** human authoring, editorial operations, review, and observability;
- **CLI:** scripting, local authoring, compilation, inspection, and admin;
- **HTTP:** typed application API with concurrency and idempotency;
- **MCP:** agent-friendly commands, resources, and capability discovery;
- **events:** durable signals about facts, not an alternate command surface;
- **portfolio-native clients:** narrow typed integrations where useful.

The application service layer owns semantics. Interface handlers adapt that
layer and do not query the repository to invent their own transitions.

Interfaces expose stable identities, exact revisions and digests, idempotency,
expected-revision preconditions, actor and scope, correlation and causation,
typed errors, pagination, and capability discovery.

The current single service layer behind HTTP, CLI, and MCP is a strong model to
preserve. Interface parity is semantic rather than requiring every interface
to expose every bulk or interactive convenience identically.

MCP caller identity must be verified rather than trusted from a freeform
`caller_id`. Cache scoping and authorization derive from the transport
principal or a validated capability.

## Plugin and adapter model

Useful extension classes include:

- source resolvers and source-packet providers;
- import and export codecs;
- content-type, generator, template, voice, and policy providers;
- directive handlers;
- deterministic compilers, transformers, linters, and auditors;
- research and model providers;
- agent and workflow execution adapters;
- review-surface publishers;
- publication destinations;
- artifact stores, search indexes, and observability sinks.

Each extension declares stable identity, publisher, version, digest, extension
class, contract versions, configuration schema, capabilities, effects,
credential-reference kinds, supported inputs and outputs, isolation needs,
health contract, provenance, and trust evidence.

The host validates compatibility and policy, then grants the minimum required
capabilities. Declared, granted, and actually used effects are separate.

Provider integration preference:

1. official Go client when healthy and adequate;
2. official SDK behind a bounded adapter;
3. official CLI with explicit version and structured output;
4. documented provider API;
5. custom protocol implementation only when necessary.

The Hollis Labs plugin SDK should provide common manifest, lifecycle,
capability, configuration, trust, diagnostics, and conformance primitives.
Loom retains content-domain contracts. Nanite's plugin system is prior art, not
a reason to import Nanite product semantics.

Trusted in-process Go adapters share process authority. Less trusted or
independently versioned extensions belong behind process, WASM, or protocol
isolation with explicit grants.

## Authentication, authorization, and secrets

Localhost is not identity. The current unauthenticated local API and caller-
supplied MCP IDs are implementation facts, not target boundaries.

Authorization evaluates principal, represented role, owning scope, requested
action, exact revision, sensitivity, source restrictions, definition trust,
processing effects, execution target, destination policy, and required
approvals or separation of duties.

Human-readable `generated_by` and verification `by` fields are useful display
and provenance metadata; they are not sufficient authorization identities.
User, agent, service, provider, and publisher identities remain distinct.

Loom must not become a secret store. Configuration carries references such as:

```text
credential://scope/provider/anthropic
secret://authority/path#field
grant://run/capability
```

An external authority resolves the reference at the narrow execution boundary
into an environment value, file, stdin payload, SDK credential object, or
brokered call. Loom persists the reference and grant metadata, never the secret.
Logs, run inputs, error messages, traces, caches, and receipts are scrubbed.

Redaction is defense in depth, not authorization to send arbitrary source
material to a model. Egress policy evaluates the source's sensitivity and use
restrictions before provider selection.

Materializing credentials for one compile or publication operation is valid
plumbing. Owning rotation, recovery, or long-term storage is not. A central
Cerberus configuration containing raw provider keys is therefore a current
deployment compromise, not the desired credential boundary.

## Configuration ownership

Configuration should be typed and layered:

```text
compiled defaults
  < system installation
  < user profile
  < project/publication scope
  < release/deployment configuration
  < invocation override
```

Editorial definitions, application configuration, release configuration, and
invocation input are distinct. Publisher-owned definitions never become the
place to hide deployment settings or credentials.

Paths, sources, exclusions, destinations, models, and attached resources use
portable identifiers and explicit roots. User-specific absolute paths belong
to local deployment configuration, not source-controlled product definitions.

Every run records the resolved, redacted configuration fingerprint needed to
explain its behavior. Environment variables are an input transport, not Loom's
canonical configuration model.

## Persistence and recovery

SQLite is an appropriate default for a local-first, single-operator Loom. The
target durable model includes:

- scopes and publication spaces;
- content, brief, source-packet, and definition bindings;
- immutable content revisions and lineage;
- adopted-head and editorial-state projections;
- compile, generation, lint, export, and publication runs;
- candidate and compiled artifacts;
- reviews, attestations, and verification observations;
- materialization manifests;
- delivery attempts, receipts, and destination observations;
- extension registrations and compatibility facts;
- audit and retention decisions.

Persist facts that cannot be reconstructed. Rebuild search indexes, counts,
dashboards, result caches, rendered frontmatter, and other projections.

Writes use transactions, idempotency keys, and expected-revision checks. An API
retry must not create duplicate content revisions, repeated publication, or
ambiguous directive effects.

Large content and media may live in a content-addressed blob store or external
artifact service while SQLite stores identities and references. Storage
location does not decide authority.

Backups cover the database and all non-reconstructible artifacts as one
consistency domain. Migration, repair, import, export, and reconciliation run
as one-off modes from the same release and record auditable outcomes.

## Process and local service model

Loom should remain a restartable Go process with explicit attached resources.
The single binary serving HTTP, MCP, and embedded UI is a strong production
shape. Stdio MCP is another interface mode over the same contracts, not a
second source of truth.

Only one writer authority may own a SQLite store at a time unless the storage
contract explicitly supports concurrent processes. A daemon and separately
spawned stdio MCP server pointing at one file need an explicit concurrency and
migration-owner model; process coincidence is not coordination.

Loom exposes liveness, readiness, deeper diagnostics, graceful shutdown, and
one-off administration. Readiness verifies required stores and recovery state;
optional provider failures appear as capability degradation rather than making
deterministic authoring unavailable.

Loom does not need to install or supervise itself as a daemon. A native OS
service manager, container runtime, or standard supervisor owns process
lifetime. Cerberus may validate and materialize the service registration,
release configuration, attached-resource references, and health contract, and
may observe it, without becoming the content owner or long-term supervisor.

Explicit development-session resources may remain Cerberus-managed. The mode
is part of the resource definition and does not set the production boundary.

## Events, diagnostics, and observability

Operational telemetry and durable domain audit are separate streams.

Operational telemetry includes structured logs to stdout/stderr, traces across
API/resolver/compiler/model/workflow/publication boundaries, metrics for run
latency and failure, adapter health, queue depth, storage behavior, and redacted
diagnostic bundles.

Durable domain events include:

- idea captured, triaged, adopted, or declined;
- brief or source-packet revision created, approved, stale, or superseded;
- content revision created, submitted, adopted, merged, or superseded;
- content run requested, started, completed, failed, or cancelled;
- candidate produced, staged, rejected, or adopted;
- verification or review recorded, superseded, or revoked;
- editorial state or blocking condition changed;
- materialization rendered, verified, applied, diverged, or replaced;
- publication scheduled, attempted, acknowledged, reconciled, or failed;
- definition or extension resolution changed.

Events carry stable event identity, schema version, scope, actor, subject and
revision, causation, correlation, trace context, timestamp, and redacted
payload or immutable references.

Events are not implicit commands. Cross-application delivery uses an outbox or
equivalent durable handoff when loss would violate the contract.

Loom should answer:

- Which brief, source packet, definitions, prompts, and model inputs produced
  this revision or artifact?
- Which parts were deterministic, model-produced, imported, or manually edited?
- Who adopted, reviewed, approved, blocked, scheduled, or published it?
- What changed between revisions and why?
- Which source facts were stale, unavailable, redacted, or transformed?
- Which external session, workflow, task, interaction, or destination object
  participated?
- What did Loom materialize or request, what was acknowledged, and what does
  the destination report now?

## Portfolio composition

### Glyph

Glyph is the predecessor for the incoming content-authoring and content-ops
role, not a peer authority in the desired state. Loom owns the resulting
editorial records after they are explicitly adopted into Loom custody.

Historical Glyph identifiers and provenance remain traceable through import or
alias references. No permanent dual-write or conflict-resolution layer defines
the target architecture. Clients use Loom's contracts for new editorial work.
This is an ownership conclusion, not a migration procedure.

### Fragments Engine

Fragments Engine owns broad capture, normalization, enrichment, provenance
triage, routing, and delivery of fragment-sized material. Loom consumes
explicit fragment revisions or delivery bundles and turns selected material
into governed authored content and outputs.

Loom does not duplicate Fragments Engine's inbox and routing domain. Fragments
Engine does not own Loom's brief, content revision, review, compile, or
publication state. Delivery binds immutable identities and records receipts.

### Tesseract

Tesseract owns authorized portfolio knowledge and memory. Loom owns the
authored content corpus entrusted to it, including wiki-shaped content.

Loom consumes explicit Tesseract references in source packets. It may propose
or promote selected findings, summaries, or published pointers back to
Tesseract under namespace policy. A Loom wiki page is not automatically
canonical knowledge merely because it is searchable or OKF-conformant.

### Hadron

Hadron owns reusable workflow definitions and durable workflow execution. Loom
owns content-production intent, candidates, adopted revisions, and editorial
outcomes.

Loom may invoke a pinned Hadron workflow for multi-step research, generation,
review gates, packaging, or publication. It records the run reference and maps
outputs into Loom artifacts. It does not copy Hadron step state into editorial
statuses or grow a parallel general workflow engine.

### Nanite

Nanite owns agent sessions and its runtime. Loom can request bounded curator,
writer, researcher, or reviewer sessions and correlate returned artifacts.
Loom does not own the conversation, provider process, or tool loop.

The existing Curator/Weaver pilot proves this composition shape but does not
make Nanite required for Loom. Agent procedure quality, tool visibility, and
session policy remain Nanite-side concerns; content adoption and compilation
policy remain Loom-side concerns.

### Tether

Tether is optional composition for LLM/MCP gateways, messaging, federation,
and identity transport. Loom owns editorial and content-egress settings;
Tether owns transport and gateway settings. Shared settings require an explicit
authority and materialization contract.

### Torque

Torque owns managed work, assignment, acceptance, execution attempts, and work
history. Loom owns ideas, briefs, content revisions, reviews, compilation, and
publication intent.

A Loom item may create or reference Torque work when there is a real external
commitment. It does not turn every idea, compile job, verification, or review
note into a task or mirror Torque state.

### Tangent

Tangent may render durable review, comparison, annotation, and approval
surfaces. Loom publishes an interaction bound to an exact revision or artifact
and consumes a typed resolution. Tangent owns the surface; Loom owns policy
validation and application to its editorial record.

### Sigil

Sigil compiles publisher-owned UI definitions into application artifacts. Loom
may use Sigil-generated UI or shared Sysop components, but Sigil does not define
Loom's content domain. The products can share registry, provenance, generator,
and materialization patterns without sharing one compiler or runtime.

### Cerberus and Coder

Cerberus owns infrastructure-resource reconciliation and may materialize Loom's
deployment and attached-resource definitions. The OS or deployment runtime
supervises long-running processes. Coder may host a workspace used by an agent
or operator, but the workspace and agent process do not become Loom state.

Loom consumes endpoints, data roots, and credential references. Cerberus does
not inspect content semantics, and Coder does not own content revisions.

## Glyph-to-Loom desired end state

The architectural end state has one content identity and editorial authority
model in Loom:

- Campaigns, ideas, briefs, source packets, drafts, reviews, calendars, and
  publication records are Loom editorial concepts.
- Wiki pages are projections or adopted content forms, not a separate authority
  from editorial content.
- Glyph-origin records retain original IDs, timestamps, actors, lineage, and
  provenance references where they matter.
- Imported records receive an explicit custody and authority classification.
- Loom content identities are stable independently of output path or wiki slug.
- Existing Loom pages become content revisions or imported adopted artifacts
  with their compile and source history preserved.
- Templates and blueprints resolve through one versioned definition model.
- Glyph's content ledger and Loom's ingest/directive/compile ledgers converge
  conceptually into content, source, run, artifact, and audit facts.
- New operations do not dual-write Glyph and Loom.
- Historical links may resolve through aliases or redirects without preserving
  two active systems of record.

This describes the state to arrive at, not the sequence for arriving there.

## Twelve-factor and Go operating model

| Factor | Loom direction |
|---|---|
| One codebase | One revision-controlled Loom codebase produces local, development, and service deployments |
| Dependencies | Declare Go modules, frontend packages, required CLIs, adapters, and toolchains explicitly |
| Config | Parse flags, environment, and files into typed configuration; retain only secret references |
| Backing services | Treat databases, artifact stores, models, knowledge, workflows, and publication destinations as attached resources |
| Build/release/run | Build an immutable Go binary and UI; release binds configuration; run does not compile source or mutate the release |
| Processes | Keep process memory disposable; persist editorial and run facts in declared stores |
| Port binding | Serve HTTP, hosted MCP, and embedded UI from explicit addresses |
| Concurrency | Use goroutines for bounded work and additional durable workers or processes only when required |
| Disposability | Start quickly, recover durable runs, honor cancellation, and shut down gracefully within a bound |
| Dev/prod parity | Use the same binary, schema, service contracts, and adapter contracts across environments |
| Logs | Emit structured logs to stdout/stderr; keep domain audit in an explicit durable store |
| Admin processes | Run migration, import, export, repair, and reconciliation from the same release |

## Current strengths to preserve

- One Go service layer backs HTTP, CLI, MCP, and the embedded UI.
- The single-binary production shape is simple and independently useful.
- SQLite provides a pragmatic local-first durable store and migrations.
- Bundles, pages, links, verifications, compile jobs, events, directives,
  ingestion, templates, and exports already form a coherent content compiler.
- Deterministic and LLM generation converge on one compilation path.
- Provider-backed generation is optional; deterministic compilation works
  offline.
- Pre-egress redaction exists and accepts configured patterns.
- Content hashes exclude changing render metadata such as generation time.
- Link extraction creates real internal relationships while retaining unresolved
  and external targets.
- Automated checks and actor attribution are conceptually distinct.
- Compile jobs and event histories provide an operational run foundation.
- Directive and ingest ledgers provide idempotency foundations.
- OKF-style Markdown exports are regenerated projections, not hand-maintained
  competing truth.
- Compile outputs expose diffs and similarity information to callers.
- MCP large-result caches are TTL-bound and avoid a shared default caller
  bucket.
- OTel tracing is wired through shared portfolio libraries.
- Fragments Engine and Nanite integration already uses pointer-based,
  independently owned components rather than a hard app dependency.

## Architectural tensions to resolve

### Compile currently writes before adoption

The current compiler upserts a wiki page as part of a successful job and only
then returns similarity and diff signals that a caller is supposed to use for
write-versus-stage decisions. The target must produce an immutable candidate
first and apply an explicit adoption decision before replacing the adopted
head.

### Mutable page rows erase revision lineage

`wiki_pages` represents identity, current body, output metadata, and lifecycle
in one mutable row. Content revision, adopted head, compiled artifact, output
path, and publication state need distinct identities and history.

### Wiki page as universal content type

ADR, reminder, extraction, draft, and generic directive output all become wiki
pages. The target needs a general content and artifact model with the wiki as a
declared profile rather than the fallback shape for every domain.

### Confidence overload

Current confidence is line similarity and assigns new pages `1.0`. It measures
overwrite risk, not factual trust, editorial quality, model confidence, or
publication readiness. The target names and separates these signals.

### Unknown directives execute a generic compile

Falling back from an unrecognized directive to a content write makes untrusted
or misspelled executable intent too powerful. Unknown directives should remain
inert or fail explicitly until a registered, authorized handler resolves them.

### Mutable templates lack historical definition identity

Templates are upserted by name. Historical runs need immutable template,
generator, voice, and policy revisions with publisher identity and digest.

### Source fields are pointers, not resolved evidence

`source`, `sources`, and `source_fragment_ids` retain useful provenance hints
but do not bind source revisions, digests, snapshots, freshness, sensitivity,
or transformations. The target requires source bindings and versioned packets.

### Verification mixes subject stability

Checks attach to a mutable page ID. Findings and attestations need an immutable
subject revision or artifact so later writes do not change what was verified.

### Actor fields are not principals

`generated_by`, verification `by`, and MCP `caller_id` are useful attribution
strings but do not establish identity or authorization. The target needs
verified principals and roles.

### Shared SQLite process authority

The HTTP daemon and separately spawned stdio MCP process can point at the same
SQLite file. The target must make migration ownership, concurrent writes,
leases, shutdown, and backup consistency explicit.

### Configuration mixes product and historical source concerns

The config still includes legacy source paths, draft provider settings, Nanite
details, and direct model wiring. The target separates publisher definitions,
attached resources, adapter registrations, and invocation inputs.

### Secrets are materialized into long-lived config

The current central Cerberus configuration holds a raw Anthropic key. This
works operationally but conflicts with the desired no-secret-custody boundary.
The target stores a credential reference and resolves a narrow grant at run
time.

### Current deployment language assigns supervision to Cerberus

Cerberus currently launches the local daemon. That can remain a declared
development or local deployment mode, but long-running process supervision is
an OS/runtime concern and not Loom's or Cerberus's content-domain role.

### Current UI is compiler-observability oriented

Bundles, pages, jobs, exports, and directives expose the present compiler well.
The desired product also needs authoring, briefs, source packets, revisions,
review, editorial attention, and publication intent as first-class semantics.
The UI should project the domain rather than become the only place those
transitions exist.

## Boundary guidance

A capability belongs in Loom core when it protects or expresses content
authoring, editorial operations, and compilation semantics:

- content identity and immutable revision lineage;
- briefs, source packets, review, and editorial state;
- candidate production and adoption;
- compile, validation, and materialization records;
- calendar and publication intent;
- destination bindings and receipts;
- provenance, custody, policy application, and audit.

A capability belongs in a shared library when it is reusable mechanism without
Loom business semantics:

- directive parsing;
- SQLite and migration helpers;
- standard content codecs;
- model and MCP contracts;
- plugin manifests and lifecycle;
- telemetry and structured events;
- safe filesystem materialization primitives.

A capability belongs behind an adapter when it varies by provider:

- source acquisition and knowledge lookup;
- model and agent execution;
- workflow launching;
- import/export formats;
- publication destinations;
- artifact storage, search, identity, and credentials.

A capability belongs in another application when it has its own durable domain:

- capture, fragment provenance, triage, and routing in Fragments Engine;
- knowledge and memory in Tesseract;
- workflow execution in Hadron;
- managed work in Torque;
- agent sessions in Nanite;
- gateways and messaging in Tether;
- rich interaction surfaces in Tangent;
- UI compilation in Sigil;
- infrastructure reconciliation in Cerberus.

Warning signs that Loom is crossing its boundary:

- source truth is copied without revision or provenance;
- mutable definitions reinterpret historical output;
- every content operation becomes a custom workflow engine;
- every idea or review becomes a Torque task;
- provider mechanics leak into content domain types;
- compile, agent, or workflow success automatically adopts content;
- structural conformance is presented as factual trust;
- exported or delivered is presented as destination-live;
- credentials appear in definitions, rows, logs, events, or traces;
- plugins receive ambient filesystem, network, or secret access;
- broad capture and triage are duplicated from Fragments Engine;
- wiki search is treated as portfolio memory authority;
- Loom cannot perform its core deterministic work without another app;
- Glyph and Loom remain dual authorities for new editorial records.

## Questions for the next architecture session

1. What is Loom's primary aggregate: content item, editorial work item,
   publication space, or a composition of those identities?
2. How do current bundles and pages map to publication spaces, content items,
   adopted revisions, and compiled artifacts?
3. What is the minimal general content representation beneath the wiki profile?
4. Which records are immutable revisions and which are mutable projections or
   working copies?
5. What authority and custody modes apply to native drafts, Glyph-origin
   records, imported files, source snapshots, and published renditions?
6. What source-packet contract makes generation, review, and publication
   reproducible without copying external authorities wholesale?
7. Which definition families share a registry contract, and which need
   domain-specific resolution semantics?
8. How are working edits checkpointed, branched, merged, and protected from
   simultaneous human and agent writes?
9. What explicit adoption policies replace compile-time page upsert?
10. Which comparison, conformance, quality, trust, and readiness signals are
    distinct, and which can be derived?
11. Which review decisions invalidate when content, brief, sources, generator,
    output profile, or policy changes?
12. What evidence is required before Loom reports content as published or live?
13. Which operations execute locally, through a model adapter, through Nanite,
    or through Hadron, and what stable result contract unifies them?
14. Which current direct-ingest use cases remain Loom-native versus delegated
    to Fragments Engine?
15. Which content-search and Weaver capabilities remain domain-local, and which
    results should be promoted to Tesseract?
16. Which extensions are trusted in-process code and which require isolation?
17. What local principal model preserves single-user convenience while making
    future multi-principal authorization correct?
18. What single-writer or service topology governs HTTP, hosted MCP, stdio MCP,
    migrations, background runs, and one SQLite store?
19. Which artifacts and source snapshots must be backed up with the database,
    and which are reproducible projections?
20. What stable aliases preserve Glyph and existing Loom references without
    retaining dual authority?
