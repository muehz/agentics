# Agentics

This glossary records domain terms as Agentics designs settle. The terms below describe what the guards report, what the
shell parser refuses, and what the flow commands read: the rows of a work folder's `state.tsv`, its approved artifacts,
and the clone's flow ledger.

## Language

**Threat**: A command-guardian detection with a category, severity, match, and suggestion.

**Fixed safety rule**: A guard rule with no governing workflow-policy setting.

**Workflow policy**: A guard rule governed by an Agentics hook settings key.

**Threat report**: The retained threats and resulting outcome, diagnostics, and log evidence for one proposed tool call.

**Refusal**: The reason the parser will not vouch for an invocation, from a closed set.

**Tool call**: One request from a host to run a tool, with its argument layers and environment overrides.

**Inspection**: What a guard may judge about a shell tool call: its invocations, script evidence, and authority.

**Evidence cell**: The eighth column of a flow row, as quoted text or tokens.

**Trail**: The rows of a flow with their validity and this clone's attestation.

**Located flow**: One work folder resolved into every spelling the commands need.

## Relationships

- A **Threat report** contains zero or more retained **Threats**, in rule-reporting order.
- A **Workflow policy** contributes no retained **Threat** when its governing setting is disabled.
- A **Fixed safety rule** remains effective regardless of workflow-policy settings.
- An **Inspection** is read from one tool call per check, and a **Refusal** is what it gives for an invocation a guard
  must not judge as read.

## Example dialogue

> **Developer:** If a fixed safety rule and a workflow policy both block a call, which names the denial? **Maintainer:**
> The fixed blocking threats determine the denial reason, while the count includes every retained blocking threat. Warn
> mode selects from all retained threats.

## Flagged ambiguities

None currently recorded.
