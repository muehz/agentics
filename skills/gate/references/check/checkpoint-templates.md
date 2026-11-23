# Checkpoint templates

## Checkpoint 1: Input Validation

Used immediately after you parse the task's inputs (command-line arguments, a config file, environment variables, or the
parameters in a request):

```markdown
✅ CHECKPOINT 1: Input Validation

**Purpose**: Ensure all required inputs are present and valid before running anything that depends on them

**Validation Criteria**:

- [ ] All REQUIRED inputs are present
- [ ] All inputs match their format requirements (paths exist, ids and versions parse)
- [ ] All inputs pass domain-specific validation rules (allowed values, ranges)
- [ ] Optional inputs are defaulted if not provided
- [ ] Cross-input validation passed (e.g., start before end, source and target differ)

**IF ANY UNCHECKED**: → Report the failure to the user with the specific input name → Provide correction guidance
(format, allowed values) → STOP workflow until the input is corrected

**IF ALL CHECKED**: → Proceed to the next phase
```

## Checkpoint 2: Command Success

Used after you run a build, test suite, migration, script, or API call:

```markdown
✅ CHECKPOINT 2: Command Success

**Purpose**: Verify the command ran to completion and produced what its contract promises

**Validation Criteria**:

- [ ] Exit code is the expected one (usually 0)
- [ ] The output or artifact the command's contract promises exists (a silent success with no promised output passes)
- [ ] Where the contract promises output: it contains the expected fields or structure
- [ ] Where the contract states a result count: the count is within that range
- [ ] No timeout, crash, or connection errors (for an API call: no 401/403/404/429/500)

**IF ANY UNCHECKED**: → Read the error output before you retry anything → Retry only when the failure is transient (rate
limit, timeout, server error) → Request user verification if an input looks invalid (404 on a path or id)

**IF ALL CHECKED**: → Proceed to CHECKPOINT 3: Output Validation
```

## Checkpoint 3: Output Validation

Used after you receive output from a command or an API, or after a script writes a file:

```markdown
✅ CHECKPOINT 3: Output Validation

**Purpose**: Ensure the output meets quality standards and expected patterns before the next step consumes it

**Validation Criteria**:

- [ ] Record count matches the expected range or the count in the source
- [ ] All required fields present (the schema or type definition names them)
- [ ] No null/missing values in critical fields
- [ ] Values fall inside the documented bounds (dates, enums, ids)
- [ ] Data types correct (numbers are numeric, dates are valid)
- [ ] No anomalies (e.g., future timestamps, negative counts, duplicate keys)

**Validation Steps**:

1. Count the records in the output
2. Check the first 3 records for field completeness
3. Verify values against the bounds the input parameters or schema set
4. Check for duplicates and suspicious values

**IF ANY UNCHECKED**: → Re-run with corrected inputs → Investigate the code or source that produced the bad output →
Request user confirmation if the data looks suspicious → Log the issues for debugging

**IF ALL CHECKED**: → Proceed to CHECKPOINT 4: Transformation Validation
```

## Checkpoint 4: Transformation Validation

Used after a transformation: a migration, code generation, a mechanical rewrite, or a computed metric:

```markdown
✅ CHECKPOINT 4: Transformation Validation

**Purpose**: Verify the transformed result is accurate and within expected ranges

**Validation Criteria**:

- [ ] The transformation completed without errors
- [ ] Results are within logical ranges (e.g., 0-100% for percentages)
- [ ] Totals match the sum of their components
- [ ] The record count before and after matches, or the difference is explained (rows dropped, rows split)
- [ ] No division by zero, NaN, or infinity values
- [ ] Rounding applied consistently
- [ ] Derived values align with the source data

**Validation Steps**:

1. Verify the transformation was applied as specified
2. Check for mathematical errors (NaN, Infinity)
3. Validate percentages sum to 100% (where applicable)
4. Cross-check a sample of derived values against the source data

**IF ANY UNCHECKED**: → Correct the transformation and re-run it → Investigate the input anomalies that caused the
errors → Request user review of unexpected results

**IF ALL CHECKED**: → Proceed to CHECKPOINT 5: Handoff Readiness
```

## Checkpoint 5: Handoff Readiness

Used before you produce the artifact the next phase consumes: a commit, a pull request, a release, or a generated
report:

```markdown
✅ CHECKPOINT 5: Handoff Readiness

**Purpose**: Ensure everything the artifact needs is present and valid before you produce it

**Validation Criteria**:

- [ ] All required sections or files are populated
- [ ] Every value the template needs is available
- [ ] No null/undefined values in the data the artifact embeds
- [ ] Charts or tables have at least the number of data points the artifact's spec or template requires
- [ ] The template or source file exists and is readable
- [ ] The output location is writable

**Validation Steps**:

1. Check that each section or file has non-empty content
2. Verify every template placeholder can be filled
3. Validate that chart or table data meets the required minimum, if the spec or template states one
4. Test write access to the output location

**IF ANY UNCHECKED**: → Identify the missing sections → Request user input for the missing values → Report which
optional sections you left out, and why → Log template or file system errors

**IF ALL CHECKED**: → Proceed to produce the artifact → Fill the template with the validated data
```

## Worked example: output of a route-listing script

A script `scripts/list-routes` writes `routes.json`, an array of `{ "method", "path", "handler" }` objects, one for each
route the app registers. The next task generates API docs from that file. The checkpoint runs after the script writes
the file and before the docs task starts. The router source registers 42 routes (counted from the source, not from the
script's own output).

```markdown
✅ CHECKPOINT 3: Output Validation (routes.json)

**Purpose**: Ensure routes.json is complete and well-formed before the docs generator reads it

**Validation Criteria**:

- [ ] Route count matches the router source (42)
- [ ] Every entry has method, path, and handler
- [ ] Every method is one of GET, POST, PUT, PATCH, DELETE
- [ ] No method and path pair appears twice

**Validation Steps**:

1. **Count Validation**:
    - Check: `jq length routes.json`
    - Expected: 42
    - Actual: 42
    - Status: ✅ PASS

2. **Field Completeness**:
    - Check: `jq '[.[] | select(.method == null or .path == null or .handler == null)] | length' routes.json`
    - Expected: 0
    - Actual: 0
    - Status: ✅ PASS

3. **Method Validity**:
    - Check: `jq '[.[] | select(.method | IN("GET","POST","PUT","PATCH","DELETE") | not)] | length' routes.json`
    - Expected: 0
    - Actual: 0
    - Status: ✅ PASS

4. **Duplicate Check**:
    - Check: `jq 'group_by([.method, .path]) | map(select(length > 1)) | length' routes.json`
    - Expected: 0
    - Actual: 0
    - Status: ✅ PASS

**Checkpoint Status**:

- [x] Route count matches the router source (42)
- [x] Every entry has method, path, and handler
- [x] Every method is valid
- [x] No duplicate method and path pair

**IF ALL CHECKED**: Proceed to the docs generation task
```
