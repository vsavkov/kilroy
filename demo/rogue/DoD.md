# Rogue 5.4.4 WebAssembly Port — Definition of Done

## Scope

### In Scope

- Complete Rust reimplementation of all Rogue 5.4.4 game systems compiled to `wasm32-unknown-unknown`
- Single-page HTML deliverable with embedded JS terminal renderer at `demo/rogue/rogue-wasm/www/index.html`
- Classic 80x24 ASCII terminal display with monospace font on dark background
- All player commands (movement, inventory, combat, meta)
- Full monster AI and daemon/fuse timing system
- Wizard mode: create objects, teleport, level skip, map reveal, identify, and all debug commands
- Deterministic seed support for reproducible games
- Save/load via browser localStorage
- Keyboard input handling in the browser

### Out of Scope

- Mobile/touch input
- Sound or music
- Graphical tiles or non-ASCII rendering
- Multiplayer or networking
- High-score server or cross-session leaderboards (local only)
- Accessibility features beyond what the original had
- Performance optimization beyond "runs smoothly at 60fps"
- Wizard password authentication (replaced by simpler browser-friendly mechanism)
- Score file encryption (`xcrypt.c` XOR cipher)
- Multi-user score isolation (UID-based)
- Signal handling (SIGINT/SIGTSTP/SIGUSR)
- Cross-compilation to native binary
- Pixel-perfect terminal rendering (cursor blinking, terminal escape fidelity)
- Original save file format compatibility

### Assumptions

1. User has Rust stable, `cargo`, `rustc`, and `wasm-pack` installed
2. Deliverable served locally (e.g., `python -m http.server`)
3. Original C source at `demo/rogue/original-rogue/` is authoritative reference
4. Browser target: modern evergreen browsers (Chrome, Firefox, Safari, Edge -- latest two versions)
5. Wizard mode enabled via JS API call, URL parameter, or equivalent browser-friendly mechanism

## Deliverables

| Artifact | Location | Description |
|----------|----------|-------------|
| Rust crate | `demo/rogue/rogue-wasm/` | Complete Rust reimplementation of Rogue 5.4.4, compilable to WASM |
| WASM module | `demo/rogue/rogue-wasm/pkg/` (build output) | Compiled `wasm32-unknown-unknown` binary + JS bindings |
| HTML deliverable | `demo/rogue/rogue-wasm/www/index.html` | Single-page HTML that loads WASM, renders 80x24 grid, accepts keyboard input |
| Unit/integration tests | `demo/rogue/rogue-wasm/src/**/*_test.rs` or `tests/` | `cargo test` test suite covering RNG, combat, dungeon gen, items, monsters, gameplay |

## Acceptance Criteria

### 1. Build

| ID | Criterion | Covered by |
|----|-----------|------------|
| AC-1.1 | `cargo build --target wasm32-unknown-unknown` exits 0 with no errors | IT-1 |
| AC-1.2 | `cargo fmt --check` exits 0 | IT-1 |
| AC-1.3 | `cargo test` (native target) exits 0 | IT-2, IT-3, IT-4, IT-5, IT-6 |

### 2. Deliverable

| ID | Criterion | Covered by |
|----|-----------|------------|
| AC-2.1 | `demo/rogue/rogue-wasm/www/index.html` exists | IT-1 |
| AC-2.2 | `index.html` references the WASM module (contains import/load path to `.wasm` or `pkg/`) | IT-1 |
| AC-2.3 | No build artifacts (`target/`, `node_modules/`, `dist/`, `pkg/`) are committed to git | IT-1 |
| AC-2.4 | The HTML page loads in a browser, renders an 80x24 character grid with monospace font on dark background | IT-7 |

### 3. RNG Fidelity

| ID | Criterion | Covered by |
|----|-----------|------------|
| AC-3.1 | LCG formula `seed = seed * 11109 + 13849`, output `(seed >> 16) & 0xffff` produces identical sequence to C version for any given initial seed | IT-2 |
| AC-3.2 | `rnd(range)` returns `abs(RN) % range` (returns 0 when range is 0) | IT-2 |
| AC-3.3 | `roll(number, sides)` returns sum of `number` calls to `rnd(sides) + 1` | IT-2 |

### 4. Dungeon Generation Fidelity

| ID | Criterion | Covered by |
|----|-----------|------------|
| AC-4.1 | Rooms placed on 3x3 grid (`MAXROOMS=9`) with dimensions derived from `bsze.x = NUMCOLS/3`, `bsze.y = NUMLINES/3` | IT-3 |
| AC-4.2 | Room removal: `rnd(4)` rooms marked ISGONE; dark rooms: `rnd(10) < level-1`; maze rooms: `rnd(15) == 0` when dark | IT-3 |
| AC-4.3 | Passage connectivity via spanning tree over hardcoded `rdes[]` adjacency matrix, with extra cycle-creating passages | IT-3 |
| AC-4.4 | For a fixed seed, the generated dungeon layout (room positions, sizes, doors, passages, gone/dark/maze flags) is identical to the C version | IT-3 |

### 5. Monster Fidelity

| ID | Criterion | Covered by |
|----|-----------|------------|
| AC-5.1 | All 26 monsters (A-Z) have exact stats matching `extern.c`: carry%, flags, strength, experience, level, armor, HP formula (`roll(lvl, 8)`), damage strings | IT-4 |
| AC-5.2 | Monster level/armor/exp scaling beyond `AMULETLEVEL=26` matches the `lev_add` formula | IT-4 |

### 6. Combat Fidelity

| ID | Criterion | Covered by |
|----|-----------|------------|
| AC-6.1 | To-hit formula: `rnd(20) + wplus >= (20 - at_lvl) - op_arm` | IT-4 |
| AC-6.2 | Damage formula: `roll(ndice, nsides) + dplus + add_dam[str]` | IT-4 |
| AC-6.3 | `str_plus[]` and `add_dam[]` arrays match C arrays exactly (32 entries each) | IT-4 |
| AC-6.4 | All special attacks (aquator rust, rattlesnake poison, wraith drain, vampire drain, ice monster freeze, venus flytrap hold, leprechaun gold steal, nymph item steal, xeroc reveal) reproduce same probability checks and effects | IT-4 |

### 7. Item Fidelity

| ID | Criterion | Covered by |
|----|-----------|------------|
| AC-7.1 | Item type distribution matches: potion 26%, scroll 36%, food 16%, weapon 7%, armor 7%, ring 4%, stick 4% | IT-5 |
| AC-7.2 | Per-type probabilities for all 14 potions, 18 scrolls, 9 weapons, 8 armors, 14 rings, 14 wands/staves match `extern.c` | IT-5 |
| AC-7.3 | Weapon damage tables (wield and throw), armor AC values, ring/wand charge ranges match C originals | IT-5 |

### 8. Daemon/Fuse System

| ID | Criterion | Covered by |
|----|-----------|------------|
| AC-8.1 | `MAXDAEMONS=20`; all daemon timing (`doctor`, `runners`, `swander`, `stomach`) matches C version | IT-3 |
| AC-8.2 | Fuse scheduling matches BEFORE/AFTER execution order and `spread()` formula | IT-3 |

