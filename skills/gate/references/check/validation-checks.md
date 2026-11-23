# Data integrity verification: detailed templates

These templates check structured data that code reads or produces: parsed config, fixtures, JSON from a command or an
API, a generated file, or the rows a migration touched. Check mode inspects the data and reports. It does not rewrite
it. A fix belongs to the code that produced the data, or to the user's decision.

---

## Pattern

### Step 1: Null/undefined check

```markdown
### Data Integrity Check 1: Null/Undefined Validation

**Purpose**: Ensure all critical fields have values

**Critical Fields**:

- `{field_name_1}`: {expected_type}
- `{field_name_2}`: {expected_type}
- `{field_name_n}`: {expected_type}

**Validation**:

- Check each critical field for null/undefined/empty values
- Count null occurrences per field
- Identify records with missing critical data

**Results**:

- Total records: {record_count}
- Records with null in {field_name_1}: {null_count_1}
- Records with null in {field_name_2}: {null_count_2}
- Records with null in {field_name_n}: {null_count_n}

**Status**: ✅ PASS (0 nulls) / ⚠️ WARNING (some nulls in optional fields) / ❌ FAIL (nulls in critical fields)

**IF FAIL**: → List the records with null critical fields (by id or position) → Report them to the user → Request a fix
to the code that produced them, or the user's decision to proceed without them
```

### Step 2: Range check

```markdown
### Data Integrity Check 2: Range Validation

**Purpose**: Ensure numerical values are within logical bounds

**Range Rules**:

1. **{metric_name_1}**:
    - Expected Range: {min_value} to {max_value}
    - Logic: {constraint_description} (e.g., "0-100% for percentages")

2. **{metric_name_2}**:
    - Expected Range: >= {min_value} (no upper bound)
    - Logic: {constraint_description} (e.g., "a quantity must be non-negative")

3. **{metric_name_n}**:
    - Expected Range: {min_value} to {max_value}
    - Logic: {constraint_description}

**Validation Results**:

1. **{metric_name_1}** range check:
    - Values outside range: {out_of_range_count}
    - Examples: {example_invalid_values}
    - Status: ✅ PASS / ❌ FAIL

2. **{metric_name_2}** range check:
    - Values outside range: {out_of_range_count}
    - Examples: {example_invalid_values}
    - Status: ✅ PASS / ❌ FAIL

**IF FAIL**: → List the records with out-of-range values → Investigate the source or code for corruption → Request user
confirmation if the values look suspicious
```

### Step 3: Type check

```markdown
### Data Integrity Check 3: Type Validation

**Purpose**: Ensure fields have correct data types

**Type Expectations**:

1. **{field_name_1}**: {expected_type} (e.g., "number", "string", "date")
2. **{field_name_2}**: {expected_type}
3. **{field_name_n}**: {expected_type}

**Validation Process**:

- For each field, verify type matches expectation
- Check date fields are valid dates (not invalid date objects)
- Check numeric fields are not NaN or Infinity
- Check string fields are not numeric (if unexpected)

**Results**:

1. **{field_name_1}** type check:
    - Expected: {expected_type}
    - Type mismatches: {mismatch_count}
    - Examples: {example_mismatched_values}
    - Status: ✅ PASS / ❌ FAIL

2. **Date field validation**:
    - Invalid dates: {invalid_date_count}
    - Examples: {example_invalid_dates}
    - Status: ✅ PASS / ❌ FAIL

3. **Numeric field validation**:
    - NaN values: {nan_count}
    - Infinity values: {infinity_count}
    - Status: ✅ PASS / ❌ FAIL

**IF FAIL**: → Report the type mismatches to the user → Name the coercion that would fix them (e.g., string "123" →
number 123) and leave it to the code that produced the data → List the records with type issues that no safe coercion
fixes
```

### Step 4: Consistency check

```markdown
### Data Integrity Check 4: Consistency Validation

**Purpose**: Ensure related fields have consistent values

**Consistency Rules**:

1. **{rule_name_1}**:
    - Rule: {field_A} {operator} {field_B} (e.g., "completed <= total")
    - Logic: {explanation}

2. **{rule_name_2}**:
    - Rule: {field_C} before {field_D} (e.g., "start_date < end_date")
    - Logic: {explanation}

3. **{rule_name_n}**:
    - Rule: {consistency_constraint}
    - Logic: {explanation}

**Validation Results**:

1. **{rule_name_1}** consistency check:
    - Violations: {violation_count}
    - Examples: {example_violations} (e.g., "Record #42: completed=50, total=42")
    - Status: ✅ PASS / ❌ FAIL

2. **{rule_name_2}** consistency check:
    - Violations: {violation_count}
    - Examples: {example_violations}
    - Status: ✅ PASS / ❌ FAIL

**Common Consistency Checks**:

- Completed <= total items
- Start date <= end date
- Sum of child values = parent total
- Status transitions are valid (e.g., closed items have a closed date)
- Dates are chronological (created < modified < closed)

**IF FAIL**: → Report the inconsistent records to the user → Request a fix → Flag suspicious patterns (e.g., many future
closed dates)
```

### Step 5: Outlier detection

