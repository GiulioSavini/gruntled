//go:build race

package tfsurface

// raceSlowdown scales wall-clock bounds in tests: the race detector makes
// lexing a hostile ~4 MB file roughly ten times slower.
const raceSlowdown = 10