### 9. Hunger System

| ID | Criterion | Covered by |
|----|-----------|------------|
| AC-9.1 | `HUNGERTIME=1300`, `STARVETIME=850`, `STOMACHSIZE=2000` constants match | IT-6 |
| AC-9.2 | Food consumption rate, slow digestion ring effect, hunger state transitions, and starvation death all match C behavior | IT-6 |

### 10. Level Progression

| ID | Criterion | Covered by |
|----|-----------|------------|
| AC-10.1 | `AMULETLEVEL=26`; monster selection via `lvl_mons[]`/`wand_mons[]` | IT-3, IT-4 |
| AC-10.2 | Trap generation: `rnd(10) < level`, count = `rnd(level/4) + 1` capped at `MAXTRAPS=10` | IT-3 |
| AC-10.3 | Item generation: 9 attempts per level at 36% each | IT-3 |
| AC-10.4 | Gold formula: `rnd(50 + 10*level) + 2` | IT-3 |

### 11. Player Commands

| ID | Criterion | Covered by |
|----|-----------|------------|
| AC-11.1 | All movement commands (h/j/k/l/y/u/b/n, uppercase run, ctrl-run-until-adjacent) function correctly | IT-6 |
| AC-11.2 | All action commands (quaff, read, eat, wield, wear, take off, put on ring, remove ring, drop, zap, throw, search, call, identify, help, options, save, quit, fight, rest, pick up, stairs, inventory, version, repeat) function correctly | IT-6 |
| AC-11.3 | Numeric prefix (count) commands work for applicable commands | IT-6 |

### 12. Display

| ID | Criterion | Covered by |
|----|-----------|------------|
| AC-12.1 | Player displayed as `@`, corridors as `#`, floors as `.`, doors as `+`, monster letters A-Z, and all standard Rogue display characters (see rogue.h: `%` stairs, `*` gold, `!` potion, `?` scroll, `:` food, `)` weapon, `]` armor, `,` amulet, `=` ring, `/` stick, `^` trap) | IT-7 |
| AC-12.2 | Status line displayed on row 24 (0-indexed row 23) with format: `Level: N  Gold: N  Hp: N(N)  Str: N(N)  Arm: N  Exp: N/N  [hunger_state]` | IT-6, IT-7 |
| AC-12.3 | Messages displayed at top of screen (row 0) with `--More--` prompt for overflow | IT-6, IT-7 |

### 13. Save/Load

| ID | Criterion | Covered by |
|----|-----------|------------|
| AC-13.1 | Save captures all game state: player stats/position, inventory, current level map, monster positions/states, daemon/fuse queue, room states, discovered item names, RNG seed | IT-8 |
| AC-13.2 | A save from one session restores identically in another (close tab, reopen, restore -- game state preserved) | IT-8 |

### 14. Wizard Mode

| ID | Criterion | Covered by |
|----|-----------|------------|
| AC-14.1 | Wizard mode can be enabled via JS API call, URL parameter, or equivalent browser-friendly mechanism | IT-9 |
| AC-14.2 | Wizard create object command (`C`) functions: prompts for type, which, blessing; adds item to pack | IT-9 |
| AC-14.3 | Wizard teleport (`Ctrl-T`) moves player to random room | IT-9 |
| AC-14.4 | Wizard level skip down (`Ctrl-D`) and up (`Ctrl-A`) change dungeon level | IT-9 |
| AC-14.5 | Wizard map reveal (`Ctrl-F`) shows full level map | IT-9 |
| AC-14.6 | Wizard identify (`Ctrl-W`) identifies items in pack | IT-9 |
| AC-14.7 | Wizard toggle wizard off (`+`) produces "not wizard any more" | IT-9 |
| AC-14.8 | Wizard see monsters (`Ctrl-X`), show position (`|`), show inpack count (`$`), show food left (`Ctrl-E`), show level objects (`Ctrl-G`), charge stick (`Ctrl-~`), super-equip (`Ctrl-I`), item probability list (`*`) all function | IT-9 |

### 15. Deterministic Replay

| ID | Criterion | Covered by |
|----|-----------|------------|
| AC-15.1 | Given the same RNG seed, the game engine produces identical game states as the C original across multi-turn sequences | IT-2, IT-3, IT-4, IT-6 |

## User-Facing Message Inventory

### Status Line & Core Display

| ID | Message surface | Trigger condition | Covered by |
|----|----------------|-------------------|------------|
| MSG-1 | Status line: `Level: N  Gold: N  Hp: N(N)  Str: N(N)  Arm: N  Exp: N/N  [state]` | Every turn (displayed on STATLINE row 23) | IT-6 |
| MSG-2 | Hunger states in status line: `""`, `"Hungry"`, `"Weak"`, `"Faint"` | `hungry_state` changes (0/1/2/3) | IT-6 |
| MSG-3 | `--More--` prompt | Message overflow when previous message still displayed | IT-6 |
| MSG-4 | `@` for player, `#` corridor, `.` floor, `+` door, `%` stairs, `*` gold, `!` potion, `?` scroll, `:` food, `)` weapon, `]` armor, `,` amulet, `=` ring, `/` stick, `^` trap, `A-Z` monsters, `|`/`-` walls, ` ` rock | Map rendering on every look() | IT-7 |

### Movement & Exploration

| ID | Message surface | Trigger condition | Covered by |
|----|----------------|-------------------|------------|
| MSG-5 | `"you are still stuck in the bear trap"` | Movement attempted while `no_move > 0` (bear trap) | IT-6 |
| MSG-6 | `"you are being held"` | Movement attempted while ISHELD and not moving toward `F` | IT-6 |
| MSG-7 | `"you can move again"` | `no_command` countdown reaches 0 (unfreeze after ice monster/sleep) | IT-6 |

### Hunger & Food

| ID | Message surface | Trigger condition | Covered by |
|----|----------------|-------------------|------------|
| MSG-8 | `"you are starting to get hungry"` / `"getting hungry"` (terse) | `food_left` drops below `2 * MORETIME` (300) | IT-6 |
| MSG-9 | `"you are starting to feel weak"` / `"getting the munchies"` (hallucinating) | `food_left` drops below `MORETIME` (150) | IT-6 |
| MSG-10 | `"you feel too weak from lack of food.  You faint"` / `"the munchies overpower..."` (hallucinating) | `food_left <= 0` and faint roll succeeds | IT-6 |
| MSG-11 | `"my, that was a yummy [fruit]"` | Eat fruit-type food (food `o_which == 1`) | IT-6 |
| MSG-12 | `"yuk, this food tastes awful"` / `"bummer, this food tastes awful"` (hallucinating) | Eat regular food, `rnd(100) > 70` | IT-6 |
| MSG-13 | `"yum, that tasted good"` / `"oh, wow, that tasted good"` (hallucinating) | Eat regular food, `rnd(100) <= 70` | IT-6 |
| MSG-14 | `"ugh, you would get ill if you ate that"` / `"that's Inedible!"` (terse) | Try to eat non-food item | IT-6 |

