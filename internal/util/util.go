package util

// Pointer simply returns a pointer to the supplied value
func Pointer[T any](v T) *T {
	return &v
}
