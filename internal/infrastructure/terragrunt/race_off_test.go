//go:build !race

package terragrunt_test

// raceSlowdown scales wall-clock bounds in tests; 1 without -race.
const raceSlowdown = 1