### Combat

| ID | Message surface | Trigger condition | Covered by |
|----|----------------|-------------------|------------|
| MSG-15 | Hit messages: `"You scored an excellent hit on"`, `"You hit"`, `"You have injured"`, `"You swing and hit"` (player hits monster, non-terse) | Player melee hit, `roll_em` succeeds | IT-4 |
| MSG-16 | Miss messages: `"You miss"`, `"You swing and miss"`, `"You barely miss"`, `"You don't hit"` (player misses, non-terse) | Player melee miss, `roll_em` fails | IT-4 |
| MSG-17 | Monster hit messages: `"The [monster] scored an excellent hit on"`, etc. | Monster attacks player, `roll_em` succeeds | IT-4 |
| MSG-18 | Monster miss messages: `"The [monster] misses"`, etc. | Monster attacks player, `roll_em` fails | IT-4 |
| MSG-19 | Terse hit: `"You hit"` / `"The [monster] hit"` | Combat in terse mode | IT-4 |
| MSG-20 | `"defeated [monster]"` / `"you have defeated [monster]"` | Monster killed by player | IT-4 |
| MSG-21 | `"the [weapon] hits [monster]"` | Thrown weapon hits monster | IT-4 |
| MSG-22 | `"you hit [monster]"` | Non-weapon thrown item hits | IT-4 |
| MSG-23 | `"the [weapon] misses [monster]"` | Thrown weapon misses monster | IT-4 |
| MSG-24 | `"you missed [monster]"` | Non-weapon thrown item misses | IT-4 |
| MSG-25 | `"I see no monster there"` / `"no monster there"` (terse) | Fight command (`f`) with no visible monster | IT-6 |
| MSG-26 | `"wait! That's a xeroc!"` / `"heavy! That's a nasty critter!"` (hallucinating) | Attack or be attacked by disguised Xeroc | IT-4 |
| MSG-27 | `"[monster] appears confused"` | Player with CANHUH flag hits monster, conferring confusion | IT-4 |
| MSG-28 | `"your hands stop glowing [color]"` | CANHUH expended on hit | IT-4 |

### Special Monster Attacks

| ID | Message surface | Trigger condition | Covered by |
|----|----------------|-------------------|------------|
| MSG-29 | `"you are frozen"` / `"you are frozen by the [monster]"` | Ice monster (I) freezes player | IT-4 |
| MSG-30 | `"you feel a bite in your leg and now feel weaker"` / `"a bite has weakened you"` (terse) | Rattlesnake (R) poison, no sustain strength ring | IT-4 |
| MSG-31 | `"a bite momentarily weakens you"` / `"bite has no effect"` (terse) | Rattlesnake (R) poison, wearing sustain strength ring | IT-4 |
| MSG-32 | `"you suddenly feel weaker"` | Wraith (W) drains level or Vampire (V) drains HP | IT-4 |
| MSG-33 | `"your purse feels lighter"` | Leprechaun (L) steals gold | IT-4 |
| MSG-34 | `"she stole [item]!"` | Nymph (N) steals magic item | IT-4 |

### Armor & Rust

| ID | Message surface | Trigger condition | Covered by |
|----|----------------|-------------------|------------|
| MSG-35 | `"your armor appears to be weaker now. Oh my!"` / `"your armor weakens"` (terse) | Aquator (A) rusts armor, or rust trap, and armor not protected | IT-4, IT-6 |
| MSG-36 | `"the rust vanishes instantly"` | Aquator/rust trap but armor is protected (ISPROT) or wearing sustain armor ring | IT-4, IT-6 |

### Potions

| ID | Message surface | Trigger condition | Covered by |
|----|----------------|-------------------|------------|
| MSG-37 | `"wait, what's going on here. Huh? What? Who?"` / `"what a tripy feeling!"` (hallucinating) | Quaff confusion potion | IT-5 |
| MSG-38 | `"Oh, wow! Everything seems so cosmic!"` | Quaff hallucination (LSD) potion | IT-5 |
| MSG-39 | `"you feel very sick now"` | Quaff poison potion, no sustain strength | IT-5 |
| MSG-40 | `"you feel momentarily sick"` | Quaff poison potion, wearing sustain strength | IT-5 |
| MSG-41 | `"you begin to feel better"` | Quaff healing potion | IT-5 |
| MSG-42 | `"you feel stronger, now. What bulging muscles!"` | Quaff strength potion | IT-5 |
| MSG-43 | `"you have a strange feeling for a moment, then it passes"` / `"...normal feeling..."` (hallucinating) | Quaff monster/magic detection with nothing to detect | IT-5 |
| MSG-44 | `"You sense the presence of magic on this level.--More--"` | Quaff magic detection, magic items present (show_win overlay) | IT-5 |
| MSG-45 | `"this potion tastes like [fruit] juice"` | Quaff see invisible potion | IT-5 |
| MSG-46 | `"you suddenly feel much more skillful"` | Quaff raise level potion | IT-5 |
| MSG-47 | `"you begin to feel much better"` | Quaff extra healing potion | IT-5 |
| MSG-48 | `"you feel yourself moving much faster"` | Quaff haste potion (first time) | IT-5 |
| MSG-49 | `"you faint from exhaustion"` | Quaff haste potion while already hasted | IT-5 |
| MSG-50 | `"hey, this tastes great. It make you feel warm all over"` | Quaff restore strength potion | IT-5 |
| MSG-51 | `"a cloak of darkness falls around you"` / `"oh, bummer! Everything is dark! Help!"` (hallucinating) | Quaff blindness potion | IT-5 |
| MSG-52 | `"you start to float in the air"` / `"oh, wow! You're floating in the air!"` (hallucinating) | Quaff levitation potion | IT-5 |
| MSG-53 | `"yuk! Why would you want to drink that?"` / `"that's undrinkable"` (terse) | Try to quaff non-potion | IT-5 |

### Scrolls

