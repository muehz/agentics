# Eval pattern examples

Language-agnostic pseudocode for each eval pattern. Adapt it to your project's tooling.

**What check mode uses.** `/agentics:gate check` grades an artifact once against criteria from its contract, so it uses
only the [evaluation strategies](#evaluation-strategies) below. The three loop patterns describe code that generates and
refines its own output; they belong to that code, or to `/agentics:hillclimb` when the user wants an output improved
over repeated attempts. Check mode never runs a refine loop.

Every number in these sketches (`max_iterations=3`, `score_threshold=0.8`, the rubric weights) is a placeholder. Take
the real value from the caller's contract; never use the placeholder as a pass bar.

---

## Pattern 1: Basic reflection

```
function reflect_and_refine(task, criteria, max_iterations=3):
    output = generate(task)

    repeat up to max_iterations:
        critique = evaluate(output, criteria)
        # critique returns: { criterion: { status: "PASS"|"FAIL", feedback: "..." }, ... }

        if all criteria PASS:
            return output

        failed = { k: v.feedback for k, v in critique if v.status == "FAIL" }
        output = refine(output, failed_feedback=failed)

    return output
```

**Key:** Use structured output (e.g., JSON) for the critique so that pass/fail parses reliably.

---

## Pattern 2: Evaluator-Optimizer

```
EvaluatorOptimizer(score_threshold=0.8):

    generate(task) → output

    evaluate(output, task) → {
        overall_score: 0.0–1.0,
        dimensions: { accuracy: score, clarity: score, ... }
    }

    optimize(output, feedback) → improved_output

    run(task, max_iterations=3):
        output = generate(task)
        repeat up to max_iterations:
            evaluation = evaluate(output, task)
            if evaluation.overall_score >= threshold: stop
            output = optimize(output, evaluation)
        return output
```

**Key:** The separation of generation, evaluation, and optimization makes each component independently testable and
replaceable.

---

## Pattern 3: Output-specific reflection

Use when the output has an external correctness signal (e.g., tests, linter, build).

```
function output_reflect_and_fix(spec, max_iterations=3):
    output = generate(spec)          # "Produce output for: {spec}"
    checks = generate_checks(spec)   # "Generate verification steps for: {spec}"

    repeat up to max_iterations:
        result = run_checks(output, checks)
        if result.success: return output
        output = fix(output, error=result.error)

    return output
```

**Key:** When available, the correctness signal (tests, linter, schema validator) replaces the LLM judge. It is faster
and more reliable.

---

## Evaluation strategies

### Outcome-based

```
evaluate_outcome(task, output, expected):
    ask LLM: "Does this output achieve the expected outcome?
              Task: {task}
              Expected: {expected}
              Output: {output}
              Answer YES/NO with explanation."
```

### LLM-as-Judge

```
llm_judge(output_a, output_b, criteria):
    ask LLM: "Compare outputs A and B against this criteria: {criteria}
              Which is better and why? Return structured verdict."
```

### Rubric-based

```
rubric = { accuracy: weight=0.4, clarity: weight=0.3, completeness: weight=0.3 }

evaluate_with_rubric(output, rubric):
    scores = ask LLM: "Rate this output 1–5 on each dimension: {rubric.keys}
                       Output: {output}
                       Return structured scores."
    return weighted_average(scores, rubric.weights)
```
