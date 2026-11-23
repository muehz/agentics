# Security Policy

## Supported versions

Agentics releases directly from `main`. Security fixes apply only to the current version on `main`.

| Version                   | Supported |
| ------------------------- | --------- |
| Current version on `main` | Yes       |
| Earlier versions          | No        |

Use the [verified package guide](docs/guide/01-setup.md) to update to the current release before reporting a
vulnerability that might already be fixed. Keep the archive and runtime checksum checks enabled.

## Report a vulnerability

Do not report security vulnerabilities in a public issue.

Submit a [private vulnerability report](https://github.com/muehz/agentics/security/advisories/new) with the following
information:

- The affected Agentics version or commit
- The affected provider surface and operating system
- Steps to reproduce the vulnerability
- The potential impact
- Any known mitigation or workaround

If private vulnerability reporting is unavailable, open a public issue without sensitive details and ask the maintainer
for a private security contact.

The maintainer will acknowledge the report, investigate it, and coordinate any fix and disclosure with the reporter.