| ID | Message surface | Trigger condition | Covered by |
|----|----------------|-------------------|------------|
| MSG-54 | `"your hands begin to glow [color]"` | Read scroll of monster confusion | IT-5 |
| MSG-55 | `"your armor glows [color] for a moment"` | Read scroll of enchant armor (while wearing armor) | IT-5 |
| MSG-56 | `"the monster(s) freeze(s)"` / `"the monsters around you freeze"` | Read scroll of hold monster, monsters nearby | IT-5 |
| MSG-57 | `"you feel a strange sense of loss"` | Read hold monster (no monsters), enchant weapon (no weapon), or protect armor (no armor) | IT-5 |
| MSG-58 | `"you fall asleep"` | Read scroll of sleep | IT-5 |
| MSG-59 | `"you hear a faint cry of anguish in the distance"` | Read create monster scroll, no room for monster | IT-5 |
| MSG-60 | `"this scroll is an [name] scroll"` | Read identify scroll (any type) | IT-5 |
| MSG-61 | `"oh, now this scroll has a map on it"` | Read magic mapping scroll | IT-5 |
| MSG-62 | `"Your nose tingles and you smell food.--More--"` | Read food detection scroll, food present (show_win overlay) | IT-5 |
| MSG-63 | `"your nose tingles"` | Read food detection scroll, no food present | IT-5 |
| MSG-64 | `"your [weapon] glows [color] for a moment"` | Read enchant weapon scroll | IT-5 |
| MSG-65 | `"you hear maniacal laughter in the distance"` | Read scare monster scroll | IT-5 |
| MSG-66 | `"you feel as if somebody is watching over you"` / `"you feel in touch with the Universal Onenes"` (hallucinating) | Read remove curse scroll | IT-5 |
| MSG-67 | `"you hear a high pitched humming noise"` | Read aggravate monsters scroll | IT-5 |
| MSG-68 | `"your armor is covered by a shimmering [color] shield"` | Read protect armor scroll (while wearing armor) | IT-5 |
| MSG-69 | `"the scroll turns to dust as you pick it up"` | Pick up scare monster scroll that's already been on floor (ISFOUND) | IT-5 |
| MSG-70 | `"there is nothing on it to read"` / `"nothing to read"` (terse) | Try to read non-scroll | IT-5 |

### Daemon/Fuse Expiry Messages

| ID | Message surface | Trigger condition | Covered by |
|----|----------------|-------------------|------------|
| MSG-71 | `"you feel less confused now"` / `"you feel less trippy now"` (hallucinating) | Confusion effect wears off (unconfuse fuse) | IT-5 |
| MSG-72 | `"the veil of darkness lifts"` / `"far out! Everything is all cosmic again"` (hallucinating) | Blindness effect wears off (sight fuse) | IT-5 |
| MSG-73 | `"you feel yourself slowing down"` | Haste effect wears off (nohaste fuse) | IT-5 |
| MSG-74 | `"Everything looks SO boring now."` | Hallucination effect wears off (come_down) | IT-5 |
| MSG-75 | `"you float gently to the ground"` / `"bummer! You've hit the ground"` (hallucinating) | Levitation effect wears off (land fuse) | IT-5 |

### Traps

| ID | Message surface | Trigger condition | Covered by |
|----|----------------|-------------------|------------|
| MSG-76 | `"you fell into a trap!"` | Step on trapdoor trap (T_DOOR) | IT-6 |
| MSG-77 | `"you are caught in a bear trap"` | Step on bear trap (T_BEAR) | IT-6 |
| MSG-78 | `"a strange white mist envelops you and you fall asleep"` | Step on sleeping gas trap (T_SLEEP) | IT-6 |
| MSG-79 | `"oh no! An arrow shot you"` | Step on arrow trap, arrow hits | IT-6 |
| MSG-80 | `"an arrow shoots past you"` | Step on arrow trap, arrow misses | IT-6 |
| MSG-81 | `"an arrow killed you"` | Arrow trap kills player | IT-6 |
| MSG-82 | `"a small dart just hit you in the shoulder"` | Step on dart trap, dart hits | IT-6 |
| MSG-83 | `"a small dart whizzes by your ear and vanishes"` | Step on dart trap, dart misses | IT-6 |
| MSG-84 | `"a poisoned dart killed you"` | Dart trap kills player | IT-6 |
| MSG-85 | `"a gush of water hits you on the head"` | Step on rust trap (T_RUST) | IT-6 |
| MSG-86 | Mysterious trap messages (11 variants): `"you are suddenly in a parallel dimension"`, `"the light in here suddenly seems [color]"`, `"you feel a sting in the side of your neck"`, `"multi-colored lines swirl around you, then fade"`, `"a [color] light flashes in your eyes"`, `"a spike shoots past your ear!"`, `"[color] sparks dance across your armor"`, `"you suddenly feel very thirsty"`, `"you feel time speed up suddenly"`, `"time now seems to be going slower"`, `"you pack turns [color]!"` | Step on mysterious trap (T_MYST), one chosen by `rnd(11)` | IT-6 |
| MSG-87 | Trap names via search/identify: `"a trapdoor"`, `"an arrow trap"`, `"a sleeping gas trap"`, `"a beartrap"`, `"a teleport trap"`, `"a poison dart trap"`, `"a rust trap"`, `"a mysterious trap"` | Search finds trap, or `^` identify trap command | IT-6 |
| MSG-88 | `"a secret door"` | Search finds secret door | IT-6 |

### Sticks (Wands/Staves)

| ID | Message surface | Trigger condition | Covered by |
|----|----------------|-------------------|------------|
| MSG-89 | `"the corridor glows and then fades"` | Zap light wand in corridor | IT-5 |
| MSG-90 | `"the room is lit"` / `"the room is lit by a shimmering [color] light"` | Zap light wand in dark room | IT-5 |
| MSG-91 | `"you are too weak to use it"` | Zap drain life wand with HP < 2 | IT-5 |
| MSG-92 | `"you have a tingling feeling"` | Zap drain life wand, no monsters in room | IT-5 |
| MSG-93 | `"missle vanishes"` / `"the missle vanishes with a puff of smoke"` | Zap missile wand, missile misses all targets | IT-5 |
| MSG-94 | `"the [bolt/flame/ice] bounces"` | Bolt/flame/ice wand bolt reflects off wall | IT-5 |
| MSG-95 | `"the flame bounces"` / `"the flame bounces off the dragon"` | Flame wand used against dragon (immune) | IT-5 |
| MSG-96 | `"[bolt/flame/ice] misses"` / `"the [name] whizzes past [monster]"` | Bolt wand: monster saves against magic | IT-5 |
| MSG-97 | `"the [name] hits"` / `"you are hit by the [name]"` | Bolt wand: bolt hits player (reflected) | IT-5 |
| MSG-98 | `"the [name] whizzes by you"` | Bolt wand: player saves against reflected bolt | IT-5 |
| MSG-99 | `"nothing happens"` | Zap wand with 0 charges | IT-5 |
| MSG-100 | `"you can't zap with that!"` | Try to zap non-stick | IT-5 |

### Inventory & Items

