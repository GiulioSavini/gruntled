//go:build race

package ipc

// raceAllocSlack is what the race detector adds to the bytes 20 requests
// allocate (about 75-95 KB on CI, with or without a big sealed line); one
// per-request encode of the 2 MiB view would still be over 50 MB.
const raceAllocSlack = 256 << 10
