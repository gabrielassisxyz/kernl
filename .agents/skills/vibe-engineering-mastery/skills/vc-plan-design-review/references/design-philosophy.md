# Design Review — Design Philosophy & Principles

> Reference for `vc-plan-design-review`. Apply these throughout the review.

## Design Philosophy

You are not here to rubber-stamp this plan's UI. You are here to ensure that when
this ships, users feel the design is intentional — not generated, not accidental,
not "we'll polish it later." Your posture is opinionated but collaborative: find
every gap, explain why it matters, fix the obvious ones, and ask about the genuine
choices.

Do NOT make any code changes. Do NOT start implementation. Your only job right now
is to review and improve the plan's design decisions with maximum rigor.

## Design Principles

1. Empty states are features. "No items found." is not a design. Every empty state needs warmth, a primary action, and context.
2. Every screen has a hierarchy. What does the user see first, second, third? If everything competes, nothing wins.
3. Specificity over vibes. "Clean, modern UI" is not a design decision. Name the font, the spacing scale, the interaction pattern.
4. Edge cases are user experiences. 47-char names, zero results, error states, first-time vs power user — these are features, not afterthoughts.
5. AI slop is the enemy. Generic card grids, hero sections, 3-column features — if it looks like every other AI-generated site, it fails.
6. Responsive is not "stacked on mobile." Each viewport gets intentional design.
7. Accessibility is not optional. Keyboard nav, screen readers, contrast, touch targets — specify them in the plan or they won't exist.
8. Subtraction default. If a UI element doesn't earn its pixels, cut it. Feature bloat kills products faster than missing features.
9. Trust is earned at the pixel level. Every interface decision either builds or erodes user trust.

## Cognitive Patterns — How Great Designers See

These aren't a checklist — they're how you see. The perceptual instincts that separate "looked at the design" from "understood why it feels wrong." Let them run automatically as you review.

1. **Seeing the system, not the screen** — Never evaluate in isolation; what comes before, after, and when things break.
2. **Empathy as simulation** — Not "I feel for the user" but running mental simulations: bad signal, one hand free, boss watching, first time vs. 1000th time.
3. **Hierarchy as service** — Every decision answers "what should the user see first, second, third?" Respecting their time, not prettifying pixels.
4. **Constraint worship** — Limitations force clarity. "If I can only show 3 things, which 3 matter most?"
5. **The question reflex** — First instinct is questions, not opinions. "Who is this for? What did they try before this?"
6. **Edge case paranoia** — What if the name is 47 chars? Zero results? Network fails? Colorblind? RTL language?
7. **The "Would I notice?" test** — Invisible = perfect. The highest compliment is not noticing the design.
8. **Principled taste** — "This feels wrong" is traceable to a broken principle. Taste is *debuggable*, not subjective (Zhuo: "A great designer defends her work based on principles that last").
9. **Subtraction default** — "As little design as possible" (Rams). "Subtract the obvious, add the meaningful" (Maeda).
10. **Time-horizon design** — First 5 seconds (visceral), 5 minutes (behavioral), 5-year relationship (reflective) — design for all three simultaneously (Norman, Emotional Design).
11. **Design for trust** — Every design decision either builds or erodes trust. Strangers sharing a home requires pixel-level intentionality about safety, identity, and belonging (Gebbia, Airbnb).
12. **Storyboard the journey** — Before touching pixels, storyboard the full emotional arc of the user's experience. The "Snow White" method: every moment is a scene with a mood, not just a screen with a layout (Gebbia).

Key references: Dieter Rams' 10 Principles, Don Norman's 3 Levels of Design, Nielsen's 10 Heuristics, Gestalt Principles (proximity, similarity, closure, continuity), Steve Krug ("Don't make me think" — the 3-second scan test, the trunk test, satisficing, the goodwill reservoir), Ginny Redish (Letting Go of the Words — writing for scanning), Caroline Jarrett (Forms that Work — mindless form interactions), Ira Glass ("Your taste is why your work disappoints you"), Jony Ive ("People can sense care and can sense carelessness. Different and new is relatively easy. Doing something that's genuinely better is very hard."), Joe Gebbia (designing for trust between strangers, storyboarding emotional journeys).

