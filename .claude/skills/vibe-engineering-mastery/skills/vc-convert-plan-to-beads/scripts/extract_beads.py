#!/usr/bin/env python3
"""
extract_beads.py — Deterministic markdown plan -> bd graph JSON.

Reads a plan markdown file where every `### Task <id>: <title>` heading is
immediately followed by a fenced ```json``` block (the "bead block") containing
the structured bead fields. Captures surrounding markdown as description and
acceptance_criteria, validates the graph, and emits the bd graph JSON.

This script is pure stdlib Python 3.10+. No external dependencies.

Exits 0 on success, 2 on validation failure (with errors on stderr).

Usage:
    python3 extract_beads.py docs/plans/2026-05-17-feature-tasks.md \
        --output beads/2026-05-17-feature-plan.json \
        --commit-message "Create project plan for feature"

    # Validate only (parse + check, no output written):
    python3 extract_beads.py docs/plans/2026-05-17-feature-tasks.md --validate
"""

import argparse
import json
import re
import sys
from pathlib import Path
from typing import Any

TASK_HEADING_RE = re.compile(
    r'^###\s+Task\s+([\w.\-]+)\s*:\s*(.+?)\s*$', re.MULTILINE
)
JSON_FENCE_RE = re.compile(r'```json\s*\n(.*?)\n```', re.DOTALL)
ACCEPTANCE_RE = re.compile(
    r'(?:\*\*Acceptance\s+Criteria:?\*\*|####\s+Acceptance\s+Criteria)',
    re.IGNORECASE,
)

VALID_TYPES = {
    'bug', 'feature', 'task', 'epic', 'chore', 'decision',
    'message', 'spike', 'story', 'milestone',
}


def die(msg: str) -> None:
    sys.stderr.write(f"ERROR: {msg}\n")
    sys.exit(2)


def parse_plan(md_text: str) -> list[dict]:
    """Walk the markdown; one dict per `### Task` heading.

    Each dict has the bead JSON fields, plus 'description' and
    'acceptance_criteria' captured from the markdown between the JSON block
    and the next task heading.
    """
    headings = list(TASK_HEADING_RE.finditer(md_text))
    if not headings:
        die("No `### Task <id>: <title>` headings found in input.")

    tasks: list[dict] = []
    for i, m in enumerate(headings):
        heading_id = m.group(1)
        heading_title = m.group(2).strip()
        section_start = m.end()
        section_end = (
            headings[i + 1].start() if i + 1 < len(headings) else len(md_text)
        )
        section = md_text[section_start:section_end]

        json_match = JSON_FENCE_RE.search(section)
        if not json_match:
            die(
                f"Task '{heading_id}: {heading_title}' is missing the required "
                f"```json``` bead block immediately after the heading."
            )

        try:
            bead = json.loads(json_match.group(1))
        except json.JSONDecodeError as e:
            die(
                f"Task '{heading_id}: {heading_title}' has invalid JSON in its "
                f"bead block: {e.msg} (line {e.lineno}, col {e.colno})"
            )

        if not isinstance(bead, dict):
            die(f"Task '{heading_id}: {heading_title}' bead block must be a JSON object.")

        # Capture markdown after the JSON block, up to next task heading.
        after = section[json_match.end():].strip()
        ac_match = ACCEPTANCE_RE.search(after)
        if ac_match:
            description = after[:ac_match.start()].strip()
            acceptance = after[ac_match.end():].strip()
        else:
            description = after
            acceptance = ""

        # JSON fields win if present; markdown fills missing ones.
        bead.setdefault('description', description)
        bead.setdefault('acceptance_criteria', acceptance)
        bead.setdefault('title', heading_title)

        tasks.append(bead)

    return tasks


