// Package that defines useful structs for constructing JSON Patch operations.
package jsonpatch

type Operation string

const (
	AddOp     Operation = "add"
	RemoveOp  Operation = "remove"
	ReplaceOp Operation = "replace"
	MoveOp    Operation = "move"
	CopyOp    Operation = "copy"
	TestOp    Operation = "test"
)

type PatchString struct {
	Op    Operation `json:"op"`
	Path  string    `json:"path"`
	Value string    `json:"value"`
}
