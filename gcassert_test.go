package gcassert

import (
	"bytes"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/tools/go/packages"
)

func TestParseDirectives(t *testing.T) {
	fileSet := token.NewFileSet()
	pkgs, err := packages.Load(&packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedCompiledGoFiles |
			packages.NeedTypes | packages.NeedTypesInfo,
		Fset: fileSet,
	}, "./testdata")
	if err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	var errOut bytes.Buffer
	absMap, err := parseDirectives(pkgs, fileSet, cwd, &errOut)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, `testdata/bad_directive.go:4:	//gcassert:foo
func badDirective1()	{}: unknown directive "foo"
testdata/bad_directive.go:8:	badDirective1(): unknown directive "bar"
testdata/bad_directive.go:12:	//gcassert:inline,afterinline
func badDirective3() {
	badDirective2()
}: unknown directive "afterinline"
`, errOut.String())

	// Convert the map into relative paths for ease of testing, and remove
	// the syntax node so we don't have to test that as well.
	relMap := make(directiveMap, len(absMap))
	for absPath, m := range absMap {
		for k, info := range m {
			info.n = nil
			m[k] = info
		}
		relPath, err := filepath.Rel(cwd, absPath)
		if err != nil {
			t.Fatal(err)
		}
		relMap[relPath] = m
	}

	expectedMap := directiveMap{
		"testdata/bad_directive.go": {
			8: {directives: []assertDirective{bce, inline}},
		},
		"testdata/bce.go": {
			8:  {directives: []assertDirective{bce}},
			11: {directives: []assertDirective{bce, inline}},
			13: {directives: []assertDirective{bce, inline}},
			17: {directives: []assertDirective{bce, inline}},
			19: {directives: []assertDirective{bce, inline}},
			23: {directives: []assertDirective{bce}},
		},
		"testdata/inline.go": {
			46: {inlinableCallsites: []passInfo{{colNo: 15}}},
			50: {directives: []assertDirective{inline}},
			52: {directives: []assertDirective{inline}},
			56: {directives: []assertDirective{inline}},
			58: {inlinableCallsites: []passInfo{{colNo: 36}}},
			59: {inlinableCallsites: []passInfo{{colNo: 35}}},
		},
		"testdata/noescape.go": {
			13: {directives: []assertDirective{noescape}},
			20: {directives: []assertDirective{noescape}},
			27: {directives: []assertDirective{noescape}},
			35: {directives: []assertDirective{noescape}},
			38: {directives: []assertDirective{noescape}},
			49: {directives: []assertDirective{noescape}},
			57: {directives: []assertDirective{noescape}},
		},
		"testdata/issue5.go": {
			4: {inlinableCallsites: []passInfo{{colNo: 14}}},
		},
	}
	assert.Equal(t, expectedMap, relMap)
}

func TestGCAssert(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	expectedOutput := `testdata/bad_directive.go:4:	//gcassert:foo
func badDirective1()	{}: unknown directive "foo"
testdata/bad_directive.go:8:	badDirective1(): unknown directive "bar"
testdata/bad_directive.go:12:	//gcassert:inline,afterinline
func badDirective3() {
	badDirective2()
}: unknown directive "afterinline"
testdata/noescape.go:13:	foo := foo{a: 1, b: 2}: foo escapes to heap:
testdata/noescape.go:27:	// This annotation should fail, because f will escape to the heap.
//
//gcassert:noescape
func (f foo) setA(a int) *foo {
	f.a = a
	return &f
}: f escapes to heap:
testdata/noescape.go:38:	: a escapes to heap:
testdata/noescape.go:49:	// This annotation should fail, because the parameter f is leaked.
// Specifically this means that if you call this method where f was a value
// (not a pointer) then this will cause a heap allocation.
//
//gcassert:noescape
func (f *foo) printReceiver() {
	fmt.Printf("#v", f)
}: leaking param: f
testdata/bce.go:8:	fmt.Println(ints[5]): Found IsInBounds
testdata/bce.go:23:	fmt.Println(ints[1:7]): Found IsSliceInBounds
testdata/bce.go:17:	sum += notInlinable(ints[i]): call was not inlined
testdata/bce.go:19:	sum += notInlinable(ints[i]): call was not inlined
testdata/inline.go:46:	alwaysInlined(3): call was not inlined
testdata/inline.go:52:	sum += notInlinable(i): call was not inlined
testdata/inline.go:56:	sum += 1: call was not inlined
testdata/inline.go:59:	test(0).neverInlinedMethod(10): call was not inlined
testdata/inline.go:61:	otherpkg.A{}.NeverInlined(sum): call was not inlined
testdata/inline.go:63:	otherpkg.NeverInlinedFunc(sum): call was not inlined
testdata/issue5.go:4:	Gen().Layout(): call was not inlined
`

	testCases := []struct {
		name     string
		pkgs     []string
		cwd      string
		expected string
	}{
		{
			name: "relative",
			pkgs: []string{
				"./testdata",
				"./testdata/otherpkg",
			},
			expected: expectedOutput,
		},
		{
			name: "absolute",
			pkgs: []string{
				filepath.Join(cwd, "testdata"),
				filepath.Join(cwd, "testdata/otherpkg"),
			},
			expected: expectedOutput,
		},
		{
			name: "relative-cwd",
			pkgs: []string{
				"./testdata",
				"./testdata/otherpkg",
			},
			cwd:      cwd,
			expected: expectedOutput,
		},
	}
	for _, testCase := range testCases {
		var w strings.Builder
		t.Run(testCase.name, func(t *testing.T) {
			var err error
			if testCase.cwd == "" {
				err = GCAssert(&w, testCase.pkgs...)
			} else {
				err = GCAssertCwd(&w, testCase.cwd, testCase.pkgs...)
			}
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, testCase.expected, w.String())
		})
	}
}
