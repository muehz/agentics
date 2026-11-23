# Compliance audit procedures

This procedure is an automated audit of the repository's assets (skills and agents) for conformance to the repository's
own standards and conventions. It scores the results and produces a structured findings report. The parent
[SKILL.md](../../SKILL.md) holds the core authoring framework.

The requirements come from the repository's own standards: its contributing guide, editorial rules, and conventions
documents. Name the sources you loaded in the report. Apply the rules below. Waive any rule that those sources
contradict.

## Quick start

1. Run the audit against the repository.
2. Review the generated compliance report.
3. Triage findings by severity. Open remediation work items.

## Full 5-phase workflow

### Phase 1: Discover scope

- Glob skills: where the repository keeps them (for example `skills/*/SKILL.md`).
- Glob agents: where the repository keeps its agent files (for example `agents/*.agent.md`).
- Load the repository's standards sources.
- If the repository has a registration source, identify it (see Phase 3).
- Report the count discovered in each category:

```text
Audit Scope:
  Skills       : {n}
  Agents       : {n}
```

Keep the file lists for later phases.

### Phase 2: Validate front matter and naming

For each Phase 1 asset, validate the YAML front matter.

**Step 2a: Parse front matter.**

- Confirm that the file opens with a valid, parseable YAML front-matter block.
- If the block is missing or fails to parse, record a **Critical** finding (`C-01`).

**Step 2b: Validate `name` (skills).**

- Confirm that `name` is present, lowercase kebab-case, and matches the skill's directory (`skills/<name>/`).
- The `name` field is the deduplication key. A mismatch silently shadows another skill or is shadowed by it.
- Record a **Critical** finding on mismatch (`C-02`).

**Step 2c: Validate `description`.**

- Confirm that `description` is present, written in the third person, and phrased as an unambiguous trigger.
- The description should begin with "Use when …". The "Use when …" trigger form counts as third person.
- Vague or first-person descriptions blur routing. Record a **Major** finding (`C-03`).

**Step 2d: Validate surface declaration.**

- Confirm that `metadata.surfaces` is declared. Every asset states its surfaces in front matter rather than let breakage
  reveal them.
- If `metadata.surfaces` is absent, record a **Minor** finding (`C-06`).

### Phase 3: Validate registration integrity

Cross-reference each asset against the registration source of truth the repository uses.

**Step 3a: Find the registration source.**

- Discover how the repository registers its skills and agents.
- The repository can list them in a plugin or marketplace manifest, or in a registry or index file. It can also register
  nothing at all.
- Name the source in the report.
- If the repository registers nothing, do these steps:
    1. Waive `C-04` and `C-05` as not-applicable.
    2. Continue to Phase 4.

**Step 3b: Registered on disk.**

- Confirm that each skill and each agent found on disk appears in the registration source.
- Record a **Major** finding for an unregistered skill or agent (`C-04`).

**Step 3c: No dangling registration.**

- Confirm that every entry in the registration source resolves to an existing file or directory on disk.
- A registered path with no file can make the plugin fail to load. Record a **Critical** finding (`C-05`).

### Phase 4: Validate structure and conventions

For each asset, check adherence to the structural conventions.

**Step 4a: Reference content loads on demand.**

- Confirm that long-form material lives in `references/` and the `SKILL.md` remains an entry point rather than an
  always-loaded monolith.
- Record a **Minor** finding (`C-07`) for an oversized SKILL.md that inlines reference-grade detail.

**Step 4b: Cross-platform examples.**

- Confirm that command examples follow the target project's existing tooling or are clearly illustrative. Record a
  **Minor** finding (`C-08`).

**Step 4c: Link integrity.**

- Confirm that every relative link in the asset and its `references/` resolves to an existing file.
- Dangling links are the most common form of rot. Record a **Major** finding for each broken link (`C-09`).

### Phase 5: Score and report

**Step 5a: Score per asset.**

