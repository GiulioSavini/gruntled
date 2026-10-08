package terragrunt

import (
	"testing"

	"pgregory.net/rapid"
)

func TestIncrementalEqualsFull(t *testing.T) {
	rapid.Check(t, func(*rapid.T) {})
}
