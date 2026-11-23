# Scaffold reference: SUPPORT.md

## Questions to ask

Ask what is open as one ❓ round. Give each question a ➡️ recommendation drawn from the request and the repo. Skip a
topic the request already settles.

- **Primary help channel:** Microsoft Teams channel name, email alias, GitHub Discussions, etc.
- **Ticketing system,** if any: its project key, queue, or form.
- **On-call rotation or operational runbook,** if any: PagerDuty service, runbook URL.
- **SLA or response time expectations,** if any.
- **FAQ, wiki, or knowledge base** to reference, if any.

## Structure

Follow this order:

1. **Getting help:** primary support channel(s) with direct links or contact info
2. **Reporting issues:** where and how to file a bug or request a feature
3. **Runbooks and operations** _(for services/APIs)_: link to operational runbooks, monitoring dashboards, or on-call
   documentation
4. **FAQ / knowledge base** _(optional)_: link to internal wiki or FAQ page
5. **Escalation:** what to do if standard support channels don't resolve the issue

## Example output

```markdown
# Support: {repo-name}

## Getting help

For questions about {repo-name}, reach the owning team through:

- **{Channel service}:** [`#{team-channel}`]({channel URL})
- **Email:** {support email address}

## Reporting issues

- **Bugs:** Open a [GitHub Issue]({repository URL}/issues/new) and use the bug report template
- **Feature requests:** Open a [GitHub Issue]({repository URL}/issues/new) and use the feature request template
- **{Ticketing system}:** {how to file a ticket: project key, queue, or form}

## Runbooks and operations

| Resource             | Link                                |
| -------------------- | ----------------------------------- |
| Operational Runbook  | [{runbook title}]({runbook path})   |
| Monitoring Dashboard | [{dashboard name}]({dashboard URL}) |
| On-Call Rotation     | [{rotation name}]({rotation URL})   |

## Escalation

If your issue is not resolved within the expected timeframe:

1. Tag `@{team-name}` in the {channel service} channel
2. Escalate to `@{lead-name}`, the {lead role}
3. For production incidents, follow the [incident response process]({incident process path})
```

Fill every link and address from the owner's answers. Take `{repository URL}` from the `origin` remote, as
`https://github.com/{owner}/{repo}`. Keep "and use the … template" only when `.github/ISSUE_TEMPLATE/` holds that
template. Link a repository path only when that file exists. Omit a line, a table row, or an escalation step whose value
the owner did not give. Never write a placeholder URL, a `#` link, or an `example.com` address.

## Notes

- SUPPORT.md is valid in `.github/`, the repo root, or `docs/`.
    - GitHub checks those locations in that order and uses the first file it finds.
    - The idiomatic location is the **root**.
- For internal-only repos, omit public-facing contact information
- If GitHub Discussions is enabled, link to it as a support channel alternative

## Conformance check (audit mode)

✅ Conformant if the file contains at least one support contact, channel, or ticketing reference.
