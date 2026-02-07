# SAM2 (Segment Anything Model 2) -- Technical Research Summary

**Date:** 2026-02-06
**Sources:** Meta AI official docs, arXiv papers, community projects, benchmark evaluations

---

## 1. What SAM2 Does -- Core Capability and Differences from SAM1

SAM2 is Meta's second-generation foundation model for **promptable visual segmentation** in
both images and videos. Released July 2024 (with SAM 2.1 improvements in September 2024),
it is the successor to the original Segment Anything Model (SAM1, released April 2023).

### Core capability

Given an image or video and a spatial prompt (point click, bounding box, or mask), SAM2
produces pixel-accurate segmentation masks for the indicated object. In video mode, it
tracks and segments that object across all frames with temporal consistency.

### Key differences from SAM1

| Aspect | SAM1 | SAM2 |
|---|---|---|
| **Domain** | Images only | Images AND video (unified model) |
| **Image encoder** | ViT (Vision Transformer) | Hiera (hierarchical, also by Meta) -- multi-scale feature extraction |
| **Temporal modeling** | None | Streaming memory architecture: memory attention module, memory encoder, memory bank |
| **Parameters** | ~630M | ~224M (Large) -- roughly 1/3 the size |
| **Speed** | Baseline | ~6x faster than running SAM1 per-frame on video |
| **Accuracy** | Baseline | Surpasses SAM1 on the original 23-dataset zero-shot benchmark |
| **Training data** | SA-1B (11M images, 1B masks) | SA-V dataset (largest video segmentation dataset) + SA-1B |

The streaming architecture is a natural generalization: when applied to a single image, the
memory module is empty and SAM2 behaves like SAM. When applied to video, it accumulates
temporal key-value memories that maintain object identity across frames.

---

## 2. Input/Output Format

### Input

- **Image encoder input:** Tensor of shape `[1, 3, 1024, 1024]` (NCHW). All input images
  are resized/padded to 1024x1024 before encoding.
- **Prompts:** Encoded separately by `Sam2PromptEncoder` (see Section 3).
- **Video:** Processed frame-by-frame in streaming fashion. Prompts can be provided on any
  frame(s); the model propagates segmentation to all other frames.

### Output

SAM2 produces masks at multiple resolutions:

1. **Low-resolution mask logits:** Shape `[B, M, H*4, W*4]` -- 1/4 stride of backbone
   features. These can be fed back as mask prompts for iterative refinement.
2. **High-resolution masks:** Shape `[B, M, H*16, W*16]` -- full image resolution (stride 1
   pixel). Uses skip connections from Hiera stages 1 and 2 (stride 4 and stride 8 features)
   during upsampling.
3. **IoU predictions:** Per-mask confidence scores (intersection over union estimates).
4. **Object pointer tokens** and **object score logits** for internal tracking.

Where `M = 3` if `multimask_output=True` (returns 3 candidate masks ranked by IoU) or
`M = 1` if `multimask_output=False` (returns single best mask).

The final output masks are binary segmentation masks at the original image resolution. They
can be converted to bounding boxes, polygons, or RLE (run-length encoding) as needed
downstream.

---

## 3. Prompt Types

SAM2 accepts three native prompt types (no text/language prompts natively):

### 3a. Point prompts
- One or more (x, y) coordinate clicks on the image.
- Each point is labeled as **foreground (positive)** or **background (negative)**.
- Encoded via positional encoding of the coordinates plus a learned type embedding.
- API: `predictor.add_new_points_or_box(points=[[x,y]], labels=[1])`

### 3b. Bounding box prompts
- A rectangle specified by top-left and bottom-right corners: `[x1, y1, x2, y2]`.
- Encoded as two positional encodings (top-left + bottom-right).
- Generally produces better masks than single-point prompts for ambiguous objects.
- API: `predictor.add_new_points_or_box(box=[x1, y1, x2, y2])`

### 3c. Mask prompts
- A coarse mask (e.g., from a previous iteration or another model) provided as input.
- The low-resolution mask logits from a prior SAM2 pass can be fed back for refinement.
- Useful for iterative click-to-refine workflows.

