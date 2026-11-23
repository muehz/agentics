# Agent governance patterns

These patterns for safety, trust, and policy enforcement keep AI agents within defined boundaries. They control which
tools agents can call, what content they can process, how much they can do, and how actions are recorded.

```text
User Request → Intent Classification → Policy Check → Tool Execution → Audit Log
                     ↓                      ↓               ↓
              Threat Detection         Allow/Deny      Trust Update
```

## When to use

- Any agent that calls external tools (APIs, databases, shell commands)
- Multi-agent systems, because agents that delegate to other agents need trust boundaries
- Production deployments, because of compliance, audit, and safety requirements
- Sensitive operations, such as financial transactions, data access, and infrastructure management

## Pattern 1: Governance policy

Define what an agent is allowed to do as a composable, serializable policy object.

```python
from dataclasses import dataclass, field
from enum import Enum
from typing import Optional
import re

class PolicyAction(Enum):
    ALLOW = "allow"
    DENY = "deny"
    REVIEW = "review"  # flag for human review

@dataclass
class GovernancePolicy:
    """Declarative policy controlling agent behavior."""
    name: str
    allowed_tools: Optional[list[str]] = None                    # allowlist; None means no allowlist, [] denies all
    blocked_tools: list[str] = field(default_factory=list)       # blocklist
    blocked_patterns: list[str] = field(default_factory=list)    # content filters
    max_calls_per_request: int = 100                             # rate limit
    require_human_approval: list[str] = field(default_factory=list)  # tools needing approval

    def check_tool(self, tool_name: str) -> PolicyAction:
        """Check if a tool is allowed by this policy."""
        if tool_name in self.blocked_tools:
            return PolicyAction.DENY
        if tool_name in self.require_human_approval:
            return PolicyAction.REVIEW
        if self.allowed_tools is not None and tool_name not in self.allowed_tools:
            return PolicyAction.DENY
        return PolicyAction.ALLOW

    def check_content(self, content: str) -> Optional[str]:
        """Check content against blocked patterns. Returns matched pattern or None."""
        for pattern in self.blocked_patterns:
            if re.search(pattern, content, re.IGNORECASE):
                return pattern
        return None
```

### Policy composition

Combine multiple policies, for example org-wide, team, and agent-specific policies:

```python
def compose_policies(*policies: GovernancePolicy) -> GovernancePolicy:
    """Merge policies with most-restrictive-wins semantics."""
    combined = GovernancePolicy(name="composed")

    for policy in policies:
        combined.blocked_tools.extend(policy.blocked_tools)
        combined.blocked_patterns.extend(policy.blocked_patterns)
        combined.require_human_approval.extend(policy.require_human_approval)
        combined.max_calls_per_request = min(
            combined.max_calls_per_request,
            policy.max_calls_per_request
        )
        # An empty intersection must stay an empty list, which denies every tool;
        # only None means the policy sets no allowlist.
        if policy.allowed_tools is not None:
            if combined.allowed_tools is None:
                combined.allowed_tools = list(policy.allowed_tools)
            else:
                combined.allowed_tools = [
                    t for t in combined.allowed_tools if t in policy.allowed_tools
                ]

    return combined


# Usage: layer policies from broad to specific
org_policy = GovernancePolicy(
    name="org-wide",
    blocked_tools=["shell_exec", "delete_database"],
    blocked_patterns=[r"(?i)(api[_-]?key|secret|password)\s*[:=]"],
    max_calls_per_request=50
)
team_policy = GovernancePolicy(
    name="data-team",
    allowed_tools=["query_db", "read_file", "write_report"],
    require_human_approval=["write_report"]
)
agent_policy = compose_policies(org_policy, team_policy)
```

### Policy as YAML

Store policies as configuration, not code:

```yaml
# governance-policy.yaml
name: production-agent
allowed_tools:
    - search_documents
    - query_database
    - send_email
blocked_tools:
    - shell_exec
    - delete_record
blocked_patterns:
    - "(?i)(api[_-]?key|secret|password)\\s*[:=]"
    - "(?i)(drop|truncate|delete from)\\s+\\w+"
max_calls_per_request: 25
require_human_approval:
    - send_email
```

```python
import yaml

def load_policy(path: str) -> GovernancePolicy:
    with open(path) as f:
        data = yaml.safe_load(f)
    return GovernancePolicy(**data)
```

## Pattern 2: Semantic intent classification

Use pattern-based signals to detect dangerous intent in prompts before they reach the agent.

