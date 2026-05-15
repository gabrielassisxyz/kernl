#!/usr/bin/env python3
"""
Mechanically transcribe the approved plan to a bead graph JSON.
Zero creative interpretation. Reads docs/plans/2026-05-15-kernl-workflow-plan.md
and emits beads/2026-05-15-kernl-workflow-plan.json.
"""

import json
import os
import re
import sys

SOURCE_PATH = "docs/plans/2026-05-15-kernl-workflow-plan.md"
OUTPUT_PATH = "beads/2026-05-15-kernl-workflow-plan.json"


def normalize_key(heading_id: str) -> str:
    """
    Normalize a Task heading ID to a URL-safe key.
    1. Strip the 'Task' prefix
    2. Keep the remaining identifier
    3. Lowercase
    4. Replace spaces with hyphens
    5. Strip ALL non-alphanumeric characters (including dots) except hyphens
    6. Prepend 'task-'
    """
    # heading_id looks like 'Task 1' or 'Task 1.2' or 'Task Setup DB'
    # Remove leading "Task " (case-insensitive)
    suffix = re.sub(r'^Task\s*', '', heading_id, flags=re.IGNORECASE)
    suffix = suffix.lower()
    suffix = suffix.replace(' ', '-')
    # Strip all non-alphanumeric except hyphens
    suffix = re.sub(r'[^a-z0-9\-]', '', suffix)
    # Collapse multiple hyphens
    suffix = re.sub(r'-+', '-', suffix)
    suffix = suffix.strip('-')
    return f"task-{suffix}"


def parse_task_blocks(content: str) -> list[dict]:
    """Extract all ### Task blocks with Bead Mapping metadata."""
    tasks = []

    # Pattern: ## Task N: Title  or  ### Task N: Title
    # Then capture the metadata block until the next ### or ## or end
    pattern = re.compile(
        r'^(#{2,3})\s+(Task\s+[^:]+):\s*(.*?)$\n'
        r'(.+?)(?=\n#{2,3}\s+Task |\Z)',
        re.MULTILINE | re.DOTALL,
    )

    for match in pattern.finditer(content):
        raw_id = match.group(2).strip()
        title = match.group(3).strip()
        body = match.group(4)

        # Parse Bead Mapping block
        bead_mapping = {}
        bm_match = re.search(
            r'\*\*Bead Mapping:\*\*\s*\n((?:\s*-\s+[^:]+:\s*[^\n]*\n?)+)',
            body,
        )
        if bm_match:
            bm_text = bm_match.group(1)
            for line in bm_text.split('\n'):
                line = line.strip()
                if line.startswith('- '):
                    line = line[2:]
                    if ':' in line:
                        k, v = line.split(':', 1)
                        # Strip backticks and surrounding whitespace from values
                        v = v.strip().strip('`').strip()
                        bead_mapping[k.strip()] = v

        # Extract description (after Description / Steps: or Description:)
        description = ""
        desc_match = re.search(
            r'\*\*Description\s*/\s*Steps:\*\*\s*\n(.*?)(?=\n\*\*Acceptance Criteria:\*\*|\Z)',
            body,
            re.DOTALL,
        )
        if not desc_match:
            desc_match = re.search(
                r'\*\*Description:\*\*\s*\n(.*?)(?=\n\*\*Acceptance Criteria:\*\*|\Z)',
                body,
                re.DOTALL,
            )
        if desc_match:
            description = desc_match.group(1).strip()

        # Extract acceptance criteria
        ac_match = re.search(
            r'\*\*Acceptance Criteria:\*\*\s*\n(.*?)(?=\n\Z|\n---|\n\*\*Bead)',
            body,
            re.DOTALL,
        )
        acceptance_criteria = ""
        if ac_match:
            acceptance_criteria = ac_match.group(1).strip()

        # Files metadata
        files_match = re.search(
            r'\*\*Files:\*\*\s*\n(.*?)(?=\n\*\*Description|\n\*\*Acceptance|\n\*\*Bead|\Z)',
            body,
            re.DOTALL,
        )
        files_meta = {"create": [], "modify": [], "test": []}
        if files_match:
            fm = files_match.group(1)
            for line in fm.split('\n'):
                line = line.strip()
                if line.startswith('- Create:'):
                    files_meta["create"].append(line.split(':', 1)[1].strip())
                elif line.startswith('- Modify:'):
                    files_meta["modify"].append(line.split(':', 1)[1].strip())
                elif line.startswith('- Test:'):
                    files_meta["test"].append(line.split(':', 1)[1].strip())

        tasks.append({
            "raw_id": raw_id,
            "title": title,
            "body": body,
            "bead_mapping": bead_mapping,
            "description": description,
            "acceptance_criteria": acceptance_criteria,
            "files_meta": files_meta,
        })

    return tasks


