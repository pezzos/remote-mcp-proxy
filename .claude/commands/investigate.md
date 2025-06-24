# /investigate - Systematic Problem Analysis

**Purpose**: Launch systematic investigation mode for complex technical issues

**Usage**: `/investigate [problem-description]`

## What it does:

1. **Sets up investigation structure**:
   - Creates TodoWrite list breaking down the problem into phases
   - Initializes or updates INVESTIGATIONS.md with problem statement
   - Establishes investigation timeline and success criteria

2. **Guides systematic analysis**:
   - Provides structured approach to evidence gathering
   - Implements hypothesis testing methodology
   - Documents breakthroughs and dead ends systematically

3. **Ensures comprehensive documentation**:
   - Maintains real-time investigation log
   - Cross-references findings with existing documentation
   - Prepares final solution documentation

## Example workflow:
```bash
/investigate "Claude.ai shows connected but no tools appear"
# Sets up investigation todos, creates INVESTIGATIONS.md section
# Guides through systematic analysis of symptoms, hypotheses, testing
# Documents final solution and updates relevant documentation
```

## When to use:
- Complex bugs affecting multiple system components
- Protocol compliance issues requiring systematic analysis
- Performance problems needing methodical investigation
- Integration failures with unclear root causes

## Investigation Process:

### Phase 1: Problem Definition
- Document clear problem statement
- Identify observable symptoms
- Establish success criteria for resolution

### Phase 2: Evidence Gathering
- Use available tools systematically (Read, Grep, Bash, etc.)
- Document all findings in INVESTIGATIONS.md
- Test hypotheses with concrete evidence

### Phase 3: Root Cause Analysis
- Analyze patterns in evidence
- Cross-reference with existing documentation
- Identify most likely root causes

### Phase 4: Solution Implementation
- Implement fixes based on root cause analysis
- Test solutions thoroughly
- Update all relevant documentation

### Phase 5: Documentation
- Document final solution in INVESTIGATIONS.md
- Update README.md, PRD.md as needed
- Ensure knowledge is preserved for future reference