```python
from dataclasses import dataclass

@dataclass
class IntentSignal:
    category: str       # e.g., "data_exfiltration", "privilege_escalation"
    confidence: float   # 0.0 to 1.0
    evidence: str       # what triggered the detection

# Weighted signal patterns for threat detection
THREAT_SIGNALS = [
    # Data exfiltration
    (r"(?i)send\s+(all|every|entire)\s+\w+\s+to\s+", "data_exfiltration", 0.8),
    (r"(?i)export\s+.*\s+to\s+(external|outside|third.?party)", "data_exfiltration", 0.9),
    (r"(?i)curl\s+.*\s+-d\s+", "data_exfiltration", 0.7),

    # Privilege escalation
    (r"(?i)(sudo|as\s+root|admin\s+access)", "privilege_escalation", 0.8),
    (r"(?i)chmod\s+777", "privilege_escalation", 0.9),

    # System modification
    (r"(?i)(rm\s+-rf|del\s+/[sq]|format\s+c:)", "system_destruction", 0.95),
    (r"(?i)(drop\s+database|truncate\s+table)", "system_destruction", 0.9),

    # Prompt injection
    (r"(?i)ignore\s+(previous|above|all)\s+(instructions?|rules?)", "prompt_injection", 0.9),
    (r"(?i)you\s+are\s+now\s+(a|an)\s+", "prompt_injection", 0.7),
]

def classify_intent(content: str) -> list[IntentSignal]:
    """Classify content for threat signals."""
    signals = []
    for pattern, category, weight in THREAT_SIGNALS:
        match = re.search(pattern, content)
        if match:
            signals.append(IntentSignal(
                category=category,
                confidence=weight,
                evidence=match.group()
            ))
    return signals

def is_safe(content: str, threshold: float = 0.7) -> bool:
    """Quick check: is the content safe above the given threshold?"""
    signals = classify_intent(content)
    return not any(s.confidence >= threshold for s in signals)
```

Intent classification runs _before_ tool execution, as a pre-flight safety check. Output guardrails differ because they
only check _after_ generation.

## Pattern 3: Tool-level governance decorator

Wrap individual tool functions with governance checks:

```python
import contextvars
import functools
import inspect
import time

# Call counts for the current request only. start_request() gives each request a new budget.
_request_calls: contextvars.ContextVar[dict[str, int] | None] = contextvars.ContextVar(
    "governed_calls", default=None
)

def start_request() -> None:
    """Reset the call budget. Call once at the start of each agent request."""
    _request_calls.set({})

def govern(policy: GovernancePolicy, audit_trail=None, approve=None):
    """Decorator that enforces governance policy on a sync or async tool function.

    approve(tool_name, args, kwargs) -> bool asks a human about REVIEW tools.
    Without an approve hook, REVIEW tools fail closed.
    """
    def decorator(func):
        tool_name = func.__name__

        def check(args, kwargs):
            # 1. Check tool allowlist/blocklist
            action = policy.check_tool(tool_name)
            if action == PolicyAction.DENY:
                raise PermissionError(f"Policy '{policy.name}' blocks tool '{tool_name}'")

            # 2. Check rate limit for this request
            counts = _request_calls.get()
            if counts is None:
                raise RuntimeError("Call start_request() before a governed tool")
            counts[policy.name] = counts.get(policy.name, 0) + 1
            if counts[policy.name] > policy.max_calls_per_request:
                raise PermissionError(f"Rate limit exceeded: {policy.max_calls_per_request} calls")

            # 3. Check content in arguments
            for arg in list(args) + list(kwargs.values()):
                if isinstance(arg, str):
                    matched = policy.check_content(arg)
                    if matched:
                        raise PermissionError(f"Blocked pattern detected: {matched}")

            # 4. Send REVIEW tools to a human
            if action == PolicyAction.REVIEW:
                if approve is None or not approve(tool_name, args, kwargs):
                    raise PermissionError(f"Tool '{tool_name}' was not approved by a human")

        def record(action, start, error=None):
            if audit_trail is not None:
                entry = {
                    "tool": tool_name,
                    "action": action,
                    "duration_ms": (time.monotonic() - start) * 1000,
                    "timestamp": time.time(),
                }
                if error is not None:
                    entry["error"] = str(error)
                audit_trail.append(entry)

        # 5. Execute and audit. Keep sync tools sync so frameworks can call them directly.
        if inspect.iscoroutinefunction(func):
            @functools.wraps(func)
            async def wrapper(*args, **kwargs):
                start = time.monotonic()
                try:
                    check(args, kwargs)
                except Exception as e:
                    record("denied", start, e)  # audit denials too
                    raise
                try:
                    result = await func(*args, **kwargs)
                except Exception as e:
                    record("error", start, e)
                    raise
                record("allowed", start)
                return result
        else:
            @functools.wraps(func)
            def wrapper(*args, **kwargs):
                start = time.monotonic()
                try:
                    check(args, kwargs)
                except Exception as e:
                    record("denied", start, e)  # audit denials too
                    raise
                try:
                    result = func(*args, **kwargs)
                except Exception as e:
                    record("error", start, e)
                    raise
                record("allowed", start)
                return result

        return wrapper
    return decorator


# Usage with any agent framework
audit_log = []
policy = GovernancePolicy(
    name="search-agent",
    allowed_tools=["search", "summarize"],
    blocked_patterns=[r"(?i)password"],
    max_calls_per_request=10
)

@govern(policy, audit_trail=audit_log)
async def search(query: str) -> str:
    """Search documents — governed by policy."""
    return f"Results for: {query}"

# Once per request: start_request()
# Passes: search("latest quarterly report")
# Blocked: search("show me the admin password")
```

