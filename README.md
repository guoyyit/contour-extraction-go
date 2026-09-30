# contour-extraction-go

A fully local contour (isoline) extraction service over regular scalar grids,
implemented in pure Go with no external services. Contours are extracted with
marching squares, processed in parallel over independent blocks, stitched
seamlessly at block boundaries, and optionally simplified with topology
preservation.

## Quick start

```bash
# build the CLI
go build ./...

# generate a 200x160 synthetic grid and extract three levels
go run ./cmd/contour \
  -gen hills -nx 200 -ny 160 \
  -levels -0.5,0,0.75 \
  -tile-nx 64 -tile-ny 64 -concurrency 8 \
  -simplify 0.25 \
  -nan-policy mask \
  -out results.json

# read a local grid file instead
go run ./cmd/contour -grid data.txt -levels 0,10,20 -out results.json
```

The grid text format is:

```
NX NY
v00 v10 ... vNX-1,0
...                    # NY rows total
```

`nan` / `NaN` tokens denote undefined samples. Generated grids can also be
written with `-save-grid` for inspection or later reuse.

## Library API

```go
svc, _ := contour.NewService(grid, contour.DefaultOptions())
lines, _ := svc.Extract([]float64{0, 10, 20}) // only new levels are computed
lines, _ = svc.Extract([]float64{30})         // level 30 only
_ = svc.HasLevel(10)
_ = svc.Results(10)
_ = svc.SaveResults("results.json")           // deterministic JSON
rec, _ := contour.LoadResults("results.json") // rec.Algorithm versions the semantics
```

Each `Contour` is either:

- `Closed == true`, `Point == false`: a closed ring (no duplicated closing
  vertex);
- `Closed == false`: a path whose two ends lie on the outer data boundary or on
  an undefined-value mask boundary;
- `Point == true`: a degenerate single-point contour (only emitted when
  enabled; see below).

## Connection rules (why outputs are unique and non-crossing)

1. Cells are processed with marching squares. A corner is "inside" when
   `value >= level`, so a vertex whose value equals the level is a contour
   vertex and the contour passes through it.
2. Edge crossing coordinates use linear interpolation; an endpoint that equals
   the level produces a crossing at the endpoint itself (`t = 0` or `t = 1`).
3. Every interior grid edge is shared by exactly two cells, so its crossing
   node always has degree two. Nodes on the outer data boundary (or on a mask
   boundary, see below) have degree one. The segment graph therefore contains
   only cycles and boundary-to-boundary paths. Chaining is unique: there are no
   dangling interior segments and no spurious closures.
4. Ambiguous (saddle) cells are resolved deterministically from the bilinear
   value at the cell center: `center >= level` joins the two inside corners
   through the cell interior; `center < level` joins the two outside corners.
   The center-equals-level tie is "center inside".
5. Output order is also canonical: open paths are oriented from the
   lexicographically smaller end, closed rings are rotated to their smallest
   vertex and oriented in the smaller of the two traversal directions, and
   contours are sorted lexicographically (bitwise float ordering). Consecutive
   duplicate vertices are collapsed. Hence results are byte-identical for any
   tile size, worker count, scheduling order, or extraction request order.

## Undefined values (NaN)

Undefined samples are represented by `math.NaN()` (the `nan` token in files).
Two configurable policies are provided via `Options.NaNPolicy`:

- `mask` (default): every cell touching a NaN vertex emits no segments.
  Contours coming from adjacent cells end on the edges surrounding the masked
  region; these are legitimate boundary-to-boundary paths (they are not closed
  around the hole and never pass through it). There are no dangling segments
  inside the grid.
- `fill`: all NaN samples are replaced by `Options.FillValue` on an internal
  copy of the grid before extraction; the original grid is never modified.

## Degenerate inputs

