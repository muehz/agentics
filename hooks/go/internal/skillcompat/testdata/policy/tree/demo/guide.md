# Demo guide

Where the repository sets `policy.draft_prs` to false, mark the pull request ready:

```bash
gh pr ready 1
```

A setting that does not exist: `policy.not_a_key`.

The tiers are described in `references/checks/checks-tier-policy.md`.

A checker of another tool:

```python
# The rule is named `policy.check_tool`.
policy.check_tool
```
