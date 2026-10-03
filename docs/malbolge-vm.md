# The Malbolge VM

`internal/malbolge` executes Malbolge exactly like Ben Olmstead's 1998
reference interpreter (`testdata/reference/malbolge.c`, public domain). The
reference implementation, not the prose specification, is the target: every
real-world Malbolge program was written and tested against it.

## Machine

- 59049 (3¹⁰) memory cells, each a 10-trit word (0…59048), stored as `uint16`.
- Registers `a` (accumulator), `c` (code pointer), `d` (data pointer), all start at 0.

## Loading

1. Whitespace (`space \t \n \v \f \r`) is skipped.
2. Every other byte must be printable ASCII (33…126) and must decode, at its
   cell index `i`, to one of the eight instructions:
   `Xlat1[(byte - 33 + i) % 94]` ∈ `j i * p < / v o`.
3. The rest of memory is filled with `mem[i] = crz(mem[i-1], mem[i-2])`.

## Execution loop

```
for {
    if mem[c] ∉ 33..126  → stop (ErrInvalidInstruction)
    switch Xlat1[(mem[c] - 33 + c) % 94] {
    case 'j': d = mem[d]                      // mov d, [d]
    case 'i': c = mem[d]                      // jmp [d]
    case '*': a = mem[d] = rotr(mem[d])       // rotate right one trit
    case 'p': a = mem[d] = crz(a, mem[d])     // the "crazy" operation
    case '<': output byte(a % 256)
    case '/': a = next input byte, or 59048 at EOF
    case 'v': halt
    default:  nop                             // 'o'
    }
    mem[c] = Xlat2[mem[c] - 33]   // encrypt the executed cell (the *target* cell after a jump)
    c++, d++                      // wrapping at 59049
}
```

`crz(a, d)` works trit by trit with the table (rows `d`, columns `a`):

| d \ a | 0 | 1 | 2 |
|---|---|---|---|
| **0** | 1 | 0 | 0 |
| **1** | 1 | 0 | 2 |
| **2** | 2 | 2 | 1 |

Because it is tritwise, the VM computes it with one 243×243 lookup per
5-trit half-word.

## Deviations from the reference (only where it is undefined or hangs)

| Situation | Reference | MelHttp |
|---|---|---|
| executing a cell outside 33…126 | spins forever | stops with `ErrInvalidInstruction` |
| encrypting a cell outside 33…126 | out-of-bounds read (UB) | cell left unchanged |
| source bytes outside 33…126 | stored unchecked | rejected by the loader |
| programs shorter than 2 cells | reads `mem[-1]` (UB) | rejected by the loader |

Observable behavior of every valid program is identical: the reference
differential test runs the patched reference interpreter in Docker and compares
output, final status and exact step count over golden programs and 1000 random
programs.

## Limits and results

`Program.Run(ctx, in, out, Limits{MaxSteps, MaxOutput})` returns
`Result{Steps, Output, ReadInput, Halted}`. `ReadInput == false` means the
program never executed `/`, so its output depends only on the program —
MelHttp caches such responses.

Each run copies the immutable 118 KB memory image from a pool, so one loaded
`Program` can be executed concurrently. Throughput is roughly 480 million
instructions per second on a modern desktop CPU (`go test -bench Run ./internal/malbolge`).

## Tests

- `ternary_test.go` — crz against Wikipedia's asymmetric vector and a trit-by-trit definition, rotr, verbatim tables.
- `loader_test.go` — whitespace, invalid input, length bounds, fill rule, `FuzzLoad`.
- `vm_test.go` — golden programs (`testdata/programs`), EOF, limits, cancellation, concurrency.
- `reference_test.go` (`-tags reference`, needs Docker) — tables parsed from the C source; 1000+ program differential.