| Case | Semantics |
| --- | --- |
| Fully flat region (`v == level` everywhere) | Every corner is inside; marching squares emits nothing, at any level. |
| Flat plateau at/above the level | One closed contour around the plateau; no interior artifacts. |
| Vertex value exactly equal to the level | Contour passes through that grid vertex (`>=` classification); adjacent segments meet uniquely at the shared node. |
| Single-point extremum sampled at the level | Collapses to a degenerate point contour. Emitted only when `EmitPointContours` is true; otherwise suppressed. |
| NaN region | `mask` or `fill` semantics as described above. |
| Level outside the value range | Empty result, deterministically. |

## Block parallelism and stitching

The grid of cells is partitioned into row-major blocks (`TileNX` x `TileNY`
cells). Each block is extracted and chained independently in a worker pool.
Block traces carry the global grid-edge keys of their end nodes; the serial
stitch phase joins traces on shared boundaries through those keys. Because a
crossing node depends only on the edge's endpoint values, the two incident
blocks always produce the same key and coordinate, so block seams are exact:

- tiled output is identical (including point order) to single-block output;
- seams never duplicate or omit a segment;
- the result is independent of block ordering and worker count.

The serial phase touches only tile-boundary ports (work proportional to total
boundary length), so throughput grows with `Concurrency`.

## Topology-preserving simplification

`SimplifyPrecision > 0` enables Douglas–Peucker simplification with three
validity tests on every candidate chord:

1. all removed vertices are within the precision tolerance of the chord;
2. the chord does not properly cross any original segment of the level
   (shared-endpoint contact is allowed), preventing self-intersection and
   cross-contour intersections;
3. the "lens" between the removed sub-path and the chord contains grid vertices
   of only one strict sign relative to the level (and no NaN vertex), so a
   chord never crosses into the value region on the other side of the contour.

Failed candidates keep their vertices; closed rings never collapse below three
vertices, and open contours never move their endpoints. Simplified vertices are
always a subset of the original vertices.

## Determinism and reproducibility

- All ordering decisions are bitwise and deterministic; see connection rule 5.
- Levels are stored independently. Adding a level never recomputes previously
  extracted levels, and the union of incrementally extracted levels equals a
  one-shot extraction of the same levels.
- `SaveResults` writes pretty-printed JSON embedding `algorithmVersion` and the
  exact `Options`; re-saving the same state produces byte-identical files.

## Configuration reference (`contour.Options`)

| Field | Default | Meaning |
| --- | --- | --- |
| `TileNX`, `TileNY` | `64` | Block size in cells; `<= 0` means one block. |
| `Concurrency` | `runtime.NumCPU()` | Parallel block workers. |
| `SimplifyPrecision` | `0` (off) | Max DP tolerance; also enforces topology tests. |
| `NaNPolicy` | `"mask"` | `"mask"` or `"fill"`. |
| `FillValue` | `0` | Replacement used with `NaNPolicy == "fill"`. |
| `EmitPointContours` | `false` | Keep degenerate single-point contours. |

## Memory and performance

- Per-block memory is bounded by `TileNX*TileNY`; raw segments are retained
  block-by-block (and only when simplification is enabled). Final output size is
  bounded by the contour length of the grid.
- `go test -bench .` reports throughput. On the development machine a 1024x1024
  grid over four levels scales ~3x between 1 and 8 workers, with total
  allocation independent of worker count.
- `TestMemoryBoundedByTileSize` checks that allocations grow linearly with cell
  count rather than super-linearly.

## Tests

```bash
go test ./...          # all unit/integration tests
go test -bench .       # benchmarks (extraction + concurrency scaling)
```

Coverage includes:

- open/closed extraction, value consistency, no crossings;
- degenerate inputs: flat regions, exact-level vertices, single-point
  extrema, NaN holes with mask and fill policies;
- block stitching equivalence across five tiling schemes and several grids and
  levels, including order independence;
- simplification: no self/cross intersections, stays on the correct side of the
  level, endpoints preserved, no new vertices;
- determinism across worker counts/orders;
- incremental level extraction vs one-shot extraction;
- JSON persistence/reload and byte-stable serialization.
