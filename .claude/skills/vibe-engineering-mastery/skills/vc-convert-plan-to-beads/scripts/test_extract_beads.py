"""
Tests for extract_beads.py — pytest, stdlib only (no plugins required).

Run:
    pytest .claude/skills/vibe-engineering-mastery/skills/vc-convert-plan-to-beads/scripts/

Each parse_plan / validate test calls the functions directly. Each CLI test
invokes the script as a subprocess so argv / exit-code behavior is exercised
end-to-end.
"""

import json
import subprocess
import sys
from pathlib import Path
from textwrap import dedent

import pytest

SCRIPT_DIR = Path(__file__).parent
sys.path.insert(0, str(SCRIPT_DIR))
import extract_beads as eb  # noqa: E402

SCRIPT_PATH = SCRIPT_DIR / "extract_beads.py"


# ----------------------------- helpers -----------------------------


def task(heading_id: str, **fields) -> str:
    """Render a `### Task` heading + JSON bead block for fixture authoring.

    `heading_id` is the bare identifier in the heading (e.g. "1", "1-2",
    "epic-foo"). The bead `key` becomes `task-<heading_id>` unless the caller
    passed a `key=` override or the id already starts with `task-`.
    """
    title = fields.pop("__title", f"Task {heading_id}")
    body = fields.pop("__body", "")
    if "key" in fields:
        bead_key = fields.pop("key")
    elif heading_id.startswith("task-"):
        bead_key = heading_id
    else:
        bead_key = f"task-{heading_id}"
    bead = {"key": bead_key, **fields}
    return "\n".join([
        f"### Task {heading_id}: {title}",
        "",
        "```json",
        json.dumps(bead, indent=2),
        "```",
        "",
        body,
        "",
    ])


def epic(heading_id: str, **fields) -> str:
    return task(heading_id, type="epic", priority=0, dependencies=[], **fields)


# ----------------------------- parse_plan -----------------------------


def test_parse_happy_path_extracts_all_fields():
    md = task(
        "1",
        type="task",
        priority=1,
        dependencies=[],
        estimated_minutes=15,
        files={"create": ["a.py"], "modify": [], "test": ["t.py"]},
        __body="**Description:** does a thing.\n\n**Acceptance Criteria:**\n- It works",
    )
    tasks = eb.parse_plan(md)
    assert len(tasks) == 1
    t = tasks[0]
    assert t["key"] == "task-1"
    assert t["type"] == "task"
    assert t["priority"] == 1
    assert t["estimated_minutes"] == 15
    assert t["files"]["create"] == ["a.py"]
    assert "does a thing" in t["description"]
    assert "It works" in t["acceptance_criteria"]


def test_parse_uses_key_verbatim_from_json_no_normalization():
    """key in JSON is what bd gets — no runtime normalization."""
    md = task("task-1", type="task", priority=0, dependencies=[])
    tasks = eb.parse_plan(md)
    assert tasks[0]["key"] == "task-1"


def test_parse_title_defaults_to_heading_when_omitted():
    md = dedent(
        """\
        ### Task 1: Heading Title

        ```json
        {"key": "task-1", "type": "task", "priority": 0, "dependencies": []}
        ```
        """
    )
    tasks = eb.parse_plan(md)
    assert tasks[0]["title"] == "Heading Title"


def test_parse_title_in_json_wins_over_heading():
    md = dedent(
        """\
        ### Task 1: Heading Title

        ```json
        {"key": "task-1", "title": "JSON Title", "type": "task", "priority": 0, "dependencies": []}
        ```
        """
    )
    tasks = eb.parse_plan(md)
    assert tasks[0]["title"] == "JSON Title"


def test_parse_splits_description_and_acceptance():
    md = dedent(
        """\
        ### Task 1: T

        ```json
        {"key": "task-1", "type": "task", "priority": 0, "dependencies": []}
        ```

        **Description:** Do the thing.

        Some steps here.

        **Acceptance Criteria:**
        - All tests pass
        - No warnings
        """
    )
    tasks = eb.parse_plan(md)
    assert "Do the thing" in tasks[0]["description"]
    assert "Some steps here" in tasks[0]["description"]
    assert "All tests pass" in tasks[0]["acceptance_criteria"]
    assert "No warnings" in tasks[0]["acceptance_criteria"]
    assert "Acceptance" not in tasks[0]["description"]


