# Custom Workflow Example (Escape Hatch Demo)

This directory demonstrates the custom workflow and escape hatch capability of the **Kernl** engine, allowing developers to plug subprocess script stages between native stages.

## File Structure

- `custom.yaml`: The custom workflow descriptor. It plugs the subprocess script stage `qa` between the native `planning` and `implementation` stages, and maps out standard stages for the rest of the flow.
- `stages/qa.py`: A Python 3 script implementing the `qa` stage. It reads handoff JSON from STDIN, verifies the input, optionally triggers a failure/crash, writes a verdict exit gate file, and emits updated JSON to STDOUT.

## How It Works

1. **Loader Integration**: The custom workflow `custom.yaml` is parsed and loaded via the standard `LoadWorkflowYAML` parser. It is registered in the engine matching the `profileId` or `workflowId` on the bead (e.g., `custom_workflow`).
2. **Subprocess Dispatch**: When a bead transitions to the `qa` state, the engine invokes the subprocess command (`python3 examples/custom-workflow/stages/qa.py`) under the hood.
3. **STDIN JSON Handoff**: The engine pipes a standard JSON payload directly to the script's `STDIN`:
   ```json
   {
     "epic_id": "parent-epic-id",
     "bead_id": "bead-id",
     "worktree_path": "/path/to/bead/worktree",
     "context_payload": "previous-state-payload"
   }
   ```
4. **STDOUT Response Processing**: The subprocess executes its custom QA/testing logic. Upon successful completion, it must output a JSON response containing the updated `context_payload` to `STDOUT`:
   ```json
   {
     "context_payload": "QA_PASSED:parent-epic-id:bead-id:previous-state-payload"
   }
   ```
5. **Exit Gate Validation**: The custom exit gate configured for the `qa` stage requires `qa_verdict.txt` in the worktree root containing `VERDICT: PASS`.

## Best Practices and Guidelines

### 1. `context_payload` Size Management

> [!IMPORTANT] The engine stores `context_payload` in the agent state store to carry state between stages. However, keeping this payload small is highly recommended. Authors should avoid dumping large raw files, datasets, or complex objects inside the `context_payload` string itself.
>
> Instead, follow this pattern:
> - Store large output files directly within the bead's `worktree_path`.
> - Keep `context_payload` as a lightweight metadata store, referring to specific filenames or hashes located in the worktree.

### 2. The 64KB STDOUT Cap

> [!WARNING] The escape hatch runner enforces a strict **64KB (65,536 bytes) limit** on `STDOUT` and `STDERR` separately.
>
> If your script emits output exceeding this cap, the runner will truncate the stream and transition the bead to a `blocked` status with a `CauseOutputTooLarge` error.
> 
> Ensure your script only outputs the required JSON response object to `STDOUT`. Any diagnostic logging, debugging text, or secondary output should be written to `STDERR` or directed to files in the worktree.
