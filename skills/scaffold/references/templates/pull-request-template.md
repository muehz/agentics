# Scaffold reference: .github/PULL_REQUEST_TEMPLATE.md

## Questions to ask

> ❓ **Q1** - **PR template sections**: What sections should the PR template include? Defaults: Description, Changes,
> Testing, and Checklist. Add or remove sections as needed.
>
> ➡️ Keep the defaults (Description, Changes, Testing, Checklist) unless the repo's existing PRs show a different
> pattern.

## Default output

```markdown
## Description

<!-- Describe what this PR changes and why -->

## Changes

-

## Testing

<!-- Describe how you validated these changes -->

- [ ] Ran lint/build/test locally
- [ ] No new warnings introduced

## Checklist

- [ ] PR title follows commit convention
- [ ] Documentation updated (if applicable)
- [ ] Tests added or updated
```

## Customization notes

- If the repo includes Bicep/Terraform, add infra-specific checklist items: `- [ ] Bicep validated with az bicep build`
- If the repo handles sensitive data, add security items: `- [ ] No secrets or PII included in this PR`
- Keep the template short. Contributors tend to ignore long templates.

## Conformance check (audit mode)

✅ Conformant if the file exists and is non-empty.
