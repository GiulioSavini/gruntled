// Package terragrunt is the Terragrunt anti-corruption layer: it turns
// terragrunt.hcl files into ports.UnitConfig using structural decoding
// only. The only evaluation gruntled ever performs is of the three
// path-bearing attributes (include.path, dependency.config_path,
// terraform.source), each inside a closed hcl.EvalContext with exactly six
// functions and no variables. Everything uncertain becomes an unknown
// reason: this package leans toward "unknown" at every hand-written
// decision point, never toward a diagnostic.
//
// This plan (02-03) lands only the leaf pieces every later adapter needs:
// the six path functions and closed evaluation (pathfuncs.go, eval.go).
// Plans 04-05 add the walker, parser and loader that make this package a
// complete ports.UnitLoader implementation.
package terragrunt
