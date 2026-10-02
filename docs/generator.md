# The Malbolge generator

`internal/gen` (and `melc gen` / `melc build`) turns any bytes into Malbolge
programs that print exactly those bytes. Every program is verified by running
it before it is written.

## The problem

A Malbolge program cannot just "print a character". Every instruction is
self-encrypting, the only arithmetic is ternary rotation and the "crazy"
operation, and both read their operand from the cell the data pointer `d`
points to. So the generator has to steer the accumulator `a` to a value with
`a % 256 == byte` using whatever values `[d]` happens to hold, and then emit
the output instruction `<`.

## Lag-1 chunks (most text)

Every program starts with a fixed 42-cell prefix: `oji` + 37×`o` + `vo`
(normalized ops). Its `j` sets `d = 39` and its `i` jumps to cell 41, after
which `d == c - 1` forever. From then on, when the cell at position *l*
executes, `[d]` is cell *l−1* after its post-execution encryption, a value
determined only by which op we wrote there and *l mod 94*. No cell is ever
overwritten before it runs.

So the generator state is just `(a, previous op, position mod 94)`, and the
moves are `o` (nop), `*` (`a = rotr([d])`), `p` (`a = crz(a, [d])`) and `<`.
A `*` makes `a` independent of its old value, so after any `*` the state is one
of 4 × 94 = 376 **reset states**. Tables built at startup hold a shortest move
sequence from every reset state to every byte. Per byte, the encoder takes
the cheaper of:

- a depth-limited search from the current state, or
- *k* nops, then `*`, then the table path.

Cost: about **7.1–7.6 cells per byte** for ASCII text.

**The limit.** In lag-1 mode `[d]` is always a re-encrypted printable
character (33–126). Such small operands confine `a` to just 355 values, which
cover only **201 of the 256 byte values**. Bytes 154–208 are unreachable,
including UTF-8 continuation bytes such as the second byte of `é` (`C3 A9`).
`TestLag1CannotPrintAllBytes` documents this.

## Tape chunks (everything else)

The fix is to read large values. In lag-1 mode every `*` or `p` writes the new
`a` into the previous cell, so executed code leaves a trail of large values.
A tape chunk is laid out as:

```
prefix | tape: T fixed lag-1 moves | k nops | prev | j | content | v
```

1. The **tape** is a fixed, content-independent move sequence (seeded PRNG,
   mix of `*`, `p`, `o`). It writes a trail of known large values.
2. The `j` executes `d = [d]`. The nops and the `prev` op are chosen so that
   `[d]` is exactly `sStar − 1`, which moves `d` back to cell `sStar` (34).
3. The **content** then executes while `d` walks forward through the trail,
   one cell per instruction. `[d]` takes rich 10-trit values, and all 256 byte
   values become reachable.

Because the tape is the same in every chunk, the trail (the **stream**) is the
same too. A reach table then records, for every stream index, which bytes are
printable within 5 moves after a `*` there.

Cost: about **31–38 cells per byte** (content plus the tape it consumes).
Tape chunks are now the proven **fallback**; sweep chunks below do the same job
about four times more compactly.

## Sweep chunks (everything else, by default)

A tape chunk throws its tape away after one pass, and pays a tape cell for
every content cell. Sweep chunks reuse the region instead:

```
prefix | head: lag-1 moves | k nops | prev | j | sweep moves (+ forced j) | v
```

1. The **head** prints the leading lag-1-printable bytes in cheap lag-1 mode,
   padded with filler moves to at least 320 cells. Its trail is the
   **region**: the cells from `sStar` (34) up to the cell before the `j`.
2. The `j` moves `d` to `sStar`, as in a tape chunk. The cell before that `j`,
   the **anchor**, still holds `sStar − 1`.
3. The content runs in **sweep mode**. `d` walks through the region; whenever
   it reaches the anchor, a forced `j` (one extra cell) rewinds it to `sStar`.
   The region is therefore read again and again.