Each conformance check (see [Conformance Rules](#conformance-rules)) either passes, fails, or is waived as
not-applicable. For example, C-08 is waived when an asset contains no shell commands, and C-04 and C-05 are waived when
the repository registers nothing.

```text
Asset Score = (Count of checks passed) / (Count of applicable checks)
```

If every check is waived, report the asset as `N/A`.

**Step 5b: Score per asset type.**

```text
Type Score = (Sum of asset scores in the type) / (Count of scored assets in the type)
```

Report a score for each of Skills and Agents.

**Step 5c: Calculate the overall score.**

```text
Overall Score = (Sum of all asset scores) / (Count of scored assets)
```

**Step 5d: Apply threshold ratings.**

| Score Range | Rating | Color |
| ----------- | ------ | ----- |
| 90–100%     | Green  | 🟢    |
| 70–89%      | Yellow | 🟡    |
| 50–69%      | Orange | 🟠    |
| Below 50%   | Red    | 🔴    |

**Step 5e: Generate the report.**

Use the output format below.

## Output format

Produce the audit report as one markdown document with this structure:

```markdown
# Conformance Audit Report

**Audit date:** {YYYY-MM-DD} **Scope:** {count} skills, {count} agents **Auditor:** skills/forge (automated;
compliance-audit-procedures reference) **Standards:** {standards sources loaded}

---

## Summary Scorecard

| Metric             | Value                                               |
| ------------------ | --------------------------------------------------- |
| **Overall Score**  | {score}% {rating_emoji} {rating_label}              |
| **Assets Audited** | {count}                                             |
| **Findings**       | {critical} Critical · {major} Major · {minor} Minor |

## Per-Type Breakdown

| Asset Type | Count | Score    | Rating  |
| ---------- | ----- | -------- | ------- |
| Skills     | {n}   | {score}% | {emoji} |
| Agents     | {n}   | {score}% | {emoji} |

## Findings

### Critical

| #   | Check  | Artifact    | Description   |
| --- | ------ | ----------- | ------------- |
| 1   | {C-0n} | {file_path} | {description} |

### Major

| #   | Check  | Artifact    | Description   |
| --- | ------ | ----------- | ------------- |
| 1   | {C-0n} | {file_path} | {description} |

### Minor

| #   | Check  | Artifact    | Description   |
| --- | ------ | ----------- | ------------- |
| 1   | {C-0n} | {file_path} | {description} |

## Recommendations

1. {Top recommendation based on highest-severity findings}
2. {Next recommendation}
3. {Next recommendation}

---

_Generated by the `forge` governance audit — standards: {standards sources loaded}._
```

## Severity definitions

- **Critical.** The asset cannot load or route correctly.
- **Major.** The asset loads but violates a routing or integrity rule.
- **Minor.** A convention or structural gap.

The [Conformance rules](#conformance-rules) table assigns the severity for each check.

## Conformance rules

Apply these rules (C-01 through C-09). Waive any rule that does not apply to the asset. Exclude each waived rule from
the denominator.

| Rule | Check                                                                                                                      | Severity     |
| ---- | -------------------------------------------------------------------------------------------------------------------------- | ------------ |
| C-01 | The asset opens with a valid, parseable YAML front-matter block.                                                           | **Critical** |
| C-02 | `name` (skills) is present, lowercase kebab-case, and matches the directory `skills/<name>/`; it is the deduplication key. | **Critical** |
| C-03 | `description` is present, third-person, and an unambiguous trigger that opens "Use when …" (this counts as third person).  | **Major**    |
| C-04 | Each skill and agent on disk is listed in the repository's registration source (a manifest, registry, or index).           | **Major**    |
| C-05 | Every entry in the repository's registration source resolves to an existing file or directory on disk.                     | **Critical** |
| C-06 | `metadata.surfaces` is declared.                                                                                           | **Minor**    |
| C-07 | Long-form content lives in `references/`; the `SKILL.md` stays an entry point, not a monolith.                             | **Minor**    |
| C-08 | Command examples follow the target project's existing tooling or are clearly illustrative.                                 | **Minor**    |
| C-09 | Every relative link in the asset and its `references/` resolves to an existing file.                                       | **Major**    |