### Text/language prompts -- NOT natively supported

SAM2 does **not** accept text prompts. To achieve text-prompted segmentation, the community
uses a two-stage pipeline:

1. **Grounding DINO** (or Florence-2, OWL-ViT) takes a text query and produces bounding
   boxes for matching objects.
2. **SAM2** takes those bounding boxes as prompts and produces segmentation masks.

The canonical implementation is **Grounded-SAM-2** (IDEA-Research/Grounded-SAM-2 on GitHub),
which chains Grounding DINO + SAM2 for text-prompted segmentation and tracking in video.

### Automatic mask generation (no prompt)

SAM2 also supports an "automatic" mode that generates masks for all objects in an image by
placing a dense grid of point prompts, similar to SAM1. However, SAM2 in auto mode
aggressively prunes low-confidence proposals, resulting in fewer mask candidates than SAM1,
especially for camouflaged or small structures.

---

## 4. Text/Glyph Segmentation -- OCR-Adjacent Usage

SAM2 was not designed for text segmentation, but there is significant community and research
activity adapting SAM-family models for OCR-adjacent tasks:

### 4a. OCR-SAM (yeungchenwa/OCR-SAM)
- Combines **MMOCR** (text detection + recognition) with SAM + Stable Diffusion.
- Pipeline: detect text regions -> recognize text -> segment text pixels with SAM.
- Downstream tasks include text removal and text inpainting.
- Built on SAM1; could be adapted to SAM2.

### 4b. Hi-SAM -- Hierarchical Text Segmentation (IEEE TPAMI, 2024)
- Extends SAM for **four hierarchies** of text segmentation: pixel-level text, word,
  text-line, and paragraph.
- Achieves state-of-the-art: **84.86% fgIoU on Total-Text**, **88.96% fgIoU on TextSeg**
  for pixel-level text segmentation.
- Also performs layout analysis.
- Demonstrates that SAM's architecture can be effectively adapted for text-specific tasks.

### 4c. Char-SAM -- Character-Level Text Segmentation (AAAI/arXiv, Dec 2024)
- Turns SAM into a **character-level** text segmentation annotator.
- Key insight: English characters follow fixed glyph structures regardless of font, size,
  or direction.
- **Character Bounding-box Refinement (CBR):** Generates character-level bounding boxes
  from word-level annotations (no per-character annotation needed).
- **Character Glyph Refinement (CGR):** Uses glyph templates from known text categories
  across various fonts to guide SAM into producing more accurate per-character masks.
- Addresses SAM's tendency to over-segment or under-segment individual characters.
- Character-level box prompts show "significant performance improvement" over word-level
  boxes.

### 4d. Font specimen / typography segmentation -- No direct SAM2 work found

No published work was found specifically using SAM2 for:
- Font specimen sheet segmentation
- Glyph extraction from type specimens
- Stroke-level decomposition of letterforms

