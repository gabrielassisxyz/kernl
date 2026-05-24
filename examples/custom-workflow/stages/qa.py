#!/usr/bin/env python3
import sys
import json
import os

def main():
    try:
        # Read standard JSON handoff from STDIN
        input_data = json.load(sys.stdin)
    except Exception as e:
        sys.stderr.write(f"Error parsing STDIN JSON: {e}\n")
        sys.exit(1)

    # Extract rigid handoff fields
    epic_id = input_data.get("epic_id", "")
    bead_id = input_data.get("bead_id", "")
    worktree_path = input_data.get("worktree_path", "")
    context_payload = input_data.get("context_payload", "")

    # Print debug info to stderr (so it is captured in logs / stderr output)
    sys.stderr.write(f"QA stage running: epic_id={epic_id}, bead_id={bead_id}, worktree={worktree_path}\n")

    # Crashing variant trigger
    if "crash" in context_payload:
        sys.stderr.write("Simulating standard Python traceback crash...\n")
        raise ValueError("Deliberate crash requested via context_payload trigger")

    # Success path: write exit gate artifact
    if worktree_path:
        os.makedirs(worktree_path, exist_ok=True)
        artifact_path = os.path.join(worktree_path, "qa_verdict.txt")
        with open(artifact_path, "w") as f:
            f.write("VERDICT: PASS\n")
        sys.stderr.write(f"Wrote verdict PASS to {artifact_path}\n")

    # Accumulate or set context payload
    new_payload = f"QA_PASSED:{epic_id}:{bead_id}:{context_payload}"

    # Write response JSON to STDOUT
    response = {
        "context_payload": new_payload
    }
    json.dump(response, sys.stdout)
    sys.stdout.write("\n")

if __name__ == "__main__":
    main()
