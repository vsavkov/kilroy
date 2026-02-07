# Kilroy Font Factory: Research Findings

## Project Concept

Build an autonomous software factory demo: **bitmap images of letters in, finished font file out.** The engine traces bitmap glyphs into vector outlines, assembles them into a valid OTF/TTF font, and optimizes quality — all guided by a computable objective function with zero human judgment in the loop.

The test corpus is self-generating: render known fonts to bitmaps, run them through the engine, compare the output font against the source. Every font on Google Fonts (1,907 families) is a test case.

**Target language:** Rust (with Go as a fallback option)

---

## 1. Tool & Library Landscape

### 1.1 Bitmap-to-Vector Tracing

#### Potrace (C, GPL-2.0+)
- Industry standard bitmap tracer. Input: 1-bit PBM. Output: SVG/EPS/PDF.
- **Algorithm (4 phases):** path decomposition → optimal polygon → Bezier curve fitting → curve optimization
- Last release: v1.16 (2019). Mature, stable, single maintainer (Peter Selinger).
- **Test suite:** Minimal (`make check` only). No regression test images.
- **Font-specific settings:** `opttolerance=0.1` (more accuracy), `turdsize=0` (preserve small details), `unit=100+` (coordinate precision), `alphamax=1.0` (default corner threshold)
- **Complexity:** O(n^2) fitting algorithm
- FontForge uses Potrace internally for auto-trace; warns results are "fairly bad unless you have a large (100+ pixel) font to trace"
- References: [Homepage](https://potrace.sourceforge.net/), [Algorithm paper (PDF)](https://potrace.sourceforge.net/potrace.pdf)

#### VTracer (Rust, MIT/Apache-2.0)
- Pure Rust raster-to-vector converter. Handles color/grayscale directly (no pre-binarization needed).
- **Algorithm:** O(n) fitting (vs Potrace's O(n^2)). Uses clustering to segment image into regions.
- Originally designed for gigapixel historic blueprint scans.
- Actively maintained. Available as Rust crate and Python package (via pyo3).
- **No documented test suite.**
- References: [GitHub](https://github.com/visioncortex/vtracer), [Docs](https://www.visioncortex.org/vtracer-docs)

#### AutoTrace (C, GPL-2.0+ / LGPL-2.1+ for I/O)
- Supports **centerline tracing** (medial axis), useful for single-stroke fonts.
- Originally abandoned (>19 years), revived with build modernization patches (v0.31.10).
- No formal test suite.
- References: [GitHub](https://github.com/autotrace/autotrace)

#### Recommendation
**VTracer for the Rust pipeline.** Pure Rust, O(n), handles grayscale input, MIT/Apache license, actively maintained. Potrace is better-tested for font work specifically but requires C FFI and binarized input.

### 1.2 Font Manipulation Libraries

#### fonttools (Python, MIT)
- The Swiss Army knife of font engineering. Read/write/modify every OpenType table.
- **Extensive test suite** tracked on Coveralls. ~5k GitHub stars. Very actively maintained (v4.61.1, Dec 2025).
- Key capabilities: TTX (font↔XML), subsetting, merging, variable fonts, UFO support.
- `fontTools.pens.FreeTypePen` renders glyphs to numpy arrays (perfect for test pipeline).
- `fontTools.pens.StatisticsPen` computes per-glyph area, centroid, slant.
- References: [GitHub](https://github.com/fonttools/fonttools), [Docs](https://fonttools.readthedocs.io/)

#### Google Fontations (Rust, MIT/Apache-2.0)
- Google's strategic Rust font tooling investment. **Integrated into Chrome** (v133+, replacing FreeType).
- **read-fonts:** Zero-allocation font parser. Production quality.
- **write-fonts:** Font modification and serialization. Active development.
- **skrifa:** High-level font access, glyph outlines, scaling. Replacing FreeType in Chrome.
- **fontc:** Rust font compiler (replaces Python fontmake). Has a GitHub Action.
- ~700 unit tests + OSS-Fuzz fuzzing.
- References: [GitHub](https://github.com/googlefonts/fontations), [Chrome blog](https://developer.chrome.com/blog/memory-safety-fonts)

#### fontmake + ufo2ft (Python, Apache-2.0)
- Google Fonts production pipeline: source (.glyphs/.ufo) → binary (.otf/.ttf).
- References: [fontmake](https://github.com/googlefonts/fontmake), [ufo2ft](https://github.com/googlefonts/ufo2ft)

### 1.3 Font Validation / QA

#### fontspector (Rust, Apache-2.0)
- **Successor to fontbakery.** Rust port built on skrifa/read-fonts.
- ~200+ checks: OpenType compliance, naming, metrics, glyph coverage, hinting.
- **~1000x faster** than fontbakery — scans entire Google Fonts library in seconds.
- WASM version available for browser-based testing.
- References: [GitHub](https://github.com/fonttools/fontspector)

#### fontbakery (Python, Apache-2.0)
- Legacy QA tool being replaced by fontspector. Still comprehensive: universal, Google Fonts, Adobe, Noto profiles.
- References: [GitHub](https://github.com/fonttools/fontbakery)

#### OpenType Sanitizer / OTS (C++)
- Used by Chrome/Firefox to validate fonts before rendering. If OTS rejects a font, browsers won't render it.
- Checks: table structure integrity, offset validity, required tables, internal consistency.
- References: [GitHub](https://github.com/khaledhosny/ots)

#### Microsoft Font Validator (C#)
- Validates against full OpenType spec. Detailed error/warning/info reports.
- References: [GitHub](https://github.com/microsoft/Font-Validator)

### 1.4 Font Editors with Auto-Trace

#### FontForge (C + Python, GPL-3.0/BSD-3)
- Can invoke Potrace/AutoTrace on background bitmaps. Python scripting API.
- Active (Oct 2025 release).
- References: [GitHub](https://github.com/fontforge/fontforge), [Auto-trace docs](https://fontforge.org/docs/techref/autotrace.html)

#### BirdFont (Vala, GPL-3.0)
- Built-in autotrace (no external dependency). Parameters: threshold, detail, simplification, quad/cubic.
- References: [GitHub](https://github.com/johanmattssonm/birdfont)

### 1.5 SVG-to-Font Pipeline (Node.js)

```
Bitmap → Potrace → SVG per glyph → svgicons2svgfont → SVG font → svg2ttf → TTF
```

- **svg2ttf** (MIT): SVG font → TTF. Mocha tests.
- **svgicons2svgfont** (MIT): SVG icons → SVG font. Jest/Mocha tests.
- Higher-level wrappers: svgtofont, webfont.

### 1.6 Other Rust Font Crates

| Crate | Purpose | Status |
|-------|---------|--------|
| `kurbo` | 2D curves / Bezier path math | Active (linebender) |
| `norad` | UFO file read/write | Active (linebender) |
| `fontdue` | Fastest pure Rust rasterizer, no_std | Maintained |
| `swash` | Font introspection + rasterization, 10-20% faster than FreeType | Maintained |
| `pixglyph` | Simple glyph rasterizer (used by Typst), no unsafe | Maintained |
| `ab_glyph` | 1.5-8.8x faster than rusttype | Maintained |
| `rustybuzz` | Pure Rust HarfBuzz port (text shaping) | Maintained |
| `ttf-parser` | Zero-allocation TrueType/OpenType parser | Maintained |
| `font-kit` | Cross-platform font loading/discovery | Maintained |
| `dssim` | Multi-scale SSIM in Rust | Maintained |

### 1.7 Go Font Ecosystem

| Package | Purpose | Notes |
|---------|---------|-------|
| `golang.org/x/image/font/sfnt` | Low-level TrueType/OpenType parser | Official Go team, stable |
| `golang.org/x/image/font/opentype` | High-level font.Face for rasterization | Official Go team, stable |
| `github.com/golang/freetype` | Pure Go FreeType rasterizer port | FreeType license |
| `seehuhn/go-sfnt` | Font read/write (more features than stdlib) | GPL-3.0, active |

**Go limitation:** No pure-Go equivalent of fonttools/fontmake for font compilation. Would need CGo for tracing/assembly. Rust is the stronger choice.

### 1.8 Existing Pipelines (None Turnkey)

**No single open-source tool takes bitmap images and produces a finished font.** Existing approaches chain tools:

- **Pipeline A (FontForge):** Scan → crop glyphs → import as backgrounds → Autotrace → manual cleanup → Generate
- **Pipeline B (mftrace):** METAFONT → high-res bitmap → Potrace → t1asm → Type 1 font
- **Pipeline C (SVG chain):** Bitmap → Potrace → SVG → svgicons2svgfont → svg2ttf → TTF
- **Pipeline D (Google Fonts):** Glyphs/UFO source → fontmake → ufo2ft → fonttools → OTF/TTF → fontbakery

### 1.9 Commercial References

- **Calligraphr:** Web service, handwriting → font. Template-based, 300-600 DPI scan recommended.
- **Fontself:** Adobe Illustrator/Photoshop extension, drag-and-drop font creation. $39-59.

---

## 2. Font Format Specification

### 2.1 TrueType vs CFF (OpenType)

| Aspect | TrueType (glyf) | CFF (CFF table) |
|--------|-----------------|------------------|
| Curve type | Quadratic Bezier (3 points/curve) | Cubic Bezier (4 points/curve) |
| Programmatic generation | **Simpler** — arrays of points + flags | Complex — stack-based charstring bytecode |
| Binary encoding | Straightforward coordinates + flags | Compact but intricate (INDEX structures, DICT encoding) |
| Hinting | Very complex (Turing-complete VM) | Simpler (stem declarations) |
| Traced bitmap fit | Needs more points (quadratic approximation) | Fewer segments for same quality |
| File size | Larger for complex outlines | 20-50% smaller (subroutinization) |

**Recommendation: TrueType (glyf table) for v1.** Significantly simpler to generate. Universal support. Cubic-to-quadratic conversion (via `cu2qu` algorithm) is well-understood and introduces minimal quality loss.

### 2.2 Required Tables (Minimum Viable Font)

10 tables for a loadable TrueType font:

| Table | Purpose | Size/Complexity |
|-------|---------|-----------------|
| `head` | Font header (global metadata) | Fixed 54 bytes. Trivial. |
| `maxp` | Maximum profile (memory requirements) | Fixed 32 bytes. Trivial. |
| `hhea` | Horizontal header (line metrics) | Fixed 36 bytes. Trivial. |
| `hmtx` | Horizontal metrics (advance widths, LSBs) | 4 bytes/glyph. Easy. |
| `OS/2` | OS/2 and Windows metrics | Fixed 96 bytes (v4). Easy. |
| `name` | Naming table (font name, copyright, etc.) | Variable. UTF-16BE encoding. Easy. |
| `post` | PostScript name mapping | Fixed 32 bytes (v3.0 = no names). Trivial. |
| `cmap` | Character-to-glyph mapping | Format 4 (BMP) is moderate. |
| `glyf` | Glyph outlines | **The hard part.** Delta encoding, flag compression, quadratic splines. |
| `loca` | Index to glyph locations | Array of offsets. Easy. |

Optional but recommended:
- `gasp`: Grid-fitting hints for unhinted fonts (12 bytes, dramatically improves Windows rendering)
- `kern`: Kerning pairs (format 0 is simple sorted array)

### 2.3 Implementation Difficulty Ranking

**Tier 1 — Trivial (< 1 day each):** head, maxp, hhea, post, gasp, OS/2
**Tier 2 — Easy (1-2 days each):** hmtx, loca, name, kern
**Tier 3 — Moderate (2-5 days each):** cmap (format 4), glyf (with bitmap→quadratic conversion), GDEF
**Tier 4 — Hard (1-2 weeks each):** CFF, GPOS, GSUB
**Tier 5 — Expert (weeks+):** TrueType hinting (full VM), CFF2/variable fonts

### 2.4 Key Technical Details

#### Vertical Metrics (Notorious Cross-Platform Gotcha)

Three conflicting metric systems must agree:

```
hhea table:    ascender / descender / lineGap     → macOS primary
OS/2 typo:     sTypoAscender / sTypoDescender / sTypoLineGap  → Windows (with USE_TYPO_METRICS)
OS/2 win:      usWinAscent / usWinDescent          → Windows clipping boundary
```

**Best practice:** Set all three to consistent values. Set `fsSelection` bit 7 (USE_TYPO_METRICS). Set lineGap = 0.

#### Advance Width & Sidebearings

```
|←LSB→|←─glyph─→|←RSB→|
|      ┌──┐      |     |
|      │  │      |     |
|      └──┘      |     |
origin                  next origin
|←── advanceWidth ────→|
```

Stored in `hmtx` table as (uint16 advanceWidth, int16 leftSideBearing) pairs.

#### Units Per Em (UPEm)

All coordinates expressed relative to this value:
- **1000:** PostScript/CFF convention
- **2048:** TrueType convention (power of 2, faster rasterization)
- `pixels = font_units * point_size * dpi / (72 * unitsPerEm)`

#### Hinting

**Skip for v1.** Impact by platform:
- macOS: Excellent without hints (CoreText ignores TT hints)
- Linux/FreeType: Good (auto-hinter handles it)
- Windows/DirectWrite: Acceptable at 12pt+
- Add `gasp` table with smoothing enabled at all sizes (12 bytes, major quality improvement)
- Post-process with `ttfautohint` later if needed

#### Kerning (Simplest Approach)

`kern` table format 0: sorted array of (left_glyph, right_glyph, adjustment) triples. Under 50 lines of code. Sufficient for <10,000 pairs.

### 2.5 Recommended Implementation Order

```
Phase 1 — Minimum Viable Font (~7-10 days):
  1. head, maxp, hhea, post, gasp     (trivial, ~1 day total)
  2. hmtx, loca, OS/2                 (easy, ~1 day total)
  3. name                             (easy, ~1 day)
  4. cmap (format 4)                  (moderate, ~1-2 days)
  5. glyf (with bitmap→quadratic)     (moderate, ~2-3 days)
  6. File assembly + checksums        (~1 day)

Phase 2 — Quality:
  7. kern table (format 0)            (~1 day)
  8. Validation pass (OTS/fontspector) (~1-2 days of fixes)

Phase 3 — Advanced (if needed):
  9. GPOS (class-based kerning)       (~1 week)
  10. GSUB (ligatures)                (~1 week)
  11. Auto-hinting via ttfautohint    (integration, ~1 day)
```

---

## 3. Quality Metrics & Objective Function

### 3.1 The Core Insight

The factory needs a **computable, single-number objective function** it can maximize autonomously. This decomposes into six layers, all fully computable with zero human judgment:

### 3.2 Layer 1: Font Validity (Binary Pass/Fail)

| Check | Tool | Type |
|-------|------|------|
| OpenType spec compliance | fontspector | 200+ checks, pass/fail |
| Browser compatibility | OTS (ot-sanitise) | Binary gate |
| Contours closed | fontTools validation | Binary |
| Correct winding direction | AreaPen (negative=CW, positive=CCW) | Binary per contour |
| No self-intersections | `bezierTools.segmentSegmentIntersections()` | Binary |
| Points at extrema | fontbakery/fontspector check | Count violations |

### 3.3 Layer 2: Pixel Fidelity at Input Resolution (SSIM, 0.000–1.000)

Render source font → bitmap → trace → assemble → render output font → compare.

**At the input resolution (200-400px/em), the tracer's job is to reproduce those exact pixels.** SSIM here should approach 0.99+.

**Recommended metric:** SSIM (Structural Similarity Index)
- Structure-aware: 1px shift doesn't catastrophically degrade score
- Well-defined 0–1 range
- Rust implementation: `dssim` crate (multi-scale SSIM)
- Python implementation: `skimage.metrics.structural_similarity`

**Thresholds:**
- SSIM > 0.99: Excellent (sub-pixel differences only)
- SSIM > 0.95: Good (minor outline deviations)
- SSIM > 0.90: Acceptable
- SSIM < 0.85: Poor

**Alternative/complementary metric:** IoU (Intersection over Union)
- `IoU = |A AND B| / |A OR B|`
- Natural for binary glyph bitmaps. 1.0 = perfect match.
- More intuitive than SSIM for binary images.

### 3.4 Layer 3: Multi-Scale Fidelity (SSIM at Unseen Sizes)

Render output font at sizes the tracer **never saw** (12, 16, 24, 48, 96px).

Good Bezier curves generalize; bad ones only look right at the training size. This separates "good tracing" from "pixel overfitting."

**Expected quality benchmarks** (at 400px/em input, Potrace-class tracing):

| Output Size | Expected IoU | Expected SSIM |
|-------------|-------------|---------------|
| 200px/em | 0.95–0.99 | 0.97–0.99+ |
| 48px/em | 0.93–0.97 | 0.95–0.98 |
| 12px/em | 0.85–0.93 | 0.88–0.95 |

### 3.5 Layer 4: Curve Quality (Integers/Booleans)

| Metric | How to Measure | Target |
|--------|---------------|--------|
| Control points per glyph | Count via RecordingPen | ≤ 1.5x source font |
| Points at extrema | Check derivatives for roots in [0,1] | 0 violations |
| Winding direction | AreaPen signed area | All correct |
| Self-intersections | segmentSegmentIntersections() | 0 |
| Short segments (< 2 units) | Distance between consecutive on-curve points | 0 |
| G1 continuity | Tangent angle difference at junctions | < 1 degree |

### 3.6 Layer 5: Metric Accuracy (Numeric Comparison)

| Metric | Comparison | Tolerance |
|--------|-----------|-----------|
| Advance widths | Per-glyph absolute diff / UPM | < 2% |
| Left sidebearings | Per-glyph absolute diff / UPM | < 2% |
| Right sidebearings | Computed, per-glyph diff / UPM | < 2% |
| Ascender/Descender | OS/2 + hhea values | Exact match |
| UPM | head.unitsPerEm | Exact match |
| Bounding boxes | Per-glyph xMin/yMin/xMax/yMax | < 2% |
| Glyph area | StatisticsPen.area | < 3% |
| Centroid | StatisticsPen.meanX/meanY | < 2% |

### 3.7 Layer 6: Text-Level Coherence

Render a paragraph with both fonts, compare:
- Full-page SSIM
- Total string width (sum of advances) — should differ < 1%
- Line break positions at fixed width — should be identical
- HarfBuzz shaping output (`hb-shape`) — glyph IDs + positions should match

### 3.8 Composite Score

```
score = w1 * validity_pass_rate        (binary checks, 0 or 1)
      + w2 * ssim_input_resolution     (0.0–1.0, primary)
      + w3 * ssim_multi_scale_avg      (0.0–1.0, generalization)
      + w4 * curve_quality_score       (0.0–1.0, normalized violations)
      + w5 * metric_accuracy_score     (0.0–1.0, normalized error)
      + w6 * text_coherence_score      (0.0–1.0)
```

The factory maximizes this single number. Every component is deterministic and computable.

---

## 4. Font Rendering & Test Pipeline

### 4.1 Rendering Fonts to Bitmaps

#### FreeType (C, industry standard)
- `FT_RENDER_MODE_MONO`: 1-bit monochrome, no anti-aliasing
- `FT_RENDER_MODE_NORMAL`: 8-bit grayscale anti-aliased
- Rust bindings: `freetype-rs` (PistonDevelopers), `freetype-sys`
- Go: `github.com/golang/freetype` (pure Go port)

#### Rust-Native Rasterizers
| Crate | Notes |
|-------|-------|
| `fontdue` | Fastest pure Rust rasterizer. Simplest API: glyph ID in, bitmap out. `no_std`. |
| `swash` | Introspection + rasterization. 10-20% faster than FreeType. Subpixel rendering. |
| `skrifa` | Google's FreeType replacement. Production quality (Chrome). Glyph outlines + scaling. |
| `pixglyph` | Simple, no unsafe. Used by Typst. |

#### Recommendation
**skrifa** for reading/scaling reference fonts (production quality, Google-backed). **fontdue** for rasterizing to bitmaps (simplest API, pure Rust).

### 4.2 Bitmap Format Decisions

| Decision | Recommendation | Rationale |
|----------|---------------|-----------|
| In-memory format | Raw pixel arrays | Zero overhead, direct comparison |
| Persisted format | PNG | Lossless, compact, human-inspectable |
| Tracer input format | PBM/PGM (Potrace) or PNG (VTracer) | VTracer handles PNG directly |
| Bit depth for rendering | 8-bit grayscale | Smoother edges for better traces |
| Bit depth for tracing | Threshold grayscale → 1-bit | Gives control over threshold value |

**Key insight:** Render in grayscale (8-bit), then threshold to 1-bit before tracing. This produces cleaner results than rendering directly in monochrome, especially at lower resolutions. FontLab confirms: grayscale source produces smoother edges that lead to better outlines.

### 4.3 Input Resolution

- **200-400px per em:** Good quality traces, reasonable performance
- **400-600px per em:** Excellent quality, diminishing returns above 600px
- **< 100px per em:** FontForge warns of "fairly bad results"
- **mftrace default:** ~1000px equivalent magnification

**Recommendation:** 400px/em as the default. Configurable for quality/speed tradeoff.

### 4.4 Disabling Anti-Aliasing (for clean test bitmaps)

- FreeType: `FT_RENDER_MODE_MONO` + `FT_LOAD_MONOCHROME` + `FT_LOAD_TARGET_MONO`
- fontdue: `rasterize_config` method, binary output mode
- Skia: `SkFont::setEdging(SkFont::Edging::kAlias)`

### 4.5 Test Corpus

#### Google Fonts
- 1,907 font families, SIL Open Font License
- Programmatic access: [Developer API](https://developers.google.com/fonts/docs/developer_api), [GitHub repo](https://github.com/google/fonts)
- `gftools` Python package for batch operations

#### Recommended Starter Set (12 fonts)

| Category | Font | Why |
|----------|------|-----|
| Serif | Noto Serif | Comprehensive Unicode, clean |
| Serif | Merriweather | Large x-height, popular web |
| Sans-serif | Roboto | Most-downloaded Google Font |
| Sans-serif | Open Sans | 30M+ websites, humanist |
| Monospace | Source Code Pro | Adobe, well-hinted |
| Monospace | JetBrains Mono | Modern, ligatures |
| Display | Playfair Display | High contrast, stress-tests thin strokes |
| Handwriting | Caveat | Contextual alternates |
| Slab | Roboto Slab | Tests slab serifs |
| Variable | Recursive | Multi-axis (weight, slant, CASL, MONO) |
| CJK | Noto Sans CJK | Complex CJK at scale |
| Arabic | Noto Sans Arabic | RTL + complex shaping |

### 4.6 Complete Rust Pipeline Architecture

```
Source Font (.ttf/.otf)
    │
    ├─ [skrifa / read-fonts]         Parse font, extract glyph outlines
    │
    ├─ [fontdue or skrifa scaler]    Rasterize glyphs to 8-bit grayscale
    │
    ├─ [image crate]                 Threshold to 1-bit, save as PNG
    │
    ├─ [vtracer]                     Trace bitmaps → SVG vector outlines
    │
    ├─ [kurbo]                       Convert SVG paths to Bezier curves,
    │                                optimize, simplify
    │
    ├─ [write-fonts]                 Assemble outlines + metrics into OTF/TTF
    │
    ├─ [fontspector]                 Validate output font (200+ checks)
    │
    └─ [skrifa + image + dssim]      Re-render output font → bitmap,
                                     compute SSIM vs source renders
```

Every crate: MIT/Apache-2.0, actively maintained, several used in Chrome production.

---

## 5. Bezier Curve Fitting Theory

### 5.1 Raph Levien's Key Results

Raph Levien (Google, linebender project, kurbo crate author) has published extensive research on curve fitting:

- **O(n^6) error scaling:** Subdividing a cubic Bezier in half reduces fitting error by a factor of 64. This means a small number of segments can approximate curves very precisely.
- **Practical convergence:** Changing error tolerance from 0.15 to 0.05 increases segment count from 44 to 60 (only 36% more for 3x tighter tolerance).
- **Area-preserving fits:** "When simplifying the outline of a glyph in a font, an area-preserving curve fit means that the amount of ink in a stroke is exactly preserved." Recommended for font work.
- **Local minima warning:** C-shaped curves have three parameter sets producing nearly identical shapes. The optimizer can get stuck.
- **Algorithm implemented in FontForge** as the "merge/simplify" operation.

References:
- [Fitting cubic Bezier curves (2021)](https://raphlinus.github.io/curves/2021/03/11/bezier-fitting.html)
- [Simplifying Bezier paths (2023)](https://raphlinus.github.io/curves/2023/04/18/bezpath-simplify.html)
- [Parallel curves (2022)](https://raphlinus.github.io/curves/2022/09/09/parallel-beziers.html)

### 5.2 Potrace's Accuracy

- Default opttolerance of 0.2 allows moderate curve joining
- Error rates reported: 0.13%, < 0.06% (relates to Bezier approximation accuracy)
- For font work: use opttolerance=0.1 or lower for more accuracy at cost of more points

### 5.3 Comparison of Tracing Approaches

| Method | Type | Complexity | Input | Best For |
|--------|------|-----------|-------|----------|
| Potrace | Classical polygon-based | O(n^2) | 1-bit only | Sharp monochrome, proven for fonts |
| VTracer | Classical clustering | O(n) | Color/grayscale | Speed, Rust-native, flexibility |
| AutoTrace | Classical | — | Various | Centerline tracing |
| StarVector | AI (8B LLM, CVPR 2025) | GPU-heavy | Any | Research comparison (non-deterministic) |
| Raph Levien's fitting | Math optimization | O(n^6 convergence) | Existing paths | Post-processing/simplification |

---

## 6. Academic References

### Classical
1. **Selinger, P. (2003). "Potrace: a polygon-based tracing algorithm."** [PDF](https://potrace.sourceforge.net/potrace.pdf)
2. **Levien, R. "From Spiral to Spline: Optimal Techniques in Interactive Curve Design."** PhD thesis. [PDF](https://levien.com/phd/thesis.pdf)

### Deep Learning (for context, not for use — user requirement: deterministic, no AI)
3. **Reddy et al. (CVPR 2021). "Im2Vec: Synthesizing Vector Graphics without Vector Supervision."** [arXiv](https://arxiv.org/abs/2102.02798)
4. **Wang & Lian (SIGGRAPH Asia 2021). "DeepVecFont."** [GitHub](https://github.com/yizhiwang96/deepvecfont)
5. **Wang & Lian (2023). "DeepVecFont-v2."** [arXiv](https://arxiv.org/abs/2303.14585)

---

## 7. Key Risks & Mitigations

### Risk 1: glyf table generation is the hardest part
Coordinate delta encoding, flag compression, quadratic spline conversion, correct winding order. This is where the factory proves (or fails to prove) it can handle real systems programming.

**Mitigation:** Use `write-fonts` crate from Google fontations — it handles much of the binary encoding. The factory's job becomes: produce correct glyph data structures, let write-fonts serialize them.

### Risk 2: Vertical metrics cross-platform inconsistency
Three conflicting metric systems. Wrong values → glyphs clipped on Windows or wrong line spacing on macOS.

**Mitigation:** Copy metrics directly from reference fonts for round-trip tests. For novel fonts, use the well-documented "set all three to the same values + USE_TYPO_METRICS" pattern.

### Risk 3: Curve fitting local minima
C-shaped curves have three nearly-identical parameter sets. Optimizer can get stuck.

**Mitigation:** Use VTracer/Potrace for initial tracing (they handle this), then optimize with kurbo's path simplification. Multiple restart strategies if needed — this is where Rust performance enables brute-force approaches.

### Risk 4: No existing turnkey tool to port
This is a build-from-scratch project, not a port. Harder factory demo.

**Mitigation:** The quality function IS the spec. The factory doesn't need an existing codebase to reference — it has a computable objective to maximize. And every component library exists; it's the integration that's novel.

### Risk 5: VTracer less proven for font work than Potrace
Potrace has FontForge integration and decades of font use. VTracer is general-purpose.

**Mitigation:** Benchmark both early. VTracer's O(n) and Rust-native advantages may outweigh Potrace's font-specific tuning. Can always fall back to Potrace via FFI.

---

## 8. Performance / "Glitter" Features

Performance gains from Rust enable features that would be impractical in Python:

- **Real-time tracing:** Trace letter as user draws on canvas, <100ms feedback loop
- **Batch font generation:** Process 26+ glyphs in parallel, complete font in <1 second
- **Brute-force optimization:** Try thousands of tracing parameter combinations, pick best quality score
- **Style exploration:** Generate font variants with different corner thresholds, smoothing levels
- **Massive test suite:** Run round-trip tests against all 1,907 Google Fonts in minutes, not hours
- **Interactive parameter tuning:** Real-time preview as user adjusts tracing parameters

---

## 9. Open Questions for Spec Phase

1. **Scope of v1 character set:** Full ASCII (95 glyphs)? Basic Latin (128)? Extended Latin? Start minimal, expand.
2. **Kerning strategy:** Auto-derive from bitmap spacing analysis, or require explicit kerning input?
3. **Input format:** Individual PNG per glyph? Template grid image (Calligraphr-style)? Both?
4. **API surface:** CLI tool? HTTP API? Library crate? All three?
5. **Test infrastructure ownership:** Does the factory build the test harness from the spec, or is it provided?
6. **Output format:** TTF only? OTF option? WOFF/WOFF2 for web?