def validate(tasks: list[dict]) -> tuple[list[dict], list[dict]]:
    """Validate the task list. Returns (nodes, edges) for bd graph JSON.

    Hard-stops on any validation failure.
    """
    errors: list[str] = []
    keys: set[str] = set()
    nodes_by_key: dict[str, dict] = {}

    # Pass 1: key uniqueness + presence
    for idx, t in enumerate(tasks):
        if 'key' not in t or not isinstance(t['key'], str) or not t['key']:
            errors.append(f"Task #{idx + 1} ('{t.get('title', '?')}') has no 'key'.")
            continue
        if t['key'] in keys:
            errors.append(f"Duplicate key: {t['key']}")
        keys.add(t['key'])
        nodes_by_key[t['key']] = t

    if errors:
        for e in errors:
            sys.stderr.write(f"ERROR: {e}\n")
        sys.exit(2)

    # Pass 2: per-field + edges
    edges: list[dict] = []
    for t in tasks:
        key = t['key']

        if not t.get('title'):
            errors.append(f"{key}: missing 'title'.")
        elif len(t['title']) > 500:
            errors.append(f"{key}: title exceeds 500 chars ({len(t['title'])}).")

        node_type = t.get('type')
        if node_type not in VALID_TYPES:
            errors.append(
                f"{key}: invalid type {node_type!r}. Must be one of {sorted(VALID_TYPES)}."
            )

        pri = t.get('priority')
        if not isinstance(pri, int) or not (0 <= pri <= 4):
            errors.append(f"{key}: priority must be int 0-4, got {pri!r}.")

        em = t.get('estimated_minutes')
        if em is not None and (not isinstance(em, int) or em < 0):
            errors.append(f"{key}: estimated_minutes must be a non-negative int, got {em!r}.")

        deps = t.get('dependencies', [])
        if not isinstance(deps, list):
            errors.append(f"{key}: 'dependencies' must be a list, got {type(deps).__name__}.")
            deps = []

        for d in deps:
            if not isinstance(d, str):
                errors.append(f"{key}: dependency entries must be strings, got {d!r}.")
                continue
            if d not in keys:
                errors.append(f"{key}: dependency '{d}' is not a known task key.")
                continue
            dep_pri = nodes_by_key[d].get('priority')
            if (
                isinstance(dep_pri, int)
                and isinstance(pri, int)
                and pri <= dep_pri
            ):
                errors.append(
                    f"{key}: priority {pri} must be strictly greater than "
                    f"dependency '{d}' priority {dep_pri}."
                )
            edges.append({"from_key": key, "to_key": d, "type": "blocks"})

        parent = t.get('parent')
        if parent:
            if parent not in keys:
                errors.append(f"{key}: parent '{parent}' is not a known task key.")
            elif nodes_by_key[parent].get('type') != 'epic':
                errors.append(
                    f"{key}: parent '{parent}' has type "
                    f"{nodes_by_key[parent].get('type')!r}, expected 'epic'."
                )
            else:
                edges.append({"from_key": key, "to_key": parent, "type": "parent-child"})

    # Pass 3: cycle detection over blocks edges (DFS with 3-coloring)
    graph: dict[str, list[str]] = {}
    for e in edges:
        if e['type'] == 'blocks':
            graph.setdefault(e['from_key'], []).append(e['to_key'])

    WHITE, GRAY, BLACK = 0, 1, 2
    color = {k: WHITE for k in keys}
    cycle_path: list[str] = []

    def dfs(node: str, path: list[str]) -> bool:
        color[node] = GRAY
        path.append(node)
        for nxt in graph.get(node, []):
            if color.get(nxt) == GRAY:
                idx = path.index(nxt)
                cycle_path.extend(path[idx:] + [nxt])
                return True
            if color.get(nxt) == WHITE and dfs(nxt, path):
                return True
        path.pop()
        color[node] = BLACK
        return False

    for k in keys:
        if color[k] == WHITE and dfs(k, []):
            errors.append(f"Cycle detected in dependency graph: {' -> '.join(cycle_path)}")
            break

    if errors:
        for e in errors:
            sys.stderr.write(f"ERROR: {e}\n")
        sys.exit(2)

    # Build bd-shaped node objects
    nodes: list[dict] = []
    for t in tasks:
        node: dict[str, Any] = {
            "key": t['key'],
            "title": t['title'],
            "type": t['type'],
            "priority": t['priority'],
            "status": t.get('status', 'open'),
        }
        for field in ('description', 'acceptance_criteria', 'estimated_minutes', 'external_ref'):
            val = t.get(field)
            if val not in (None, "", []):
                node[field] = val
        if t.get('parent'):
            node['parent_key'] = t['parent']
        if 'files' in t and t['files']:
            node['metadata'] = {'files': t['files']}
        nodes.append(node)

    return nodes, edges


def main() -> int:
    ap = argparse.ArgumentParser(
        description="Extract bd graph JSON from a markdown plan with ```json``` bead blocks.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__,
    )
    ap.add_argument('input', type=Path, help='Plan markdown file.')
    ap.add_argument('--output', type=Path, default=None,
                    help='Write bd graph JSON here. Default: stdout.')
    ap.add_argument('--commit-message', default=None,
                    help='commit_message field. Default: derived from input filename.')
    ap.add_argument('--validate', action='store_true',
                    help='Parse + validate only. Do not write output.')
    args = ap.parse_args()

    if not args.input.is_file():
        die(f"Input file not found: {args.input}")

    md_text = args.input.read_text(encoding='utf-8')
    tasks = parse_plan(md_text)
    if not tasks:
        die("No tasks parsed from input.")

    nodes, edges = validate(tasks)

    if args.validate:
        sys.stderr.write(
            f"OK: {len(nodes)} nodes, {len(edges)} edges, graph valid.\n"
        )
        return 0

    commit_msg = args.commit_message
    if commit_msg is None:
        feature = re.sub(r'-tasks$', '', args.input.stem)
        commit_msg = f"Create project plan for {feature}"

    output = {
        "commit_message": commit_msg,
        "nodes": nodes,
        "edges": edges,
    }

    json_text = json.dumps(output, indent=2, ensure_ascii=False)
    if args.output:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(json_text + "\n", encoding='utf-8')
        sys.stderr.write(
            f"Wrote {len(nodes)} nodes, {len(edges)} edges -> {args.output}\n"
        )
    else:
        print(json_text)

    return 0


if __name__ == '__main__':
    sys.exit(main())