When reviewing a plan, empathy as simulation runs automatically. When rating, principled taste makes your judgment debuggable — never say "this feels off" without tracing it to a broken principle. When something seems cluttered, apply subtraction default before suggesting additions.

## UX Principles: How Users Actually Behave

These principles govern how real humans interact with interfaces. They are observed
behavior, not preferences. Apply them before, during, and after every design decision.

### The Three Laws of Usability

1. **Don't make me think.** Every page should be self-evident. If a user stops
   to think "What do I click?" or "What does this mean?", the design has failed.
   Self-evident > self-explanatory > requires explanation.

2. **Clicks don't matter, thinking does.** Three mindless, unambiguous clicks
   beat one click that requires thought. Each step should feel like an obvious
   choice (animal, vegetable, or mineral), not a puzzle.

3. **Omit, then omit again.** Get rid of half the words on each page, then get
   rid of half of what's left. Happy talk (self-congratulatory text) must die.
   Instructions must die. If they need reading, the design has failed.

### How Users Actually Behave

- **Users scan, they don't read.** Design for scanning: visual hierarchy
  (prominence = importance), clearly defined areas, headings and bullet lists,
  highlighted key terms. We're designing billboards going by at 60 mph, not
  product brochures people will study.
- **Users satisfice.** They pick the first reasonable option, not the best.
  Make the right choice the most visible choice.
- **Users muddle through.** They don't figure out how things work. They wing
  it. If they accomplish their goal by accident, they won't seek the "right" way.
  Once they find something that works, no matter how badly, they stick to it.
- **Users don't read instructions.** They dive in. Guidance must be brief,
  timely, and unavoidable, or it won't be seen.

### Billboard Design for Interfaces

- **Use conventions.** Logo top-left, nav top/left, search = magnifying glass.
  Don't innovate on navigation to be clever. Innovate when you KNOW you have a
  better idea, otherwise use conventions. Even across languages and cultures,
  web conventions let people identify the logo, nav, search, and main content.
- **Visual hierarchy is everything.** Related things are visually grouped. Nested
  things are visually contained. More important = more prominent. If everything
  shouts, nothing is heard. Start with the assumption everything is visual noise,
  guilty until proven innocent.
- **Make clickable things obviously clickable.** No relying on hover states for
  discoverability, especially on mobile where hover doesn't exist. Shape, location,
  and formatting (color, underlining) must signal clickability without interaction.
- **Eliminate noise.** Three sources: too many things shouting for attention
  (shouting), things not organized logically (disorganization), and too much stuff
  (clutter). Fix noise by removal, not addition.
- **Clarity trumps consistency.** If making something significantly clearer
  requires making it slightly inconsistent, choose clarity every time.

### Navigation as Wayfinding

Users on the web have no sense of scale, direction, or location. Navigation
must always answer: What site is this? What page am I on? What are the major
sections? What are my options at this level? Where am I? How can I search?

Persistent navigation on every page. Breadcrumbs for deep hierarchies.
Current section visually indicated. The "trunk test": cover everything except
the navigation. You should still know what site this is, what page you're on,
and what the major sections are. If not, the navigation has failed.

### The Goodwill Reservoir

Users start with a reservoir of goodwill. Every friction point depletes it.

**Deplete faster:** Hiding info users want (pricing, contact, shipping). Punishing
users for not doing things your way (formatting requirements on phone numbers).
Asking for unnecessary information. Putting sizzle in their way (splash screens,
forced tours, interstitials). Unprofessional or sloppy appearance.

**Replenish:** Know what users want to do and make it obvious. Tell them what they
want to know upfront. Save them steps wherever possible. Make it easy to recover
from errors. When in doubt, apologize.

### Mobile: Same Rules, Higher Stakes

All the above applies on mobile, just more so. Real estate is scarce, but never
sacrifice usability for space savings. Affordances must be VISIBLE: no cursor
means no hover-to-discover. Touch targets must be big enough (44px minimum).
Flat design can strip away useful visual information that signals interactivity.
Prioritize ruthlessly: things needed in a hurry go close at hand, everything
else a few taps away with an obvious path to get there.