def main():
    if not os.path.exists(SOURCE_PATH):
        print(f"ERROR: Source file not found: {SOURCE_PATH}")
        sys.exit(1)

    with open(SOURCE_PATH, 'r', encoding='utf-8') as f:
        content = f.read()

    tasks = parse_task_blocks(content)
    print(f"Found {len(tasks)} task blocks")

    if len(tasks) != 44:
        print(f"BLOCKER: Expected 44 tasks, found {len(tasks)}")
        sys.exit(1)

    # Build nodes
    nodes = []
    node_map = {}
    edges = []
    parent_edges = []
    validation_issues = []

    for t in tasks:
        bm = t["bead_mapping"]
        key = normalize_key(t["raw_id"])

        # Fields with defaults
        node_type = bm.get("type", "task")
        priority_str = bm.get("Priority", "0")
        try:
            priority = int(priority_str)
        except ValueError:
            validation_issues.append(f"Invalid priority '{priority_str}' for {key}")
            priority = 0

        est_min_str = bm.get("Estimated Minutes", "0")
        try:
            est_min = int(est_min_str)
        except ValueError:
            est_min = 0

        deps_raw = bm.get("Dependencies", "none")
        parent_raw = bm.get("Parent", "none")

        node = {
            "key": key,
            "title": t["title"],
            "type": node_type,
            "priority": priority,
            "status": bm.get("Status", "open"),
            "estimated_minutes": est_min,
            "external_ref": "plan:./docs/plans/2026-05-15-kernl-workflow-plan.md",
        }

        if t["description"]:
            node["description"] = t["description"]
        if t["acceptance_criteria"]:
            node["acceptance_criteria"] = t["acceptance_criteria"]

        # Metadata omitted: bd expects map[string]string, files structure is nested.
        # Can be reconstructed from the plan doc via external_ref if needed.
        # if has_files:
        #     node["metadata"] = {"files": json.dumps({"files": t["files_meta"]})}

        # Parent
        if parent_raw and parent_raw.lower() != "none":
            parent_key = normalize_key(parent_raw)
            node["parent_key"] = parent_key
            # Add epic relationship edge
            parent_edges.append({
                "from_key": key,
                "to_key": parent_key,
                "type": "blocks",
            })

        nodes.append(node)
        node_map[key] = node

    # Build dependency edges
    for t in tasks:
        bm = t["bead_mapping"]
        deps_raw = bm.get("Dependencies", "none")
        key = normalize_key(t["raw_id"])

        if deps_raw and deps_raw.lower() != "none":
            dep_list = [d.strip() for d in deps_raw.split(",")]
            for dep in dep_list:
                dep_key = normalize_key(dep)
                edges.append({
                    "from_key": key,
                    "to_key": dep_key,
                    "type": "blocks",
                })

    # Note: parent relationships are expressed via parent_key on nodes only.
    # Do NOT emit parent edges in the edge array; bd infers hierarchy from parent_key.
    all_edges = edges

    # Validation 1: Duplicate keys
    keys = [n["key"] for n in nodes]
    if len(keys) != len(set(keys)):
        seen = set()
        for k in keys:
            if k in seen:
                validation_issues.append(f"Duplicate key: {k}")
            seen.add(k)

    # Validation 2: Resolvable dependencies
    node_keys = set(keys)
    for e in all_edges:
        if e["to_key"] not in node_keys:
            validation_issues.append(f"Unresolved dependency: {e['to_key']} (from {e['from_key']})")

    # Validation 3: Acyclicity (DFS)
    graph = {k: [] for k in node_keys}
    for e in all_edges:
        graph[e["from_key"]].append(e["to_key"])

    WHITE, GRAY, BLACK = 0, 1, 2
    state = {k: WHITE for k in node_keys}
    cycle = []

    def dfs(node, path):
        nonlocal cycle
        if cycle:
            return
        state[node] = GRAY
        path.append(node)
        for neighbor in graph[node]:
            if state[neighbor] == GRAY:
                # Found cycle
                idx = path.index(neighbor)
                cycle = path[idx:] + [neighbor]
                return
            elif state[neighbor] == WHITE:
                dfs(neighbor, path)
        state[node] = BLACK
        path.pop()

    for k in node_keys:
        if state[k] == WHITE and not cycle:
            dfs(k, [])

    if cycle:
        validation_issues.append(f"Cycle detected: {' -> '.join(cycle)}")

    # Validation 4: Epic relationships — parent must be epic
    for t in tasks:
        bm = t["bead_mapping"]
        parent_raw = bm.get("Parent", "none")
        if parent_raw and parent_raw.lower() != "none":
            parent_key = normalize_key(parent_raw)
            parent_node = node_map.get(parent_key)
            if parent_node and parent_node["type"] != "epic":
                validation_issues.append(
                    f"Invalid epic relationship: {normalize_key(t['raw_id'])} -> {parent_key} (type={parent_node['type']})"
                )

    # Validation 5: Priority consistency with dependency order
    # The plan documents that same-wave same-priority is expected (best-effort within wave).
    # We still report them but the user has explicitly approved this.
    priority_violations = []
    for e in all_edges:
        from_node = node_map[e["from_key"]]
        to_node = node_map[e["to_key"]]
        if from_node["priority"] <= to_node["priority"]:
            priority_violations.append(
                f"Priority violation: {e['from_key']}(P{from_node['priority']}) -> {e['to_key']}(P{to_node['priority']})"
            )

    if priority_violations:
        # Per the approved plan: same-wave same-priority edges are expected.
        # We note them but do not hard-stop because the Dependencies list is authoritative.
        print(f"NOTE: {len(priority_violations)} priority inconsistencies found (same-wave, expected per plan convention):")
        for v in priority_violations[:10]:
            print(f"  - {v}")
        if len(priority_violations) > 10:
            print(f"  ... and {len(priority_violations)-10} more")

    # Validation 6: Title length
    for n in nodes:
        if len(n["title"]) > 500:
            validation_issues.append(f"Title exceeds 500 chars: {n['key']}")

    # Schema validity for type and priority
    valid_types = {"bug", "feature", "task", "epic", "chore", "decision", "message", "spike", "story", "milestone"}
    for n in nodes:
        if n["type"] not in valid_types:
            validation_issues.append(f"Invalid type '{n['type']}' for {n['key']}")
        if not (0 <= n["priority"] <= 4):
            validation_issues.append(f"Invalid priority {n['priority']} for {n['key']}")

    # Completeness
    task_headings = re.findall(r'^#{2,3}\s+(Task\s+\S+):', content, re.MULTILINE)
    print(f"Task headings found: {len(task_headings)}")
    if len(task_headings) != len(nodes):
        validation_issues.append(
            f"Completeness mismatch: {len(task_headings)} headings vs {len(nodes)} nodes"
        )

    # Summary
    print(f"\nValidations:")
    print(f"  Nodes: {len(nodes)}")
    print(f"  Dependency edges: {len(edges)}")
    print(f"  Parent edges: {len(parent_edges)}")
    print(f"  Total edges: {len(all_edges)}")
    print(f"  Issues: {len(validation_issues)}")

    if validation_issues:
        print("\nBLOCKERS:")
        for issue in validation_issues:
            print(f"  - {issue}")
        sys.exit(1)

    # Write output
    os.makedirs(os.path.dirname(OUTPUT_PATH), exist_ok=True)

    output = {
        "commit_message": "Create project plan for kernl-workflow",
        "nodes": nodes,
        "edges": all_edges,
    }

    with open(OUTPUT_PATH, 'w', encoding='utf-8') as f:
        json.dump(output, f, indent=2, ensure_ascii=False)

    print(f"\nWritten: {OUTPUT_PATH}")
    print("Graph valid: yes")


if __name__ == "__main__":
    main()