| ID | Message surface | Trigger condition | Covered by |
|----|----------------|-------------------|------------|
| MSG-101 | `"you now have [item] ([letter])"` / `"[item] ([letter])"` (terse) | Pick up item from floor | IT-6 |
| MSG-102 | `"[N] gold pieces"` / `"you found [N] gold pieces"` | Pick up gold | IT-6 |
| MSG-103 | `"you moved onto [item]"` / `"moved onto [item]"` (terse) | Move onto item with `m` (move-without-pickup) | IT-6 |
| MSG-104 | `"there is nothing here to pick up"` / `"nothing here"` (terse) | `,` pick up command on empty square | IT-6 |
| MSG-105 | `"there's no room in your pack"` / `"no room"` (terse) | Pack full (`MAXPACK=23`) | IT-6 |
| MSG-106 | `"you are empty handed"` / `"empty handed"` (terse) | Full inventory (`i`) with empty pack | IT-6 |
| MSG-107 | `"you don't have anything appropriate"` / `"nothing appropriate"` (terse) | Inventory for specific type but none carried | IT-6 |
| MSG-108 | `"you aren't carrying anything"` | `get_item` or `picky_inven` with empty pack | IT-6 |
| MSG-109 | `"which object do you want to [purpose]? (* for list): "` / `"[purpose] what? (* for list): "` (terse) | `get_item` prompt for selecting an item | IT-6 |
| MSG-110 | `"'[ch]' is not a valid item"` | Invalid pack character entered in `get_item` | IT-6 |
| MSG-111 | `"which item do you wish to inventory: "` / `"item: "` (terse) | Selective inventory (`I`) prompt | IT-6 |
| MSG-112 | `"'[ch]' not in pack"` | Invalid character in selective inventory | IT-6 |
| MSG-113 | `"you ran out"` | Repeat command (`a`) but last picked item exhausted | IT-6 |
| MSG-114 | Inventory list display (per-item: `"[ch]) [item_name]"`) | Full inventory command (`i`) or `*` in get_item | IT-6 |

### Equipment Management

| ID | Message surface | Trigger condition | Covered by |
|----|----------------|-------------------|------------|
| MSG-115 | `"wielding [item] ([letter])"` / `"you are now wielding [item] ([letter])"` | Wield weapon command (`w`) | IT-6 |
| MSG-116 | `"you can't wield armor"` | Try to wield armor | IT-6 |
| MSG-117 | `"wearing [item]"` / `"you are now wearing [item]"` | Wear armor command (`W`) | IT-6 |
| MSG-118 | `"you are already wearing some"` / `"...You'll have to take it off first"` | Wear armor while already wearing armor | IT-6 |
| MSG-119 | `"you can't wear that"` | Try to wear non-armor | IT-6 |
| MSG-120 | `"was wearing [item] ([letter])"` / `"you used to be wearing [item] ([letter])"` | Take off armor (`T`) | IT-6 |
| MSG-121 | `"not wearing armor"` / `"you aren't wearing any armor"` (non-terse) | Take off armor with nothing worn | IT-6 |
| MSG-122 | `"you are now wearing [ring] ([letter])"` / `"[ring] ([letter])"` (terse) | Put on ring (`P`) | IT-6 |
| MSG-123 | `"it would be difficult to wrap that around a finger"` / `"not a ring"` (terse) | Try to put on non-ring | IT-6 |
| MSG-124 | `"you already have a ring on each hand"` / `"wearing two"` (terse) | Put on ring with both hands full | IT-6 |
| MSG-125 | `"left hand or right hand? "` / `"left or right ring? "` (terse) | Ring hand selection prompt | IT-6 |
| MSG-126 | `"please type L or R"` / `"L or R"` (terse) | Invalid ring hand input | IT-6 |
| MSG-127 | `"was wearing [ring]([letter])"` | Remove ring (`R`) | IT-6 |
| MSG-128 | `"you aren't wearing any rings"` / `"no rings"` (terse) | Remove ring with none worn | IT-6 |
| MSG-129 | `"not wearing such a ring"` | Remove ring from empty hand | IT-6 |
| MSG-130 | `"That's already in use"` / `"in use"` (terse) | Try to wield/wear/put on an item currently equipped | IT-6 |
| MSG-131 | `"dropped [item]"` | Drop item (`d`) | IT-6 |
| MSG-132 | `"there is something there already"` | Drop item on occupied square | IT-6 |
| MSG-133 | `"you can't. It appears to be cursed"` | Drop/remove cursed item | IT-6 |

### Naming & Identification

| ID | Message surface | Trigger condition | Covered by |
|----|----------------|-------------------|------------|
| MSG-134 | `"what do you want to call it? "` / `"call it: "` (terse) | Call/name command (`c`) or auto-prompt after using unknown item | IT-6 |
| MSG-135 | `"that has already been identified"` | Call an already-identified item type | IT-6 |
| MSG-136 | `"you can't call that anything"` | Try to call food | IT-6 |
| MSG-137 | `"was called \"[name]\""` / `"called \"[name]\""` (terse) | Call item that already has a guess name | IT-6 |
| MSG-138 | `"you don't have anything in your pack to identify"` | Identify with empty pack | IT-6 |
| MSG-139 | `"you must identify something"` | Cancel required identify action | IT-5 |
| MSG-140 | `"you must identify a [type]"` | Identify scroll used on wrong type | IT-5 |

### Help & Identify Character

| ID | Message surface | Trigger condition | Covered by |
|----|----------------|-------------------|------------|
| MSG-141 | `"character you want help for (* for all): "` | Help command (`?`) prompt | IT-7 |
| MSG-142 | Full help list display (two-column format with all commands and descriptions) | Help `*` option | IT-7 |
| MSG-143 | Single-character help: `"[ch] [description]"` | Help for specific character | IT-7 |
| MSG-144 | `"unknown character '[ch]'"` | Help for unrecognized character | IT-7 |
| MSG-145 | `"--Press space to continue--"` | After help list, options screen, map display | IT-7 |
| MSG-146 | `"what do you want identified? "` | Identify character (`/`) prompt | IT-7 |
| MSG-147 | `"'[ch]': [description]"` (e.g., `"'@': you"`, `"'!': potion"`, `"'A': aquator"`) | Character identified via `/` | IT-7 |
| MSG-148 | `"'[ch]': unknown character"` | Unrecognized character via `/` | IT-7 |

### Direction Prompts

| ID | Message surface | Trigger condition | Covered by |
|----|----------------|-------------------|------------|
| MSG-149 | `"which direction? "` / `"direction: "` (terse) | Direction prompt for throw, zap, fight, move, identify trap | IT-6 |

### Current Equipment Display

| ID | Message surface | Trigger condition | Covered by |
|----|----------------|-------------------|------------|
| MSG-150 | `"you are wielding ([ch]) [weapon]"` / `"wielding nothing"` | `)` command (current weapon) | IT-6 |
| MSG-151 | `"you are wearing ([ch]) [armor]"` / `"wearing nothing"` | `]` command (current armor) | IT-6 |
| MSG-152 | `"you are wearing ([ch]) [ring] on left/right hand"` / `"wearing nothing (L)/(R)"` | `=` command (current rings) | IT-6 |

### Level Transitions

| ID | Message surface | Trigger condition | Covered by |
|----|----------------|-------------------|------------|
| MSG-153 | `"I see no way down"` | `>` (descend) not on stairs | IT-6 |
| MSG-154 | `"I see no way up"` | `<` (ascend) not on stairs | IT-6 |
| MSG-155 | `"your way is magically blocked"` | `<` (ascend) on stairs but no Amulet | IT-6 |
| MSG-156 | `"you feel a wrenching sensation in your gut"` | `<` (ascend) on stairs with Amulet, ascending | IT-6 |
| MSG-157 | `"You can't. You're floating off the ground!"` | Try to use stairs or pick up while levitating | IT-5 |

### Level Up

