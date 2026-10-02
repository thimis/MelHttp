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

Data is split per chunk: runs of lag-1-printable bytes of at least 256 bytes
get cheap lag-1 chunks, and everything else goes to tape chunks.

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
| Generated programs print exactly their input (VM) | `TestRoundTrip`, `TestCorpus`, `FuzzRoundTrip` |
| Generated programs print exactly their input on the original 1998 C interpreter | `TestReferenceRunsGeneratedPrograms` (`-tags reference`) |
| Every `melc` output is run and compared before it is written | `gen.Verify` in `melc gen` / `melc build` |

The completeness tests show that from every state, every byte can be printed
within a bounded number of cells. **Every input is therefore compilable.**

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
| Random binary (fonts, images) | ~37 | ~5.5 MB/s |
| Small UTF-8 text files | ~35 (tape chunks) | — |

Loading a program (including the memory fill) takes about 22 µs.

## Options

`gen.Options{Seed: n}` (and `melc gen -seed n`) inserts random nops and
shuffles the search order. The same input then yields different programs
with identical output, which is groundwork for traffic obfuscation (see
[roadmap.md](roadmap.md)).