4. Every `*` or `p` overwrites the region cell it just read with its result. So
   each sweep leaves the region holding values derived from the last one, and
   the values get richer with every pass.
5. Per byte, a breadth-first search over `(a, d)` states (depth ≤ 8, each state
   visited once) finds the shortest move sequence. If the bytes ahead can't
   reach the target quickly, it skips forward with nops.

The region's starting contents are read from the real VM: the head is
executed with a `Machine`, so the model cannot drift from Malbolge. A sweep
chunk runs until the 59049-cell limit, printing any bytes. It stops early only
when a long ASCII run begins, which a lag-1 chunk prints more cheaply.

Cost: about **8–10 cells per byte** for binary data and UTF-8 text, close to
ASCII prices and about 4× smaller than tape chunks.

Sweep chunks have no static completeness proof, because their region depends
on the content. If one can't print a byte, the encoder falls back to a tape
chunk, which has a proof. **Every input is still compilable.** The fallback is
kept tested with `Options.noSweep`.

**Chunk selection.** A run of at least 256 lag-1-printable bytes gets a lag-1
chunk (~7.3 cells/byte). Everything else gets a sweep chunk, and a tape chunk
only as the fallback.

### Why not the fill region?

Memory after the program is filled by the loader with
`mem[i] = crz(mem[i-1], mem[i-2])`. That fill looks like a free source of
large values, but it falls into a cycle of period 6 with only **4 distinct
values**. Reading it reaches just 64 byte values.

## Proof and verification

| Check | Test |
|---|---|
| Lag-1 tables reach all 201 lag-1-printable bytes from every reset state; worst case 67 cells | `TestLag1Completeness` |
| From every stream index, every one of the 256 bytes is reachable; worst case 362 cells, average fallback 19 | `TestTapeCompleteness` |
| The tape layout (d, a, every stream value) matches the real VM for many tape lengths | `TestTapeOnRealVM` |
| Sweep chunks rewind correctly on the real VM (e.g. 3000 binary bytes over 74 sweeps) | `TestSweepRegionMatchesVM` |
| The proven tape fallback still round-trips | `TestTapeFallbackStillWorks` |
| Generated programs print exactly their input (VM) | `TestRoundTrip`, `TestCorpus`, `FuzzRoundTrip` |
| Generated programs print exactly their input on the original 1998 C interpreter | `TestReferenceRunsGeneratedPrograms` (`-tags reference`) |
| Every `melc` output is run and compared before it is written | `gen.Verify` in `melc gen` / `melc build` |

The completeness tests show that lag-1 and tape chunks can print every byte from every
state, within a bounded number of cells. Sweep chunks fall back to tape chunks, so
**every input is compilable**.

## Chunks

A program holds at most 59049 cells, so larger files become several programs,
called chunks. Each chunk is independent: it starts in a fresh state and needs
no shared memory. `page.html.mb/000.mb`, `001.mb`, … run in name order, and
their outputs are concatenated. Segments of 16 KB compile in parallel, and the
output does not depend on the number of workers.

## Numbers (Intel Core Ultra 9 285K, 24 cores)

| Input | Cells per byte | Compile speed |
|---|---|---|
| ASCII HTML/JS/CSS | ~7.1–7.6 | ~12 MB/s |
| Random binary (fonts, images) | ~8.5 (sweep chunks; tape chunks were ~37) | ~0.9 MB/s |
| UTF-8 text with accents, UTF-16, BOM files | ~8–11 | — |
| Real corpus (PNG, JPEG, GIF, wasm, JSON, SVG, …) | 7.3–10.9 | — |

Loading a program (including the memory fill) takes about 22 µs.

## Options

`gen.Options{Seed: n}` (and `melc gen -seed n`) inserts random nops and
shuffles the search order. The same input then yields different programs
with identical output, which is groundwork for traffic obfuscation (see
[roadmap.md](roadmap.md)).
