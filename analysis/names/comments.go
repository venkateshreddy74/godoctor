// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package names

import (
	"fmt"
	"go/ast"
	"go/token"
	"regexp"
	"strings"

	"golang-refactoring.org/go-doctor/text"
)

// FindInComments searches the comments of the given packages' source files for
// occurrences of the given name (as a word) and returns their source locations.
func FindInComments(name string, f *ast.File, fset *token.FileSet) []text.Extent {
	result := []text.Extent{}
	for _, comment := range f.Comments {
		slashIdx := fset.Position(comment.List[0].Slash).Offset
		whitespaceIdx := 1
		regexpstring := fmt.Sprintf("[\\PL]%s[\\PL]|//%s[\\PL]|/\\*%s[\\PL]|[\\PL]%s$", name, name, name, name)
		re := regexp.MustCompile(regexpstring)
		matchcount := strings.Count(comment.List[0].Text, name)
		for _, idx := range re.FindAllStringIndex(comment.List[0].Text, matchcount) {
			var offset int
			if idx[0] == 0 {
				offset = slashIdx + idx[0] + whitespaceIdx + 1
			} else {
				offset = slashIdx + idx[0] + whitespaceIdx
			}
			result = append(result, text.Extent{offset, len(name)})
		}

	}
	return result
}