| ID | Message surface | Trigger condition | Covered by |
|----|----------------|-------------------|------------|
| MSG-158 | `"welcome to level [N]"` | Player gains enough XP to level up | IT-4 |

### Search & Discovery

| ID | Message surface | Trigger condition | Covered by |
|----|----------------|-------------------|------------|
| MSG-159 | `"you found [trap_name]"` / `"[trap_name]"` (terse) | Search (`s`) discovers hidden trap | IT-6 |
| MSG-160 | Discovered items list display | `D` command (recall discoveries) | IT-6 |

### Options

| ID | Message surface | Trigger condition | Covered by |
|----|----------------|-------------------|------------|
| MSG-161 | Options screen with labeled options: terse, flush, jump, seefloor, passgo, tombstone, inven, name, fruit, file | `o` command (examine/set options) | IT-7 |
| MSG-162 | `"(T or F)"` | Invalid boolean option input | IT-7 |
| MSG-163 | `"(O, S, or C)"` | Invalid inventory type input | IT-7 |

### Version

| ID | Message surface | Trigger condition | Covered by |
|----|----------------|-------------------|------------|
| MSG-164 | `"version [version]. (mctesq was here)"` | `v` command | IT-7 |

### Save/Load

| ID | Message surface | Trigger condition | Covered by |
|----|----------------|-------------------|------------|
| MSG-165 | `"save file ([filename])? "` | `S` command with existing save filename | IT-8 |
| MSG-166 | `"file name: "` | `S` command, entering new save filename | IT-8 |
| MSG-167 | `"please answer Y or N"` | Invalid Y/N response during save | IT-8 |
| MSG-168 | `"file name: [name]"` | Restore game, showing restored filename | IT-8 |

### Quit & Death

| ID | Message surface | Trigger condition | Covered by |
|----|----------------|-------------------|------------|
| MSG-169 | `"really quit?"` | `Q` command (quit) | IT-6 |
| MSG-170 | `"You quit with [N] gold pieces"` | Quit confirmed | IT-6 |
| MSG-171 | Tombstone ASCII art with player name, gold, year, killer | Death (with tombstone option enabled) | IT-6 |
| MSG-172 | `"Killed by [killer] with [N] gold"` | Death (tombstone option disabled) | IT-6 |
| MSG-173 | Top scores display: `"Top [N] [Scores/Rogueists]:"` with ranked entries | After death or quit | IT-6 |
| MSG-174 | `"[Press return to continue]"` | After death tombstone, after score display | IT-6 |

### Victory

| ID | Message surface | Trigger condition | Covered by |
|----|----------------|-------------------|------------|
| MSG-175 | `"You Made It!"` ASCII art banner | `total_winner()` -- ascend level 0 with Amulet | IT-6 |
| MSG-176 | `"Congratulations, you have made it to the light of day!"` + victory text | `total_winner()` congratulations text | IT-6 |
| MSG-177 | Itemized inventory with gold values | `total_winner()` loot valuation display | IT-6 |

### Miscellaneous

| ID | Message surface | Trigger condition | Covered by |
|----|----------------|-------------------|------------|
| MSG-178 | `"illegal command '[ch]'"` | Unknown/invalid command key pressed | IT-7 |
| MSG-179 | `"you haven't typed a command yet"` | Repeat (`a`) with no previous command | IT-6 |
| MSG-180 | `"the [weapon] vanishes as it hits the ground"` | Thrown weapon lands but no room for it to fall | IT-6 |
| MSG-181 | `"no trap there"` | `^` identify trap: no trap at that position | IT-6 |
| MSG-182 | `"You have found [trap_name]"` / `"[trap_name]"` (terse) | `^` identify trap: trap found | IT-6 |
| MSG-183 | Previous message repeat | `Ctrl-P` command | IT-7 |
| MSG-184 | `"@` stat display: `"Level: N  Gold: N  Hp: N(N)  Str: N(N)  Arm: N  Exp: N/N [state]"` on message line | `@` command (stat_msg mode) | IT-6 |

### Wizard Mode Messages

| ID | Message surface | Trigger condition | Covered by |
|----|----------------|-------------------|------------|
| MSG-185 | `"you are suddenly as smart as Ken Arnold in dungeon #[N]"` | Successfully enter wizard mode (`+`) | IT-9 |
| MSG-186 | `"not wizard any more"` | Toggle wizard mode off (`+`) | IT-9 |
| MSG-187 | `"sorry"` | Failed wizard password | IT-9 |
| MSG-188 | `"type of item: "` | Wizard create object prompt 1 | IT-9 |
| MSG-189 | `"which [type] do you want? (0-f)"` | Wizard create object prompt 2 | IT-9 |
| MSG-190 | `"blessing? (+,-,n)"` | Wizard create object prompt 3 (weapons/armor/certain rings) | IT-9 |
| MSG-191 | `"@ [y],[x]"` | Wizard position display (`\|`) | IT-9 |
| MSG-192 | `"inpack = [N]"` | Wizard pack count (`$`) | IT-9 |
| MSG-193 | `"food left: [N]"` | Wizard food display (`Ctrl-E`) | IT-9 |
| MSG-194 | `"---More (level map)---"` | Wizard map reveal (`Ctrl-F`) | IT-9 |
| MSG-195 | `"how much?"` | Wizard create gold prompt | IT-9 |

### Browser/WASM-Specific

| ID | Message surface | Trigger condition | Covered by |
|----|----------------|-------------------|------------|
| MSG-196 | `"Hello [name], just a moment while I dig the dungeon..."` or equivalent startup message | Game initialization in browser | IT-7 |

## Integration Test Scenarios