def test_parse_description_only_when_no_acceptance_heading():
    md = dedent(
        """\
        ### Task 1: T

        ```json
        {"key": "task-1", "type": "task", "priority": 0, "dependencies": []}
        ```

        Just a description, no acceptance criteria section.
        """
    )
    tasks = eb.parse_plan(md)
    assert "Just a description" in tasks[0]["description"]
    assert tasks[0]["acceptance_criteria"] == ""


def test_parse_no_tasks_dies():
    with pytest.raises(SystemExit) as exc:
        eb.parse_plan("# Plan\n\nSome prose.\n")
    assert exc.value.code == 2


def test_parse_missing_json_block_dies(capsys):
    md = dedent(
        """\
        ### Task 1: No bead block

        Just prose, no JSON.
        """
    )
    with pytest.raises(SystemExit) as exc:
        eb.parse_plan(md)
    assert exc.value.code == 2
    assert "missing the required" in capsys.readouterr().err


def test_parse_invalid_json_dies(capsys):
    md = dedent(
        """\
        ### Task 1: Bad JSON

        ```json
        {"key": "task-1", "type": "task",,, broken}
        ```
        """
    )
    with pytest.raises(SystemExit) as exc:
        eb.parse_plan(md)
    assert exc.value.code == 2
    assert "invalid JSON" in capsys.readouterr().err


def test_parse_non_object_json_dies(capsys):
    md = dedent(
        """\
        ### Task 1: T

        ```json
        ["not", "an", "object"]
        ```
        """
    )
    with pytest.raises(SystemExit) as exc:
        eb.parse_plan(md)
    assert "must be a JSON object" in capsys.readouterr().err


def test_parse_acceptance_heading_h4_style_works():
    md = dedent(
        """\
        ### Task 1: T

        ```json
        {"key": "task-1", "type": "task", "priority": 0, "dependencies": []}
        ```

        Desc.

        #### Acceptance Criteria

        - ok
        """
    )
    tasks = eb.parse_plan(md)
    assert "Desc" in tasks[0]["description"]
    assert "ok" in tasks[0]["acceptance_criteria"]


# ----------------------------- validate -----------------------------


def _tasks_from_dicts(*dicts) -> list[dict]:
    """Helper: build the post-parse_plan shape from raw dicts."""
    return [{**d, "description": d.get("description", ""),
             "acceptance_criteria": d.get("acceptance_criteria", "")}
            for d in dicts]


def test_validate_happy_path():
    tasks = _tasks_from_dicts(
        {"key": "task-epic", "title": "Epic", "type": "epic", "priority": 0,
         "dependencies": []},
        {"key": "task-1", "title": "A", "type": "task", "priority": 1,
         "dependencies": [], "parent": "task-epic"},
        {"key": "task-2", "title": "B", "type": "task", "priority": 2,
         "dependencies": ["task-1"], "parent": "task-epic"},
    )
    nodes, edges = eb.validate(tasks)
    assert len(nodes) == 3
    # 1 blocks edge (2 -> 1) + 2 parent-child edges
    assert {(e["from_key"], e["to_key"], e["type"]) for e in edges} == {
        ("task-2", "task-1", "blocks"),
        ("task-1", "task-epic", "parent-child"),
        ("task-2", "task-epic", "parent-child"),
    }


def test_validate_duplicate_key_dies(capsys):
    tasks = _tasks_from_dicts(
        {"key": "task-1", "title": "A", "type": "task", "priority": 0, "dependencies": []},
        {"key": "task-1", "title": "B", "type": "task", "priority": 1, "dependencies": []},
    )
    with pytest.raises(SystemExit) as exc:
        eb.validate(tasks)
    assert exc.value.code == 2
    assert "Duplicate key" in capsys.readouterr().err


def test_validate_missing_key_dies(capsys):
    tasks = _tasks_from_dicts(
        {"title": "Keyless", "type": "task", "priority": 0, "dependencies": []},
    )
    with pytest.raises(SystemExit):
        eb.validate(tasks)
    assert "has no 'key'" in capsys.readouterr().err


def test_validate_invalid_type_dies(capsys):
    tasks = _tasks_from_dicts(
        {"key": "task-1", "title": "A", "type": "widget", "priority": 0, "dependencies": []},
    )
    with pytest.raises(SystemExit):
        eb.validate(tasks)
    assert "invalid type" in capsys.readouterr().err