```markdown
### Data Integrity Check 5: Outlier Detection

**Purpose**: Flag values that deviate significantly from the rest of the data

**Outlier Detection Methods**:

1. **Standard Deviation Method**:
    - Calculate mean and standard deviation for {metric_name}
    - Flag values > 3 standard deviations from mean
    - Formula: |value - mean| > 3 \* std_dev

2. **Interquartile Range (IQR) Method**:
    - Calculate Q1 (25th percentile) and Q3 (75th percentile)
    - IQR = Q3 - Q1
    - Flag values < Q1 - 1.5*IQR or > Q3 + 1.5*IQR

3. **Bounds from the contract**:
    - Flag values outside a bound the upstream contract states (the spec, the schema, or the acceptance criteria)
    - Example: "discount_pct <= 50" from a pricing spec

**Results**:

1. **{metric_name_1}** outlier detection:
    - Mean: {mean_value}, Std Dev: {std_dev_value}
    - Outliers (>3 std dev): {outlier_count}
    - Examples: {example_outliers} (e.g., "Record #15: quantity=500, mean=12")
    - Status: ✅ PASS / ⚠️ WARNING

2. **{metric_name_2}** outlier detection:
    - Q1: {q1_value}, Q3: {q3_value}, IQR: {iqr_value}
    - Outliers (IQR method): {outlier_count}
    - Examples: {example_outliers}
    - Status: ✅ PASS / ⚠️ WARNING

3. **{metric_name_3}** contract bound:
    - Bound: {bound_and_source}
    - Values outside the bound: {violation_count}
    - Status: ✅ PASS / ❌ FAIL

**Statistical flags are warnings, never fails**: the 3σ and IQR rules are heuristics, and they have no source in the
contract. Only a bound the contract states can fail the check.

**IF OUTLIERS DETECTED**: → Flag the outlier records for user review → Do NOT automatically exclude them (they may be
legitimate) → Provide context: "Value {outlier_value} is {deviation}x above the mean" → Request user confirmation before
the next step relies on them
```

### Step 6: Data integrity checkpoint

```markdown
✅ CHECKPOINT {N}: Data Integrity Validation

**Purpose**: Ensure the data meets quality standards before the next step consumes it

**Validation Summary**:

- [ ] Null/Undefined Check: All critical fields populated
- [ ] Range Check: All values within expected bounds
- [ ] Type Check: All fields have correct data types
- [ ] Consistency Check: Cross-field relationships valid
- [ ] Outlier Detection: No suspicious anomalies (or flagged for review)

**Detailed Results**:

- Total records: {record_count}
- Records with issues: {issue_count}
- Critical failures: {critical_failure_count}
- Warnings: {warning_count}

**IF ANY CRITICAL FAILURES**: → List the problem records → Report the data quality issues to the user → Request a
correction, or the user's decision to proceed with the clean subset → STOP until one of the two happens

**IF WARNINGS ONLY**: → Flag the outliers for user awareness → Proceed with caution → Include the data quality notes in
the handoff

**IF ALL CHECKS PASS**: → Data validated and ready for the next step → Proceed to {next_phase_name}
```

---

## Data integrity example

From a script that emits invoice lines as `invoice-lines.json`. The invoice generator reads the file next, so the
checkpoint runs before that task starts. The pricing spec sets these rules: `quantity` is an integer >= 1,
`discount_pct` is 0-50, and `total_cents` is `unit_price_cents * quantity * (100 - discount_pct) / 100`, rounded to the
nearest cent.

```markdown
## Checkpoint: Data Integrity Validation (invoice-lines.json)

### Data Integrity Check 1: Null/Undefined Validation

**Critical Fields**:

- `sku`: string
- `unit_price_cents`: integer
- `quantity`: integer
- `discount_pct`: number
- `total_cents`: integer

**Validation**:

- Total records: 12
- Records with null in any critical field: 0

**Status**: ✅ PASS (0 nulls in critical fields)

### Data Integrity Check 2: Range Validation

**Range Rules**:

1. **quantity**: >= 1 (pricing spec)
2. **discount_pct**: 0-50 (pricing spec)

**Validation Results**:

1. **quantity** range check:
    - Values < 1: 0
    - Status: ✅ PASS

2. **discount_pct** range check:
    - Values outside 0-50: 0 (observed range 0-25)
    - Status: ✅ PASS

### Data Integrity Check 3: Type Validation

**Results**:

- sku type mismatches: 0 (all strings)
- discount_pct type mismatches: 0 (all numbers)
- unit_price_cents type mismatches: 0 (all integers)
- quantity type mismatches: 0 (all integers)
- total_cents type mismatches: 0 (all integers)
- NaN values: 0
- Infinity values: 0

**Status**: ✅ PASS

### Data Integrity Check 4: Consistency Validation

**Consistency Rules**:

1. **Line total**: total_cents equals unit_price_cents \* quantity \* (100 - discount_pct) / 100, rounded to the nearest
   cent

**Validation Results**:

1. **Line total** consistency check:
    - Violations: 0
    - Spot check: SKU-002 has unit_price_cents=2499, quantity=2, discount_pct=10, so 2499 \* 2 \* 0.90 = 4498.2, which
      rounds to total_cents=4498 ✅
    - Status: ✅ PASS

### Data Integrity Check 5: Outlier Detection

**Standard Deviation Method**:

1. **unit_price_cents** outlier detection:
    - Mean: 2432, Std Dev: 1206
    - Outliers (>3 std dev): 0 (the largest deviation is 2.1 std dev)
    - Status: ✅ PASS

**IQR Method**:

1. **unit_price_cents** outlier detection:
    - Q1: 1450, Q3: 3124, IQR: 1674
    - Outliers (outside -1062 to 5635): 0
    - Status: ✅ PASS

✅ CHECKPOINT 3: Data Integrity Validation

**Validation Summary**:

- [x] Null/Undefined Check: All critical fields populated
- [x] Range Check: quantity >= 1 and discount_pct within 0-50
- [x] Type Check: All five fields have their expected types, no NaN/Infinity
- [x] Consistency Check: Every line total matches the pricing rule
- [x] Outlier Detection: No suspicious anomalies detected

**Detailed Results**:

- Total records: 12
- Records with issues: 0
- Critical failures: 0
- Warnings: 0

**IF ALL CHECKS PASS**: Data validated and ready for the invoice generator → Proceed to the invoice generation task
```