| ID | Scenario | Steps | Verification |
|----|----------|-------|--------------|
| IT-1 | Build & deliverable check | 1. Run `cargo fmt --check` in `demo/rogue/rogue-wasm/` -> exits 0 2. Run `cargo build --target wasm32-unknown-unknown` -> exits 0 3. Assert `demo/rogue/rogue-wasm/www/index.html` exists 4. Assert `index.html` contains reference to WASM module (grep for `.wasm` or `pkg/` or `init`) 5. Assert `git diff --name-only` does not include paths matching `target/`, `node_modules/`, `dist/` | `test exits 0` |
| IT-2 | RNG fidelity | 1. Seed RNG with known seed (e.g., 12345) 2. Call `rnd()` 100 times, compare output sequence against precomputed C reference values 3. Verify `rnd(range)` for ranges [0, 1, 10, 100, 0] matches C behavior (range=0 returns 0) 4. Verify `roll(3, 6)` for 10 iterations with same seed matches C output 5. Reset seed, run 50-turn game sequence programmatically, compare final RNG state against C reference | `cargo test rng_fidelity` exits 0 |
| IT-3 | Dungeon generation fidelity | 1. Seed RNG with fixed seed 2. Generate level 1 dungeon 3. Assert room count, positions, sizes, door locations match C reference for that seed 4. Assert passage connectivity matches C reference 5. Assert gone/dark/maze room flags match 6. Generate level 5 dungeon with same seed progression, verify trap count and positions 7. Verify item spawn count (9 attempts at 36%) and gold amounts match 8. Verify daemon/fuse initial scheduling (swander, stomach, doctor timing) | `cargo test dungeon_gen_fidelity` exits 0 |
| IT-4 | Combat & monster fidelity | 1. Seed RNG, initialize game engine 2. Verify all 26 monster stat blocks (A-Z) match `extern.c` values: carry%, flags, str, exp, lvl, armor, HP formula, damage strings 3. Verify `str_plus[]` (32 entries) matches C array exactly 4. Verify `add_dam[]` (32 entries) matches C array exactly 5. Simulate player attack with known str/weapon/armor values: verify to-hit and damage match C formula 6. Simulate monster attack on player: verify hit/miss messages emitted 7. Simulate special attacks: aquator rust (verify armor weakening message), rattlesnake poison (verify strength drain and message), wraith level drain (verify message), leprechaun gold steal (verify purse and message), nymph item steal (verify message), ice monster freeze (verify freeze message), xeroc reveal (verify reveal message) 8. Kill a monster, verify "defeated" message and XP gain 9. Verify level-up message "welcome to level N" when XP threshold crossed 10. Verify monster scaling beyond AMULETLEVEL=26 | `cargo test combat_monster_fidelity` exits 0 |
| IT-5 | Items, potions, scrolls, sticks fidelity | 1. Verify item type distribution probabilities: potion 26%, scroll 36%, food 16%, weapon 7%, armor 7%, ring 4%, stick 4% 2. Verify per-type probabilities for all 14 potions, 18 scrolls, 9 weapons, 8 armors, 14 rings, 14 sticks match `extern.c` 3. Verify weapon damage tables and armor AC values 4. Programmatically quaff each of the 14 potion types with fixed seed, verify correct message emitted for each (MSG-37 through MSG-53) 5. Programmatically read each of the 18 scroll types with fixed seed, verify correct message emitted for each (MSG-54 through MSG-70) 6. Programmatically zap representative stick types (light, drain, missile, bolt/fire/ice), verify messages (MSG-89 through MSG-100) 7. Verify confusion/blindness/haste/hallucination/levitation duration fuses expire with correct messages (MSG-71 through MSG-75) 8. Verify levitation blocks stair use and pickup (MSG-157) 9. Verify identify scroll flow (MSG-139, MSG-140) 10. Verify ring/wand charge ranges | `cargo test item_fidelity` exits 0 |
| IT-6 | Multi-turn gameplay integration | 1. Seed RNG with fixed seed 2. Initialize game engine with deterministic state 3. Execute a scripted multi-turn sequence (50+ turns): move in all 8 directions, pick up gold (verify MSG-102), pick up item (verify MSG-101), eat food (verify MSG-11/12/13), wield weapon (verify MSG-115), wear armor (verify MSG-117), take off armor (verify MSG-120), equip/remove ring (verify MSG-122, MSG-127), drop item (verify MSG-131) 4. Verify status line (MSG-1) updates correctly after each action: Level, Gold, Hp, Str, Arm, Exp, hunger state (MSG-2) 5. Verify hunger progression: satiated -> hungry (MSG-8) -> weak (MSG-9) -> faint (MSG-10) by consuming turns without eating 6. Trigger trap via level generation with known seed: verify trap message (one of MSG-76 through MSG-86) 7. Search for hidden door/trap (verify MSG-88, MSG-87/MSG-159) 8. Test direction prompt (MSG-149) 9. Verify stair navigation messages (MSG-153 through MSG-156) 10. Verify pack-full message (MSG-105) by filling inventory to 23 items 11. Verify empty-inventory messages (MSG-106, MSG-108) 12. Verify current equipment display messages (MSG-150, MSG-151, MSG-152) 13. Test quit flow: Q -> "really quit?" (MSG-169) -> confirm -> "You quit with N gold pieces" (MSG-170) 14. Verify combat messages during movement into monster (MSG-15 through MSG-20 covered via IT-4) 15. Verify repeat command (MSG-179 if no previous command) 16. Verify "you can move again" (MSG-7) after freeze duration expires 17. Verify "you are being held" (MSG-6) and "stuck in bear trap" (MSG-5) 18. Verify item naming flow: call command (MSG-134 through MSG-137) 19. Verify throw flow and "vanishes as it hits the ground" (MSG-180) 20. Verify "nothing here to pick up" (MSG-104), "moved onto" (MSG-103) 21. Verify invalid item selection (MSG-110, MSG-112, MSG-113) 22. Verify cursed item behavior (MSG-133) 23. Verify armor rust messages (MSG-35, MSG-36) 24. Verify "@" stat display (MSG-184) 25. Compare final game state (player position, HP, inventory, level) against C reference for same seed | `cargo test gameplay_integration` exits 0 |
| IT-7 | Browser deliverable (semantic verification) | 1. Serve `demo/rogue/rogue-wasm/www/` via HTTP server 2. Load `index.html` in headless browser (or verify via WASM test harness) 3. Confirm 80x24 character grid renders with monospace font on dark background 4. Confirm `@` player symbol, `#` corridors, `.` floors, `+` doors, `|`/`-` walls, all object symbols visible (MSG-4) 5. Confirm status line on row 24 (MSG-1) 6. Confirm message line at row 0 with `--More--` support (MSG-3) 7. Press `?` -> confirm help prompt "character you want help for..." (MSG-141) 8. Press `*` in help -> confirm full help list displayed (MSG-142) with "--Press space to continue--" (MSG-145) 9. Press `/` -> confirm identify prompt (MSG-146), type `@` -> see "'@': you" (MSG-147) 10. Press `v` -> confirm version message (MSG-164) 11. Press invalid key -> confirm "illegal command" (MSG-178) 12. Press `Ctrl-P` -> confirm previous message repeated (MSG-183) 13. Press `o` -> confirm options screen (MSG-161) with all option labels 14. Confirm keyboard input for all Rogue commands (movement keys, action keys respond) 15. Confirm startup/welcome message (MSG-196) | Semantic verification: examiner confirms each check passes |
| IT-8 | Save/load via localStorage | 1. Seed game with fixed seed, play 20 turns programmatically (move, pick up items, descend level) 2. Trigger save (or programmatic save API call) 3. Record complete game state: player position, HP, max_hp, str, exp, level, inventory contents, current armor/weapon/rings, RNG seed, room states, monster list, daemon/fuse queue, hunger state, discovered items 4. Destroy game instance (simulate tab close) 5. Create new game instance, trigger restore from localStorage 6. Compare all recorded state fields: every field must be identical 7. Play 10 more turns and verify game continues correctly (no crashes, state consistent) | `cargo test save_load_integration` exits 0 |
| IT-9 | Wizard mode integration | 1. Initialize game engine 2. Enable wizard mode via browser-friendly mechanism (JS API / URL param) 3. Verify wizard mode active (noscore set) 4. Verify wizard mode enable message (MSG-185 or equivalent) 5. Create object (`C`): verify prompts (MSG-188, MSG-189, MSG-190), verify item added to pack 6. Teleport (`Ctrl-T`): verify player position changed 7. Level skip down (`Ctrl-D`): verify `level` incremented and new dungeon generated 8. Level skip up (`Ctrl-A`): verify `level` decremented 9. Map reveal (`Ctrl-F`): verify map overlay shown (MSG-194) 10. Identify (`Ctrl-W`): verify item identified in pack 11. Show position (`\|`): verify position message (MSG-191) 12. Show inpack count (`$`): verify count message (MSG-192) 13. Show food left (`Ctrl-E`): verify food message (MSG-193) 14. Toggle wizard off (`+`): verify "not wizard any more" (MSG-186) 15. Show level objects (`Ctrl-G`): verify level object inventory displayed 16. Charge stick (`Ctrl-~`): verify stick charges set to 10000 17. Super-equip (`Ctrl-I`): verify 9 level-ups, two-handed sword, plate mail added | `cargo test wizard_mode_integration` exits 0 |