def test_validate_priority_out_of_range_dies(capsys):
    tasks = _tasks_from_dicts(
        {"key": "task-1", "title": "A", "type": "task", "priority": 5, "dependencies": []},
    )
    with pytest.raises(SystemExit):
        eb.validate(tasks)
    assert "priority must be int 0-4" in capsys.readouterr().err


def test_validate_priority_not_int_dies(capsys):
    tasks = _tasks_from_dicts(
        {"key": "task-1", "title": "A", "type": "task", "priority": "high", "dependencies": []},
    )
    with pytest.raises(SystemExit):
        eb.validate(tasks)
    assert "priority must be int" in capsys.readouterr().err


def test_validate_negative_estimated_minutes_dies(capsys):
    tasks = _tasks_from_dicts(
        {"key": "task-1", "title": "A", "type": "task", "priority": 0,
         "dependencies": [], "estimated_minutes": -5},
    )
    with pytest.raises(SystemExit):
        eb.validate(tasks)
    assert "non-negative int" in capsys.readouterr().err


def test_validate_long_title_dies(capsys):
    tasks = _tasks_from_dicts(
        {"key": "task-1", "title": "x" * 501, "type": "task",
         "priority": 0, "dependencies": []},
    )
    with pytest.raises(SystemExit):
        eb.validate(tasks)
    assert "title exceeds 500" in capsys.readouterr().err


def test_validate_unknown_dependency_dies(capsys):
    tasks = _tasks_from_dicts(
        {"key": "task-1", "title": "A", "type": "task", "priority": 1,
         "dependencies": ["task-missing"]},
    )
    with pytest.raises(SystemExit):
        eb.validate(tasks)
    assert "'task-missing' is not a known task key" in capsys.readouterr().err


def test_validate_priority_monotonicity_violation_dies(capsys):
    tasks = _tasks_from_dicts(
        {"key": "task-1", "title": "A", "type": "task", "priority": 2, "dependencies": []},
        {"key": "task-2", "title": "B", "type": "task", "priority": 1,
         "dependencies": ["task-1"]},
    )
    with pytest.raises(SystemExit):
        eb.validate(tasks)
    err = capsys.readouterr().err
    assert "task-2" in err and "strictly greater" in err


def test_validate_cycle_dies(capsys):
    tasks = _tasks_from_dicts(
        {"key": "task-1", "title": "A", "type": "task", "priority": 0,
         "dependencies": ["task-2"]},
        {"key": "task-2", "title": "B", "type": "task", "priority": 0,
         "dependencies": ["task-1"]},
    )
    with pytest.raises(SystemExit):
        eb.validate(tasks)
    assert "Cycle detected" in capsys.readouterr().err


def test_validate_parent_must_be_epic(capsys):
    tasks = _tasks_from_dicts(
        {"key": "task-parent", "title": "P", "type": "task", "priority": 0,
         "dependencies": []},
        {"key": "task-child", "title": "C", "type": "task", "priority": 1,
         "dependencies": [], "parent": "task-parent"},
    )
    with pytest.raises(SystemExit):
        eb.validate(tasks)
    assert "expected 'epic'" in capsys.readouterr().err


def test_validate_parent_must_exist(capsys):
    tasks = _tasks_from_dicts(
        {"key": "task-1", "title": "A", "type": "task", "priority": 0,
         "dependencies": [], "parent": "task-missing"},
    )
    with pytest.raises(SystemExit):
        eb.validate(tasks)
    assert "parent 'task-missing' is not a known" in capsys.readouterr().err


def test_validate_dependencies_must_be_list(capsys):
    tasks = _tasks_from_dicts(
        {"key": "task-1", "title": "A", "type": "task", "priority": 0,
         "dependencies": "task-2"},
    )
    with pytest.raises(SystemExit):
        eb.validate(tasks)
    assert "must be a list" in capsys.readouterr().err


def test_validate_files_metadata_propagates():
    tasks = _tasks_from_dicts(
        {"key": "task-1", "title": "A", "type": "task", "priority": 0,
         "dependencies": [],
         "files": {"create": ["a.go"], "modify": [], "test": ["a_test.go"]}},
    )
    nodes, _ = eb.validate(tasks)
    assert nodes[0]["metadata"]["files"]["create"] == ["a.go"]


