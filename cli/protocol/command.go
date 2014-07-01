// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package protocol

type Command interface {
	Run(*State, map[string]interface{}) (Reply, error)
	Validate(*State, map[string]interface{}) (bool, error)
}