## Crosscheck

### Per Scenario

| Scenario | Exercises delivered artifact? | Automatable, bounded, proportional, independent? | Crosses multiple AC groups? |
|----------|-------------------------------|--------------------------------------------------|----------------------------|
| IT-1 | Yes -- builds the WASM artifact and checks deliverable files | Yes: runs build commands, checks files, deterministic | Build (AC-1), Deliverable (AC-2) |
| IT-2 | Yes -- exercises the Rust game engine's RNG module | Yes: fixed seed, finite sequence, deterministic comparison | RNG Fidelity (AC-3), Deterministic Replay (AC-15) |
| IT-3 | Yes -- exercises the Rust game engine's dungeon generator | Yes: fixed seed, specific level count, deterministic | Dungeon Gen (AC-4), Daemon/Fuse (AC-8), Level Progression (AC-10) |
| IT-4 | Yes -- exercises the Rust game engine's combat system | Yes: fixed seed, scripted combat sequences, finite monsters | Monster Fidelity (AC-5), Combat Fidelity (AC-6), Level Up (AC-15) |
| IT-5 | Yes -- exercises the Rust game engine's item system | Yes: fixed seed, all item types enumerable, bounded | Item Fidelity (AC-7), Display (AC-12) |
| IT-6 | Yes -- exercises the Rust game engine end-to-end | Yes: fixed seed, 50+ scripted turns, deterministic | Commands (AC-11), Display (AC-12), Hunger (AC-9), Deterministic Replay (AC-15) |
| IT-7 | Yes -- loads the HTML deliverable in a browser | Semantic: manual checklist but bounded (15 checks). Proportional: covers all display ACs | Deliverable (AC-2), Display (AC-12), Commands (AC-11) |
| IT-8 | Yes -- exercises the Rust game engine's save/load | Yes: fixed seed, deterministic state comparison | Save/Load (AC-13), Commands (AC-11) |
| IT-9 | Yes -- exercises the Rust game engine's wizard mode | Yes: fixed seed, each wizard command tested once, deterministic | Wizard Mode (AC-14), Commands (AC-11) |

### Per AC

| AC | Covered by scenario(s) | Gap? |
|----|------------------------|------|
| AC-1.1 | IT-1 | No |
| AC-1.2 | IT-1 | No |
| AC-1.3 | IT-2, IT-3, IT-4, IT-5, IT-6, IT-8, IT-9 | No |
| AC-2.1 | IT-1 | No |
| AC-2.2 | IT-1 | No |
| AC-2.3 | IT-1 | No |
| AC-2.4 | IT-7 | No |
| AC-3.1 | IT-2 | No |
| AC-3.2 | IT-2 | No |
| AC-3.3 | IT-2 | No |
| AC-4.1 | IT-3 | No |
| AC-4.2 | IT-3 | No |
| AC-4.3 | IT-3 | No |
| AC-4.4 | IT-3 | No |
| AC-5.1 | IT-4 | No |
| AC-5.2 | IT-4 | No |
| AC-6.1 | IT-4 | No |
| AC-6.2 | IT-4 | No |
| AC-6.3 | IT-4 | No |
| AC-6.4 | IT-4 | No |
| AC-7.1 | IT-5 | No |
| AC-7.2 | IT-5 | No |
| AC-7.3 | IT-5 | No |
| AC-8.1 | IT-3 | No |
| AC-8.2 | IT-3 | No |
| AC-9.1 | IT-6 | No |
| AC-9.2 | IT-6 | No |
| AC-10.1 | IT-3, IT-4 | No |
| AC-10.2 | IT-3 | No |
| AC-10.3 | IT-3 | No |
| AC-10.4 | IT-3 | No |
| AC-11.1 | IT-6 | No |
| AC-11.2 | IT-6 | No |
| AC-11.3 | IT-6 | No |
| AC-12.1 | IT-7 | No |
| AC-12.2 | IT-6, IT-7 | No |
| AC-12.3 | IT-6, IT-7 | No |
| AC-13.1 | IT-8 | No |
| AC-13.2 | IT-8 | No |
| AC-14.1 | IT-9 | No |
| AC-14.2 | IT-9 | No |
| AC-14.3 | IT-9 | No |
| AC-14.4 | IT-9 | No |
| AC-14.5 | IT-9 | No |
| AC-14.6 | IT-9 | No |
| AC-14.7 | IT-9 | No |
| AC-14.8 | IT-9 | No |
| AC-15.1 | IT-2, IT-3, IT-4, IT-6 | No |

### Overall

1. **At least one scenario tests the deliverable in its delivery form**: IT-7 loads the HTML deliverable in a browser and verifies it renders and accepts input. IT-1 verifies the WASM build succeeds and the HTML file exists and references the module.

2. **Every user-facing message from the inventory is triggered and validated by at least one scenario**: All 196 message surfaces (MSG-1 through MSG-196) are mapped to at least one IT scenario. MSG-1 through MSG-4 (core display) are covered by IT-6 and IT-7. MSG-5 through MSG-14 (movement/hunger) by IT-6. MSG-15 through MSG-36 (combat/monsters) by IT-4 and IT-6. MSG-37 through MSG-75 (potions/scrolls/daemon expiry) by IT-5. MSG-76 through MSG-88 (traps/search) by IT-6. MSG-89 through MSG-100 (sticks) by IT-5. MSG-101 through MSG-184 (inventory/equipment/misc) by IT-6. MSG-185 through MSG-195 (wizard) by IT-9. MSG-196 (browser startup) by IT-7.

3. **Scenarios collectively cover every AC group**: Build (IT-1), Deliverable (IT-1, IT-7), RNG Fidelity (IT-2), Dungeon Gen (IT-3), Monster Fidelity (IT-4), Combat Fidelity (IT-4), Item Fidelity (IT-5), Daemon/Fuse (IT-3), Hunger (IT-6), Level Progression (IT-3), Commands (IT-6), Display (IT-7), Save/Load (IT-8), Wizard Mode (IT-9), Deterministic Replay (IT-2, IT-3, IT-4, IT-6).
