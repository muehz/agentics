# Postmortem-mode references

`/agentics:ops postmortem` loads these files to run a blameless retrospective on a resolved incident.

| File                                             | Purpose                                                             |
| ------------------------------------------------ | ------------------------------------------------------------------- |
| [postmortem-workflow.md](postmortem-workflow.md) | Facilitation workflow: prepare → meet → write → review → publish    |
| [postmortem-template.md](postmortem-template.md) | The artifact produced (full markdown structure with field guidance) |

Postmortem mode works on any closed incident. The response-mode tracking issue is the typical input but is not required.
With no incident-id argument, the workflow asks for the incident reference and any pre-collected timeline notes.
