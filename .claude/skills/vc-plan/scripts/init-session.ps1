# Initialize planning files for a new session
# Usage: .\init-session.ps1 [-Template TYPE] [project-name]
# Templates: default, analytics

param(
    [string]$ProjectName = "project",
    [string]$Template = "default"
)

$DATE = Get-Date -Format "yyyy-MM-dd"

# Resolve template directory (skill root is one level up from scripts/)
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$SkillRoot = Split-Path -Parent $ScriptDir
$TemplateDir = Join-Path $SkillRoot "templates"

Write-Host "Initializing planning files for: $ProjectName (template: $Template)"

# Validate template
if ($Template -ne "default" -and $Template -ne "analytics") {
    Write-Host "Unknown template: $Template (available: default, analytics). Using default."
    $Template = "default"
}

# Ensure docs/ directory exists
$DocsDir = Join-Path (Get-Location) "docs"
if (-not (Test-Path $DocsDir)) {
    New-Item -ItemType Directory -Path $DocsDir -Force | Out-Null
}

$PlanFile = Join-Path $DocsDir "task_plan.md"
$FindingsFile = Join-Path $DocsDir "findings.md"

# Create task_plan.md if it doesn't exist
if (-not (Test-Path $PlanFile)) {
    $AnalyticsPlan = Join-Path $TemplateDir "analytics_task_plan.md"
    if ($Template -eq "analytics" -and (Test-Path $AnalyticsPlan)) {
        Copy-Item $AnalyticsPlan $PlanFile
    } else {
        @"
# Task Plan: [Brief Description]

## Goal
[One sentence describing the end state]

## Current Phase
Phase 1

## Phases

### Phase 1: Requirements & Discovery
- [ ] Understand user intent
- [ ] Identify constraints
- [ ] Document in findings.md
- **Status:** in_progress

### Phase 2: Planning & Structure
- [ ] Define approach
- [ ] Create project structure
- **Status:** pending

### Phase 3: Implementation
- [ ] Execute the plan
- [ ] Write to files before executing
- **Status:** pending

### Phase 4: Testing & Verification
- [ ] Verify requirements met
- [ ] Document test results
- **Status:** pending

### Phase 5: Delivery
- [ ] Review outputs
- [ ] Deliver to user
- **Status:** pending

## Decisions Made
| Decision | Rationale |
|----------|-----------|

## Errors Encountered
| Error | Resolution |
|-------|------------|
"@ | Out-File -FilePath $PlanFile -Encoding UTF8
    }
    Write-Host "Created docs/task_plan.md"
} else {
    Write-Host "docs/task_plan.md already exists, skipping"
}

# Create findings.md if it doesn't exist
if (-not (Test-Path $FindingsFile)) {
    $AnalyticsFindings = Join-Path $TemplateDir "analytics_findings.md"
    if ($Template -eq "analytics" -and (Test-Path $AnalyticsFindings)) {
        Copy-Item $AnalyticsFindings $FindingsFile
    } else {
        @"
# Findings & Decisions

## Requirements
-

## Research Findings
-

## Technical Decisions
| Decision | Rationale |
|----------|-----------|

## Issues Encountered
| Issue | Resolution |
|-------|------------|

## Resources
-
"@ | Out-File -FilePath $FindingsFile -Encoding UTF8
    }
    Write-Host "Created docs/findings.md"
} else {
    Write-Host "docs/findings.md already exists, skipping"
}

Write-Host ""
Write-Host "Planning files initialized!"
Write-Host "Files: docs/task_plan.md, docs/findings.md"
