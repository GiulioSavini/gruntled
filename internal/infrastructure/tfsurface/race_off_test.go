//go:build !race

package tfsurface

// raceSlowdown scales wall-clock bounds in tests; 1 without -race.
const raceSlowdown = 1
