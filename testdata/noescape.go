package gcassert

import "fmt"

type foo struct {
	a int
	b int
}

func returnsStackVarPtr() *foo {
	// this should fail
	//gcassert:noescape
	foo := foo{a: 1, b: 2}
	return &foo
}

func returnsStackVar() foo {
	// this should succeed
	//gcassert:noescape
	foo := foo{a: 1, b: 2}
	return foo
}

// This annotation should fail, because f will escape to the heap.
//
//gcassert:noescape
func (f foo) setA(a int) *foo {
	f.a = a
	return &f
}

// This annotation should pass, because f does not escape.
//
//gcassert:noescape
func (f foo) returnA(
	// This annotation should fail, because a will escape to the heap.
	//gcassert:noescape
	a int,
	b int,
) *int {
	return &a
}

// This annotation should fail, because the parameter f is leaked.
// Specifically this means that if you call this method where f was a value
// (not a pointer) then this will cause a heap allocation.
//
//gcassert:noescape
func (f *foo) printReceiver() {
	fmt.Printf("#v", f)
}

// This annotation should succeed, because the parameter f is not leaked.
// This is because we don't pass the pointer value f, but its value (*f) instead.
//
//gcassert:noescape
func (f *foo) printValue() {
	fmt.Printf("#v", *f)
}
