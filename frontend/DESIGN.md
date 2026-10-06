# Design System: VitalWatch

## 1. Visual Theme & Atmosphere
VitalWatch operates on a hybrid dual-theme architecture. Both surfaces share identical core typography, border-radius metrics, and color primitives, but they execute in fundamentally different ways.

- **Patient Portal (Persuade & Read):** *Daily App Balanced*. A restrained, gallery-airy interface with generous whitespace and confident typography. The atmosphere is clinical yet warm, prioritizing low cognitive load and clear calls to action.
- **Doctor Workspace (Operate):** *Cockpit Dense*. Utilitarian, precise, and border-driven. Maximizes screen real estate for high cognitive load, replacing heavy card shadows with 1px structural dividers.

## 2. Color Palette & Roles
The design uses a unified palette applied differently depending on the active theme.

- **Canvas White** (`#F9FAFB`) — Primary background surface for Patient Portal.
- **Pure Surface** (`#FFFFFF`) — Card fill for Patient Portal; primary background for Doctor Workspace.
- **Slate Surface** (`#F8FAFC`) — Secondary background for dense tables and headers.
- **Charcoal Ink** (`#18181B`) — Primary text, Zinc-950 depth. Never use pure black (`#000000`).
- **Muted Steel** (`#71717A`) — Secondary text, descriptions, metadata.
- **Whisper Border** (`rgba(226,232,240,0.8)`) — 1px structural lines, table borders.
- **Ocean Teal** (`#0F766E`) — The single Brand Accent. Used for CTAs, active states, and focus rings. (Max 1 accent. Saturation < 80%. No purple/neon).
- **Cobalt Blue** (`#2563EB`) — Secondary action color (Doctor Workspace only).
- **Critical Red** (`#DC2626`) — Strictly reserved for Safety Interrupts and critical vitals. Never used for marketing or generic active states.

## 3. Typography Rules
- **Display & UI:** `Geist` — Clean, modern sans-serif. Track-tight, controlled scale, weight-driven hierarchy.
- **Data & Mono:** `Geist Mono` — Used strictly for timestamps, medical codes, patient IDs, and all vitals measurements to ensure vertical tabular alignment.
- **Banned:** `Inter`, `Times New Roman`, and any generic serif font.

## 4. Component Stylings
- **Buttons:** 
  - Patient: Flat, pill-shaped (`rounded-full`), tactile -1px translate on active state (spring physics). No outer glow.
  - Doctor: Sharp corners (`rounded-md`), compact padding, border-outlines preferred over solid fills to reduce visual noise.
- **Cards & Elevation:** 
  - Patient: Generously rounded corners (`1.5rem` to `2.5rem`). Diffused whisper shadow. Used to communicate hierarchy and draw focus.
  - Doctor: Elevation is banned. Replace cards with 1px `Whisper Border` dividers or negative space. Sharp corners (`rounded-sm`).
- **Inputs:** Label above input, error below. Focus ring in accent color. No floating labels.
- **Loaders:** Skeletal shimmer matching exact layout dimensions. No generic circular spinners.
- **Empty States:** Composed, illustrated compositions indicating how to populate data — not just "No data" text.

## 5. Layout Principles
- **Grid-First:** CSS Grid over Flexbox math. Never use `calc()` percentage hacks.
- **No Overlapping Elements:** Every element occupies its own clear spatial zone. No absolute-positioned content stacking.
- **Mobile-First Collapse (< 768px):** All multi-column layouts (including Doctor Workspace) collapse to a single column on mobile. No exceptions. No horizontal scroll.
- **Patient Density:** Max-width containment (`max-w-2xl` for core forms), generous internal padding (`p-8`), massive touch targets (minimum `44px`).
- **Doctor Density:** Fluid width (`max-w-7xl` or full width), compact padding (`p-4`), dense data tables.

## 6. Motion & Interaction
- **Spring Physics Default:** `stiffness: 100, damping: 20` — premium, weighty feel. No linear easing.
- **Performance:** Animate exclusively via `transform` and `opacity`. Never animate `top`, `left`, `width`, `height`.
- **Staggered Orchestration:** Never mount lists instantly — use cascade delays for waterfall reveals on the Patient dashboard.
- **Micro-Interactions:** Subtle hover states on table rows and border transitions on inputs.

## 7. Anti-Patterns (Banned)
*These are AI tells that make designs look generic or cheap. NEVER DO THESE.*
- No emojis anywhere.
- No `Inter` font.
- No generic serif fonts.
- No pure black (`#000000`).
- No neon/outer glow shadows.
- No oversaturated accents.
- No generic names ("John Doe", "Acme").
- No 3-column equal card layouts for features.
- No AI copywriting clichés ("Elevate", "Seamless", "Unleash").
- No filler UI text ("Scroll to explore").