def test_validate_status_defaults_to_open():
    tasks = _tasks_from_dicts(
        {"key": "task-1", "title": "A", "type": "task", "priority": 0, "dependencies": []},
    )
    nodes, _ = eb.validate(tasks)
    assert nodes[0]["status"] == "open"


def test_validate_status_override_preserved():
    tasks = _tasks_from_dicts(
        {"key": "task-1", "title": "A", "type": "task", "priority": 0,
         "dependencies": [], "status": "blocked"},
    )
    nodes, _ = eb.validate(tasks)
    assert nodes[0]["status"] == "blocked"


def test_validate_empty_optional_fields_omitted():
    """estimated_minutes / description / acceptance_criteria absent or empty
    should not show up as null fields in the output."""
    tasks = _tasks_from_dicts(
        {"key": "task-1", "title": "A", "type": "task", "priority": 0,
         "dependencies": [], "description": "", "acceptance_criteria": ""},
    )
    nodes, _ = eb.validate(tasks)
    assert "description" not in nodes[0]
    assert "acceptance_criteria" not in nodes[0]
    assert "estimated_minutes" not in nodes[0]


# ----------------------------- CLI -----------------------------


def _run(args: list[str], cwd: Path | None = None) -> subprocess.CompletedProcess:
    return subprocess.run(
        [sys.executable, str(SCRIPT_PATH), *args],
        capture_output=True, text=True, cwd=cwd,
    )


def test_cli_validate_ok(tmp_path: Path):
    plan = tmp_path / "plan.md"
    plan.write_text(
        epic("epic-1", __title="Epic")
        + task("1", type="task", priority=1, dependencies=[], parent="task-epic-1")
    )
    res = _run([str(plan), "--validate"])
    assert res.returncode == 0, f"stderr: {res.stderr}"
    assert "graph valid" in res.stderr
    assert res.stdout == ""  # --validate writes nothing to stdout


def test_cli_extract_writes_output_file(tmp_path: Path):
    plan = tmp_path / "2026-05-17-feature-tasks.md"
    plan.write_text(task("1", type="task", priority=0, dependencies=[]))
    out = tmp_path / "out" / "feature-plan.json"
    res = _run([str(plan), "--output", str(out)])
    assert res.returncode == 0
    assert out.exists()
    data = json.loads(out.read_text())
    assert data["commit_message"] == "Create project plan for 2026-05-17-feature"
    assert len(data["nodes"]) == 1
    assert data["nodes"][0]["key"] == "task-1"


def test_cli_commit_message_override(tmp_path: Path):
    plan = tmp_path / "p.md"
    plan.write_text(task("1", type="task", priority=0, dependencies=[]))
    out = tmp_path / "g.json"
    res = _run([str(plan), "--output", str(out), "--commit-message", "Custom msg"])
    assert res.returncode == 0
    assert json.loads(out.read_text())["commit_message"] == "Custom msg"


def test_cli_missing_input_dies():
    res = _run(["/nonexistent/path.md", "--validate"])
    assert res.returncode == 2
    assert "Input file not found" in res.stderr


def test_cli_stdout_when_no_output(tmp_path: Path):
    plan = tmp_path / "p.md"
    plan.write_text(task("1", type="task", priority=0, dependencies=[]))
    res = _run([str(plan)])
    assert res.returncode == 0
    data = json.loads(res.stdout)
    assert data["nodes"][0]["key"] == "task-1"


def test_cli_validation_failure_exits_2_with_stderr(tmp_path: Path):
    plan = tmp_path / "p.md"
    plan.write_text(
        task("1", type="task", priority=0, dependencies=["task-2"])
        + task("2", type="task", priority=1, dependencies=["task-1"])
    )
    res = _run([str(plan), "--validate"])
    assert res.returncode == 2
    # Both cycle and priority issues should be reported
    assert "Cycle detected" in res.stderr or "strictly greater" in res.stderr


def test_cli_creates_output_dir(tmp_path: Path):
    plan = tmp_path / "p.md"
    plan.write_text(task("1", type="task", priority=0, dependencies=[]))
    out = tmp_path / "deeply" / "nested" / "out.json"
    res = _run([str(plan), "--output", str(out)])
    assert res.returncode == 0
    assert out.parent.is_dir()
    assert out.exists()
