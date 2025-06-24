# /verify-implementation - Systematic Implementation Verification

**Purpose**: Systematically verify that claimed implementations actually exist and work correctly

**Usage**: `/verify-implementation [component-name] [feature-description]`

## What it does:

1. **Conducts comprehensive verification**:
   - Checks actual file contents for claimed implementations
   - Verifies build compilation success
   - Runs relevant tests to ensure functionality
   - Validates documentation accuracy

2. **Provides structured verification process**:
   - File content verification using Read tool
   - Build verification using go build
   - Test execution using go test
   - Documentation cross-reference checking

3. **Updates status accurately**:
   - Only marks items as complete after full verification
   - Identifies gaps between claims and reality
   - Provides specific feedback on missing components

## Example workflow:
```bash
/verify-implementation "subdomain-routing" "Steps 1-4 of subdomain-based URL format"
# Systematically checks each component:
# - Reads proxy/server.go for middleware implementation
# - Verifies router configuration in same file
# - Checks docker-compose.yml for Traefik configuration
# - Runs build and tests
# - Updates PRD.md only after verification
```

## When to use:
- Before marking any PRD.md phase as "COMPLETED"
- When user questions implementation status
- After major feature implementations
- When resuming work on a project to verify current state

## Verification Process:

### Phase 1: File Content Verification
- Use Read tool to examine claimed implementation files
- Verify specific functions, configurations, or code blocks exist
- Check implementation matches specification requirements
- Identify any gaps between claims and actual code

### Phase 2: Build Verification
- Run `go build -o remote-mcp-proxy .` to verify compilation
- Ensure no build errors or warnings exist
- Confirm binary can be created successfully
- Validate all imports and dependencies resolve

### Phase 3: Functional Verification
- Execute relevant test suites
- Run `go test ./...` for comprehensive testing
- Verify new functionality works as specified
- Check that existing functionality still works (no regressions)

### Phase 4: Documentation Verification
- Cross-reference documentation with actual implementation
- Verify examples and code snippets are accurate
- Check that status claims in PRD.md match reality
- Ensure README.md reflects actual capabilities

### Phase 5: Status Update
- Only after ALL verification phases pass
- Update PRD.md, CHANGELOG.md, and other docs
- Mark todos as completed in TodoWrite
- Provide summary of verification results

## Output Format:
```
VERIFICATION RESULTS for [component-name]

✅ File Content: [Details of what was found/verified]
✅ Build Status: [Build success/failure with details]  
✅ Tests: [Test results and coverage]
✅ Documentation: [Accuracy verification results]

OVERALL STATUS: [VERIFIED/INCOMPLETE/FAILED]
NEXT ACTIONS: [What needs to be done if not fully verified]
```

## Critical Rules:
- **NEVER** mark anything as complete without full verification
- **ALWAYS** check actual file contents, not just assumptions
- **MUST** run build verification before claiming completion
- **REQUIRED** to update documentation only after verification