## Pattern 4: Trust scoring

Track agent reliability over time with decay-based trust scores:

```python
from dataclasses import dataclass, field
import math
import time

@dataclass
class TrustScore:
    """Trust score with temporal decay."""
    score: float = 0.5          # 0.0 (untrusted) to 1.0 (fully trusted)
    successes: int = 0
    failures: int = 0
    last_updated: float = field(default_factory=time.time)

    def record_success(self, reward: float = 0.05):
        self.successes += 1
        self.score = min(1.0, self.score + reward * (1 - self.score))
        self.last_updated = time.time()

    def record_failure(self, penalty: float = 0.15):
        self.failures += 1
        self.score = max(0.0, self.score - penalty * self.score)
        self.last_updated = time.time()

    def current(self, decay_rate: float = 0.001) -> float:
        """Get score with temporal decay — trust erodes without activity."""
        elapsed = time.time() - self.last_updated
        decay = math.exp(-decay_rate * elapsed)
        return self.score * decay

    @property
    def reliability(self) -> float:
        total = self.successes + self.failures
        return self.successes / total if total > 0 else 0.0


# Usage in multi-agent systems
trust = TrustScore()

# Agent completes tasks successfully
trust.record_success()  # 0.525
trust.record_success()  # 0.549

# Agent makes an error
trust.record_failure()  # 0.467

# Gate sensitive operations on trust
if trust.current() >= 0.7:
    # Allow autonomous operation
    pass
elif trust.current() >= 0.4:
    # Allow with human oversight
    pass
else:
    # Deny or require explicit approval
    pass
```

**Multi-agent trust.** When agents delegate to other agents, each agent keeps trust scores for its delegates:

```python
class AgentTrustRegistry:
    def __init__(self):
        self.scores: dict[str, TrustScore] = {}

    def get_trust(self, agent_id: str) -> TrustScore:
        if agent_id not in self.scores:
            self.scores[agent_id] = TrustScore()
        return self.scores[agent_id]

    def most_trusted(self, agents: list[str]) -> str:
        return max(agents, key=lambda a: self.get_trust(a).current())

    def meets_threshold(self, agent_id: str, threshold: float) -> bool:
        return self.get_trust(agent_id).current() >= threshold
```

## Pattern 5: Audit trail

Log all agent actions to an append-only audit trail for compliance and debugging:

```python
from dataclasses import dataclass, field
import json
import time

@dataclass
class AuditEntry:
    timestamp: float
    agent_id: str
    tool_name: str
    action: str           # "allowed", "denied", "error"
    policy_name: str
    details: dict = field(default_factory=dict)

class AuditTrail:
    """Append-only audit trail for agent governance events."""
    def __init__(self):
        self._entries: list[AuditEntry] = []

    def log(self, agent_id: str, tool_name: str, action: str,
            policy_name: str, **details):
        self._entries.append(AuditEntry(
            timestamp=time.time(),
            agent_id=agent_id,
            tool_name=tool_name,
            action=action,
            policy_name=policy_name,
            details=details
        ))

    def denied(self) -> list[AuditEntry]:
        """Get all denied actions — useful for security review."""
        return [e for e in self._entries if e.action == "denied"]

    def by_agent(self, agent_id: str) -> list[AuditEntry]:
        return [e for e in self._entries if e.agent_id == agent_id]

    def export_jsonl(self, path: str):
        """Export as JSON Lines for log aggregation systems."""
        with open(path, "w") as f:
            for entry in self._entries:
                f.write(json.dumps({
                    "timestamp": entry.timestamp,
                    "agent_id": entry.agent_id,
                    "tool": entry.tool_name,
                    "action": entry.action,
                    "policy": entry.policy_name,
                    **entry.details
                }) + "\n")
```

