//go:build faults

package malbolge

import "os"

// faultMode injects a deliberate bug, selected by MELHTTP_FAULT, so that the
// test suite can prove it would catch it ("tests that test the tests"):
//
//	crz-swap     crz uses its arguments in the wrong order
//	no-encrypt   executed cells are not encrypted
//	eof-zero     EOF reads as 0 instead of 59048
//	fill-swap    the loader's memory fill swaps its arguments
var faultMode = os.Getenv("MELHTTP_FAULT")
