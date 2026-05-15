"""Generate parallel-demo.gif — simulated terminal recording of `kernl epic run parallel-demo`.

Produces a GIF showing the epic executing with beads changing state in parallel waves:
  Wave 1: bead a (sequential)
  Wave 2: bead b, bead c (parallel)
"""

from PIL import Image, ImageDraw, ImageFont
import os

W, H = 780, 420
BG = (15, 23, 42)          # #0f172a
FG = (226, 232, 240)       # #e2e8f0
DIM = (75, 85, 99)         # #4b5563
CYAN = (56, 189, 248)      # #38bdf8
GREEN = (16, 185, 129)     # #10b981
BLUE = (59, 130, 246)      # #3b82f6
AMBER = (245, 158, 11)     # #f59e0b
PURPLE = (139, 92, 246)    # #8b5cf6
GRAY = (107, 114, 128)     # #6b7280
RED = (239, 68, 68)        # #ef4444
BAR_BG = (30, 41, 59)      # #1e293b
PADDING = 24
LINE_H = 20
TITLE_H = 26

try:
    font = ImageFont.truetype("/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf", 14)
    font_sm = ImageFont.truetype("/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf", 12)
    font_title = ImageFont.truetype("/usr/share/fonts/truetype/dejavu/DejaVuSansMono-Bold.ttf", 15)
except Exception:
    font = ImageFont.load_default()
    font_sm = ImageFont.load_default()
    font_title = ImageFont.load_default()


def new_frame():
    """Create a blank terminal frame."""
    img = Image.new("RGB", (W, H), BG)
    draw = ImageDraw.Draw(img)
    return img, draw


def draw_gui_header(draw, y, connected=True):
    """Draw the Kernl monitor title bar."""
    status = "connected" if connected else "disconnected"
    status_color = (110, 231, 183) if connected else (252, 165, 165)  # green / salmon
    draw.text((PADDING, y), "Kernl", fill=(56, 189, 248), font=font_title)
    sw = draw.textlength(status, font=font_sm)
    draw.text((W - PADDING - sw, y + 2), status, fill=status_color, font=font_sm)


def draw_bead_card(draw, y, bead_id, state, title):
    """Draw a bead card with colored left border."""
    state_colors = {
        "queue": GRAY, "active": BLUE, "review": AMBER,
        "done": PURPLE, "blocked": RED, "cancelled": (156, 163, 175),
    }
    color = state_colors.get(state, GRAY)
    # Left border
    draw.rectangle([PADDING, y, PADDING + 4, y + 40], fill=color)
    # Card background
    draw.rectangle([PADDING + 4, y, W - PADDING, y + 40], fill=BAR_BG, outline=(30, 41, 59))
    # Bead ID in color
    draw.text((PADDING + 14, y + 4), bead_id, fill=color, font=font)
    # State badge
    draw.text((PADDING + 14, y + 22), state, fill=DIM, font=font_sm)
    # Title right-aligned
    tw = draw.textlength(title, font=font_sm) if hasattr(draw, 'textlength') else len(title) * 8
    draw.text((W - PADDING - tw - 10, y + 10), title, fill=FG, font=font_sm)


def draw_status_bar(draw, y, text, color=FG):
    """Draw a status line at the bottom."""
    draw.text((PADDING, y), text, fill=color, font=font_sm)


def render_frame(frame_func, duration_ms):
    """Execute frame_func(draw), return (image, duration_ms)."""
    img, draw = new_frame()
    frame_func(draw)
    return img, duration_ms


def frame_0(draw):
    """Frame 0: Command prompt."""
    y = PADDING
    draw.text((PADDING, y), "$ kernl epic run parallel-demo", fill=CYAN, font=font)
    y += LINE_H + 4
    draw.text((PADDING, y), "loading epic...", fill=DIM, font=font_sm)


def frame_1(draw):
    """Frame 1: GUI URL printed."""
    y = PADDING
    draw.text((PADDING, y), "$ kernl epic run parallel-demo", fill=CYAN, font=font)
    y += LINE_H + 4
    draw.text((PADDING, y), "GUI em http://localhost:53421", fill=GREEN, font=font_sm)
    y += LINE_H + 4
    draw.text((PADDING, y), "epic parallel-demo: 3 beads, max parallelism 2", fill=FG, font=font_sm)


def frame_2(draw):
    """Frame 2: Beads queued — all 3 visible."""
    draw_gui_header(draw, PADDING)
    y = PADDING + TITLE_H + 10
    draw.text((PADDING, y), "BEADS", fill=DIM, font=font_sm)
    y += LINE_H + 8
    draw_bead_card(draw, y, "a", "queue", "Setup")
    draw_bead_card(draw, y + 46, "b", "queue", "Frontend")
    draw_bead_card(draw, y + 92, "c", "queue", "Backend")
    # Wave indicator
    draw_status_bar(draw, H - 40, "wave 0 — 3 beads queued | pico 0 | max 2")


