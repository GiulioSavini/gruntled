package repograph

import (
	"errors"
	"slices"
	"strconv"
)

// Surface is the public variable and output names of a Module (design doc
// §5.3): the contract a Unit binds against when it declares dependency
// blocks and inputs. Variables and Outputs are each stored sorted and
// duplicate-free.
type Surface struct {
	variables []string
	outputs   []string
}

// NewSurface validates variables and outputs and returns a Surface. Neither
// list may contain an empty name or a duplicate within itself; the stored
// copies are sorted.
func NewSurface(variables, outputs []string) (Surface, error) {
	sortedVars, err := sortedUniqueNames("variable", variables)
	if err != nil {
		return Surface{}, err
	}
	sortedOuts, err := sortedUniqueNames("output", outputs)
	if err != nil {
		return Surface{}, err
	}
	return Surface{variables: sortedVars, outputs: sortedOuts}, nil
}

func sortedUniqueNames(kind string, names []string) ([]string, error) {
	sorted := slices.Clone(names)
	slices.Sort(sorted)
	for i, name := range sorted {
		if name == "" {
			return nil, errors.New("repograph: invalid surface " + kind + " name: must not be empty")
		}
		if i > 0 && sorted[i-1] == name {
			return nil, errors.New("repograph: invalid surface " + kind + " name " + strconv.Quote(name) + ": duplicate")
		}
	}
	return sorted, nil
}

// Variables returns a sorted copy of the surface's variable names.
func (s Surface) Variables() []string {
	return slices.Clone(s.variables)
}

// Outputs returns a sorted copy of the surface's output names.
func (s Surface) Outputs() []string {
	return slices.Clone(s.outputs)
}

// HasVariable reports whether name is declared in the surface's variables.
func (s Surface) HasVariable(name string) bool {
	_, ok := slices.BinarySearch(s.variables, name)
	return ok
}

// HasOutput reports whether name is declared in the surface's outputs.
func (s Surface) HasOutput(name string) bool {
	_, ok := slices.BinarySearch(s.outputs, name)
	return ok
}
