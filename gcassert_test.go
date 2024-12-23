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
			45: {inlinableCallsites: []passInfo{{colNo: 15}}},
			49: {directives: []assertDirective{inline}},
			51: {directives: []assertDirective{inline}},
			55: {directives: []assertDirective{inline}},
			57: {inlinableCallsites: []passInfo{{colNo: 36}}},
			58: {inlinableCallsites: []passInfo{{colNo: 35}}},
		},
		"testdata/noescape.go": {
			11: {directives: []assertDirective{noescape}},
			18: {directives: []assertDirective{noescape}},
			25: {directives: []assertDirective{noescape}},
			33: {directives: []assertDirective{noescape}},
			36: {directives: []assertDirective{noescape}},
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
testdata/noescape.go:11:	foo := foo{a: 1, b: 2}: foo escapes to heap:
testdata/noescape.go:25:	// This annotation should fail, because f will escape to the heap.
//
//gcassert:noescape
func (f foo) setA(a int) *foo {
	f.a = a
	return &f
}: f escapes to heap:
testdata/noescape.go:36:	: a escapes to heap:
testdata/bce.go:8:	fmt.Println(ints[5]): Found IsInBounds
testdata/bce.go:23:	fmt.Println(ints[1:7]): Found IsSliceInBounds
testdata/bce.go:17:	sum += notInlinable(ints[i]): call was not inlined
testdata/bce.go:19:	sum += notInlinable(ints[i]): call was not inlined
testdata/inline.go:45:	alwaysInlined(3): call was not inlined
testdata/inline.go:51:	sum += notInlinable(i): call was not inlined
testdata/inline.go:55:	sum += 1: call was not inlined
testdata/inline.go:58:	test(0).neverInlinedMethod(10): call was not inlined
testdata/inline.go:60:	otherpkg.A{}.NeverInlined(sum): call was not inlined
testdata/inline.go:62:	otherpkg.NeverInlinedFunc(sum): call was not inlined
testdata/issue5.go:4:	Gen().Layout(): call was not inlined
`

	testCases := []struct{
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
