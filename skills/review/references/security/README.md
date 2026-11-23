# Security references

This folder holds every security review reference this skill uses. Three consumers load files from here.

- **`post` mode, subagent #8.** One security reviewer inside the PR review pipeline. Brief:
  [../post/security-reviewer.md](../post/security-reviewer.md).
- **`axes` mode, Security lane, full depth.** Six parallel domain reviewers and batched scorers. Orchestration:
  [security-axis.md](security-axis.md).
- **`axes` mode, Security lane at `--fast`.** One reviewer over eleven high-signal categories, for a pre-PR check.
    - Workflow: [security-fast-scan.md](security-fast-scan.md).
    - `gate` calls this depth.

All three score on one rubric with one default threshold (75) and differ in how much they read.

| File                                                       | Holds                                                                                                                                                            |
| ---------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [security-principles.md](security-principles.md)           | Priority hierarchy, severity taxonomy and its shared mapping, scope discipline, false-positive suppression, what a clean result means, the `--composed` envelope |
| [security-scoring.md](security-scoring.md)                 | The five-anchor confidence rubric, the threshold, scope for reachability, calibration anchors, and batched scoring                                               |
| [security-findings-format.md](security-findings-format.md) | How a security finding is written wherever it appears, the tally, and the limits statement                                                                       |
| [security-axis.md](security-axis.md)                       | The six-domain fan-out that `axes` mode runs at full depth, with its dispatch count                                                                              |
| [security-fast-scan.md](security-fast-scan.md)             | The eleven-category single-reviewer sweep that `--fast` runs                                                                                                     |
| [domains/](domains/)                                       | One checklist per domain, loaded on demand                                                                                                                       |

**Read [security-principles.md](security-principles.md) first, in every mode.** It holds the rule that governs every
file here, and it wins over a domain checklist where they conflict. A security result says what one reading found, what
ran and what was not read. It never reports a clean result as an assurance.
