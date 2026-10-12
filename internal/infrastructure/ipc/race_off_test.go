//go:build !race

package ipc

// raceAllocSlack is what the race detector adds to per-request
// allocations; 0 without -race.
const raceAllocSlack = 0
