//go:build !faults

package malbolge

// faultMode is always empty in normal builds, so every fault check below is
// removed by the compiler. See faults_on.go.
const faultMode = ""