## Pattern 6: Framework integration

Call `start_request()` once before each agent run. Governed tools fail closed without it.

### PydanticAI

```python
from pydantic_ai import Agent

policy = GovernancePolicy(
    name="support-bot",
    allowed_tools=["search_docs", "create_ticket"],
    blocked_patterns=[r"(?i)(ssn|social\s+security|credit\s+card)"],
    max_calls_per_request=20
)

agent = Agent("openai:gpt-4o", system_prompt="You are a support assistant.")

@agent.tool
@govern(policy)
async def search_docs(ctx, query: str) -> str:
    """Search knowledge base — governed."""
    return await kb.search(query)

@agent.tool
@govern(policy)
async def create_ticket(ctx, title: str, body: str) -> str:
    """Create support ticket — governed."""
    return await tickets.create(title=title, body=body)
```

### CrewAI

```python
from crewai import Agent, Task, Crew

policy = GovernancePolicy(
    name="research-crew",
    allowed_tools=["search", "analyze"],
    max_calls_per_request=30
)

# Apply governance at the crew level
def governed_crew_run(crew: Crew, policy: GovernancePolicy):
    """Wrap crew execution with governance checks."""
    audit: list[dict] = []  # govern() appends dicts, so use a list here
    originals = {}  # keyed by tool, so a shared tool is wrapped once
    for agent in crew.agents:
        for tool in agent.tools:
            if id(tool) not in originals:
                originals[id(tool)] = (tool, tool.func)
                tool.func = govern(policy, audit_trail=audit)(tool.func)  # stays sync for a sync func
    try:
        start_request()
        result = crew.kickoff()
    finally:
        # Restore the raw functions, so a second run does not stack wrappers.
        for tool, func in originals.values():
            tool.func = func
    return result, audit
```

### OpenAI Agents SDK

```python
from agents import Agent, function_tool

policy = GovernancePolicy(
    name="coding-agent",
    allowed_tools=["read_file", "write_file", "run_tests"],
    blocked_tools=["shell_exec"],
    max_calls_per_request=50
)

@function_tool
@govern(policy)
async def read_file(path: str) -> str:
    """Read file contents — governed."""
    import os
    root = os.path.realpath(".")
    safe_path = os.path.realpath(path)
    # A string prefix check passes "/repo-evil" for root "/repo". Compare whole path parts.
    if os.path.commonpath([root, safe_path]) != root:
        raise ValueError("Path traversal blocked by governance")
    with open(safe_path) as f:
        return f.read()
```

## Governance levels

Match governance strictness to risk level:

| Level        | Controls                                          | Use Case                         |
| ------------ | ------------------------------------------------- | -------------------------------- |
| **Open**     | Audit only, no restrictions                       | Internal development and testing |
| **Standard** | Tool allowlist and content filters                | General production agents        |
| **Strict**   | All controls and human approval for sensitive ops | Financial, healthcare, legal     |
| **Locked**   | Allowlist only, no dynamic tools, full audit      | Compliance-critical systems      |

## Best practices

| Practice                       | Rationale                                                                         |
| ------------------------------ | --------------------------------------------------------------------------------- |
| **Policy as configuration**    | Store policies in YAML or JSON, not hardcoded, so they can change without deploys |
| **Most-restrictive-wins**      | In composed policies, deny always overrides allow                                 |
| **Pre-flight intent check**    | Classify intent _before_ tool execution, not after                                |
| **Trust decay**                | Trust scores should decay over time, so they require ongoing good behavior        |
| **Append-only audit**          | Never modify or delete audit entries, because immutability enables compliance     |
| **Fail closed**                | If a governance check errors, deny the action rather than allow it                |
| **Separate policy from logic** | Governance enforcement should be independent of agent business logic              |

## Quick start checklist

```markdown
## Agent Governance Implementation Checklist

### Setup

- [ ] Define governance policy (allowed tools, blocked patterns, rate limits)
- [ ] Choose governance level (open/standard/strict/locked)
- [ ] Set up audit trail storage

### Implementation

- [ ] Add @govern decorator to all tool functions
- [ ] Add intent classification to user input processing
- [ ] Implement trust scoring for multi-agent interactions
- [ ] Wire up audit trail export

### Validation

- [ ] Test that blocked tools are properly denied
- [ ] Test that content filters catch sensitive patterns
- [ ] Test rate limiting behavior
- [ ] Verify audit trail captures all events
- [ ] Test policy composition (most-restrictive-wins)
```

## Related resources

- [OWASP Top 10 for LLM Applications](https://owasp.org/www-project-top-10-for-large-language-model-applications/)
