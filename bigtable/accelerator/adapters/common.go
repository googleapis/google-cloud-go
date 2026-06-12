package adapters

// Adapter defines a generic interface for adapting one type to another.
type Adapter[From any, To any] interface {
	Adapt(from From) (To, error)
}

// RequestAdapter represents a specialized adapter for request routing.
type RequestAdapter[From any, To any] interface {
	Adapter[From, To]
	ExtractResource(from From) (string, error)
}

// Default request and response adapter singletons.
var (
	DefaultReadRowRequestAdapter    = &ReadRowRequestAdapter{}
	DefaultReadRowResponseAdapter   = &ReadRowResponseAdapter{}
	DefaultMutateRowRequestAdapter  = &MutateRowRequestAdapter{}
	DefaultMutateRowResponseAdapter = &MutateRowResponseAdapter{}
)