def frame_3(draw):
    """Frame 3: Wave 1 — bead a active."""
    draw_gui_header(draw, PADDING)
    y = PADDING + TITLE_H + 10
    draw.text((PADDING, y), "BEADS", fill=DIM, font=font_sm)
    y += LINE_H + 8
    draw_bead_card(draw, y, "a", "active", "Setup")
    draw_bead_card(draw, y + 46, "b", "queue", "Frontend")
    draw_bead_card(draw, y + 92, "c", "queue", "Backend")
    y2 = PADDING + TITLE_H + 10
    draw.text((W // 2 + 20, y2), "SESSIONS", fill=DIM, font=font_sm)
    y2 += LINE_H + 8
    draw.rectangle([W // 2 + 20, y2, W - PADDING, y2 + 48], fill=BAR_BG, outline=(30, 41, 59))
    draw.text((W // 2 + 30, y2 + 4), "sess-a-01", fill=BLUE, font=font_sm)
    draw.text((W // 2 + 30, y2 + 22), "bead: a  opencode", fill=DIM, font=font_sm)
    draw_status_bar(draw, H - 40, "wave 1 — bead a executando | pico 1 | max 2", color=BLUE)


def frame_4(draw):
    """Frame 4: Wave 1 complete — bead a done. Wave 2 starts — b and c active."""
    draw_gui_header(draw, PADDING)
    y = PADDING + TITLE_H + 10
    draw.text((PADDING, y), "BEADS", fill=DIM, font=font_sm)
    y += LINE_H + 8
    draw_bead_card(draw, y, "a", "done", "Setup")
    draw_bead_card(draw, y + 46, "b", "active", "Frontend")
    draw_bead_card(draw, y + 92, "c", "active", "Backend")
    y2 = PADDING + TITLE_H + 10
    draw.text((W // 2 + 20, y2), "SESSIONS", fill=DIM, font=font_sm)
    y2 += LINE_H + 8
    draw.rectangle([W // 2 + 20, y2, W - PADDING, y2 + 48], fill=BAR_BG, outline=(30, 41, 59))
    draw.text((W // 2 + 30, y2 + 4), "sess-b-02", fill=BLUE, font=font_sm)
    draw.text((W // 2 + 30, y2 + 22), "bead: b  opencode", fill=DIM, font=font_sm)
    draw.rectangle([W // 2 + 20, y2 + 52, W - PADDING, y2 + 100], fill=BAR_BG, outline=(30, 41, 59))
    draw.text((W // 2 + 30, y2 + 56), "sess-c-03", fill=BLUE, font=font_sm)
    draw.text((W // 2 + 30, y2 + 74), "bead: c  opencode", fill=DIM, font=font_sm)
    draw_status_bar(draw, H - 40, "wave 2 — beads b, c em paralelo | pico 2 | max 2", color=GREEN)


def frame_5(draw):
    """Frame 5: All beads done."""
    draw_gui_header(draw, PADDING)
    y = PADDING + TITLE_H + 10
    draw.text((PADDING, y), "BEADS", fill=DIM, font=font_sm)
    y += LINE_H + 8
    draw_bead_card(draw, y, "a", "done", "Setup")
    draw_bead_card(draw, y + 46, "b", "done", "Frontend")
    draw_bead_card(draw, y + 92, "c", "done", "Backend")
    draw_status_bar(draw, H - 40, "wave 2 complete — 3/3 beads done", color=GREEN)


def frame_6(draw):
    """Frame 6: Epic completion summary."""
    y = PADDING
    draw.text((PADDING, y), "$ kernl epic run parallel-demo", fill=CYAN, font=font)
    y += LINE_H + 4
    draw.text((PADDING, y), "GUI em http://localhost:53421", fill=GREEN, font=font_sm)
    y += LINE_H + 8
    draw.text((PADDING, y), "bead a \u2192 done            (wave 1 - sequential)", fill=PURPLE, font=font)
    y += LINE_H + 4
    draw.text((PADDING, y), "bead b \u2192 done   bead c \u2192 done   (wave 2 - parallel \u2937)", fill=PURPLE, font=font)
    y += LINE_H + 10
    draw.rectangle([PADDING, y, W - PADDING, y + 2], fill=(30, 41, 59))
    y += 12
    draw.text((PADDING, y), "\u2714 epic parallel-demo conclu\u00eddo", fill=GREEN, font=font)
    y += LINE_H + 4
    draw.text((PADDING, y), "paralelismo realizado: 2.0x (pico 2, m\u00e1x 2)", fill=FG, font=font_sm)
    y += LINE_H + 4
    draw.text((PADDING, y), "3 beads | 2 waves | 0 out-of-gate interventions", fill=DIM, font=font_sm)


# Build frames
frames_data = [
    (frame_0, 2000),
    (frame_1, 2500),
    (frame_2, 2500),
    (frame_3, 3000),
    (frame_4, 3500),
    (frame_5, 2500),
    (frame_6, 4000),
]

frames = []
for i, (fn, dur) in enumerate(frames_data):
    img, _ms = render_frame(fn, dur)
    frames.append(img)

output_path = os.path.join(os.path.dirname(__file__), "parallel-demo.gif")
frames[0].save(
    output_path,
    save_all=True,
    append_images=frames[1:],
    duration=[d for _, d in frames_data],
    loop=0,
    optimize=False,
)
print(f"GIF saved to {output_path} ({os.path.getsize(output_path)} bytes)")