Font stroke segmentation research (e.g., Adobe's "StrokeStyles") uses geometric/medial-axis
approaches rather than neural segmentation models. However, Char-SAM's glyph refinement
approach and Hi-SAM's hierarchical architecture suggest SAM-family models could be adapted
for font specimen work with appropriate fine-tuning.

### Assessment for font/glyph tasks

SAM2 could plausibly be used for font specimen segmentation via:
1. **Bounding box prompts** around individual glyphs (if glyph positions are known or
   detected via another method).
2. **Grounding DINO + SAM2** for text-prompted detection of specific characters.
3. **Fine-tuning** on font specimen data for domain-specific accuracy.

The main risks are: thin strokes may be poorly captured (see Section 7), and SAM2's
1024x1024 encoding resolution may lose detail on dense specimen sheets with many small glyphs.

---

## 5. Resolution and Accuracy Characteristics

### Internal processing resolution
- All images are resized to **1024x1024** for the image encoder.
- The Hiera encoder produces features at multiple scales: 1/4, 1/8, 1/16, 1/32 of input.
- The mask decoder uses stride-4 and stride-8 skip features for upsampling.

### Mask precision
- Final masks are output at full input resolution (after upsampling from the decoder).
- The mask head operates at 1/4 stride internally, then upsamples with high-res feature
  guidance.
- **Boundary precision is a known weakness.** The one-shot upsampled low-resolution mask
  head does not capture thin, intricate, or high-frequency structures accurately.
- Research (MGD-SAM2, arXiv 2503.23786) confirms that "mask results from SAM2's decoder are
  directly upsampled to yield high-resolution predictions, lacking the reconstruction of
  fine-grained details."

### Accuracy benchmarks
- SAM2 surpasses SAM1 on all 23 zero-shot segmentation benchmarks.
- On video (SA-V, DAVIS, etc.), SAM2 achieves state-of-the-art with real-time speed.
- Medium-sized, centrally located, high-solidity, smooth-boundary objects get the best
  scores.
- Small, irregular, thin, or camouflaged objects see degraded performance.

### Practical resolution implications
- For a 4000x3000 pixel image, the 1024x1024 encoding means ~4x downsampling, so features
  smaller than ~4 pixels in the encoded space (~16 pixels in the original) may be lost.
- For dense font specimens with hundreds of small glyphs, individual characters may be only
  a few dozen pixels each after resizing, pushing the limits of detail preservation.

---

## 6. Deployment

### Python API

Install via pip from the official GitHub repo:
```bash
git clone https://github.com/facebookresearch/sam2.git
cd sam2
pip install -e .
```

Or via PyPI: `pip install sam2`

**Image prediction:**
```python
from sam2.build_sam import build_sam2
from sam2.sam2_image_predictor import SAM2ImagePredictor

predictor = SAM2ImagePredictor(build_sam2("sam2.1_hiera_large.yaml", "checkpoints/sam2.1_hiera_large.pt"))
predictor.set_image(image)
masks, scores, logits = predictor.predict(point_coords=[[x, y]], point_labels=[1])
```

**Video prediction:**
```python
from sam2.build_sam import build_sam2_video_predictor
predictor = build_sam2_video_predictor("sam2.1_hiera_large.yaml", "checkpoints/sam2.1_hiera_large.pt")
state = predictor.init_state(video_path="path/to/frames/")
predictor.add_new_points_or_box(state, frame_idx=0, obj_id=1, points=[[x,y]], labels=[1])
for frame_idx, obj_ids, masks in predictor.propagate_in_video(state):
    # process masks per frame
```

**Ultralytics integration:**
```python
from ultralytics import SAM
model = SAM("sam2.1_l.pt")
results = model("image.jpg", bboxes=[100, 100, 200, 200])
```

### Model sizes (SAM 2.1 recommended)

| Variant | Parameters | Checkpoint Size | FPS (A100) |
|---|---|---|---|
| sam2.1_hiera_tiny | 38.9M | ~148 MB | 47.2 |
| sam2.1_hiera_small | 46M | ~176 MB | ~43 |
| sam2.1_hiera_base_plus | 80.8M | ~308 MB | ~34 |
| sam2.1_hiera_large | 224.4M | ~856 MB | ~24 |

(Original SAM 2 checkpoints are smaller: tiny=38.9MB, large=224.4MB, but SAM 2.1 is
recommended for all new work.)

### GPU requirements
- **Recommended:** NVIDIA A100 or H100 for production workloads.
- **Minimum viable:** 8GB VRAM GPU (e.g., RTX 3070 Ti) -- confirmed working for both
  inference and fine-tuning.
- **Optimization:** `torch.compile` support (set `vos_optimized=True` for video). AOTInductor
  compilation achieves up to **13x latency reduction** vs. eager PyTorch.
- **Requirements:** Python >= 3.10, PyTorch >= 2.5.1, torchvision >= 0.20.1.
- **CUDA required** for GPU inference. CPU inference is possible but very slow.

### Inference speed
- **Image:** Near-instantaneous per image on modern GPUs (the image encoder runs once;
  prompt/mask decoding is very fast).
- **Video:** ~44 FPS on A100 (sam2_hiera_tiny), real-time capable.
- **Amortized:** The image encoder runs once per image; multiple prompts can be evaluated
  against the same encoding very quickly.
- The encoder is the bottleneck; the prompt encoder + mask decoder are lightweight.

### Alternative deployments
- **ONNX / OpenVINO** export supported for edge deployment.
- **TensorRT** integration available for high-performance inference (TIER IV blog).
- **Qualcomm AI Hub** provides optimized SAM2 for mobile/edge.
- **Browser (WASM/WebGPU)** experiments exist but are not production-ready.
- **HuggingFace Transformers** integration: `from transformers import Sam2Model`.

---

## 7. Known Limitations for Fine-Grained Segmentation

### Thin structures and font strokes

This is the most critical limitation for typography/font work:

1. **Low-res mask head:** The mask decoder operates at 1/4 stride internally. Thin structures
   (like serif details, hairline strokes, ligature connections) that are only 1-2 pixels wide
   at the encoded resolution will be lost or blurred during upsampling.

2. **Boundary imprecision:** SAM2 "is not well suited for fine-grained segmentation of
   complex object structures" and "is not particularly sensitive to segmenting high-resolution
   fine details." The upsampling from low-res mask logits smooths away thin protrusions and
   concavities.

3. **Edge deformation sensitivity:** "Poor sensitivity to fine-grained dynamic features, such
   as edge deformation or texture evolution" -- relevant for closely-spaced glyphs where
   boundaries are near each other.

### Small objects

4. **Aggressive pruning in auto mode:** In automatic (unprompted) segmentation, SAM2 prunes
   low-confidence proposals more aggressively than SAM1, resulting in **lower objectness
   recall** -- small glyphs on a specimen sheet may not be detected at all.

5. **Size-dependent accuracy:** "Medium-sized, centrally located structures with high solidity
   and smooth boundaries achieved the highest performance metrics" -- small and irregular
   structures (like individual characters in a dense layout) perform worse.

6. **Resolution bottleneck:** The 1024x1024 encoding means that on a high-res scan of a font
   specimen, individual characters may be encoded at very low effective resolution.

### Mitigation strategies

- **Crop and segment:** Instead of feeding the full specimen, crop to individual glyphs or
  small groups and run SAM2 at full 1024x1024 resolution per crop. This is the most effective
  workaround.
- **Adapter modules:** SAM2-Adapter and MGD-SAM2 add refinement modules for boundary fidelity.
- **Char-SAM's CGR module:** Uses glyph template matching to refine character masks post-SAM,
  specifically addressing over/under-segmentation of characters.
- **Fine-tuning:** SAM2 can be fine-tuned on domain-specific data (confirmed working on 8GB
  GPU). Fine-tuning on font/glyph data would likely improve stroke boundary quality.
- **Multi-scale inference:** Process at multiple crop scales and merge results.

---

## Summary Assessment

**For general object segmentation:** SAM2 is the current state of the art. It is fast,
accurate, and handles both images and video with a unified architecture that is 3x smaller
than SAM1.

**For text/font/glyph segmentation specifically:** SAM2 out of the box will struggle with:
- Thin strokes and hairlines
- Small glyphs in dense layouts
- Fine serif details and stroke terminals
- Closely spaced characters where boundaries are ambiguous

The most promising path for font specimen work would be:
1. Use a detection model (Grounding DINO, or a custom detector) to locate individual glyphs.
2. Crop each glyph region to maximize effective resolution.
3. Run SAM2 with bounding box prompts on each crop.
4. Optionally apply Char-SAM-style glyph refinement or fine-tune SAM2 on font data.

---

## Key References

- Meta SAM2 official: https://ai.meta.com/sam2/
- GitHub repo: https://github.com/facebookresearch/sam2
- Paper: https://arxiv.org/abs/2408.00714
- Grounded-SAM-2: https://github.com/IDEA-Research/Grounded-SAM-2
- Hi-SAM: https://arxiv.org/abs/2401.17904
- Char-SAM: https://arxiv.org/abs/2412.19917
- OCR-SAM: https://github.com/yeungchenwa/OCR-SAM
- MGD-SAM2 (detail enhancement): https://arxiv.org/abs/2503.23786
- SAM2-Adapter: https://arxiv.org/abs/2408.04579
