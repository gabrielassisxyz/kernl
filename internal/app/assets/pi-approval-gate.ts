/**
 * kernl approval gate for pi.
 *
 * pi has no permission-prompt flag: a tool call is gated from inside this
 * hook, which pi awaits before running the tool. The hook holds nothing back
 * for itself - it hands the call to `kernl approval bridge`, which parks the
 * request where a human can see it and blocks until it is answered, refused,
 * or expires. Keeping the policy in kernl rather than here is deliberate: this
 * file is a shim, and a second copy of the rules would be a second thing to
 * keep true.
 *
 * kernl writes this file into its own state directory and passes it with -e.
 * Do not edit the copy there; it is overwritten on every dispatch.
 */
import { spawn } from "node:child_process";

interface Verdict {
	allowed: boolean;
	reason?: string;
}

function askKernl(bin: string, payload: string): Promise<Verdict> {
	return new Promise((resolve) => {
		const child = spawn(bin, ["approval", "bridge", "--adapter", "pi"], {
			stdio: ["pipe", "pipe", "inherit"],
		});

		let out = "";
		child.stdout.on("data", (chunk) => {
			out += chunk;
		});
		child.on("error", (err) => {
			resolve({ allowed: false, reason: `kernl approval bridge could not be started: ${err.message}` });
		});
		child.on("close", () => {
			try {
				resolve(JSON.parse(out) as Verdict);
			} catch {
				// An unreadable verdict is not permission. Anything other than a
				// clear allow has to block, or a broken bridge becomes a bypass.
				resolve({ allowed: false, reason: `kernl approval bridge returned no readable verdict: ${out.trim()}` });
			}
		});

		child.stdin.write(payload);
		child.stdin.end();
	});
}

export default function (pi: any) {
	pi.on("tool_call", async (event: any) => {
		const bin = process.env.KERNL_APPROVAL_BRIDGE_BIN;
		if (!bin) {
			return {
				block: true,
				reason: "kernl dispatched this agent under an approval gate but did not say where its bridge is (KERNL_APPROVAL_BRIDGE_BIN unset), so no tool can be approved",
			};
		}

		const verdict = await askKernl(
			bin,
			JSON.stringify({
				toolName: event.toolName,
				input: event.input ?? {},
				toolCallId: event.toolCallId,
			}),
		);

		if (verdict.allowed) {
			return undefined;
		}
		return { block: true, reason: verdict.reason || "the operator declined this action" };
	});
}
