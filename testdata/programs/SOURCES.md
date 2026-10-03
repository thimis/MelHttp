# Golden Malbolge programs

Third-party programs used as test fixtures, with attribution. Each `.out` is
the expected output for the matching `.in` (or no input). All outputs are also
checked against the reference interpreter (`go test -tags reference`).

| File | Author / source | Notes |
|---|---|---|
| `hello.mb` | Wikipedia, [Malbolge](https://en.wikipedia.org/wiki/Malbolge) "Hello, world." example | prints `Hello, world.` |
| `cat.mb` | Matthias Lutter, <https://lutter.cc/malbolge/cat.html> | echoes input, never halts (prints 0xA8 after EOF) |
| `cat-terminating.mb` | Matthias Lutter, <https://lutter.cc/malbolge/cat.mal> | echoes input and halts at EOF; used for MelCGI echo |
| `quine.mb` | Matthias Lutter, <https://lutter.cc/malbolge/quine.html> | prints its own source |
| `adder.mb` | Matthias Lutter, <https://lutter.cc/malbolge/adder.html> | reads `A B`, prints `A+B` |
| `digital_root.mb` | Matthias Lutter, <https://lutter.cc/malbolge/digital_root.html> | prints the digital root of its input |
